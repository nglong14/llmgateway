// Package middleware — circuit breaker wrapper for providers.
package middleware

import (
	"context"
	"fmt"

	"github.com/sony/gobreaker/v2"

	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
)

// CircuitBreakerProvider wraps a provider.Provider and delegates every call
// through a gobreaker circuit breaker. When the circuit is open, calls fail
// immediately with a 503-style error instead of hitting the upstream provider.
type CircuitBreakerProvider struct {
	wrapped provider.Provider
	cb      *gobreaker.CircuitBreaker[any]
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
		cb:      gobreaker.NewCircuitBreaker[any](settings),
	}
}

// Name delegates to the wrapped provider.
func (c *CircuitBreakerProvider) Name() string {
	return c.wrapped.Name()
}

// ChatCompletion runs the request through the circuit breaker.
func (c *CircuitBreakerProvider) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	result, err := c.cb.Execute(func() (any, error) {
		return c.wrapped.ChatCompletion(ctx, req)
	})
	if err != nil {
		return nil, wrapCBError(c.wrapped.Name(), err)
	}
	return result.(*models.ChatCompletionResponse), nil
}

// ChatCompletionStream runs the streaming request through the circuit breaker.
// Only the initial connection is guarded; once the stream starts, chunks flow directly.
func (c *CircuitBreakerProvider) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	// We guard the stream creation in the circuit breaker.
	// If the circuit is open, we return an immediate error on the error channel.
	type streamResult struct {
		chunks <-chan *models.StreamChunk
		errs   <-chan error
	}

	result, err := c.cb.Execute(func() (any, error) {
		chunks, errs := c.wrapped.ChatCompletionStream(ctx, req)
		return &streamResult{chunks: chunks, errs: errs}, nil
	})

	if err != nil {
		// Circuit is open — return closed channels with an error.
		errCh := make(chan error, 1)
		errCh <- wrapCBError(c.wrapped.Name(), err)
		close(errCh)

		chunkCh := make(chan *models.StreamChunk)
		close(chunkCh)

		return chunkCh, errCh
	}

	sr := result.(*streamResult)
	return sr.chunks, sr.errs
}

// ListModels runs the list call through the circuit breaker.
func (c *CircuitBreakerProvider) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	result, err := c.cb.Execute(func() (any, error) {
		return c.wrapped.ListModels(ctx)
	})
	if err != nil {
		return nil, wrapCBError(c.wrapped.Name(), err)
	}
	return result.([]models.ModelInfo), nil
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
