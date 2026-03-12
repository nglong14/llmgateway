package gemini

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

// Client implements provider.Provider interface
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a Gemini provider client
func New(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Name() string { return "gemini" }

// setHeaders adds Gemini-specific headers (auth + content type) to the request.
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", c.apiKey)
}

// toGeminiRequest converts unified request into Gemini's native format
func toGeminiRequest(req *models.ChatCompletionRequest) *geminiRequest {
	gr := &geminiRequest{}

	// Separate system messages from conversation messages.
	for _, msg := range req.Messages {
		if msg.Role == models.RoleSystem {
			// Gemini uses a dedicated systemInstruction field.
			gr.SystemInstruction = &geminiContent{
				Parts: []geminiPart{{Text: msg.Content}},
			}
			continue
		}

		// Map roles: OpenAI "assistant" → Gemini "model".
		role := msg.Role
		if role == models.RoleAssistant {
			role = "model"
		}

		gr.Contents = append(gr.Contents, geminiContent{
			Role:  role,
			Parts: []geminiPart{{Text: msg.Content}},
		})
	}

	// Map generation config.
	if req.Temperature != nil || req.MaxTokens != nil {
		gr.GenerationConfig = &geminiGenConfig{
			Temperature:     req.Temperature,
			MaxOutputTokens: req.MaxTokens,
		}
	}

	return gr
}

// Converts a Gemini response into our unified format
func toUnifiedResponse(gr *geminiResponse, model string) *models.ChatCompletionResponse {
	resp := &models.ChatCompletionResponse{
		Object: "chat.completion",
		Model:  model,
	}

	for i, cand := range gr.Candidates {
		// Extract text from parts.
		var content string
		for _, part := range cand.Content.Parts {
			content += part.Text
		}

		resp.Choices = append(resp.Choices, models.Choice{
			Index: i,
			Message: models.Message{
				Role:    models.RoleAssistant,
				Content: content,
			},
			FinishReason: mapFinishReason(cand.FinishReason),
		})
	}

	// Map usage metadata.
	if gr.UsageMetadata != nil {
		resp.Usage = &models.Usage{
			PromptTokens:     gr.UsageMetadata.PromptTokenCount,
			CompletionTokens: gr.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      gr.UsageMetadata.TotalTokenCount,
		}
	}

	return resp
}

// Converts Gemini finish reasons to OpenAI's lowercase
func mapFinishReason(geminiReason string) string {
	switch geminiReason {
	case "STOP":
		return "stop"
	case "MAX_TOKENS":
		return "length"
	case "SAFETY":
		return "content_filter"
	default:
		return "stop"
	}
}

func (c *Client) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	// Convert to Gemini format.
	geminiReq := toGeminiRequest(req)

	body, err := json.Marshal(geminiReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: marshal request: %w", err)
	}

	// Build URL: /v1beta/models/{model}:generateContent
	url := fmt.Sprintf("%s/models/%s:generateContent",
		c.baseURL, req.Model)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("gemini: create request: %w", err)
	}
	c.setHeaders(httpReq)

	// Send request.
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("gemini: send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse Gemini response.
	var geminiResp geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&geminiResp); err != nil {
		return nil, fmt.Errorf("gemini: decode response: %w", err)
	}

	// Convert to unified format.
	return toUnifiedResponse(&geminiResp, req.Model), nil
}

func (c *Client) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
    chunks := make(chan *models.StreamChunk, 10)
    errCh := make(chan error, 1)

    go func() {
        defer close(chunks)
        defer close(errCh)

        geminiReq := toGeminiRequest(req)

        body, err := json.Marshal(geminiReq)
        if err != nil {
            errCh <- fmt.Errorf("gemini: marshal request: %w", err)
            return
        }

        // Use streamGenerateContent endpoint.
        url := fmt.Sprintf("%s/models/%s:streamGenerateContent?alt=sse",
            c.baseURL, req.Model)

        httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
        if err != nil {
            errCh <- fmt.Errorf("gemini: create request: %w", err)
            return
        }
        c.setHeaders(httpReq)

        resp, err := c.httpClient.Do(httpReq)
        if err != nil {
            errCh <- fmt.Errorf("gemini: send request: %w", err)
            return
        }
        defer resp.Body.Close()

        if resp.StatusCode != http.StatusOK {
            respBody, _ := io.ReadAll(resp.Body)
            errCh <- fmt.Errorf("gemini: API error (status %d): %s", resp.StatusCode, string(respBody))
            return
        }

        // Gemini with alt=sse returns SSE format: "data: {json}\n\n"
        scanner := bufio.NewScanner(resp.Body)
        scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // default 64KB start, max 1MB
        for scanner.Scan() {
            line := scanner.Text()

            if !strings.HasPrefix(line, "data: ") {
                continue
            }

            data := strings.TrimPrefix(line, "data: ")

            // Parse the Gemini response chunk.
            var geminiResp geminiResponse
            if err := json.Unmarshal([]byte(data), &geminiResp); err != nil {
                errCh <- fmt.Errorf("gemini: decode stream chunk: %w", err)
                return
            }

            // Convert each candidate to a StreamChunk.
            chunk := toStreamChunk(&geminiResp, req.Model)

            select {
            case chunks <- chunk:
            case <-ctx.Done():
                return
            }
        }

        if err := scanner.Err(); err != nil {
            errCh <- fmt.Errorf("gemini: read stream: %w", err)
        }
    }()

    return chunks, errCh
}

// toStreamChunk converts a Gemini streaming response to our unified StreamChunk.
func toStreamChunk(gr *geminiResponse, model string) *models.StreamChunk {
    chunk := &models.StreamChunk{
        Object: "chat.completion.chunk",
        Model:  model,
    }

    for i, cand := range gr.Candidates {
        var content string
        for _, part := range cand.Content.Parts {
            content += part.Text
        }

        delta := models.StreamDelta{
            Index: i,
            Delta: models.Delta{
                Content: content,
            },
        }

        if cand.FinishReason != "" {
            delta.FinishReason = mapFinishReason(cand.FinishReason)
        }

        chunk.Choices = append(chunk.Choices, delta)
    }

    return chunk
}

func (c *Client) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
    url := fmt.Sprintf("%s/models", c.baseURL)

    httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
    if err != nil {
        return nil, fmt.Errorf("gemini: create request: %w", err)
    }
    c.setHeaders(httpReq)

    resp, err := c.httpClient.Do(httpReq)
    if err != nil {
        return nil, fmt.Errorf("gemini: list models: %w", err)
    }
    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("gemini: list models returned status %d", resp.StatusCode)
    }

    var result struct {
        Models []struct {
            Name string `json:"name"` // "models/gemini-2.0-flash"
        } `json:"models"`
    }
    if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
        return nil, fmt.Errorf("gemini: decode models: %w", err)
    }

    var infos []models.ModelInfo
    for _, m := range result.Models {
        // Strip "models/" prefix to get just the model ID.
        id := strings.TrimPrefix(m.Name, "models/")
        infos = append(infos, models.ModelInfo{
            ID:      id,
            Object:  "model",
            OwnedBy: "google",
        })
    }

    return infos, nil
}

func (c *Client) HealthCheck(ctx context.Context) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    _, err := c.ListModels(ctx)
    return err
}