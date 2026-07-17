// Package logger provides the shared structured logger (PRD §3), backed by zerolog.
// All services log JSON to stdout with a "service" field; request-scoped
// logs carry a "request_id" field propagated via context.
package logger

import (
	"context"
	"os"

	"github.com/rs/zerolog"
)

// New returns a JSON logger tagged with the service name.
func New(service string) zerolog.Logger {
	return zerolog.New(os.Stdout).With().Timestamp().Str("service", service).Logger()
}

// WithRequestID returns a context carrying a child logger tagged with the request ID.
func WithRequestID(ctx context.Context, log zerolog.Logger, requestID string) context.Context {
	child := log.With().Str("request_id", requestID).Logger()
	return child.WithContext(ctx)
}

// FromContext returns the request-scoped logger stored by WithRequestID,
// or a disabled logger if none is present.
func FromContext(ctx context.Context) *zerolog.Logger {
	return zerolog.Ctx(ctx)
}
