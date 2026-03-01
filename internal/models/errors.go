// Error handling
package models

import (
	"encoding/json"
	"net/http"
)

// ErrorResponse is the standard error envelope (matches OpenAI's format).
type ErrorResponse struct {
    Error ErrorDetail `json:"error"`
}

// ErrorDetail holds the error information.
type ErrorDetail struct {
    Message string `json:"message"`
    Type    string `json:"type"`
    Code    string `json:"code"`
}

// WriteError writes a JSON error response to w with the given HTTP status.
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

// Common error helpers.

func WriteInvalidRequest(w http.ResponseWriter, message string) {
    WriteError(w, http.StatusBadRequest, message, "invalid_request_error", "invalid_request")
}

func WriteProviderError(w http.ResponseWriter, message string) {
    WriteError(w, http.StatusBadGateway, message, "upstream_error", "provider_error")
}

func WriteNotFound(w http.ResponseWriter, message string) {
    WriteError(w, http.StatusNotFound, message, "invalid_request_error", "model_not_found")
}