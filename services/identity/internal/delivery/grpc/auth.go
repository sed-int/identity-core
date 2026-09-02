package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc/metadata"
)

// bearerToken extracts the Authorization header forwarded by gRPC-gateway.
func bearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, key := range []string{"authorization", "grpcgateway-authorization"} {
		for _, value := range md.Get(key) {
			if strings.HasPrefix(strings.ToLower(value), "bearer ") {
				return strings.TrimSpace(value[len("bearer "):])
			}
		}
	}
	return ""
}
