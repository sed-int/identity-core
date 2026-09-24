// Package jwks verifies IdP-issued RS256 JWTs statelessly (PRD §6):
// public keys are fetched from the IdP's JWKS endpoint through a circuit
// breaker, cached in-memory by kid, and served stale when the IdP is down —
// resource servers never query the identity DB.
package jwks

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/sony/gobreaker/v2"
)

var (
	ErrInvalidToken = errors.New("invalid access token")
	ErrUnknownKey   = errors.New("token signed by unknown key")
)

// Claims are the verified access-token claims resource servers care about.
type Claims struct {
	Subject string // user ID
	Status  string // account status claim (PRD §4.3)
}

type Options struct {
	// RefreshCooldown bounds how often an unknown kid may trigger a refetch.
	RefreshCooldown time.Duration
	// CacheTTL marks the cache stale, triggering a best-effort background
	// refresh on the next Verify. Stale keys are still USED if refresh fails.
	CacheTTL       time.Duration
	BreakerTimeout time.Duration
	HTTPClient     *http.Client
	OnStateChange  func(name, from, to string)
}

type Verifier struct {
	jwksURL  string
	issuer   string // expected iss claim (identifier, not the fetch URL)
	audience string
	opts     Options
	breaker  *gobreaker.CircuitBreaker[map[string]*rsa.PublicKey]

	mu              sync.RWMutex
	keys            map[string]*rsa.PublicKey
	fetchedAt       time.Time
	lastMissRefresh time.Time // last refetch triggered by an unknown kid
	fetchAttempts   atomic.Uint64
	fetchSuccesses  atomic.Uint64
	fetchFailures   atomic.Uint64
}

// Stats is a safe diagnostic snapshot used by the Phase 7 resilience harness.
// It exposes no keys or token material.
type Stats struct {
	KeyCount            int       `json:"key_count"`
	LastSuccessfulFetch time.Time `json:"last_successful_fetch"`
	FetchAttempts       uint64    `json:"fetch_attempts"`
	FetchSuccesses      uint64    `json:"fetch_successes"`
	FetchFailures       uint64    `json:"fetch_failures"`
	BreakerState        string    `json:"breaker_state"`
}

func New(jwksURL, issuer, audience string, opts ...Options) *Verifier {
	o := Options{RefreshCooldown: 10 * time.Second, CacheTTL: 5 * time.Minute, BreakerTimeout: 60 * time.Second}
	if len(opts) > 0 {
		if opts[0].RefreshCooldown > 0 {
			o.RefreshCooldown = opts[0].RefreshCooldown
		}
		if opts[0].CacheTTL > 0 {
			o.CacheTTL = opts[0].CacheTTL
		}
		if opts[0].BreakerTimeout > 0 {
			o.BreakerTimeout = opts[0].BreakerTimeout
		}
		o.HTTPClient = opts[0].HTTPClient
		o.OnStateChange = opts[0].OnStateChange
	}
	if o.HTTPClient == nil {
		o.HTTPClient = &http.Client{Timeout: 3 * time.Second}
	}
	return &Verifier{
		jwksURL:  jwksURL,
		issuer:   issuer,
		audience: audience,
		opts:     o,
		breaker:  gobreaker.NewCircuitBreaker[map[string]*rsa.PublicKey](breakerSettings(o)),
	}
}

func breakerSettings(o Options) gobreaker.Settings {
	settings := gobreaker.Settings{Name: "jwks-fetch", Timeout: o.BreakerTimeout}
	if o.OnStateChange != nil {
		settings.OnStateChange = func(name string, from, to gobreaker.State) {
			o.OnStateChange(name, from.String(), to.String())
		}
	}
	return settings
}

// Verify checks signature, expiry, issuer and audience, returning the claims.
func (v *Verifier) Verify(ctx context.Context, tokenStr string) (*Claims, error) {
	v.refreshIfStale(ctx)

	parsed, err := jwt.Parse(tokenStr,
		func(t *jwt.Token) (any, error) {
			kid, _ := t.Header["kid"].(string)
			if kid == "" {
				return nil, ErrUnknownKey
			}
			return v.keyFor(ctx, kid)
		},
		jwt.WithValidMethods([]string{"RS256"}),
		jwt.WithIssuer(v.issuer),
		jwt.WithAudience(v.audience),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsed.Valid {
		if errors.Is(err, ErrUnknownKey) {
			return nil, ErrUnknownKey
		}
		return nil, ErrInvalidToken
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok {
		return nil, ErrInvalidToken
	}
	sub, err := claims.GetSubject()
	if err != nil || sub == "" {
		return nil, ErrInvalidToken
	}
	status, _ := claims["status"].(string)
	return &Claims{Subject: sub, Status: status}, nil
}

func (v *Verifier) Stats() Stats {
	v.mu.RLock()
	keyCount, fetchedAt := len(v.keys), v.fetchedAt
	v.mu.RUnlock()
	return Stats{
		KeyCount:            keyCount,
		LastSuccessfulFetch: fetchedAt,
		FetchAttempts:       v.fetchAttempts.Load(),
		FetchSuccesses:      v.fetchSuccesses.Load(),
		FetchFailures:       v.fetchFailures.Load(),
		BreakerState:        v.breaker.State().String(),
	}
}

// keyFor returns the cached key for kid, refetching once (rate-limited) when
// the kid is unknown — that's how key rotation propagates (PRD §6).
func (v *Verifier) keyFor(ctx context.Context, kid string) (*rsa.PublicKey, error) {
	v.mu.RLock()
	key, ok := v.keys[kid]
	v.mu.RUnlock()
	if ok {
		return key, nil
	}

	// Unknown kid (rotation, or a forged token): refetch at most once per
	// cooldown window so a flood of bogus kids can't hammer the IdP. Routine
	// staleness refreshes don't count against this window — only misses do.
	v.mu.Lock()
	if time.Since(v.lastMissRefresh) < v.opts.RefreshCooldown {
		v.mu.Unlock()
		return nil, ErrUnknownKey
	}
	v.lastMissRefresh = time.Now()
	v.mu.Unlock()

	if err := v.refresh(ctx); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnknownKey, err)
	}

	v.mu.RLock()
	key, ok = v.keys[kid]
	v.mu.RUnlock()
	if !ok {
		return nil, ErrUnknownKey
	}
	return key, nil
}

func (v *Verifier) refreshIfStale(ctx context.Context) {
	v.mu.RLock()
	stale := time.Since(v.fetchedAt) > v.opts.CacheTTL
	v.mu.RUnlock()
	if stale {
		// Best-effort: failure keeps the stale cache (IdP may be down).
		_ = v.refresh(ctx)
	}
}

// refresh fetches the JWKS through the circuit breaker and swaps the cache.
func (v *Verifier) refresh(ctx context.Context) error {
	v.fetchAttempts.Add(1)
	keys, err := v.breaker.Execute(func() (map[string]*rsa.PublicKey, error) {
		return fetchJWKS(ctx, v.opts.HTTPClient, v.jwksURL)
	})
	if err != nil {
		v.fetchFailures.Add(1)
		return err
	}
	v.fetchSuccesses.Add(1)

	v.mu.Lock()
	v.keys = keys
	v.fetchedAt = time.Now()
	v.mu.Unlock()
	return nil
}

func fetchJWKS(ctx context.Context, client *http.Client, url string) (map[string]*rsa.PublicKey, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks fetch: status %d", resp.StatusCode)
	}

	var set struct {
		Keys []struct {
			Kty string `json:"kty"`
			Kid string `json:"kid"`
			N   string `json:"n"`
			E   string `json:"e"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&set); err != nil {
		return nil, err
	}

	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, k := range set.Keys {
		if k.Kty != "RSA" || k.Kid == "" {
			continue
		}
		n, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			continue
		}
		e, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			continue
		}
		keys[k.Kid] = &rsa.PublicKey{
			N: new(big.Int).SetBytes(n),
			E: int(new(big.Int).SetBytes(e).Int64()),
		}
	}
	if len(keys) == 0 {
		return nil, errors.New("jwks contains no usable RSA keys")
	}
	return keys, nil
}
