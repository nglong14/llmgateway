package models

import (
	"encoding/json"
	"net/http"
)

type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

type ErrorDetail struct {
	Message string `json:"message"`
	Type    string `json:"type"`
	Code    string `json:"code"`
}

func WriteError(w http.ResponseWriter, status int, message, errType, code string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErrorResponse{
		Error: ErrorDetail{
			Message: message,
			Type:    errType,
			Code:    code,
		},
	})
}


func WriteInvalidRequest(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadRequest, message, "invalid_request_error", "invalid_request")
}

func WriteProviderError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusBadGateway, message, "upstream_error", "provider_error")
}

func WriteNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, message, "invalid_request_error", "model_not_found")
}

func WriteRateLimited(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusTooManyRequests, message, "rate_limit_error", "rate_limit_exceeded")
}

func WriteServiceUnavailable(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusServiceUnavailable, message, "service_error", "service_unavailable")
}

func WriteUnauthorized(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusUnauthorized, message, "invalid_request_error", "unauthorized")
}

func WriteConflict(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusConflict, message, "invalid_request_error", "conflict")
}

func WriteKeyNotFound(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusNotFound, message, "invalid_request_error", "key_not_found")
}

func WriteInternalError(w http.ResponseWriter, message string) {
	WriteError(w, http.StatusInternalServerError, message, "server_error", "internal_error")
}