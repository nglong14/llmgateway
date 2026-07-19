package db

import (
	"context"
	"fmt"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nglong14/llmgateway/internal/config"
)

// Pool wraps a pgx connection pool.
type Pool struct {
	*pgxpool.Pool
}

// Connect opens a PostgreSQL pool and verifies connectivity with Ping.
func Connect(ctx context.Context, cfg config.DatabaseConfig) (*Pool, error) {
	if !cfg.Configured() {
		return nil, fmt.Errorf("db: database is not configured")
	}

	dsn := DSN(cfg)
	poolCfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("db: parse config: %w", err)
	}
	poolCfg.MaxConns = 20
	poolCfg.MinConns = 1
	poolCfg.MaxConnLifetime = 30 * time.Minute
	poolCfg.HealthCheckPeriod = 30 * time.Second

	pool, err := pgxpool.NewWithConfig(ctx, poolCfg)
	if err != nil {
		return nil, fmt.Errorf("db: connect: %w", err)
	}

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pool.Ping(pingCtx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("db: ping failed: %w", err)
	}

	return &Pool{Pool: pool}, nil
}

// Close shuts down the connection pool.
func (p *Pool) Close() {
	if p != nil && p.Pool != nil {
		p.Pool.Close()
	}
}

// DSN builds a PostgreSQL connection string from config.
func DSN(cfg config.DatabaseConfig) string {
	port := cfg.Port
	if port == "" {
		port = "5432"
	}
	sslmode := cfg.SSLMode
	if sslmode == "" {
		sslmode = "disable"
	}
	u := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(cfg.User, cfg.Password),
		Host:   fmt.Sprintf("%s:%s", cfg.Host, port),
		Path:   cfg.DBName,
	}
	q := u.Query()
	q.Set("sslmode", sslmode)
	u.RawQuery = q.Encode()
	return u.String()
}
