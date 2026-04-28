// Package middleware — Redis-backed distributed per-provider rate limiter.
package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
)

type RedisRateLimitedProvider struct {
	wrapped provider.Provider
	rdb     *redis.Client
	key     string // Redis key, e.g., "provider_rate:openai"
	rps     float64
	burst   int
	rpm     float64
}

func NewRedisRateLimitedProvider(p provider.Provider, rdb *redis.Client, cfg config.ProviderRateLimitConfig) *RedisRateLimitedProvider {
	rps := cfg.RPM / 60.0

	burst := cfg.Burst
	if burst == 0 {
		burst = 10
	}

	return &RedisRateLimitedProvider{
		wrapped: p,
		rdb:     rdb,
		key:     "provider_rate:" + p.Name(),
		rps:     rps,
		burst:   burst,
		rpm:     cfg.RPM,
	}
}

func (r *RedisRateLimitedProvider) Name() string {
	return r.wrapped.Name()
}

func (r *RedisRateLimitedProvider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	if err := r.checkLimit(ctx); err != nil {
		return nil, err
	}
	return r.wrapped.ChatCompletion(ctx, req)
}

func (r *RedisRateLimitedProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	if err := r.checkLimit(ctx); err != nil {
		errCh := make(chan error, 1)
		errCh <- err
		close(errCh)

		chunkCh := make(chan *models.StreamChunk)
		close(chunkCh)

		return chunkCh, errCh
	}
	return r.wrapped.ChatCompletionStream(ctx, req)
}

// ListModels checks the per-provider rate limit, then delegates.
func (r *RedisRateLimitedProvider) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	if err := r.checkLimit(ctx); err != nil {
		return nil, err
	}
	return r.wrapped.ListModels(ctx)
}

func (r *RedisRateLimitedProvider) HealthCheck(ctx context.Context) error {
	return r.wrapped.HealthCheck(ctx)
}

// checkLimit runs the token bucket Lua script against Redis.
func (r *RedisRateLimitedProvider) checkLimit(ctx context.Context) error {
	now := float64(time.Now().UnixMicro()) / 1e6

	result, err := tokenBucketScript.Run(ctx, r.rdb,
		[]string{r.key},
		r.burst, // ARGV[1] = capacity
		r.rps,   // ARGV[2] = tokens/sec
		now,     // ARGV[3] = current time
	).Int()
	if err != nil {
		// Redis down — allow through, don't block the provider.
		ctxutil.Logger(ctx).Warn("redis provider rate limiter error (allowing request)",
			slog.String("provider", r.wrapped.Name()),
			slog.String("error", err.Error()),
		)
		return nil
	}

	if result == 0 {
		ctxutil.Logger(ctx).Warn("provider rate limit exceeded",
			slog.String("provider", r.wrapped.Name()),
			slog.Float64("limit_rpm", r.rpm),
		)
		return fmt.Errorf("provider %s: rate limit exceeded (%.0f RPM)", r.wrapped.Name(), r.rpm)
	}

	return nil
}
