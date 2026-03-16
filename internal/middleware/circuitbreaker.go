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

// CircuitBreakerProvider wraps a provider.Provider and delegates every call
// through a gobreaker circuit breaker. When the circuit is open, calls fail
// immediately with a 503-style error instead of hitting the upstream provider.
type CircuitBreakerProvider struct {
	wrapped provider.Provider
	cb      *gobreaker.TwoStepCircuitBreaker[any]
}

// NewCircuitBreakerProvider wraps the given provider with a circuit breaker
// configured from cfg.
func NewCircuitBreakerProvider(p provider.Provider, cfg config.CircuitBreakerConfig) *CircuitBreakerProvider {
	settings := gobreaker.Settings{
		Name: p.Name(),

		// MaxRequests is the number of requests allowed in the half-open state
		// to probe whether the upstream has recovered.
		MaxRequests: cfg.MaxRequests,

		// Interval is the cyclic period of the closed state.
		// If Interval is 0, the failure count never resets while closed.
		Interval: cfg.Interval,

		// Timeout is how long the circuit stays open before transitioning
		// to the half-open state to allow probe requests.
		Timeout: cfg.Timeout,

		// ReadyToTrip decides when to open the circuit.
		// Here we open it after 5 consecutive failures.
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= 5
		},
	}

	return &CircuitBreakerProvider{
		wrapped: p,
		cb:      gobreaker.NewTwoStepCircuitBreaker[any](settings),
	}
}

// Name delegates to the wrapped provider.
func (c *CircuitBreakerProvider) Name() string {
	return c.wrapped.Name()
}

// ChatCompletion runs the request through the circuit breaker.
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

// ChatCompletionStream runs the streaming request through the circuit breaker.
// The circuit is checked upfront; the final outcome (success/failure) is
// reported asynchronously when the stream completes via proxy channels.
func (c *CircuitBreakerProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	// Check if the circuit allows the request.
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

	// Create proxy channels to intercept the stream outcome.
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

// ListModels runs the list call through the circuit breaker.
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
	done(err) // report outcome to gobreaker
	if err != nil {
		return nil, err
	}
	return result, nil
}

// HealthCheck delegates directly — we don't want the health probe to trip the breaker.
func (c *CircuitBreakerProvider) HealthCheck(ctx context.Context) error {
	return c.wrapped.HealthCheck(ctx)
}

// wrapCBError returns a descriptive error depending on the circuit breaker state.
func wrapCBError(providerName string, err error) error {
	if err == gobreaker.ErrOpenState {
		return fmt.Errorf("provider %s: circuit breaker is open — upstream is unavailable", providerName)
	}
	if err == gobreaker.ErrTooManyRequests {
		return fmt.Errorf("provider %s: circuit breaker half-open — too many probe requests", providerName)
	}
	return err
}

