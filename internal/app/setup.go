package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/nglong14/llmgateway/internal/auth"
	"github.com/nglong14/llmgateway/internal/db"
	"github.com/nglong14/llmgateway/internal/middleware"
	gatewayredis "github.com/nglong14/llmgateway/internal/redis"
	"github.com/nglong14/llmgateway/internal/repository"
)

const (
	defaultRPS        = 10.0
	defaultBurst      = 20
	defaultCleanupInt = 5 * time.Minute
	dbConnectTimeout  = 10 * time.Second
)

func (s *Server) setupDatabase() {
	if !s.cfg.Database.Configured() {
		s.logger.Warn("PostgreSQL is not configured — database auth disabled")
		return
	}

	connectCtx, cancel := context.WithTimeout(context.Background(), dbConnectTimeout)
	defer cancel()

	pool, err := db.Connect(connectCtx, s.cfg.Database)
	if err != nil {
		s.logger.Warn("PostgreSQL unavailable — database auth disabled", slog.String("error", err.Error()))
		return
	}
	s.dbPool = pool
	s.logger.Info("Connected to PostgreSQL")
}

func (s *Server) setupAuth() {
	s.keyValidators = []middleware.KeyValidator{
		middleware.NewStaticKeyValidator(s.cfg.Auth.Keys),
	}

	if s.dbPool == nil {
		return
	}

	users := repository.NewUserRepository(s.dbPool.Pool)
	keys := repository.NewAPIKeyRepository(s.dbPool.Pool)
	refreshTokens := repository.NewRefreshTokenRepository(s.dbPool.Pool)
	s.keyValidators = append(s.keyValidators, middleware.NewDatabaseKeyValidator(keys))

	tokenManager, err := auth.NewTokenManager(s.cfg.JWT)
	if err != nil {
		s.logger.Warn("JWT configuration invalid — auth management endpoints disabled", slog.String("error", err.Error()))
		return
	}
	s.tokenManager = tokenManager
	s.authService = auth.NewService(users, keys, refreshTokens, tokenManager)
}

func (s *Server) setupRedis() {
	if s.cfg.Redis.Addr == "" {
		return
	}

	client, err := gatewayredis.New(s.cfg.Redis.Addr, s.cfg.Redis.Password, s.cfg.Redis.DB)
	if err != nil {
		s.logger.Warn("Redis unavailable — falling back to in-memory middleware", slog.String("error", err.Error()))
		return
	}
	s.redisClient = client
	s.logger.Info("Connected to Redis", slog.String("address", s.cfg.Redis.Addr))
}

func (s *Server) setupRateLimiter() {
	rps := s.cfg.RateLimit.RPS
	if rps == 0 {
		rps = defaultRPS
	}
	burst := s.cfg.RateLimit.Burst
	if burst == 0 {
		burst = defaultBurst
	}
	cleanupInterval := s.cfg.RateLimit.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = defaultCleanupInt
	}

	s.memRL = middleware.NewRateLimiter(rps, burst, cleanupInterval, s.cfg.RateLimit.TrustedProxies)

	if s.redisClient != nil {
		s.rateLimiter = middleware.NewRedisRateLimiter(s.redisClient.RDB, rps, burst, s.memRL.ExtractIP)
		s.logger.Info("Per-IP rate limiter initialized",
			slog.String("type", "redis"),
			slog.Float64("rps", rps),
			slog.Int("burst", burst),
		)
		return
	}

	s.rateLimiter = s.memRL
	s.logger.Info("Per-IP rate limiter initialized",
		slog.String("type", "in-memory"),
		slog.Float64("rps", rps),
		slog.Int("burst", burst),
	)
}
