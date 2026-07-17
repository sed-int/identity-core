package token

import (
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"testing"

	"github.com/golang-jwt/jwt/v5"
)

func testIssuer(t *testing.T) *Issuer {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	return NewIssuer(key, "http://test-issuer")
}

func TestAccessTokenRoundtrip(t *testing.T) {
	iss := testIssuer(t)
	tok, err := iss.AccessToken(42, "ACTIVE")
	if err != nil {
		t.Fatal(err)
	}

	parsed, err := jwt.Parse(tok,
		func(tk *jwt.Token) (any, error) { return &iss.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience("board"),
		jwt.WithIssuer("http://test-issuer"),
	)
	if err != nil || !parsed.Valid {
		t.Fatalf("verify: %v", err)
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["sub"] != "42" || claims["status"] != "ACTIVE" {
		t.Fatalf("unexpected claims: %v", claims)
	}
	if parsed.Header["kid"] != iss.kid {
		t.Fatalf("kid missing from header: %v", parsed.Header)
	}
}

func TestFlowTokenPurposeIsEnforced(t *testing.T) {
	iss := testIssuer(t)
	tok, err := iss.FlowToken(FlowSignup, "+821012345678")
	if err != nil {
		t.Fatal(err)
	}

	sub, err := iss.VerifyFlowToken(tok, FlowSignup)
	if err != nil || sub != "+821012345678" {
		t.Fatalf("valid flow token rejected: sub=%q err=%v", sub, err)
	}

	if _, err := iss.VerifyFlowToken(tok, FlowReactivation); !errors.Is(err, ErrInvalidFlowToken) {
		t.Fatalf("wrong purpose must be rejected, got %v", err)
	}

	// A flow token must not pass as an access token elsewhere (aud=flow).
	if _, err := jwt.Parse(tok,
		func(tk *jwt.Token) (any, error) { return &iss.key.PublicKey, nil },
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithAudience("board"),
	); err == nil {
		t.Fatal("flow token accepted with aud=board")
	}
}

func TestVerifyFlowTokenRejectsOtherIssuersKey(t *testing.T) {
	a, b := testIssuer(t), testIssuer(t)
	tok, err := a.FlowToken(FlowSignup, "x")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.VerifyFlowToken(tok, FlowSignup); !errors.Is(err, ErrInvalidFlowToken) {
		t.Fatalf("token signed by another key must be rejected, got %v", err)
	}
}
