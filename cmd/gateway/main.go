package main

import (
	"context"
	"flag"
	"fmt"
	"log"
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
	"github.com/nglong14/llmgateway/internal/router"
)

func main() {
	godotenv.Load()
	// Parse --config flag.
	configPath := flag.String("config", "configs/gateway.yaml", "path to YAML config file")
	flag.Parse()

	// Load config.
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Create provider registry and register providers.
	registry := provider.NewRegistry()

	// Shared middleware configs.
	cbCfg := cfg.CircuitBreaker
	providerRL := cfg.ProviderRateLimits

	// wrapProvider applies the decorator chain: rate limiter (outer) → circuit breaker → provider.
	// Rate limiter is outermost so rejected requests never touch the circuit breaker.
	wrapProvider := func(p provider.Provider, name string) provider.Provider {
		wrapped := middleware.NewCircuitBreakerProvider(p, cbCfg)
		if rlCfg, ok := providerRL[name]; ok && rlCfg.RPM > 0 {
			return middleware.NewRateLimitedProvider(wrapped, rlCfg)
		}
		return wrapped
	}

	if pc, ok := cfg.Providers["openai"]; ok {
		oaiClient := openai.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(oaiClient, "openai"), "gpt-", "o1-", "o3-", "o4-")
		log.Println("Registered provider: openai")
	}

	if pc, ok := cfg.Providers["gemini"]; ok {
		gClient := gemini.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(gClient, "gemini"), "gemini-", "g-")
		log.Println("Registered provider: gemini")
	}

	if pc, ok := cfg.Providers["anthropic"]; ok {
		aClient := anthropic.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(aClient, "anthropic"), "claude-")
		log.Println("Registered provider: anthropic")
	}

	if pc, ok := cfg.Providers["deepseek"]; ok {
		dsClient := deepseek.New(pc.APIKey, pc.BaseURL)
		registry.Register(wrapProvider(dsClient, "deepseek"), "deepseek-")
		log.Println("Registered provider: deepseek")
	}

	// Initialize rate limiter with config values (default to safe values if zero).
	rps := cfg.RateLimit.RPS
	if rps == 0 {
		rps = 10
	}
	burst := cfg.RateLimit.Burst
	if burst == 0 {
		burst = 20
	}
	cleanupInterval := cfg.RateLimit.CleanupInterval
	if cleanupInterval == 0 {
		cleanupInterval = 5 * time.Minute
	}
	rl := middleware.NewRateLimiter(rps, burst, cleanupInterval, cfg.RateLimit.TrustedProxies)
	defer rl.Stop()
	log.Printf("Rate limiter: %.0f req/s, burst %d, trusted proxies: %v", rps, burst, cfg.RateLimit.TrustedProxies)

	// Initialize Prometheus metrics.
	metrics.Init()
	log.Println("Prometheus metrics available at /metrics")

	// Create router with all routes and middleware.
	r := router.New(registry, rl)

	// Start HTTP server.
	srv := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		fmt.Printf("LLM Gateway listening on %s\n", cfg.Server.Address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	fmt.Printf("\nReceived %s — shutting down gracefully…\n", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	fmt.Println("Server stopped.")
}
