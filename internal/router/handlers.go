// Handling requests
package router

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/nglong14/llmgateway/internal/ctxutil"
	"github.com/nglong14/llmgateway/internal/metrics"
	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider"
	"github.com/nglong14/llmgateway/internal/streaming"
	"strings"
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
			ctxutil.Logger(r.Context()).Warn("failed to list models",
				slog.String("provider", p.Name()),
				slog.String("error", err.Error()),
			)
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

func handleProviderError(w http.ResponseWriter, r *http.Request, p provider.Provider, err error, endpoint string) {
	errStr := err.Error()
	statusLabel := "error"

	if strings.Contains(errStr, "circuit breaker is open") || strings.Contains(errStr, "circuit breaker half-open") {
		statusLabel = "circuit_breaker_open"
		metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), endpoint, statusLabel).Inc()
		models.WriteServiceUnavailable(w, errStr)
		return
	} 

	if strings.Contains(errStr, "rate limit exceeded") {
		statusLabel = "rate_limit_exceeded"
		metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), endpoint, statusLabel).Inc()
		models.WriteRateLimited(w, errStr)
		return
	}

	metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), endpoint, statusLabel).Inc()
	ctxutil.Logger(r.Context()).Error("provider error",
		slog.String("provider", p.Name()),
		slog.String("endpoint", endpoint),
		slog.String("error", errStr),
	)
	models.WriteProviderError(w, errStr)
}

func (h *Handlers) handleNonStream(w http.ResponseWriter, r *http.Request, p provider.Provider, req *models.ChatCompletionRequest) {
	start := time.Now()
	logger := ctxutil.Logger(r.Context())

	logger.Info("upstream provider call start",
		slog.String("provider", p.Name()),
		slog.String("model", req.Model),
	)

	resp, err := p.ChatCompletion(r.Context(), req)

	duration := time.Since(start).Seconds()
	metrics.ProviderRequestDuration.WithLabelValues(p.Name(), "chat_completion").Observe(duration)

	if err != nil {
		handleProviderError(w, r, p, err, "chat_completion")
		return
	}

	metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion", "success").Inc()

	logger.Info("upstream provider call completed",
		slog.String("provider", p.Name()),
		slog.String("model", req.Model),
		slog.Int64("latency_total_ms", time.Since(start).Milliseconds()),
	)

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
	logger := ctxutil.Logger(r.Context())

	logger.Info("upstream provider stream start",
		slog.String("provider", p.Name()),
		slog.String("model", req.Model),
	)

	chunks, errCh := p.ChatCompletionStream(r.Context(), req)

	// Wait for the first event to decide whether to set SSE headers or return an HTTP error.
	headersSent := false
	var firstTokenLatency int64

	for {
		select {
		case err, ok := <-errCh:
			if !ok {
				// errCh closed, stream is done.
				if headersSent {
					streaming.WriteSSEDone(w)
				}
				logger.Info("upstream provider stream completed",
					slog.String("provider", p.Name()),
					slog.String("model", req.Model),
					slog.Int64("latency_first_token_ms", firstTokenLatency),
					slog.Int64("latency_total_ms", time.Since(start).Milliseconds()),
				)
				return
			}
			if err != nil {
				if !headersSent {
					// Failed immediately before any chunks were sent, return HTTP error.
					handleProviderError(w, r, p, err, "chat_completion_stream")
					metrics.ProviderRequestDuration.WithLabelValues(p.Name(), "chat_completion_stream").Observe(time.Since(start).Seconds())
					return
				}
				// Headers already sent, log the error and terminate the stream gracefully.
				logger.Error("streaming error",
					slog.String("error", err.Error()),
					slog.String("provider", p.Name()),
				)
				metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion_stream", "error").Inc()
				metrics.ProviderRequestDuration.WithLabelValues(p.Name(), "chat_completion_stream").Observe(time.Since(start).Seconds())
				streaming.WriteSSEDone(w)
				return
			}

		case chunk, ok := <-chunks:
			if !ok {
				// chunks closed, exit loop and wait for errCh.
				chunks = nil
				continue
			}
			if !headersSent {
				firstTokenLatency = time.Since(start).Milliseconds()
				streaming.SetSSEHeaders(w)
				headersSent = true
			}
			if err := streaming.WriteSSEChunk(w, chunk); err != nil {
				logger.Error("error writing SSE chunk",
					slog.String("error", err.Error()),
				)
				metrics.ProviderRequestsTotal.WithLabelValues(p.Name(), "chat_completion_stream", "error").Inc()
				return
			}
		}

		// If chunks channel is nil and errCh is closed or successfully processed, we are done.
		if chunks == nil {
			// We must wait for errCh to give us the final error or close.
		}
	}
}
