package anthropic

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/nglong14/llmgateway/internal/models"
	"github.com/nglong14/llmgateway/internal/provider/base"
)

const anthropicVersion = "2023-06-01"

type Client struct {
	*base.Client
}

func New(apiKey, baseURL string) *Client {
	return &Client{Client: base.New(baseURL, apiKey, anthropicWire{})}
}

type anthropicWire struct{}

func (anthropicWire) Name() string { return "anthropic" }

func (anthropicWire) ChatURL(baseURL string, _ *models.ChatCompletionRequest, _ bool) string {
	return baseURL + "/v1/messages"
}

func (anthropicWire) ModelListURL(baseURL, cursor string) string {
	url := baseURL + "/v1/models?limit=100"
	if cursor != "" {
		url += "&after_id=" + cursor
	}
	return url
}

func (anthropicWire) AuthHeaders(req *http.Request, apiKey string) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
}

func (anthropicWire) EncodeRequest(req *models.ChatCompletionRequest, stream bool) ([]byte, error) {
	ar := toAnthropicRequest(req)
	ar.Stream = stream
	return json.Marshal(ar)
}

func (anthropicWire) DecodeCompletion(data []byte, _ string) (*models.ChatCompletionResponse, error) {
	var result anthropicResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}
	return toUnifiedResponse(&result), nil
}

func (anthropicWire) DecodeStreamData(data []byte, model string) (*models.StreamChunk, error) {
	var event anthropicStreamEvent
	if err := json.Unmarshal(data, &event); err != nil {
		return nil, nil // skip unparsable lines
	}

	switch event.Type {
	case "content_block_delta":
		var delta anthropicBlockDelta
		if err := json.Unmarshal(data, &delta); err != nil {
			return nil, fmt.Errorf("anthropic: decode content_block_delta: %w", err)
		}
		if delta.Delta.Type != "text_delta" {
			return nil, nil
		}
		return &models.StreamChunk{
			Object: "chat.completion.chunk",
			Model:  model,
			Choices: []models.StreamDelta{
				{
					Index: delta.Index,
					Delta: models.Delta{Content: delta.Delta.Text},
				},
			},
		}, nil

	case "message_delta":
		var md anthropicMessageDelta
		if err := json.Unmarshal(data, &md); err != nil {
			return nil, fmt.Errorf("anthropic: decode message_delta: %w", err)
		}
		if md.Delta.StopReason == "" {
			return nil, nil
		}
		return &models.StreamChunk{
			Object: "chat.completion.chunk",
			Model:  model,
			Choices: []models.StreamDelta{
				{
					Index:        0,
					FinishReason: mapStopReason(md.Delta.StopReason),
				},
			},
		}, nil

	// Skip ping, message_start, content_block_start/stop, message_stop.
	default:
		return nil, nil
	}
}

// StreamDone is always false: Anthropic streams end when the HTTP body closes
// rather than with a terminal data marker.
func (anthropicWire) StreamDone([]byte) bool { return false }

func (anthropicWire) DecodeModels(data []byte) ([]models.ModelInfo, string, error) {
	var result anthropicModelsResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, "", fmt.Errorf("anthropic: decode models: %w", err)
	}

	var infos []models.ModelInfo
	for _, m := range result.Data {
		infos = append(infos, models.ModelInfo{
			ID:      m.ID,
			Object:  "model",
			OwnedBy: "anthropic",
		})
	}

	next := ""
	if result.HasMore {
		next = result.LastID
	}
	return infos, next, nil
}
