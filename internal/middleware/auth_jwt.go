package middleware

import (
	"net/http"

	"github.com/nglong14/llmgateway/internal/auth"
	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/models"
)

type accessTokenValidator interface {
	ValidateAccessToken(tokenString string) (*auth.Claims, error)
}
func JWTAuthMiddleware(tm accessTokenValidator) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := extractBearerToken(r)
			if token == "" {
				models.WriteUnauthorized(w, "missing Authorization header")
				return
			}

			claims, err := tm.ValidateAccessToken(token)
			if err != nil {
				models.WriteUnauthorized(w, "invalid or expired token")
				return
			}

			ctx := ctxutil.WithUserID(r.Context(), claims.UserID)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
