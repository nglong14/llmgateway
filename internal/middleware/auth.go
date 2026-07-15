package middleware

import (
    "crypto/sha256"
    "fmt"
    "net/http"
    "strings"

    "github.com/nglong14/llmgateway/internal/config"
    "github.com/nglong14/llmgateway/internal/ctxutil"
    "github.com/nglong14/llmgateway/internal/models"
)

func AuthMiddleware(cfg config.AuthConfig) func(http.Handler) http.Handler {
    // Build lookup map: SHA-256 hex digest → API key config.
    hashMap := make(map[string]*config.APIKeyConfig, len(cfg.Keys))
    for i := range cfg.Keys {
        hashMap[cfg.Keys[i].KeyHash] = &cfg.Keys[i]
    }

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

            keyCfg, ok := validateKey(hashMap, token)
            if !ok {
                models.WriteUnauthorized(w, "invalid API key")
                return
            }

            ctx := ctxutil.WithAPIKeyName(r.Context(), keyCfg.Name)
            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}

func extractBearerToken(r *http.Request) string {
    auth := r.Header.Get("Authorization")
    if auth == "" || !strings.HasPrefix(auth, "Bearer ") {
        return ""
    }
    return strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
}

func validateKey(hashMap map[string]*config.APIKeyConfig, token string) (*config.APIKeyConfig, bool) {
    h := sha256.Sum256([]byte(token))
    hexHash := fmt.Sprintf("%x", h)
    cfg, ok := hashMap[hexHash]
    return cfg, ok
}
