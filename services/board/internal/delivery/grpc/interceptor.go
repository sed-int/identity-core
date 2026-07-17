package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"identity-service/pkg/logger"
)

// LoggingInterceptor mirrors the identity service's request logging.
func LoggingInterceptor(log zerolog.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		ctx = logger.WithRequestID(ctx, log, uuid.NewString())

		start := time.Now()
		resp, err := handler(ctx, req)

		evt := logger.FromContext(ctx).Info()
		if err != nil {
			evt = logger.FromContext(ctx).Warn().Str("error", err.Error())
		}
		evt.Str("method", info.FullMethod).
			Str("code", status.Code(err).String()).
			Dur("duration", time.Since(start)).
			Msg("rpc")
		return resp, err
	}
}
