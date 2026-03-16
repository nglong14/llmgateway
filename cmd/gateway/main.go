package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/metrics"
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
	"github.com/nglong14/llmgateway/internal/provider/anthropic"
	"github.com/nglong14/llmgateway/internal/provider/deepseek"
	"github.com/nglong14/llmgateway/internal/provider/gemini"
	"github.com/nglong14/llmgateway/internal/provider/openai"
	gatewayredis "github.com/nglong14/llmgateway/internal/redis"
	"github.com/nglong14/llmgateway/internal/router"
)

func main() {
	// Initialize structured JSON logger
	var logWriter io.Writer = os.Stdout
	if _, err := os.Stat("/var/log/gateway"); err == nil {
		logFile, err := os.OpenFile("/var/log/gateway/gateway.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			logWriter = io.MultiWriter(os.Stdout, logFile)
		}
	}
	logger := slog.New(slog.NewJSONHandler(logWriter, nil))
	slog.SetDefault(logger)

	if err := godotenv.Load(); err != nil {
		slog.Warn("WARNING: .env file not loaded", slog.String("error", err.Error()))
	}
	// Parse --config flag.
	configPath := flag.String("config", "configs/gateway.yaml", "path to YAML config file")
	flag.Parse()

	// Load config.
	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		slog.Error("invalid config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	// Create provider registry and register providers.
	registry := provider.NewRegistry()

	// Connect to Redis (used by both per-IP and per-provider rate limiters).
	var redisClient *gatewayredis.Client
	if cfg.Redis.Addr != "" {
		rc, err := gatewayredis.New(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
		if err != nil {
			slog.Warn("Redis unavailable — falling back to in-memory middleware", slog.String("error", err.Error()))
		} else {
			redisClient = rc
			defer redisClient.Close()
			slog.Info("Connected to Redis", slog.String("address", cfg.Redis.Addr))
		}
	}

	// Shared middleware configs.
	cbCfg := cfg.CircuitBreaker
	providerRL := cfg.ProviderRateLimits

	// wrapProvider applies the decorator chain: rate limiter (outer) → circuit breaker → provider.
	// Rate limiter is outermost so rejected requests never touch the circuit breaker.
	wrapProvider := func(p provider.Provider, name string) provider.Provider {
		wrapped := middleware.NewCircuitBreakerProvider(p, cbCfg)
		if rlCfg, ok := providerRL[name]; ok && rlCfg.RPM > 0 {
			if redisClient != nil {
				return middleware.NewRedisRateLimitedProvider(wrapped, redisClient.RDB, rlCfg)
			}
			return middleware.NewRateLimitedProvider(wrapped, rlCfg)
		}
		return wrapped
	}

	if pc, ok := cfg.Providers["openai"]; ok {
		oaiClient := openai.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(oaiClient, "openai"), "gpt-", "o1-", "o3-", "o4-")
		slog.Info("Registered provider: openai")
	}

	if pc, ok := cfg.Providers["gemini"]; ok {
		gClient := gemini.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(gClient, "gemini"), "gemini-", "g-")
		slog.Info("Registered provider: gemini")
	}

	if pc, ok := cfg.Providers["anthropic"]; ok {
		aClient := anthropic.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(aClient, "anthropic"), "claude-")
		slog.Info("Registered provider: anthropic")
	}

	if pc, ok := cfg.Providers["deepseek"]; ok {
		dsClient := deepseek.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(dsClient, "deepseek"), "deepseek-")
		slog.Info("Registered provider: deepseek")
	}

	// Rate limiter defaults.
	rps := cfg.RateLimit.RPS
	if rps == 0 {
		rps = 10
	}
	burst := cfg.RateLimit.Burst
	if burst == 0 {
		burst = 20
	}

	// Build the in-memory rate limiter (always needed for IP extraction + fallback).
	cleanupInterval := cfg.RateLimit.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 5 * time.Minute
	}
	memRL := middleware.NewRateLimiter(rps, burst, cleanupInterval, cfg.RateLimit.TrustedProxies)
	defer memRL.Stop()

	// Choose per-IP rate limiter: Redis (distributed) or in-memory (single-instance fallback).
	var rl router.RateLimitMiddleware
	if redisClient != nil {
		rl = middleware.NewRedisRateLimiter(redisClient.RDB, rps, burst, memRL.ExtractIP)
		slog.Info("Per-IP rate limiter initialized", 
			slog.String("type", "redis"), 
			slog.Float64("rps", rps), 
			slog.Int("burst", burst),
		)
	} else {
		rl = memRL
		slog.Info("Per-IP rate limiter initialized", 
			slog.String("type", "in-memory"), 
			slog.Float64("rps", rps), 
			slog.Int("burst", burst),
		)
	}


	// Initialize Prometheus metrics.
	metrics.Init()
	
	// Start internal administrative server for metrics
	adminMux := http.NewServeMux()
	adminMux.Handle("/metrics", metrics.Handler())
	adminSrv := &http.Server{
		Addr:    ":9091",
		Handler: adminMux,
	}
	go func() {
		slog.Info("Internal admin server (metrics) listening on :9091")
		if err := adminSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("admin server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Create router with all routes and middleware.
	r := router.New(registry, rl)

	// Start HTTP server.
	srv := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		slog.Info("LLM Gateway listening", slog.String("address", cfg.Server.Address))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", slog.String("error", err.Error()))
			os.Exit(1)
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	fmt.Printf("\nReceived %s — shutting down gracefully…\n", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		slog.Error("main server forced shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := adminSrv.Shutdown(ctx); err != nil {
		slog.Error("admin server forced shutdown", slog.String("error", err.Error()))
		os.Exit(1)
	}
	slog.Info("Servers stopped.")
}
