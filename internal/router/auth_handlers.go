package router

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/nglong14/llmgateway/internal/auth"
	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/models"
)

// AuthHandlers serves /auth/* endpoints. When svc is nil, all handlers return 503.
type AuthHandlers struct {
	svc *auth.Service
}

type signUpRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Name     string `json:"name"`
}

type credentialsRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type createKeyRequest struct {
	Name string `json:"name"`
}

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
}

func (h *AuthHandlers) SignUp(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	var req signUpRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Email == "" || req.Password == "" {
		models.WriteInvalidRequest(w, "email and password are required")
		return
	}

	result, err := h.svc.SignUp(r.Context(), req.Email, req.Password, req.Name)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"user_id": result.User.ID.String(),
		"email":   result.User.Email,
		"api_key": result.APIKey,
	})
}

func (h *AuthHandlers) Login(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Email == "" || req.Password == "" {
		models.WriteInvalidRequest(w, "email and password are required")
		return
	}

	user, err := h.svc.Login(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"user_id": user.ID.String(),
		"email":   user.Email,
		"name":    user.Name,
	})
}

func (h *AuthHandlers) IssueTokens(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	var req credentialsRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.Email == "" || req.Password == "" {
		models.WriteInvalidRequest(w, "email and password are required")
		return
	}

	pair, err := h.svc.IssueTokens(r.Context(), req.Email, req.Password)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    pair.TokenType,
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *AuthHandlers) RefreshTokens(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}
	if req.RefreshToken == "" {
		models.WriteInvalidRequest(w, "refresh_token is required")
		return
	}

	pair, err := h.svc.RefreshTokens(r.Context(), req.RefreshToken)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, tokenResponse{
		AccessToken:  pair.AccessToken,
		RefreshToken: pair.RefreshToken,
		TokenType:    pair.TokenType,
		ExpiresIn:    pair.ExpiresIn,
	})
}

func (h *AuthHandlers) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	userID, ok := ctxutil.UserID(r.Context())
	if !ok {
		models.WriteUnauthorized(w, "missing authenticated user")
		return
	}

	var req createKeyRequest
	if err := decodeJSONOptional(w, r, &req); err != nil {
		return
	}

	result, err := h.svc.CreateAPIKey(r.Context(), userID, req.Name)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":         result.Key.ID.String(),
		"name":       result.Key.Name,
		"key_prefix": result.Key.KeyPrefix,
		"api_key":    result.APIKey,
		"created_at": result.Key.CreatedAt.UTC().Format(time.RFC3339),
	})
}

func (h *AuthHandlers) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	userID, ok := ctxutil.UserID(r.Context())
	if !ok {
		models.WriteUnauthorized(w, "missing authenticated user")
		return
	}

	keys, err := h.svc.ListAPIKeys(r.Context(), userID)
	if err != nil {
		writeAuthError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"keys": keys})
}

func (h *AuthHandlers) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
	if h.svc == nil {
		models.WriteServiceUnavailable(w, "authentication service unavailable")
		return
	}

	userID, ok := ctxutil.UserID(r.Context())
	if !ok {
		models.WriteUnauthorized(w, "missing authenticated user")
		return
	}

	keyID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		models.WriteInvalidRequest(w, "invalid key id")
		return
	}

	if err := h.svc.RevokeAPIKey(r.Context(), userID, keyID); err != nil {
		writeAuthError(w, r, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func decodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		models.WriteInvalidRequest(w, "invalid JSON: "+err.Error())
		return err
	}
	return nil
}

func decodeJSONOptional(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	err := json.NewDecoder(r.Body).Decode(dst)
	if err != nil && !errors.Is(err, io.EOF) {
		models.WriteInvalidRequest(w, "invalid JSON: "+err.Error())
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeAuthError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, auth.ErrEmailTaken):
		models.WriteConflict(w, "email already taken")
	case errors.Is(err, auth.ErrInvalidCredentials):
		models.WriteUnauthorized(w, "invalid credentials")
	case errors.Is(err, auth.ErrInvalidToken), errors.Is(err, auth.ErrTokenReuse):
		models.WriteUnauthorized(w, "invalid or expired refresh token")
	case errors.Is(err, auth.ErrKeyNotFound):
		models.WriteKeyNotFound(w, "api key not found")
	default:
		ctxutil.Logger(r.Context()).Error("auth handler error", slog.String("error", err.Error()))
		models.WriteInternalError(w, "internal server error")
	}
}
