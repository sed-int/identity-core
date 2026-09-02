package grpc

import (
	"context"
	"strings"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	boardv1 "identity-service/api/gen/board/v1"
	"identity-service/pkg/jwks"
)

type claimsKey struct{}

// methodsRequiringAuth lists RPCs that must carry a valid access token.
// ListPosts stays public.
var methodsRequiringAuth = map[string]bool{
	boardv1.BoardService_CreatePost_FullMethodName: true,
	boardv1.BoardService_UpdatePost_FullMethodName: true,
	boardv1.BoardService_DeletePost_FullMethodName: true,
}

// AuthInterceptor verifies Bearer tokens statelessly via the JWKS verifier
// (PRD §6: no identity DB access) and injects claims into the context.
func AuthInterceptor(verifier *jwks.Verifier) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		if !methodsRequiringAuth[info.FullMethod] {
			return handler(ctx, req)
		}

		token := bearerToken(ctx)
		if token == "" {
			return nil, status.Error(codes.Unauthenticated, "missing bearer token")
		}
		claims, err := verifier.Verify(ctx, token)
		if err != nil {
			return nil, status.Error(codes.Unauthenticated, "invalid access token")
		}
		return handler(context.WithValue(ctx, claimsKey{}, claims), req)
	}
}

// ClaimsFromContext returns the verified claims set by AuthInterceptor.
func ClaimsFromContext(ctx context.Context) (*jwks.Claims, bool) {
	c, ok := ctx.Value(claimsKey{}).(*jwks.Claims)
	return c, ok
}

// bearerToken extracts the Bearer token from gRPC metadata; the gRPC-gateway
// forwards the HTTP Authorization header under both keys checked here.
func bearerToken(ctx context.Context) string {
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return ""
	}
	for _, key := range []string{"authorization", "grpcgateway-authorization"} {
		for _, v := range md.Get(key) {
			if strings.HasPrefix(strings.ToLower(v), "bearer ") {
				return strings.TrimSpace(v[len("bearer "):])
			}
		}
	}
	return ""
}
