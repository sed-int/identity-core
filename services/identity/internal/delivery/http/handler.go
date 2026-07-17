// Package http builds the identity service's HTTP surface (PRD §4.1):
// the gRPC-gateway REST proxy plus the plain OIDC endpoints (JWKS, discovery).
package http

import (
	"context"
	"crypto/rsa"
	"net/http"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	identityv1 "identity-service/api/gen/identity/v1"
	"identity-service/services/identity/internal/token"
)

// NewHandler returns the full HTTP handler: OIDC endpoints served directly,
// everything else proxied to the gRPC server at grpcAddr.
func NewHandler(ctx context.Context, issuer, grpcAddr string, key *rsa.PrivateKey) (http.Handler, error) {
	gwMux := runtime.NewServeMux()
	if err := identityv1.RegisterIdentityServiceHandlerFromEndpoint(
		ctx, gwMux, grpcAddr,
		[]grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())},
	); err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /oauth2/v1/jwks", token.JWKSHandler(issuer, key))
	mux.HandleFunc("GET /.well-known/openid-configuration", token.DiscoveryHandler(issuer))
	mux.Handle("/", gwMux)
	return mux, nil
}
