package streaming

import (
    "encoding/json"
    "fmt"
    "net/http"

    "github.com/nglong14/llmgateway/internal/models"
)

// WriteSSEChunk writes a single SSE event containing a StreamChunk.
// Format: "data: {json}\n\n"
func WriteSSEChunk(w http.ResponseWriter, chunk *models.StreamChunk) error {
    data, err := json.Marshal(chunk)
    if err != nil {
        return fmt.Errorf("sse: marshal chunk: %w", err)
    }

    _, err = fmt.Fprintf(w, "data: %s\n\n", data)
    if err != nil {
        return fmt.Errorf("sse: write chunk: %w", err)
    }

    // Flush immediately so the client receives the event right away.
    if flusher, ok := w.(http.Flusher); ok {
        flusher.Flush()
    }

    return nil
}

// WriteSSEDone writes the final "data: [DONE]" marker.
func WriteSSEDone(w http.ResponseWriter) error {
    _, err := fmt.Fprintf(w, "data: [DONE]\n\n")
    if err != nil {
        return err
    }
    if flusher, ok := w.(http.Flusher); ok {
        flusher.Flush()
    }
    return nil
}

// SetSSEHeaders sets the required headers for SSE responses.
func SetSSEHeaders(w http.ResponseWriter) {
    w.Header().Set("Content-Type", "text/event-stream")
    w.Header().Set("Cache-Control", "no-cache")
    w.Header().Set("Connection", "keep-alive")
}