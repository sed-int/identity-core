// Package token implements the IdP's signing stack (PRD §4.3):
// RS256 JWTs with kid-based rotation, JWKS + discovery endpoints,
// and short-lived flow tokens for multi-step auth flows.
package token

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
)

// LoadOrGenerateKey reads an RSA private key from path, generating one there
// on first run. PoC convenience only — the enterprise target is KMS (PRD §7);
// *.pem is gitignored.
func LoadOrGenerateKey(path string) (*rsa.PrivateKey, error) {
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("no PEM block in %s", path)
		}
		return x509.ParsePKCS1PrivateKey(block.Bytes)
	}

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	data := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return key, nil
}

// KeyID derives a stable kid from the public key (SHA-256 of DER, truncated).
func KeyID(key *rsa.PrivateKey) string {
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		panic(fmt.Sprintf("marshal public key: %v", err)) // cannot fail for a valid RSA key
	}
	sum := sha256.Sum256(der)
	return hex.EncodeToString(sum[:8])
}
