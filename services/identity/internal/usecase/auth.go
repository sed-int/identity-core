// Package usecase orchestrates the auth flows (PRD §4.2 state routing).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/rs/zerolog"

	"identity-service/services/identity/internal/domain"
	"identity-service/services/identity/internal/otp"
	"identity-service/services/identity/internal/rtr"
	"identity-service/services/identity/internal/token"
)

var ErrUnsupportedGrant = errors.New("unsupported grant_type")

// UserRepo is what the auth usecase needs from persistence.
type UserRepo interface {
	CreateUser(ctx context.Context, phone string, profile domain.Profile, status domain.Status) (*domain.User, error)
	FindByCredential(ctx context.Context, authType domain.AuthType, identifier string) (*domain.User, error)
	FindByID(ctx context.Context, userID int64) (*domain.User, error)
	GetProfile(ctx context.Context, userID int64) (*domain.Profile, error)
	TouchLastLogin(ctx context.Context, userID int64) error
	UpsertDevice(ctx context.Context, userID int64, fingerprint, name string) (known bool, err error)
}

// NextStep mirrors the contract enum (identity.v1.NextStep).
type NextStep int

const (
	StepTokensIssued NextStep = iota + 1
	StepSignupRequired
	StepReactivationRequired
	StepDeviceVerificationRequired
)

type TokenPair struct {
	AccessToken  string
	IDToken      string
	RefreshToken string
	ExpiresIn    int32
}

type VerifyResult struct {
	NextStep  NextStep
	Tokens    *TokenPair
	FlowToken string
}

type Auth struct {
	repo    UserRepo
	otp     *otp.Store
	rtr     *rtr.Store
	issuer  *token.Issuer
	log     zerolog.Logger
	devMode bool
}

func NewAuth(repo UserRepo, otpStore *otp.Store, rtrStore *rtr.Store, issuer *token.Issuer, log zerolog.Logger, devMode bool) *Auth {
	return &Auth{repo: repo, otp: otpStore, rtr: rtrStore, issuer: issuer, log: log, devMode: devMode}
}

// RequestOtp issues a code and "sends" it via the mock SMS (a log line).
// debugCode is non-empty only in dev mode.
func (a *Auth) RequestOtp(ctx context.Context, phone string) (expiresIn int32, debugCode string, err error) {
	code, err := a.otp.Request(ctx, phone)
	if err != nil {
		return 0, "", err
	}
	// Mock SMS provider (PRD §2): a real integration is out of PoC scope.
	a.log.Info().Str("phone", phone).Str("code", code).Msg("mock SMS: OTP issued")
	if a.devMode {
		debugCode = code
	}
	return int32(otp.CodeTTL.Seconds()), debugCode, nil
}

// VerifyOtp validates the code and routes by account state (PRD §4.2).
func (a *Auth) VerifyOtp(ctx context.Context, phone, code, fingerprint, deviceName string) (*VerifyResult, error) {
	if err := a.otp.Verify(ctx, phone, code); err != nil {
		return nil, err
	}

	user, err := a.repo.FindByCredential(ctx, domain.AuthTypePhone, phone)
	if errors.Is(err, domain.ErrUserNotFound) {
		flowToken, err := a.issuer.FlowToken(token.FlowSignup, phone)
		if err != nil {
			return nil, err
		}
		return &VerifyResult{NextStep: StepSignupRequired, FlowToken: flowToken}, nil
	}
	if err != nil {
		return nil, err
	}

	switch user.Status {
	case domain.StatusActive:
		// Device tracking only in Phase 2; the unknown-device verification
		// flow itself lands in Phase 6.
		if _, err := a.repo.UpsertDevice(ctx, user.ID, fingerprint, deviceName); err != nil {
			return nil, err
		}
		if err := a.repo.TouchLastLogin(ctx, user.ID); err != nil {
			return nil, err
		}
		tokens, err := a.issueTokens(ctx, user)
		if err != nil {
			return nil, err
		}
		return &VerifyResult{NextStep: StepTokensIssued, Tokens: tokens}, nil

	case domain.StatusDormant:
		flowToken, err := a.issuer.FlowToken(token.FlowReactivation, strconv.FormatInt(user.ID, 10))
		if err != nil {
			return nil, err
		}
		return &VerifyResult{NextStep: StepReactivationRequired, FlowToken: flowToken}, nil

	default: // PENDING, SUSPENDED, DELETED
		return nil, fmt.Errorf("%w: %s", domain.ErrLoginNotAllowed, user.Status)
	}
}

// CompleteSignup creates the user (PENDING→ACTIVE via the domain state machine,
// persisted with a transactional user.created outbox row) and issues tokens.
func (a *Auth) CompleteSignup(ctx context.Context, flowToken, nickname, profileImageURL string) (*TokenPair, error) {
	phone, err := a.issuer.VerifyFlowToken(flowToken, token.FlowSignup)
	if err != nil {
		return nil, err
	}

	newUser := &domain.User{Status: domain.StatusPending}
	if err := newUser.TransitionTo(domain.StatusActive); err != nil {
		return nil, err
	}
	user, err := a.repo.CreateUser(ctx, phone,
		domain.Profile{Nickname: nickname, ProfileImageURL: profileImageURL}, newUser.Status)
	if err != nil {
		return nil, err
	}
	return a.issueTokens(ctx, user)
}

// Refresh implements the token endpoint's refresh_token grant (RTR, PRD §4.3).
func (a *Auth) Refresh(ctx context.Context, grantType, refreshToken string) (*TokenPair, error) {
	if grantType != "refresh_token" {
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedGrant, grantType)
	}
	newRefresh, userID, err := a.rtr.Rotate(ctx, refreshToken)
	if err != nil {
		return nil, err
	}
	user, err := a.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	pair, err := a.buildJWTs(ctx, user)
	if err != nil {
		return nil, err
	}
	pair.RefreshToken = newRefresh
	return pair, nil
}

func (a *Auth) issueTokens(ctx context.Context, user *domain.User) (*TokenPair, error) {
	pair, err := a.buildJWTs(ctx, user)
	if err != nil {
		return nil, err
	}
	refresh, err := a.rtr.Issue(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	pair.RefreshToken = refresh
	return pair, nil
}

func (a *Auth) buildJWTs(ctx context.Context, user *domain.User) (*TokenPair, error) {
	profile, err := a.repo.GetProfile(ctx, user.ID)
	if err != nil {
		return nil, err
	}
	access, err := a.issuer.AccessToken(user.ID, string(user.Status))
	if err != nil {
		return nil, err
	}
	idToken, err := a.issuer.IDToken(user.ID, profile.Nickname)
	if err != nil {
		return nil, err
	}
	return &TokenPair{
		AccessToken: access,
		IDToken:     idToken,
		ExpiresIn:   int32(token.AccessTokenTTL.Seconds()),
	}, nil
}
