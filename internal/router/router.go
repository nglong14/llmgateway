package router

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/nglong14/llmgateway/internal/auth"
	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/middleware"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
)

type RateLimitMiddleware interface {
	Handler(next http.Handler) http.Handler
}


func New(
	registry *provider.Registry,
	rl RateLimitMiddleware,
	authCfg config.AuthConfig,
	authSvc *auth.Service,
	tokenMgr *auth.TokenManager,
	keyValidators []middleware.KeyValidator,
) chi.Router {
	r := chi.NewRouter()

	r.Use(middleware.LoggingMiddleware)
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.PrometheusMiddleware)

	ah := &AuthHandlers{svc: authSvc}
	r.Route("/auth", func(ar chi.Router) {
		ar.Post("/signup", ah.SignUp)
		ar.Post("/login", ah.Login)
		ar.Post("/token", ah.IssueTokens)
		ar.Post("/refresh", ah.RefreshTokens)

		ar.Group(func(keys chi.Router) {
			if tokenMgr != nil {
				keys.Use(middleware.JWTAuthMiddleware(tokenMgr))
			} else {
				keys.Use(authUnavailableMiddleware)
			}
			keys.Post("/keys", ah.CreateAPIKey)
			keys.Get("/keys", ah.ListAPIKeys)
			keys.Delete("/keys/{id}", ah.RevokeAPIKey)
		})
	})

	r.Group(func(api chi.Router) {
		if authCfg.Enabled {
			if len(keyValidators) == 0 {
				keyValidators = []middleware.KeyValidator{
					middleware.NewStaticKeyValidator(authCfg.Keys),
				}
			}
			api.Use(middleware.AuthMiddleware(authCfg, keyValidators))
		}
		api.Use(rl.Handler)

		h := &Handlers{registry: registry}

		api.Get("/health", h.Health)
		api.Get("/v1/models", h.ListModels)
		api.Post("/v1/chat/completions", h.ChatCompletion)
	})

	return r
}

func authUnavailableMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
	})
}
