// Package middleware — per-provider rate limiter wrapper.
package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"golang.org/x/time/rate"

	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
)

// RateLimitedProvider wraps a provider.Provider and enforces an aggregate
// request rate across all clients. This protects the upstream provider's
// quota (e.g., OpenAI's 500 RPM Tier 1 limit) by rejecting requests at
// the gateway before they ever leave your infrastructure.
type RateLimitedProvider struct {
	wrapped provider.Provider
	limiter *rate.Limiter
	rpm     float64
}

// NewRateLimitedProvider wraps the given provider with a per-provider rate limiter.
// cfg.RPM is converted to a per-second rate for the token bucket.
func NewRateLimitedProvider(p provider.Provider, cfg config.ProviderRateLimitConfig) *RateLimitedProvider {
	// Convert RPM to RPS: 400 RPM = 400/60 ≈ 6.67 tokens/second.
	rps := cfg.RPM / 60.0

	burst := cfg.Burst
	if burst == 0 {
		burst = 10 // Safe default burst.
	}

	return &RateLimitedProvider{
		wrapped: p,
		limiter: rate.NewLimiter(rate.Limit(rps), burst),
		rpm:     cfg.RPM,
	}
}

// Name delegates to the wrapped provider.
func (r *RateLimitedProvider) Name() string {
	return r.wrapped.Name()
}

// ChatCompletion checks the per-provider rate limit, then delegates.
func (r *RateLimitedProvider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	if !r.limiter.Allow() {
		ctxutil.Logger(ctx).Warn("provider rate limit exceeded",
			slog.String("provider", r.wrapped.Name()),
			slog.Float64("limit_rpm", r.rpm),
		)
		return nil, r.limitError()
	}
	return r.wrapped.ChatCompletion(ctx, req)
}

// ChatCompletionStream checks the per-provider rate limit, then delegates.
// Only the stream initiation is rate-limited — once streaming begins,
// chunks flow freely (same approach as the circuit breaker).
func (r *RateLimitedProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	if !r.limiter.Allow() {
		ctxutil.Logger(ctx).Warn("provider rate limit exceeded",
			slog.String("provider", r.wrapped.Name()),
			slog.Float64("limit_rpm", r.rpm),
		)
		// Return closed chunk channel + error channel (same pattern as circuit breaker).
		errCh := make(chan error, 1)
		errCh <- r.limitError()
		close(errCh)

		chunkCh := make(chan *models.StreamChunk)
		close(chunkCh)

		return chunkCh, errCh
	}
	return r.wrapped.ChatCompletionStream(ctx, req)
}

// ListModels checks the per-provider rate limit, then delegates.
func (r *RateLimitedProvider) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	if !r.limiter.Allow() {
		ctxutil.Logger(ctx).Warn("provider rate limit exceeded",
			slog.String("provider", r.wrapped.Name()),
			slog.Float64("limit_rpm", r.rpm),
		)
		return nil, r.limitError()
	}
	return r.wrapped.ListModels(ctx)
}

// HealthCheck bypasses the rate limiter — health probes should not
// consume quota or be rejected under load.
func (r *RateLimitedProvider) HealthCheck(ctx context.Context) error {
	return r.wrapped.HealthCheck(ctx)
}

// limitError returns a descriptive error for rate limit rejections.
func (r *RateLimitedProvider) limitError() error {
	return fmt.Errorf("provider %s: rate limit exceeded (%.0f RPM)", r.wrapped.Name(), r.rpm)
}
