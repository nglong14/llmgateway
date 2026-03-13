// Package middleware — Prometheus HTTP metrics middleware.
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/nglong14/llmgateway/internal/metrics"
)

// statusRecorder wraps http.ResponseWriter to capture the status code.
type statusRecorder struct {
	http.ResponseWriter
	statusCode int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.statusCode = code
	sr.ResponseWriter.WriteHeader(code)
}

// Flush implements the http.Flusher interface to allow SSE streaming to work.
func (sr *statusRecorder) Flush() {
	if flusher, ok := sr.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// PrometheusMiddleware records HTTP-level metrics for every request:
// total count, latency histogram, and in-flight gauge.
func PrometheusMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		metrics.HTTPInFlightRequests.Inc()
		defer metrics.HTTPInFlightRequests.Dec()

		recorder := &statusRecorder{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(recorder, r)

		duration := time.Since(start).Seconds()
		status := strconv.Itoa(recorder.statusCode)

		// Use the matched route pattern (if available) to avoid high cardinality.
		// If chi hasn't routed it yet (e.g. 404), fallback to a generic label.
		routeContext := chi.RouteContext(r.Context())
		pathPattern := "unknown_route"
		if routeContext != nil && routeContext.RoutePattern() != "" {
			pathPattern = routeContext.RoutePattern()
		}

		metrics.HTTPRequestsTotal.WithLabelValues(r.Method, pathPattern, status).Inc()
		metrics.HTTPRequestDuration.WithLabelValues(r.Method, pathPattern).Observe(duration)
	})
}
