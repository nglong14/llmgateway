// Package gemini provides a Gemini provider adapter. It implements the shared
// base.Wire interface, translating between the unified models and Gemini's
// generateContent API. All HTTP/SSE/streaming plumbing lives in the base client.
package gemini

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider/base"
)

// Client implements provider.Provider for Gemini.
type Client struct {
	*base.Client
}

// New creates a Gemini provider client.
func New(apiKey, baseURL string) *Client {
	return &Client{Client: base.New(baseURL, apiKey, geminiWire{})}
}

// geminiWire adapts the unified format to Gemini's generateContent API.
type geminiWire struct{}

func (geminiWire) Name() string { return "gemini" }

func (geminiWire) ChatURL(baseURL string, req *models.ChatCompletionRequest, stream bool) string {
	if stream {
		return fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse", baseURL, req.Model)
	}
	return fmt.Sprintf("%s/models/%s:generateContent", baseURL, req.Model)
}

func (geminiWire) ModelListURL(baseURL, _ string) string {
	return baseURL + "/models"
}

func (geminiWire) AuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", apiKey)
}

func (geminiWire) EncodeRequest(req *models.ChatCompletionRequest, _ bool) ([]byte, error) {
	return json.Marshal(toGeminiRequest(req))
}

func (geminiWire) DecodeCompletion(data []byte, model string) (*models.ChatCompletionResponse, error) {
	var result geminiResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}
	return toUnifiedResponse(&result, model), nil
}

func (geminiWire) DecodeStreamData(data []byte, model string) (*models.StreamChunk, error) {
	var result geminiResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("gemini: decode stream chunk: %w", err)
	}
	return toStreamChunk(&result, model), nil
}

// StreamDone is always false: Gemini streams end when the HTTP body closes.
func (geminiWire) StreamDone([]byte) bool { return false }

func (geminiWire) DecodeModels(data []byte) ([]models.ModelInfo, string, error) {
	var result struct {
		Models []struct {
			Name string `json:"name"` // "models/gemini-2.0-flash"
		} `json:"models"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, "", fmt.Errorf("gemini: decode models: %w", err)
	}

	var infos []models.ModelInfo
	for _, m := range result.Models {
		infos = append(infos, models.ModelInfo{
			ID:      stripModelsPrefix(m.Name),
			Object:  "model",
			OwnedBy: "google",
		})
	}
	return infos, "", nil
}
