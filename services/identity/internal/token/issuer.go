package token

import (
	"crypto/rsa"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

var ErrInvalidFlowToken = errors.New("invalid flow token")

// Flow token purposes (which multi-step flow a token may continue).
const (
	FlowSignup       = "signup"
	FlowReactivation = "reactivation"
	FlowDeviceVerify = "device_verify"
)

const (
	AccessTokenTTL = 15 * time.Minute // PRD §4.3
	FlowTokenTTL   = 10 * time.Minute
)

type Issuer struct {
	key    *rsa.PrivateKey
	kid    string
	issuer string // iss claim, e.g. http://localhost:8090
}

func NewIssuer(key *rsa.PrivateKey, issuer string) *Issuer {
	return &Issuer{key: key, kid: KeyID(key), issuer: issuer}
}

func (i *Issuer) sign(claims jwt.MapClaims) (string, error) {
	t := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	t.Header["kid"] = i.kid
	return t.SignedString(i.key)
}

// AccessToken carries the claims resource servers verify statelessly (PRD §4.3).
func (i *Issuer) AccessToken(userID int64, status string) (string, error) {
	now := time.Now()
	return i.sign(jwt.MapClaims{
		"iss":    i.issuer,
		"sub":    fmt.Sprintf("%d", userID),
		"aud":    "board",
		"exp":    now.Add(AccessTokenTTL).Unix(),
		"iat":    now.Unix(),
		"jti":    uuid.NewString(),
		"status": status,
	})
}

func (i *Issuer) IDToken(userID int64, nickname string) (string, error) {
	now := time.Now()
	return i.sign(jwt.MapClaims{
		"iss":       i.issuer,
		"sub":       fmt.Sprintf("%d", userID),
		"aud":       "poc-frontend",
		"exp":       now.Add(AccessTokenTTL).Unix(),
		"iat":       now.Unix(),
		"auth_time": now.Unix(),
		"nickname":  nickname,
	})
}

// FlowToken lets a client continue a multi-step flow (signup, reactivation…).
// subject is the phone number for signup (no user exists yet) or the user ID.
func (i *Issuer) FlowToken(purpose, subject string) (string, error) {
	now := time.Now()
	return i.sign(jwt.MapClaims{
		"iss":     i.issuer,
		"sub":     subject,
		"aud":     "flow",
		"exp":     now.Add(FlowTokenTTL).Unix(),
		"iat":     now.Unix(),
		"purpose": purpose,
	})
}

// VerifyFlowToken checks signature/expiry/audience and the expected purpose,
// returning the subject.
func (i *Issuer) VerifyFlowToken(tokenStr, wantPurpose string) (subject string, err error) {
	parsed, err := jwt.Parse(tokenStr,
		func(t *jwt.Token) (any, error) { return &i.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience("flow"),
		jwt.WithIssuer(i.issuer),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		return "", ErrInvalidFlowToken
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || claims["purpose"] != wantPurpose {
		return "", ErrInvalidFlowToken
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return "", ErrInvalidFlowToken
	}
	return sub, nil
}
