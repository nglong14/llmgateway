// Routing of requests
package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/nglong14/llmgateway/internal/metrics"
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
)

// RateLimitMiddleware is satisfied by both the in-memory RateLimiter
// and the Redis-backed RedisRateLimiter.
type RateLimitMiddleware interface {
	Handler(next http.Handler) http.Handler
}

// Chi router with all routes and middleware configured
func New(registry *provider.Registry, rl RateLimitMiddleware) chi.Router {
	r := chi.NewRouter()

	//Middleware
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.PrometheusMiddleware)

	// Prometheus /metrics endpoint (serves text/plain, outside JSON header).
	r.Get("/metrics", metrics.Handler().ServeHTTP)

	// All API routes share the JSON content-type header.
	r.Group(func(api chi.Router) {
		api.Use(chimiddleware.SetHeader("Content-Type", "application/json"))
		api.Use(rl.Handler)

		//Handlers
		h := &Handlers{registry: registry}

		//Routes
		api.Get("/health", h.Health)
		api.Get("/v1/models", h.ListModels)
		api.Post("/v1/chat/completions", h.ChatCompletion)
	})

	return r
}
