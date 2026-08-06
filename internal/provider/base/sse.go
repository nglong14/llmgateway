package base

import (
	"bufio"
	"io"
	"net/http"
	"strings"
)

// scanSSE reads an SSE stream from r and invokes fn with each raw "data: "
// payload (without the prefix). The stream stops early when fn returns false,
// which providers use for terminal markers like "[DONE]".
//
// It returns the scanner error, if any. Empty lines and non-data events
// (e.g. "event:", "id:") are skipped.
func scanSSE(r io.Reader, fn func(data []byte) bool) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024) // default 64KB start, max 1MB

	for scanner.Scan() {
		line := scanner.Text()

		if !strings.HasPrefix(line, "data: ") {
			continue
		}

		data := []byte(strings.TrimPrefix(line, "data: "))
		if !fn(data) {
			return nil
		}
	}

	return scanner.Err()
}

func readAllBody(r io.Reader) string {
	data, _ := io.ReadAll(io.LimitReader(r, 1<<20))
	return string(data)
}

func readBody(resp *http.Response) ([]byte, error) {
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}
