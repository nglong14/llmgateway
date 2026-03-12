package deepseek

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

// Client implements provider.Provider for DeepSeek.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// New creates a DeepSeek provider client.
func New(apiKey, baseURL string) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (c *Client) Name() string { return "deepseek" }

// Sends a non-streaming chat request to the DeepSeek API.
func (c *Client) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	// Build the request body (unified format).
	reqCopy := *req
	reqCopy.Stream = false
	reqCopy.Provider = "" // strip gateway-only field

	body, err := json.Marshal(reqCopy)
	if err != nil {
		return nil, fmt.Errorf("deepseek: marshal request: %w", err)
	}

	// Create HTTP request.
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("deepseek: create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	// Send request.
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek: send request: %w", err)
	}
	defer resp.Body.Close()

	// Check for HTTP errors.
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("deepseek: API error (status %d): %s", resp.StatusCode, string(respBody))
	}

	// Parse response.
	var result models.ChatCompletionResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("deepseek: decode response: %w", err)
	}

	return &result, nil
}

// ChatCompletionStream sends a streaming chat request to the DeepSeek API.
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
			errCh <- fmt.Errorf("deepseek: marshal request: %w", err)
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.baseURL+"/chat/completions", bytes.NewReader(body))
		if err != nil {
			errCh <- fmt.Errorf("deepseek: create request: %w", err)
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			errCh <- fmt.Errorf("deepseek: send request: %w", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
			errCh <- fmt.Errorf("deepseek: API error (status %d): %s", resp.StatusCode, string(respBody))
			return
		}

		// Read SSE events line by line.
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // default 64KB start, max 1MB
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
				errCh <- fmt.Errorf("deepseek: decode chunk: %w", err)
				return
			}

			select {
			case chunks <- &chunk:
			case <-ctx.Done():
				return
			}
		}

		if err := scanner.Err(); err != nil {
			errCh <- fmt.Errorf("deepseek: read stream: %w", err)
		}
	}()

	return chunks, errCh
}

// ListModels returns model IDs available from DeepSeek.
func (c *Client) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("deepseek: create request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("deepseek: list models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("deepseek: list models returned status %d", resp.StatusCode)
	}

	var result struct {
		Data []models.ModelInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("deepseek: decode models: %w", err)
	}

	return result.Data, nil
}

// HealthCheck verifies the DeepSeek provider is reachable.
func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	return err
}
