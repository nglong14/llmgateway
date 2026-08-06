package anthropic

import (
	"strings"

	"github.com/nglong14/llmgateway/internal/models"
)

// toAnthropicRequest converts a unified request into Anthropic's native format.
func toAnthropicRequest(req *models.ChatCompletionRequest) *anthropicRequest {
	ar := &anthropicRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
	}

	if req.MaxTokens != nil {
		ar.MaxTokens = *req.MaxTokens
	} else {
		ar.MaxTokens = 4096
	}

	var systemParts []string
	for _, msg := range req.Messages {
		if msg.Role == models.RoleSystem {
			systemParts = append(systemParts, msg.Content)
			continue
		}

		ar.Messages = append(ar.Messages, anthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	if len(systemParts) > 0 {
		ar.System = strings.Join(systemParts, "\n")
	}

	return ar
}

// toUnifiedResponse converts an Anthropic response into the unified format.
func toUnifiedResponse(ar *anthropicResponse) *models.ChatCompletionResponse {
	var content string
	for _, block := range ar.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	resp := &models.ChatCompletionResponse{
		ID:     ar.ID,
		Object: "chat.completion",
		Model:  ar.Model,
		Choices: []models.Choice{
			{
				Index: 0,
				Message: models.Message{
					Role:    models.RoleAssistant,
					Content: content,
				},
				FinishReason: mapStopReason(ar.StopReason),
			},
		},
	}

	if ar.Usage != nil {
		resp.Usage = &models.Usage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		}
	}

	return resp
}

func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}
