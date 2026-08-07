// Native Anthropic API types.
package anthropic

type anthropicRequest struct {
	Model       string             `json:"model"`
	Messages    []anthropicMessage `json:"messages"`
	System      string             `json:"system,omitempty"`
	MaxTokens   int                `json:"max_tokens"`
	Temperature *float64           `json:"temperature,omitempty"`
	Stream      bool               `json:"stream,omitempty"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID         string                  `json:"id"`
	Type       string                  `json:"type"` // "message"
	Role       string                  `json:"role"`
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      *anthropicUsage         `json:"usage,omitempty"`
}

type anthropicContentBlock struct {
	Type string `json:"type"` // "text"
	Text string `json:"text"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicStreamEvent struct {
	Type string `json:"type"`
}

type anthropicMessageStart struct {
	Type    string            `json:"type"`
	Message anthropicResponse `json:"message"`
}

type anthropicBlockDelta struct {
	Type  string                   `json:"type"`
	Index int                      `json:"index"`
	Delta anthropicBlockDeltaInner `json:"delta"`
}

type anthropicBlockDeltaInner struct {
	Type string `json:"type"` // "text_delta"
	Text string `json:"text"`
}

type anthropicMessageDelta struct {
	Type  string                     `json:"type"`
	Delta anthropicMessageDeltaInner `json:"delta"`
	Usage *anthropicUsage            `json:"usage,omitempty"`
}

type anthropicMessageDeltaInner struct {
	StopReason string `json:"stop_reason,omitempty"`
}

type anthropicModelsResponse struct {
	Data    []anthropicModelInfo `json:"data"`
	HasMore bool                 `json:"has_more"`
	LastID  string               `json:"last_id"`
}

type anthropicModelInfo struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Type        string `json:"type"` // "model"
}
