// Package middleware — circuit breaker wrapper for providers.
package middleware

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/sony/gobreaker/v2"

	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
)

type CircuitBreakerProvider struct {
	wrapped provider.Provider
	cb      *gobreaker.TwoStepCircuitBreaker[any]
}

func NewCircuitBreakerProvider(p provider.Provider, cfg config.CircuitBreakerConfig) *CircuitBreakerProvider {
	settings := gobreaker.Settings{
		Name: p.Name(),

		MaxRequests: cfg.MaxRequests,

		Interval: cfg.Interval,

		Timeout: cfg.Timeout,

		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}

	return &CircuitBreakerProvider{
		wrapped: p,
		cb:      gobreaker.NewTwoStepCircuitBreaker[any](settings),
	}
}

func (c *CircuitBreakerProvider) Name() string {
	return c.wrapped.Name()
}

func (c *CircuitBreakerProvider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	done, err := c.cb.Allow()
	if err != nil {
		ctxutil.Logger(ctx).Warn("circuit breaker denied request",
			slog.String("provider", c.wrapped.Name()),
			slog.String("error", err.Error()),
		)
		return nil, wrapCBError(c.wrapped.Name(), err)
	}

	resp, err := c.wrapped.ChatCompletion(ctx, req)
	done(err) // report outcome to gobreaker
	if err != nil {
		return nil, err
	}
	return resp, nil
}

func (c *CircuitBreakerProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	done, err := c.cb.Allow()
	if err != nil {
		ctxutil.Logger(ctx).Warn("circuit breaker denied request",
			slog.String("provider", c.wrapped.Name()),
			slog.String("error", err.Error()),
		)

		// Circuit is open — return closed channels with an error.
		errCh := make(chan error, 1)
		errCh <- wrapCBError(c.wrapped.Name(), err)
		close(errCh)

		chunkCh := make(chan *models.StreamChunk)
		close(chunkCh)

		return chunkCh, errCh
	}

	// Call the real provider.
	chunks, errCh := c.wrapped.ChatCompletionStream(ctx, req)

	proxyChunks := make(chan *models.StreamChunk, 10)
	proxyErr := make(chan error, 1)

	go func() {
		defer close(proxyChunks)
		defer close(proxyErr)

		// Forward all chunks from the real provider to the handler.
		for chunk := range chunks {
			proxyChunks <- chunk
		}

		// Wait for the final error status from the provider.
		err := <-errCh

		// Report the stream outcome to gobreaker.
		done(err)

		// Forward the error (if any) to the handler.
		if err != nil {
			proxyErr <- err
		}
	}()

	return proxyChunks, proxyErr
}

func (c *CircuitBreakerProvider) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	done, err := c.cb.Allow()
	if err != nil {
		ctxutil.Logger(ctx).Warn("circuit breaker denied request",
			slog.String("provider", c.wrapped.Name()),
			slog.String("error", err.Error()),
		)
		return nil, wrapCBError(c.wrapped.Name(), err)
	}

	result, err := c.wrapped.ListModels(ctx)
	done(err)
	if err != nil {
		return nil, err
	}
	return result, nil
}

func (c *CircuitBreakerProvider) HealthCheck(ctx context.Context) error {
	return c.wrapped.HealthCheck(ctx)
}

func wrapCBError(providerName string, err error) error {
	if err == gobreaker.ErrOpenState {
		return fmt.Errorf("provider %s: circuit breaker is open — upstream is unavailable", providerName)
	}
	if err == gobreaker.ErrTooManyRequests {
		return fmt.Errorf("provider %s: circuit breaker half-open — too many probe requests", providerName)
	}
	return err
}
