// Native Anthropic API types.
package anthropic

// Native Anthropic Messages API request body.
type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

// A single message in an Anthropic conversation.
type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Native Anthropic Messages API response.
type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"` // "message"
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      *anthropicUsage         `json:"usage,omitempty"`
}

// One content block in the response (text only for now).
type anthropicContentBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

// Token usage from Anthropic.
type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// --- Streaming event types ---

// Envelope for every SSE data payload.
type anthropicStreamEvent struct {
	Type string `json:"type"`
}

// message_start event payload.
type anthropicMessageStart struct {
	Type    string            `json:"type"`
	Message anthropicResponse `json:"message"`
}

// content_block_delta event payload.
type anthropicBlockDelta struct {
	Type  string                   `json:"type"`
	Index int                      `json:"index"`
	Delta anthropicBlockDeltaInner `json:"delta"`
}

// The inner delta within a content_block_delta event.
type anthropicBlockDeltaInner struct {
	Type string `json:"type"` // "text_delta"
	Text string `json:"text"`
}

// message_delta event payload (carries stop_reason + final usage).
type anthropicMessageDelta struct {
	Type  string                     `json:"type"`
	Delta anthropicMessageDeltaInner `json:"delta"`
	Usage *anthropicUsage            `json:"usage,omitempty"`
}

// Inner delta of a message_delta event.
type anthropicMessageDeltaInner struct {
	StopReason string `json:"stop_reason,omitempty"`
}

// List-models API response.
type anthropicModelsResponse struct {
	Data    []anthropicModelInfo `json:"data"`
	HasMore bool                 `json:"has_more"`
	LastID  string               `json:"last_id"`
}

// Single model entry from the list-models API.
type anthropicModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"` // "model"
}
