package jwks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math/big"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testIdP is a minimal stand-in for the identity service: it signs RS256
// tokens with a kid header and serves the matching JWKS. It deliberately does
// not import the identity service (pkg/ must stay service-independent).
type testIdP struct {
	key    *rsa.PrivateKey
	kid    string
	server *httptest.Server
	jwks   atomic.Value // []byte
	hits   atomic.Int64
}

func newTestIdP(t *testing.T) *testIdP {
	t.Helper()
	idp := &testIdP{}
	idp.rotate(t)
	idp.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		idp.hits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		w.Write(idp.jwks.Load().([]byte))
	}))
	t.Cleanup(idp.server.Close)
	return idp
}

// rotate installs a fresh signing key and serves ONLY the new key's JWKS
// (harsher than production, which would serve current+previous).
func (idp *testIdP) rotate(t *testing.T) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	der, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	sum := sha256.Sum256(der)
	idp.key = key
	idp.kid = hex.EncodeToString(sum[:8])

	set := map[string]any{"keys": []map[string]string{{
		"kty": "RSA",
		"use": "sig",
		"alg": "RS256",
		"kid": idp.kid,
		"n":   base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e":   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}}
	body, _ := json.Marshal(set)
	idp.jwks.Store(body)
}

func (idp *testIdP) signToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = idp.kid
	s, err := tok.SignedString(idp.key)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func (idp *testIdP) accessToken(t *testing.T, sub string) string {
	return idp.signToken(t, jwt.MapClaims{
		"iss":    "http://test-idp",
		"sub":    sub,
		"aud":    "board",
		"exp":    time.Now().Add(15 * time.Minute).Unix(),
		"iat":    time.Now().Unix(),
		"status": "ACTIVE",
	})
}

func newVerifier(idp *testIdP, opts ...Options) *Verifier {
	return New(idp.server.URL, "http://test-idp", "board", opts...)
}

func TestVerifyRoundtrip(t *testing.T) {
	idp := newTestIdP(t)
	v := newVerifier(idp)

	claims, err := v.Verify(context.Background(), idp.accessToken(t, "42"))
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if claims.Subject != "42" || claims.Status != "ACTIVE" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	stats := v.Stats()
	if stats.KeyCount != 1 || stats.FetchAttempts != 1 || stats.FetchSuccesses != 1 || stats.FetchFailures != 0 {
		t.Fatalf("unexpected stats: %+v", stats)
	}
	if stats.BreakerState != "closed" || stats.LastSuccessfulFetch.IsZero() {
		t.Fatalf("unexpected breaker/cache stats: %+v", stats)
	}
}

func TestRejectsGarbageWrongAudienceAndExpired(t *testing.T) {
	idp := newTestIdP(t)
	v := newVerifier(idp)
	ctx := context.Background()

	if _, err := v.Verify(ctx, "not-a-jwt"); err == nil {
		t.Fatal("garbage token accepted")
	}

	wrongAud := idp.signToken(t, jwt.MapClaims{
		"iss": "http://test-idp", "sub": "42", "aud": "poc-frontend",
		"exp": time.Now().Add(time.Hour).Unix(),
	})
	if _, err := v.Verify(ctx, wrongAud); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("wrong-audience token accepted: %v", err)
	}

	expired := idp.signToken(t, jwt.MapClaims{
		"iss": "http://test-idp", "sub": "42", "aud": "board",
		"exp": time.Now().Add(-time.Minute).Unix(),
	})
	if _, err := v.Verify(ctx, expired); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expired token accepted: %v", err)
	}
}

func TestKeyRotationTriggersRefetch(t *testing.T) {
	idp := newTestIdP(t)
	v := newVerifier(idp)

	if _, err := v.Verify(context.Background(), idp.accessToken(t, "1")); err != nil {
		t.Fatalf("pre-rotation verify: %v", err)
	}

	idp.rotate(t)
	// New kid is not cached → verifier must refetch and then succeed.
	if _, err := v.Verify(context.Background(), idp.accessToken(t, "2")); err != nil {
		t.Fatalf("post-rotation verify: %v", err)
	}
}

func TestServesStaleCacheWhenIdPIsDown(t *testing.T) {
	idp := newTestIdP(t)
	v := newVerifier(idp, Options{CacheTTL: time.Nanosecond}) // always stale

	tok := idp.accessToken(t, "7")
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("warm-up verify: %v", err)
	}

	idp.server.Close() // IdP goes down
	// Cache is stale and refresh fails — cached keys must still verify (§8.4).
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("stale-cache verify with IdP down: %v", err)
	}
}

func TestUnknownKidRefetchIsRateLimited(t *testing.T) {
	idp := newTestIdP(t)
	v := newVerifier(idp, Options{RefreshCooldown: time.Hour})

	if _, err := v.Verify(context.Background(), idp.accessToken(t, "1")); err != nil {
		t.Fatalf("warm-up: %v", err)
	}
	base := idp.hits.Load()

	// Rotate the SIGNING key but keep serving the old JWKS — every token now
	// has a kid the IdP never publishes (like a forged-kid flood). Hammering
	// must trigger at most ONE refetch per cooldown window.
	oldJWKS := idp.jwks.Load()
	idp.rotate(t)
	idp.jwks.Store(oldJWKS)

	for i := 0; i < 20; i++ {
		if _, err := v.Verify(context.Background(), idp.accessToken(t, "2")); !errors.Is(err, ErrUnknownKey) {
			t.Fatalf("expected ErrUnknownKey, got %v", err)
		}
	}
	if got := idp.hits.Load(); got > base+1 {
		t.Fatalf("cooldown violated: %d extra JWKS fetches, want ≤1", got-base)
	}
}
