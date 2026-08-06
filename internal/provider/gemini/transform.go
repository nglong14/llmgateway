package gemini

import (
	"strings"

	"github.com/nglong14/llmgateway/internal/models"
)

// toGeminiRequest converts unified request into Gemini's native format.
func toGeminiRequest(req *models.ChatCompletionRequest) *geminiRequest {
	gr := &geminiRequest{}

	// Separate system messages from conversation messages.
	for _, msg := range req.Messages {
		if msg.Role == models.RoleSystem {
			// Gemini uses a dedicated systemInstruction field.
			gr.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			continue
		}

		// Map roles: OpenAI "assistant" → Gemini "model".
		role := msg.Role
		if role == models.RoleAssistant {
			role = "model"
		}

		gr.Contents = append(gr.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Map generation config.
	if req.Temperature != nil || req.MaxTokens != nil {
		gr.GenerationConfig = &geminiGenConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	return gr
}

// toUnifiedResponse converts a Gemini response into the unified format.
func toUnifiedResponse(gr *geminiResponse, model string) *models.ChatCompletionResponse {
	resp := &models.ChatCompletionResponse{
		Object: "chat.completion",
		Model:  model,
	}

	for i, cand := range gr.Candidates {
		// Extract text from parts.
		var content string
		for _, part := range cand.Content.Parts {
			content += part.Text
		}

		resp.Choices = append(resp.Choices, models.Choice{
			Index: i,
			Message: models.Message{
				Role:    models.RoleAssistant,
				Content: content,
			},
			FinishReason: mapFinishReason(cand.FinishReason),
		})
	}

	// Map usage metadata.
	if gr.UsageMetadata != nil {
		resp.Usage = &models.Usage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		}
	}

	return resp
}

// toStreamChunk converts a Gemini streaming response to a unified StreamChunk.
func toStreamChunk(gr *geminiResponse, model string) *models.StreamChunk {
	chunk := &models.StreamChunk{
		Object: "chat.completion.chunk",
		Model:  model,
	}

	for i, cand := range gr.Candidates {
		var content string
		for _, part := range cand.Content.Parts {
			content += part.Text
		}

		delta := models.StreamDelta{
			Index: i,
			Delta: models.Delta{
				Content: content,
			},
		}

		if cand.FinishReason != "" {
			delta.FinishReason = mapFinishReason(cand.FinishReason)
		}

		chunk.Choices = append(chunk.Choices, delta)
	}

	return chunk
}

// mapFinishReason converts Gemini finish reasons to OpenAI's lowercase set.
func mapFinishReason(geminiReason string) string {
	switch geminiReason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	default:
		return "stop"
	}
}

// stripModelsPrefix removes the "models/" prefix from a Gemini model name.
func stripModelsPrefix(name string) string {
	return strings.TrimPrefix(name, "models/")
}
