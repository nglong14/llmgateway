//Response handling for all providers
package models

// Unified OpenAI-compatible response structure
type ChatCompletionResponse struct{
	ID      string   `json:"id"`
    Object  string   `json:"object"`   // always "chat.completion"
    Created int64    `json:"created"`
    Model   string   `json:"model"`
    Choices []Choice `json:"choices"`
    Usage   *Usage   `json:"usage,omitempty"`
}

// One completion choice
type Choice struct {
    Index 			int     `json:"index"`
    Message 		Message `json:"message"`
    FinishReason 	string  `json:"finish_reason"`
}

// Usage statistics
type Usage struct {
    PromptTokens     int `json:"prompt_tokens"`
    CompletionTokens int `json:"completion_tokens"`
    TotalTokens      int `json:"total_tokens"`
}

// One SSE event during streaming
type StreamChunk struct {
    ID      string        `json:"id"`
    Object  string        `json:"object"` // "chat.completion.chunk"
    Created int64         `json:"created"`
    Model   string        `json:"model"`
    Choices []StreamDelta `json:"choices"`
}

// Single delta within a stream chunk.
type StreamDelta struct {
    Index        int    `json:"index"`
    Delta        Delta  `json:"delta"`
    FinishReason string `json:"finish_reason,omitempty"` // Only appears on final chunk	
}

// Incremental content during streaming.
type Delta struct {
    Role    string `json:"role,omitempty"`
    Content string `json:"content,omitempty"`
}

// Represents a model in the /v1/models list.
type ModelInfo struct {
    ID      string `json:"id"`
    Object  string `json:"object"` // "model"
    OwnedBy string `json:"owned_by"`
}

// Response for GET /v1/models.
type ModelListResponse struct {
    Object string      `json:"object"` // "list"
    Data   []ModelInfo `json:"data"`
}
