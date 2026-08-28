// Package grpc is the delivery layer: it adapts the generated gRPC contract
// to the auth usecase and maps domain errors to status codes.
package grpc

import (
	"context"
	"errors"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	identityv1 "identity-service/api/gen/identity/v1"
	"identity-service/services/identity/internal/domain"
	"identity-service/services/identity/internal/otp"
	"identity-service/services/identity/internal/rtr"
	"identity-service/services/identity/internal/token"
	"identity-service/services/identity/internal/usecase"
)

type Handler struct {
	identityv1.UnimplementedIdentityServiceServer
	auth *usecase.Auth
}

func NewHandler(auth *usecase.Auth) *Handler {
	return &Handler{auth: auth}
}

func (h *Handler) RequestOtp(ctx context.Context, req *identityv1.RequestOtpRequest) (*identityv1.RequestOtpResponse, error) {
	if req.GetPhoneNumber() == "" {
		return nil, status.Error(codes.InvalidArgument, "phone_number is required")
	}
	expiresIn, debugCode, err := h.auth.RequestOtp(ctx, req.GetPhoneNumber())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.RequestOtpResponse{ExpiresInSeconds: expiresIn, DebugCode: debugCode}, nil
}

func (h *Handler) VerifyOtp(ctx context.Context, req *identityv1.VerifyOtpRequest) (*identityv1.VerifyOtpResponse, error) {
	if req.GetPhoneNumber() == "" || req.GetCode() == "" {
		return nil, status.Error(codes.InvalidArgument, "phone_number and code are required")
	}
	res, err := h.auth.VerifyOtp(ctx, req.GetPhoneNumber(), req.GetCode(), req.GetDeviceFingerprint(), req.GetDeviceName())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.VerifyOtpResponse{
		NextStep:  identityv1.NextStep(res.NextStep),
		Tokens:    toProtoTokens(res.Tokens),
		FlowToken: res.FlowToken,
	}, nil
}

func (h *Handler) CompleteSignup(ctx context.Context, req *identityv1.CompleteSignupRequest) (*identityv1.CompleteSignupResponse, error) {
	if req.GetFlowToken() == "" || req.GetNickname() == "" {
		return nil, status.Error(codes.InvalidArgument, "flow_token and nickname are required")
	}
	tokens, err := h.auth.CompleteSignup(ctx, req.GetFlowToken(), req.GetNickname(), req.GetProfileImageUrl(),
		req.GetDeviceFingerprint(), req.GetDeviceName())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.CompleteSignupResponse{Tokens: toProtoTokens(tokens)}, nil
}

func (h *Handler) IssueToken(ctx context.Context, req *identityv1.IssueTokenRequest) (*identityv1.IssueTokenResponse, error) {
	tokens, err := h.auth.Refresh(ctx, req.GetGrantType(), req.GetRefreshToken())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.IssueTokenResponse{
		AccessToken:  tokens.AccessToken,
		IdToken:      tokens.IDToken,
		RefreshToken: tokens.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    tokens.ExpiresIn,
	}, nil
}

func toProtoTokens(t *usecase.TokenPair) *identityv1.TokenPair {
	if t == nil {
		return nil
	}
	return &identityv1.TokenPair{
		AccessToken:  t.AccessToken,
		IdToken:      t.IDToken,
		RefreshToken: t.RefreshToken,
		TokenType:    "Bearer",
		ExpiresIn:    t.ExpiresIn,
	}
}

func mapErr(err error) error {
	switch {
	case errors.Is(err, otp.ErrRateLimited):
		return status.Error(codes.ResourceExhausted, err.Error())
	case errors.Is(err, otp.ErrInvalidCode),
		errors.Is(err, otp.ErrTooManyAttempts),
		errors.Is(err, rtr.ErrInvalidToken),
		errors.Is(err, rtr.ErrReuseDetected),
		errors.Is(err, token.ErrInvalidFlowToken):
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrLoginNotAllowed):
		return status.Error(codes.PermissionDenied, err.Error())
	case errors.Is(err, domain.ErrCredentialConflict):
		return status.Error(codes.AlreadyExists, err.Error())
	case errors.Is(err, usecase.ErrUnsupportedGrant):
		return status.Error(codes.InvalidArgument, err.Error())
	default:
		return status.Error(codes.Internal, "internal error")
	}
}
