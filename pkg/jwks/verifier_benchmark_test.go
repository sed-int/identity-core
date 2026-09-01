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
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func BenchmarkVerifyWarmCache(b *testing.B) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		b.Fatal(err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		b.Fatal(err)
	}
	sum := sha256.Sum256(der)
	kid := hex.EncodeToString(sum[:8])
	body, err := json.Marshal(map[string]any{"keys": []map[string]string{{
		"kty": "RSA", "kid": kid,
		"n": base64.RawURLEncoding.EncodeToString(key.PublicKey.N.Bytes()),
		"e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.PublicKey.E)).Bytes()),
	}}})
	if err != nil {
		b.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write(body) }))
	defer server.Close()

	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iss": "http://bench-idp", "sub": "42", "aud": "board",
		"exp": time.Now().Add(time.Hour).Unix(), "status": "ACTIVE",
	})
	token.Header["kid"] = kid
	signed, err := token.SignedString(key)
	if err != nil {
		b.Fatal(err)
	}
	verifier := New(server.URL, "http://bench-idp", "board")
	if _, err := verifier.Verify(context.Background(), signed); err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			if _, err := verifier.Verify(context.Background(), signed); err != nil {
				b.Fatal(err)
			}
		}
	})
}
