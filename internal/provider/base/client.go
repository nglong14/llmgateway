package base

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/nglong14/llmgateway/internal/models"
)

// Client implements provider.Provider on top of a Wire adapter. It owns the HTTP
// transport, status-error handling, SSE streaming, model pagination, and health
// checks so that provider packages only need to define their wire format.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
	wire       Wire
}

func New(baseURL, apiKey string, wire Wire) *Client {
	return &Client{
		apiKey:  apiKey,
		baseURL: baseURL,
		httpClient: &http.Client{
			Transport: &http.Transport{
				TLSHandshakeTimeout:   10 * time.Second,
				ResponseHeaderTimeout: 30 * time.Second,
			},
		},
		wire: wire,
	}
}

func (c *Client) Name() string { return c.wire.Name() }

// ChatCompletion sends a non-streaming chat request.
func (c *Client) ChatCompletion(ctx context.Context, req *models.ChatCompletionRequest) (*models.ChatCompletionResponse, error) {
	body, err := c.wire.EncodeRequest(req, false)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.wire.ChatURL(c.baseURL, req, false), bytes.NewReader(body))
	if err != nil {
		return nil, c.apiError("create request", err)
	}
	c.wire.AuthHeaders(httpReq, c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, c.apiError("send request", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: API error (status %d): %s", c.wire.Name(), resp.StatusCode, readAllBody(resp.Body))
	}

	data, err := readBody(resp)
	if err != nil {
		return nil, c.apiError("read response", err)
	}
	return c.wire.DecodeCompletion(data, req.Model)
}

func (c *Client) ChatCompletionStream(ctx context.Context, req *models.ChatCompletionRequest) (<-chan *models.StreamChunk, <-chan error) {
	chunks := make(chan *models.StreamChunk, 10)
	errCh := make(chan error, 1)

	go func() {
		defer close(chunks)
		defer close(errCh)

		body, err := c.wire.EncodeRequest(req, true)
		if err != nil {
			errCh <- err
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			c.wire.ChatURL(c.baseURL, req, true), bytes.NewReader(body))
		if err != nil {
			errCh <- c.apiError("create request", err)
			return
		}
		c.wire.AuthHeaders(httpReq, c.apiKey)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			errCh <- c.apiError("send request", err)
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			errCh <- fmt.Errorf("%s: API error (status %d): %s", c.wire.Name(), resp.StatusCode, readAllBody(resp.Body))
			return
		}

		if err := scanSSE(resp.Body, func(data []byte) bool {
			if c.wire.StreamDone(data) {
				return false
			}

			chunk, err := c.wire.DecodeStreamData(data, req.Model)
			if err != nil {
				errCh <- err
				return false
			}
			if chunk == nil {
				return true // skip payload, keep scanning
			}

			select {
			case chunks <- chunk:
				return true
			case <-ctx.Done():
				return false
			}
		}); err != nil {
			errCh <- c.apiError("read stream", err)
		}
	}()

	return chunks, errCh
}

func (c *Client) ListModels(ctx context.Context) ([]models.ModelInfo, error) {
	var all []models.ModelInfo
	cursor := ""

	for {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet,
			c.wire.ModelListURL(c.baseURL, cursor), nil)
		if err != nil {
			return nil, c.apiError("create request", err)
		}
		c.wire.AuthHeaders(httpReq, c.apiKey)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, c.apiError("list models", err)
		}

		if resp.StatusCode != http.StatusOK {
			body := readAllBody(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("%s: list models returned status %d: %s", c.wire.Name(), resp.StatusCode, body)
		}

		data, err := readBody(resp)
		if err != nil {
			return nil, c.apiError("read models", err)
		}

		infos, next, err := c.wire.DecodeModels(data)
		if err != nil {
			return nil, err
		}
		all = append(all, infos...)

		if next == "" {
			return all, nil
		}
		cursor = next
	}
}

func (c *Client) HealthCheck(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := c.ListModels(ctx)
	return err
}

func (c *Client) apiError(context string, err error) error {
	return fmt.Errorf("%s: %s: %w", c.wire.Name(), context, err)
}
