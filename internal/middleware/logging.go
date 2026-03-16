package middleware

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/nglong14/llmgateway/internal/ctxutil"
)

// responseWriter wraps http.ResponseWriter to capture the status code
type responseWriter struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func wrapResponseWriter(w http.ResponseWriter) *responseWriter {
	return &responseWriter{ResponseWriter: w, status: http.StatusOK}
}

func (rw *responseWriter) Status() int {
	return rw.status
}

func (rw *responseWriter) WriteHeader(code int) {
	if rw.wroteHeader {
		return
	}
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
	rw.wroteHeader = true
}

// LoggingMiddleware logs the start and end of each request, and injects 
// a correlation ID and a request-scoped logger into the context.
func LoggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Generate a new correlation ID
		correlationID := uuid.New().String()

		// Create a request-scoped logger with the correlation ID attached
		reqLogger := slog.Default().With(slog.String("correlation_id", correlationID))

		// Add custom fields to the logger for this specific request
		reqLogger = reqLogger.With(
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("remote_ip", r.RemoteAddr),
		)

		// Create a new context with the logger and correlation ID
		ctx := ctxutil.WithCorrelationID(r.Context(), correlationID)
		ctx = ctxutil.WithLogger(ctx, reqLogger)
		r = r.WithContext(ctx)

		// Log request receipt
		reqLogger.Info("request received")

		// Wrap the response writer to capture the status code
		wrappedWriter := wrapResponseWriter(w)

		// Process the request
		next.ServeHTTP(wrappedWriter, r)

		// Log request completion
		latency := time.Since(start).Milliseconds()
		reqLogger.Info("request completed",
			slog.Int("status_code", wrappedWriter.Status()),
			slog.Int64("latency_total_ms", latency),
		)
	})
}
