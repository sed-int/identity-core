// Package rtr implements Refresh Token Rotation with reuse detection (PRD §4.3).
//
// Redis layout:
//
//	rt:token:<token>   → JSON{user_id, family}         (TTL = refresh TTL)
//	rt:family:<family> → the family's CURRENT token    (TTL = refresh TTL)
//
// Presenting a token that exists but is no longer its family's current one
// means the token was stolen-and-rotated (or replayed) — the whole family is
// revoked and the user must log in again.
package rtr

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidToken  = errors.New("invalid refresh token")
	ErrReuseDetected = errors.New("refresh token reuse detected; family revoked")
)

// rotateScript makes the compare-and-swap and family revocation atomic. A
// WATCH/transaction loop would also work, but the script keeps the security
// decision in one Redis operation under a reuse flood.
var rotateScript = redis.NewScript(`
local current = redis.call("GET", KEYS[3])
if not current then
  return -1
end
if current ~= ARGV[1] then
  redis.call("DEL", KEYS[3])
  redis.call("DEL", ARGV[5] .. current)
  return 0
end
redis.call("SET", KEYS[2], ARGV[2], "PX", ARGV[4])
redis.call("SET", KEYS[3], ARGV[3], "PX", ARGV[4])
return 1
`)

const RefreshTokenTTL = 14 * 24 * time.Hour // PRD §4.3

type Store struct {
	rdb *redis.Client
	ttl time.Duration
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb, ttl: RefreshTokenTTL}
}

type record struct {
	UserID int64  `json:"user_id"`
	Family string `json:"family"`
}

func newOpaqueToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func tokenKey(t string) string  { return "rt:token:" + t }
func familyKey(f string) string { return "rt:family:" + f }

// Issue starts a new token family (login) and returns its first token.
func (s *Store) Issue(ctx context.Context, userID int64) (string, error) {
	tok := newOpaqueToken()
	fam := newOpaqueToken()
	if err := s.save(ctx, tok, fam, userID); err != nil {
		return "", err
	}
	return tok, nil
}

// Rotate exchanges a refresh token for a new one, enforcing reuse detection.
func (s *Store) Rotate(ctx context.Context, oldToken string) (newToken string, userID int64, err error) {
	raw, err := s.rdb.Get(ctx, tokenKey(oldToken)).Result()
	if errors.Is(err, redis.Nil) {
		return "", 0, ErrInvalidToken
	}
	if err != nil {
		return "", 0, err
	}
	var rec record
	if err := json.Unmarshal([]byte(raw), &rec); err != nil {
		return "", 0, err
	}

	newTok := newOpaqueToken()
	newRaw, err := json.Marshal(record{UserID: rec.UserID, Family: rec.Family})
	if err != nil {
		return "", 0, err
	}
	result, err := rotateScript.Run(ctx, s.rdb,
		[]string{tokenKey(oldToken), tokenKey(newTok), familyKey(rec.Family)},
		oldToken, string(newRaw), newTok, s.ttl.Milliseconds(), "rt:token:",
	).Int()
	if err != nil {
		return "", 0, err
	}
	switch result {
	case 1:
		// Old token stays resolvable (without being current) so reuse is
		// DETECTED rather than looking like an unknown token.
		return newTok, rec.UserID, nil
	case 0:
		return "", 0, ErrReuseDetected
	case -1:
		return "", 0, ErrInvalidToken
	default:
		return "", 0, fmt.Errorf("unexpected RTR script result: %d", result)
	}
}

// RevokeFamily deletes a family and any tokens passed alongside it (logout / reuse).
func (s *Store) RevokeFamily(ctx context.Context, family string, tokens ...string) error {
	keys := []string{familyKey(family)}
	for _, t := range tokens {
		if t != "" {
			keys = append(keys, tokenKey(t))
		}
	}
	return s.rdb.Del(ctx, keys...).Err()
}

func (s *Store) save(ctx context.Context, tok, fam string, userID int64) error {
	raw, err := json.Marshal(record{UserID: userID, Family: fam})
	if err != nil {
		return err
	}
	pipe := s.rdb.TxPipeline()
	pipe.Set(ctx, tokenKey(tok), raw, s.ttl)
	pipe.Set(ctx, familyKey(fam), tok, s.ttl)
	_, err = pipe.Exec(ctx)
	return err
}
