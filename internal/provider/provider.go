package provider

import (
	"context"
	
	"github.com/nglong14/llmgateway/internal/models"
)

// Provider is the interface for every provider
type Provider interface {
	//Name of provider
	Name() string
	
	//Non-streaming chat request
	ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error)
	
	//Streaming chat request, send chunks to channel
	ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error)

	//ListModels returns model IDs available from this provider
	ListModels(ctx context.Context) ([]models.ModelInfo, error)

	//Verify the provider is valid
	HealthCheck(ctx context.Context) error
}