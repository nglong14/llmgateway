// Routing of requests
package router

import(
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nglong14/llmgateway/internal/provider"
)

//Chi router with all routes and middleware configured
func New(registry *provider.Registry) chi.Router{
	r := chi.NewRouter()

	//Middleware
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	//Handlers
	h := &Handlers{registry: registry}

	//Routes
	r.Get("/health", h.Health)
	r.Get("/v1/models", h.ListModels)
	r.Post("/v1/chat/completions", h.ChatCompletion)

	return r
}