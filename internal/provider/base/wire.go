package base

import (
	"net/http"

	"github.com/nglong14/llmgateway/internal/models"
)

type Wire interface {
	Name() string

	ChatURL(baseURL string, req *models.ChatCompletionRequest, stream bool) string

	ModelListURL(baseURL, cursor string) string

	AuthHeaders(req *http.Request, apiKey string)

	EncodeRequest(req *models.ChatCompletionRequest, stream bool) ([]byte, error)

	DecodeCompletion(data []byte, model string) (*models.ChatCompletionResponse, error)

	DecodeStreamData(data []byte, model string) (*models.StreamChunk, error)

	StreamDone(data []byte) bool

	DecodeModels(data []byte) ([]models.ModelInfo, string, error)
}
