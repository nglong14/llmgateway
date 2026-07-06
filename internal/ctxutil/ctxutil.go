package ctxutil

import (
	"context"
	"log/slog"
)

type contextKey string

const (
	correlationIDKey contextKey = "correlation_id"
	loggerKey        contextKey = "logger"
	apiKeyNameKey    contextKey = "api_key_name"
)

// WithCorrelationID returns a new context with the given correlation ID.
func WithCorrelationID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, correlationIDKey, id)
}

// CorrelationID returns the correlation ID from the context, or empty string.
func CorrelationID(ctx context.Context) string {
	if v, ok := ctx.Value(correlationIDKey).(string); ok {
		return v
	}
	return ""
}

// WithLogger returns a new context with the given slog.Logger.
func WithLogger(ctx context.Context, logger *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey, logger)
}

// Logger returns the slog.Logger from the context, or the default logger if none is set.
func Logger(ctx context.Context) *slog.Logger {
	if logger, ok := ctx.Value(loggerKey).(*slog.Logger); ok {
		return logger
	}
	return slog.Default()
}

// APIKeyName returns the authenticated key name, or "anonymous" if none.
func APIKeyName(ctx context.Context) string {
    if v, ok := ctx.Value(apiKeyNameKey).(string); ok {
        return v
    }
    return "anonymous"
}
