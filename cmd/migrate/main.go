package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/joho/godotenv"
	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/db"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	_ = godotenv.Load()

	configPath := flag.String("config", "configs/gateway.yaml", "path to YAML config file")
	flag.Parse()

	args := flag.Args()
	if len(args) != 1 {
		printUsage()
		os.Exit(2)
	}
	cmd := args[0]

	cfg, err := config.Load(*configPath)
	if err != nil {
		slog.Error("failed to load config", slog.String("error", err.Error()))
		os.Exit(1)
	}
	if !cfg.Database.Configured() {
		slog.Error("database is not configured (set DB_HOST, DB_USER, DB_NAME)")
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	pool, err := db.Connect(ctx, cfg.Database)
	if err != nil {
		slog.Error("failed to connect to database", slog.String("error", err.Error()))
		os.Exit(1)
	}
	defer pool.Close()

	migrator := db.NewMigrator(pool.Pool)

	switch cmd {
	case "up":
		n, err := migrator.Up(ctx)
		if err != nil {
			slog.Error("migrate up failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		slog.Info("migrations applied", slog.Int("count", n))
	case "down":
		if err := migrator.Down(ctx); err != nil {
			slog.Error("migrate down failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		slog.Info("rolled back latest migration")
	case "status":
		st, err := migrator.Status(ctx)
		if err != nil {
			slog.Error("migrate status failed", slog.String("error", err.Error()))
			os.Exit(1)
		}
		fmt.Printf("current version: %d\n", st.CurrentVersion)
		fmt.Printf("applied (%d):\n", len(st.Applied))
		for _, m := range st.Applied {
			fmt.Printf("  %d_%s\n", m.Version, m.Name)
		}
		fmt.Printf("pending (%d):\n", len(st.Pending))
		for _, m := range st.Pending {
			fmt.Printf("  %d_%s\n", m.Version, m.Name)
		}
	default:
		printUsage()
		os.Exit(2)
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `Usage: migrate --config <path> <command>

Commands:
  up       Apply all pending migrations
  down     Roll back the latest migration
  status   Print current version and pending migrations
`)
}
