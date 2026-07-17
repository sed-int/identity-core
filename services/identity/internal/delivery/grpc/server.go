package grpc

import (
	"github.com/rs/zerolog"
	"google.golang.org/grpc"

	identityv1 "identity-service/api/gen/identity/v1"
	"identity-service/services/identity/internal/usecase"
)

// NewServer builds the identity gRPC server with its handler and interceptors
// registered — the delivery layer owns its transport.
func NewServer(log zerolog.Logger, auth *usecase.Auth) *grpc.Server {
	s := grpc.NewServer(grpc.UnaryInterceptor(LoggingInterceptor(log)))
	identityv1.RegisterIdentityServiceServer(s, NewHandler(auth))
	return s
}
