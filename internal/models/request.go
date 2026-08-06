//Request handling for all providers
package models

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Temperature *float64  `json:"temperature,omitempty"` // When clients do not send values, automatically set to nil -> omit from JSON
	MaxTokens   *int      `json:"max_tokens,omitempty"`
	Stream      bool      `json:"stream,omitempty"`
	Provider    string    `json:"provider"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSystem    = "system"
)
