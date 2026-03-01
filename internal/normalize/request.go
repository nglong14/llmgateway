// Package normalize handles the normalization of requests and responses.
package normalize

import "github.com/nglong14/llmgateway/internal/models"

// NormalizeRequest prepares a ChatCompletionRequest for a specific provider.
func NormalizeRequest(providerName string, req *models.ChatCompletionRequest) (*models.ChatCompletionRequest, error) {
	switch providerName {
	case "openai":
		return req, nil
	case "gemini":
		return req, nil
	case "anthropic":
		return req, nil
	default:
		return req, nil
	}
}
