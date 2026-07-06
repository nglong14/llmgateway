package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
	"github.com/nglong14/llmgateway/internal/config"
)

type RateLimitMiddleware interface {
	Handler(next http.Handler) http.Handler
}

func New(registry *provider.Registry, rl RateLimitMiddleware) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.LoggingMiddleware)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.PrometheusMiddleware)

	if authCfg.Enabled {
		r.Use(middleware.AuthMiddleware(authCfg))
	}

	r.Group(func(api chi.Router) {
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
