// Handling requests
package router

import (
	"encoding/json"
	"log"
	"net/http"

	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/normalize"
	"github.com/nglong14/llmgateway/internal/provider"
	"github.com/nglong14/llmgateway/internal/streaming"
)

// Holds dependencies for all endpoint handlers.
type Handlers struct {
	registry *provider.Registry
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (h *Handlers) ListModels(w http.ResponseWriter, r *http.Request) {
	var allModels []models.ModelInfo

	for _, p := range h.registry.ListAll() {
		providerModels, err := p.ListModels(r.Context())
		if err != nil {
			log.Printf("warning: failed to list models from %s: %v", p.Name(), err)
			continue
		}
		allModels = append(allModels, providerModels...)
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models.ModelListResponse{
		Object: "list",
		Data:   allModels,
	})
}

// ChatCompletion handles both streaming and non-streaming chat requests.
func (h *Handlers) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	// Parse request body.
	var req models.ChatCompletionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		models.WriteInvalidRequest(w, "invalid JSON: "+err.Error())
		return
	}

	// Validate required fields.
	if req.Model == "" {
		models.WriteInvalidRequest(w, "model is required")
		return
	}
	if len(req.Messages) == 0 {
		models.WriteInvalidRequest(w, "messages is required and must not be empty")
		return
	}

	// Resolve provider.
	p, err := h.registry.Resolve(req.Model, req.Provider)
	if err != nil {
		models.WriteNotFound(w, err.Error())
		return
	}

	// Normalize request for the target provider.
	normalizedReq, err := normalize.NormalizeRequest(p.Name(), &req)
	if err != nil {
		models.WriteInvalidRequest(w, "normalization error: "+err.Error())
		return
	}

	// Dispatch (streaming or non-streaming).
	if normalizedReq.Stream {
		h.handleStream(w, r, p, normalizedReq)
	} else {
		h.handleNonStream(w, r, p, normalizedReq)
	}
}

func (h *Handlers) handleNonStream(w http.ResponseWriter, r *http.Request, p provider.Provider, req *models.ChatCompletionRequest) {
	resp, err := p.ChatCompletion(r.Context(), req)
	if err != nil {
		models.WriteProviderError(w, err.Error())
		return
	}

	// Normalize response back to unified format.
	normalized, err := normalize.NormalizeResponse(p.Name(), resp)
	if err != nil {
		models.WriteProviderError(w, "response normalization error: "+err.Error())
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(normalized)
}

func (h *Handlers) handleStream(w http.ResponseWriter, r *http.Request, p provider.Provider, req *models.ChatCompletionRequest) {
	// Switch to SSE headers.
	streaming.SetSSEHeaders(w)

	chunks, errCh := p.ChatCompletionStream(r.Context(), req)

	for chunk := range chunks {
		if err := streaming.WriteSSEChunk(w, chunk); err != nil {
			log.Printf("error writing SSE chunk: %v", err)
			return
		}
	}

	// Check for streaming errors.
	select {
	case err := <-errCh:
		if err != nil {
			log.Printf("streaming error: %v", err)
		}
	default:
	}

	streaming.WriteSSEDone(w)
}
