package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"github.com/nglong14/llmgateway/internal/app"
	"github.com/nglong14/llmgateway/internal/config"
)

func main() {
	logger := setupLogger()

	if err := godotenv.Load(); err != nil {
		logger.Warn("WARNING: .env file not loaded", slog.String("error", err.Error()))
	}

	configPath := flag.String("config", "configs/gateway.yaml", "path to YAML config file")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		logger.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid config", slog.String("error", err.Error()))
		os.Exit(1)
	}

	server, err := app.New(cfg, logger)
	if err != nil {
		logger.Error("failed to initialize gateway", slog.String("error", err.Error()))
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-quit
		fmt.Printf("\nReceived %s — shutting down gracefully…\n", sig)
		cancel()
	}()

	if err := server.Run(ctx); err != nil {
		logger.Error("gateway stopped with error", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func setupLogger() *slog.Logger {
	var logWriter io.Writer = os.Stdout
	if _, err := os.Stat("/var/log/gateway"); err == nil {
		logFile, err := os.OpenFile("/var/log/gateway/gateway.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0666)
		if err == nil {
			logWriter = io.MultiWriter(os.Stdout, logFile)
		}
	}
	logger := slog.New(slog.NewJSONHandler(logWriter, nil))
	slog.SetDefault(logger)
	return logger
}
