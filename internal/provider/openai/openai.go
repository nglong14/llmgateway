package openai

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

// Implements provider.Provider for OpenAI.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates an OpenAI provider client.
func New(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Name() string { return "openai" }

// Sends a non-streaming chat request to the OpenAI API.
func (c *Client) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	// Build the request body (unified format).
	reqCopy := *req
	reqCopy.Stream = false
	reqCopy.Provider = "" // strip gateway-only field

	body, err := json.Marshal(reqCopy)
	if err != nil {
		return nil, fmt.Errorf("openai: marshal request: %w", err)
	}

	// Create HTTP request.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Send request.
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("openai: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response.
	var result models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai: decode response: %w", err)
	}

	return &result, nil
}

// ChatCompletionStream sends a streaming chat request to the OpenAI API.
// Returns a channel of chunks and a channel for any error.
// The chunks channel is closed when streaming is complete.
func (c *Client) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	chunks := make(chan *models.StreamChunk, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errCh)

		// Build streaming request.
		reqCopy := *req
		reqCopy.Stream = true
		reqCopy.Provider = ""

		body, err := json.Marshal(reqCopy)
		if err != nil {
			errCh <- fmt.Errorf("openai: marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("openai: create request: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("openai: send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			errCh <- fmt.Errorf("openai: API error (status %d): %s", resp.StatusCode, string(respBody))
			return
		}

		// Read SSE events line by line.
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines and non-data lines.
			if !strings.HasPrefix(line, "data: ") {
				continue
			}

			data := strings.TrimPrefix(line, "data: ")

			// The [DONE] marker signals end of stream.
			if data == "[DONE]" {
				return
			}

			// Parse the chunk.
			var chunk models.StreamChunk
			if err := json.Unmarshal([]byte(data), &chunk); err != nil {
				errCh <- fmt.Errorf("openai: decode chunk: %w", err)
				return
			}

			select {
			case chunks <- &chunk:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("openai: read stream: %w", err)
		}
	}()

	return chunks, errCh
}

// ListModels returns model IDs available from OpenAI.
func (c *Client) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("openai: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("openai: list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("openai: list models returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []models.ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("openai: decode models: %w", err)
	}

	return result.Data, nil
}

// HealthCheck verifies the OpenAI provider is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	return err
}
