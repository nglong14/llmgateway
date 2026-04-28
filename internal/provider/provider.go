//Package provider handles the provider interface and implementations.
package provider

import (
	"context"

	"github.com/nglong14/llmgateway/internal/models"
)

type Provider interface {
	Name() string

	ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error)

	ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error)

	ListModels(ctx context.Context) ([]models.ModelInfo, error)

	HealthCheck(ctx context.Context) error
}
