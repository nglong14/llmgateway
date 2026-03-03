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

	if pc, ok := cfg.Providers["openai"]; ok {
		oaiClient := openai.New(pc.APIKey, pc.BaseURL)
		registry.Register(oaiClient, "gpt-", "o1-", "o3-", "o4-")
		log.Println("Registered provider: openai")
	}

	if pc, ok := cfg.Providers["gemini"]; ok {
		gClient := gemini.New(pc.APIKey, pc.BaseURL)
		registry.Register(gClient, "gemini-", "g-")
		log.Println("Registered provider: gemini")
	}

	if pc, ok := cfg.Providers["anthropic"]; ok {
		aClient := anthropic.New(pc.APIKey, pc.BaseURL)
		registry.Register(aClient, "claude-")
		log.Println("Registered provider: anthropic")
	}

	if pc, ok := cfg.Providers["deepseek"]; ok {
		dsClient := deepseek.New(pc.APIKey, pc.BaseURL)
		registry.Register(dsClient, "deepseek-")
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
	rl := middleware.NewRateLimiter(rps, burst, cleanupInterval)
	defer rl.Stop()
	log.Printf("Rate limiter: %.0f req/s, burst %d", rps, burst)

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
