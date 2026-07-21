package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/nglong14/llmgateway/internal/config"
	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/models"
)

type KeyInfo struct {
	Name   string
	UserID uuid.UUID // uuid.Nil for static config keys
}


type KeyValidator interface {
	Validate(ctx context.Context, token string) (*KeyInfo, error)
}

type StaticKeyValidator struct {
	hashMap map[string]*config.APIKeyConfig
}

func NewStaticKeyValidator(keys []config.APIKeyConfig) *StaticKeyValidator {
	hashMap := make(map[string]*config.APIKeyConfig, len(keys))
	for i := range keys {
		hashMap[keys[i].KeyHash] = &keys[i]
	}
	return &StaticKeyValidator{hashMap: hashMap}
}

func (v *StaticKeyValidator) Validate(_ context.Context, token string) (*KeyInfo, error) {
	cfg, ok := v.hashMap[hashAPIKey(token)]
	if !ok {
		return nil, nil
	}
	return &KeyInfo{Name: cfg.Name}, nil
}

func AuthMiddleware(cfg config.AuthConfig, validators []KeyValidator) func(http.Handler) http.Handler {
	skipSet := make(map[string]bool, len(cfg.SkipPaths))
	for _, p := range cfg.SkipPaths {
		skipSet[p] = true
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if skipSet[r.URL.Path] {
				next.ServeHTTP(w, r)
				return
			}

			token := extractBearerToken(r)
			if token == "" {
				models.WriteUnauthorized(w, "missing Authorization header")
				return
			}

			info, err := validateWithFallback(r.Context(), validators, token)
			if err != nil {
				models.WriteServiceUnavailable(w, "authentication service unavailable")
				return
			}
			if info == nil {
				models.WriteUnauthorized(w, "invalid API key")
				return
			}

			ctx := ctxutil.WithAPIKeyName(r.Context(), info.Name)
			if info.UserID != uuid.Nil {
				ctx = ctxutil.WithUserID(ctx, info.UserID)
			}
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func validateWithFallback(ctx context.Context, validators []KeyValidator, token string) (*KeyInfo, error) {
	for _, v := range validators {
		info, err := v.Validate(ctx, token)
		if err != nil {
			return nil, err
		}
		if info != nil {
			return info, nil
		}
	}
	return nil, nil
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func hashAPIKey(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
