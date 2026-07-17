package token

import (
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
)

// JWKSHandler serves GET /oauth2/v1/jwks. It serves every key it is given so
// rotation can expose current + previous keys (PRD §4.3).
func JWKSHandler(issuer string, keys ...*rsa.PrivateKey) http.HandlerFunc {
	type jwk struct {
		Kty string `json:"kty"`
		Use string `json:"use"`
		Alg string `json:"alg"`
		Kid string `json:"kid"`
		N   string `json:"n"`
		E   string `json:"e"`
	}
	set := struct {
		Keys []jwk `json:"keys"`
	}{}
	for _, k := range keys {
		set.Keys = append(set.Keys, jwk{
			Kty: "RSA",
			Use: "sig",
			Alg: "RS256",
			Kid: KeyID(k),
			N:   base64.RawURLEncoding.EncodeToString(k.PublicKey.N.Bytes()),
			E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(k.PublicKey.E)).Bytes()),
		})
	}
	body, _ := json.Marshal(set)

	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		w.Write(body)
	}
}

// DiscoveryHandler serves GET /.well-known/openid-configuration (PRD §4.1).
func DiscoveryHandler(issuer string) http.HandlerFunc {
	body, _ := json.Marshal(map[string]any{
		"issuer":                                issuer,
		"jwks_uri":                              issuer + "/oauth2/v1/jwks",
		"token_endpoint":                        issuer + "/oauth2/v1/token",
		"grant_types_supported":                 []string{"refresh_token"},
		"id_token_signing_alg_values_supported": []string{"RS256"},
		"subject_types_supported":               []string{"public"},
	})
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write(body)
	}
}
