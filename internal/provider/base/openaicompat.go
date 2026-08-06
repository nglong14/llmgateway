package base

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nglong14/llmgateway/internal/models"
)

type openAICompatWire struct {
	name string
}

func NewOpenAICompat(name, apiKey, baseURL string) *Client {
	return New(baseURL, apiKey, &openAICompatWire{name: name})
}

func (w *openAICompatWire) Name() string { return w.name }

func (w *openAICompatWire) ChatURL(baseURL string, _ *models.ChatCompletionRequest, _ bool) string {
	return baseURL + "/chat/completions"
}

func (w *openAICompatWire) ModelListURL(baseURL, _ string) string {
	return baseURL + "/models"
}

func (w *openAICompatWire) AuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
}

func (w *openAICompatWire) EncodeRequest(req *models.ChatCompletionRequest, stream bool) ([]byte, error) {
	reqCopy := *req
	reqCopy.Stream = stream
	reqCopy.Provider = "" // strip gateway-only field

	return json.Marshal(reqCopy)
}

func (w *openAICompatWire) DecodeCompletion(data []byte, _ string) (*models.ChatCompletionResponse, error) {
	var result models.ChatCompletionResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", w.name, err)
	}
	return &result, nil
}

func (w *openAICompatWire) DecodeStreamData(data []byte, _ string) (*models.StreamChunk, error) {
	var chunk models.StreamChunk
	if err := json.Unmarshal(data, &chunk); err != nil {
		return nil, fmt.Errorf("%s: decode chunk: %w", w.name, err)
	}
	return &chunk, nil
}

func (w *openAICompatWire) StreamDone(data []byte) bool {
	return string(data) == "[DONE]"
}

func (w *openAICompatWire) DecodeModels(data []byte) ([]models.ModelInfo, string, error) {
	var result struct {
		Data []models.ModelInfo `json:"data"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, "", fmt.Errorf("%s: decode models: %w", w.name, err)
	}
	return result.Data, "", nil
}
