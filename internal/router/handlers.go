// Handling requests
package router

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/nglong14/llmgateway/internal/metrics"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
	"github.com/nglong14/llmgateway/internal/streaming"
)

// Holds dependencies for all endpoint handlers.
type Handlers struct {
	registry *provider.Registry
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
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

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(models.ModelListResponse{
		Object: "list",
		Data:   allModels,
	})
}

// maxRequestBodyBytes is the maximum allowed size for incoming request bodies (1 MB).
const maxRequestBodyBytes = 1 << 20

// ChatCompletion handles both streaming and non-streaming chat requests.
func (h *Handlers) ChatCompletion(w http.ResponseWriter, r *http.Request) {
	// Limit request body size to prevent oversized payloads.
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)

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

	// Dispatch (streaming or non-streaming).
	if req.Stream {
		h.handleStream(w, r, p, &req)
	} else {
		h.handleNonStream(w, r, p, &req)
	}
}

func (h *Handlers) handleNonStream(w http.ResponseWriter, r *http.Request, p provider.Provider, req *models.ChatCompletionRequest) {
	start := time.Now()

	resp, err := p.ChatCompletion(r.Context(), req)

	duration := time.Since(start).Seconds()
	metrics.ProviderRequestDuration.WithLabelValues(p.Name(), "chat_completion").Observe(duration)

	if err != nil {
		metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion", "error").Inc()
		models.WriteProviderError(w, err.Error())
		return
	}

	metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion", "success").Inc()

	// Record token usage if available.
	if resp.Usage.PromptTokens > 0 {
		metrics.ProviderTokensTotal.WithLabelValues(p.Name(), "prompt").Add(float64(resp.Usage.PromptTokens))
	}
	if resp.Usage.CompletionTokens > 0 {
		metrics.ProviderTokensTotal.WithLabelValues(p.Name(), "completion").Add(float64(resp.Usage.CompletionTokens))
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func (h *Handlers) handleStream(w http.ResponseWriter, r *http.Request, p provider.Provider, req *models.ChatCompletionRequest) {
	start := time.Now()

	// Switch to SSE headers.
	streaming.SetSSEHeaders(w)

	chunks, errCh := p.ChatCompletionStream(r.Context(), req)

	for chunk := range chunks {
		if err := streaming.WriteSSEChunk(w, chunk); err != nil {
			log.Printf("error writing SSE chunk: %v", err)
			metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion_stream", "error").Inc()
			return
		}
	}

	// Check for streaming errors.
	if err := <-errCh; err != nil {
		log.Printf("streaming error: %v", err)
		metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion_stream", "error").Inc()
		metrics.ProviderRequestDuration.WithLabelValues(p.Name(), "chat_completion_stream").Observe(time.Since(start).Seconds())
		streaming.WriteSSEDone(w)
		return
	}

	metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion_stream", "success").Inc()
	metrics.ProviderRequestDuration.WithLabelValues(p.Name(), "chat_completion_stream").Observe(time.Since(start).Seconds())
	streaming.WriteSSEDone(w)
}
