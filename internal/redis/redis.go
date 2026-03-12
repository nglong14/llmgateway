// Package redis provides a thin wrapper around go-redis for the gateway.
package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps a go-redis client with a health-checked connection.
type Client struct {
	RDB *redis.Client
}

// New creates a Redis client and verifies the connection with a ping.
// Returns an error if Redis is unreachable (caller decides whether to
// fall back to in-memory or fail fast).
func New(addr, password string, db int) (*Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolSize:     20,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("redis: ping failed: %w", err)
	}

	return &Client{RDB: rdb}, nil
}

// Close shuts down the Redis connection pool.
func (c *Client) Close() error {
	return c.RDB.Close()
}
