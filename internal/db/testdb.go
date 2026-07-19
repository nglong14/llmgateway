package db

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nglong14/llmgateway/internal/config"
)

const testDBLockKey int64 = 872364112

// TestDatabaseConfig loads DB settings from the environment for integration tests.
func TestDatabaseConfig() config.DatabaseConfig {
	return config.DatabaseConfig{
		Host:     envOr("DB_HOST", "localhost"),
		Port:     envOr("DB_PORT", "5432"),
		User:     envOr("DB_USER", "gateway"),
		Password: envOr("DB_PASSWORD", "gateway"),
		DBName:   envOr("DB_NAME", "llmgateway"),
		SSLMode:  envOr("DB_SSLMODE", "disable"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// LockTestDB acquires a session-level advisory lock so DB integration tests
// across packages do not race on the same schema.
func LockTestDB(ctx context.Context, pool *pgxpool.Pool) (unlock func(), err error) {
	conn, err := pool.Acquire(ctx)
	if err != nil {
		return nil, fmt.Errorf("db/test: acquire conn: %w", err)
	}
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, testDBLockKey); err != nil {
		conn.Release()
		return nil, fmt.Errorf("db/test: advisory lock: %w", err)
	}
	return func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = conn.Exec(unlockCtx, `SELECT pg_advisory_unlock($1)`, testDBLockKey)
		conn.Release()
	}, nil
}
