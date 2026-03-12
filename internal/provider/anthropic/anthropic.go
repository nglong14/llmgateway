package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/nglong14/llmgateway/internal/models"
)

const anthropicVersion = "2023-06-01"

// Client implements provider.Provider for Anthropic.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates an Anthropic provider client.
func New(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Name() string { return "anthropic" }

// setHeaders adds Anthropic-specific headers to the request.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", c.apiKey)
	req.Header.Set("anthropic-version", anthropicVersion)
}

// toAnthropicRequest converts a unified request into Anthropic's native format.
func toAnthropicRequest(req *models.ChatCompletionRequest) *anthropicRequest {
	ar := &anthropicRequest{
		Model:       req.Model,
		Temperature: req.Temperature,
	}

	// Anthropic requires max_tokens; default to 4096 if not set.
	if req.MaxTokens != nil {
		ar.MaxTokens = *req.MaxTokens
	} else {
		ar.MaxTokens = 4096
	}

	// Separate system messages into the top-level system field.
	var systemParts []string
	for _, msg := range req.Messages {
		if msg.Role == models.RoleSystem {
			systemParts = append(systemParts, msg.Content)
			continue
		}

		ar.Messages = append(ar.Messages, anthropicMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	if len(systemParts) > 0 {
		ar.System = strings.Join(systemParts, "\n")
	}

	return ar
}

// toUnifiedResponse converts an Anthropic response into the unified format.
func toUnifiedResponse(ar *anthropicResponse) *models.ChatCompletionResponse {
	// Concatenate all text content blocks.
	var content string
	for _, block := range ar.Content {
		if block.Type == "text" {
			content += block.Text
		}
	}

	resp := &models.ChatCompletionResponse{
		ID:     ar.ID,
		Object: "chat.completion",
		Model:  ar.Model,
		Choices: []models.Choice{
			{
				Index: 0,
				Message: models.Message{
					Role:    models.RoleAssistant,
					Content: content,
				},
				FinishReason: mapStopReason(ar.StopReason),
			},
		},
	}

	if ar.Usage != nil {
		resp.Usage = &models.Usage{
			PromptTokens:     ar.Usage.InputTokens,
			CompletionTokens: ar.Usage.OutputTokens,
			TotalTokens:      ar.Usage.InputTokens + ar.Usage.OutputTokens,
		}
	}

	return resp
}

// mapStopReason converts Anthropic stop reasons to OpenAI-compatible ones.
func mapStopReason(reason string) string {
	switch reason {
	case "end_turn":
		return "stop"
	case "max_tokens":
		return "length"
	case "stop_sequence":
		return "stop"
	default:
		return "stop"
	}
}

// ChatCompletion sends a non-streaming chat request to the Anthropic API.
func (c *Client) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	ar := toAnthropicRequest(req)
	ar.Stream = false

	body, err := json.Marshal(ar)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/v1/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("anthropic: create request: %w", err)
	}
	c.setHeaders(httpReq)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("anthropic: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("anthropic: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	var result anthropicResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("anthropic: decode response: %w", err)
	}

	return toUnifiedResponse(&result), nil
}

// ChatCompletionStream sends a streaming chat request to the Anthropic API.
func (c *Client) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	chunks := make(chan *models.StreamChunk, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errCh)

		ar := toAnthropicRequest(req)
		ar.Stream = true

		body, err := json.Marshal(ar)
		if err != nil {
			errCh <- fmt.Errorf("anthropic: marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/v1/messages", bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("anthropic: create request: %w", err)
			return
		}
		c.setHeaders(httpReq)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("anthropic: send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("anthropic: API error (status %d): %s", resp.StatusCode, string(respBody))
			return
		}

		// Parse Anthropic SSE events.
		// Format: "event: <type>\ndata: <json>\n\n"
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // default 64KB start, max 1MB
		for scanner.Scan() {
			line := scanner.Text()

			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// Determine event type.
			var event anthropicStreamEvent
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue // skip unparsable lines
			}

			switch event.Type {
			case "content_block_delta":
				var delta anthropicBlockDelta
				if err := json.Unmarshal([]byte(data), &delta); err != nil {
					errCh <- fmt.Errorf("anthropic: decode content_block_delta: %w", err)
					return
				}
				if delta.Delta.Type == "text_delta" {
					chunk := &models.StreamChunk{
						Object: "chat.completion.chunk",
						Model:  req.Model,
						Choices: []models.StreamDelta{
							{
								Index: delta.Index,
								Delta: models.Delta{
									Content: delta.Delta.Text,
								},
							},
						},
					}
					select {
					case chunks <- chunk:
					case <-ctx.Done():
						return
					}
				}

			case "message_delta":
				var md anthropicMessageDelta
				if err := json.Unmarshal([]byte(data), &md); err != nil {
					errCh <- fmt.Errorf("anthropic: decode message_delta: %w", err)
					return
				}
				if md.Delta.StopReason != "" {
					chunk := &models.StreamChunk{
						Object: "chat.completion.chunk",
						Model:  req.Model,
						Choices: []models.StreamDelta{
							{
								Index:        0,
								FinishReason: mapStopReason(md.Delta.StopReason),
							},
						},
					}
					select {
					case chunks <- chunk:
					case <-ctx.Done():
						return
					}
				}

			// Skip ping, message_start, content_block_start/stop, message_stop.
			default:
				continue
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("anthropic: read stream: %w", err)
		}
	}()

	return chunks, errCh
}

// ListModels returns model IDs available from Anthropic.
// Handles pagination via the has_more / last_id cursor.
func (c *Client) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	var allModels []models.ModelInfo
	afterID := ""

	for {
		url := c.baseURL + "/v1/models?limit=100"
		if afterID != "" {
			url += "&after_id=" + afterID
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return nil, fmt.Errorf("anthropic: create request: %w", err)
		}
		c.setHeaders(httpReq)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("anthropic: list models: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("anthropic: list models returned status %d", resp.StatusCode)
		}

		var result anthropicModelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return nil, fmt.Errorf("anthropic: decode models: %w", err)
		}

		for _, m := range result.Data {
			allModels = append(allModels, models.ModelInfo{
				ID:      m.ID,
				Object:  "model",
				OwnedBy: "anthropic",
			})
		}

		if !result.HasMore {
			break
		}
		afterID = result.LastID
	}

	return allModels, nil
}

// HealthCheck verifies the Anthropic provider is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	return err
}
