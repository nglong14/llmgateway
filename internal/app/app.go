// Package app wires together the gateway's dependencies and runs its HTTP servers.
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/nglong14/llmgateway/internal/auth"
	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/db"
	"github.com/nglong14/llmgateway/internal/metrics"
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
	gatewayredis "github.com/nglong14/llmgateway/internal/redis"
	"github.com/nglong14/llmgateway/internal/router"
)

const (
	adminAddr       = ":9091"
	shutdownTimeout = 3 * time.Minute
)

// Server owns the gateway's runtime dependencies and lifecycle.
type Server struct {
	cfg    *config.Config
	logger *slog.Logger

	registry    *provider.Registry
	rateLimiter router.RateLimitMiddleware

	authService   *auth.Service
	tokenManager  *auth.TokenManager
	keyValidators []middleware.KeyValidator

	dbPool      *db.Pool
	redisClient *gatewayredis.Client
	memRL       *middleware.RateLimiter

	httpSrv  *http.Server
	adminSrv *http.Server
}

func New(cfg *config.Config, logger *slog.Logger) (*Server, error) {
	s := &Server{
		cfg:    cfg,
		logger: logger,
	}

	s.setupDatabase()
	s.setupAuth()
	s.setupRedis()
	s.registerProviders()
	s.setupRateLimiter()
	s.buildServers()

	return s, nil
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 2)

	go func() {
		s.logger.Info("LLM Gateway listening", slog.String("address", s.cfg.Server.Address))
		if err := s.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	go func() {
		s.logger.Info("Internal admin server (metrics) listening", slog.String("address", s.adminSrv.Addr))
		if err := s.adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		s.logger.Info("Shutting down gracefully...")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	return s.Shutdown(shutdownCtx)
}

func (s *Server) Shutdown(ctx context.Context) error {
	var errs []error

	if err := s.httpSrv.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("main server forced shutdown: %w", err))
	}
	if err := s.adminSrv.Shutdown(ctx); err != nil {
		errs = append(errs, fmt.Errorf("admin server forced shutdown: %w", err))
	}

	s.close()

	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	s.logger.Info("Servers stopped.")
	return nil
}

func (s *Server) buildServers() {
	metrics.Init()

	adminMux := http.NewServeMux()
	adminMux.Handle("/metrics", metrics.Handler())
	s.adminSrv = &http.Server{
		Addr:    adminAddr,
		Handler: adminMux,
	}

	s.httpSrv = &http.Server{
		Addr:         s.cfg.Server.Address,
		Handler:      router.New(s.registry, s.rateLimiter, s.cfg.Auth, s.authService, s.tokenManager, s.keyValidators),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}
}

func (s *Server) close() {
	if s.memRL != nil {
		s.memRL.Stop()
	}
	if s.redisClient != nil {
		_ = s.redisClient.Close()
	}
	if s.dbPool != nil {
		s.dbPool.Close()
	}
}
