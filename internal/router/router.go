// Routing of requests
package router

import (
	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
)

// Chi router with all routes and middleware configured
func New(registry *provider.Registry, rl *middleware.RateLimiter) chi.Router {
	r := chi.NewRouter()

	//Middleware
	r.Use(chimiddleware.Logger)
	r.Use(chimiddleware.Recoverer)
	r.Use(chimiddleware.SetHeader("Content-Type", "application/json"))
	r.Use(rl.Handler)

	//Handlers
	h := &Handlers{registry: registry}

	//Routes
	r.Get("/health", h.Health)
	r.Get("/v1/models", h.ListModels)
	r.Post("/v1/chat/completions", h.ChatCompletion)

	return r
}
