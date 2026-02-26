package normalize

import "github.com/nglong14/llmgateway/internal/models"

// NormalizeResponse transforms a provider-specific response into the
func NormalizeResponse(providerName string, resp *models.ChatCompletionResponse) (*models.ChatCompletionResponse, error) {
	switch providerName {
	case "openai":
		return resp, nil
	default:
		return resp, nil
	}
}
