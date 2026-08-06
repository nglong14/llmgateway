package openai

import (
	"github.com/nglong14/llmgateway/internal/provider/base"
)

func New(apiKey, baseURL string) *base.Client {
	return base.NewOpenAICompat("openai", apiKey, baseURL)
}
