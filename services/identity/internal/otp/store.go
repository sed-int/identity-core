// Package otp implements the phone OTP flow with Redis-backed state (PRD §7):
// 3-minute code TTL, 5 verify attempts per code, per-phone request rate limit.
// OTP state never touches MySQL (PRD §5).
package otp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrInvalidCode     = errors.New("invalid or expired OTP code")
	ErrTooManyAttempts = errors.New("too many failed OTP attempts")
	ErrRateLimited     = errors.New("too many OTP requests for this number")
)

const (
	CodeTTL         = 3 * time.Minute
	MaxAttempts     = 5
	RateLimitWindow = time.Hour
	RateLimitMax    = 5
)

type Store struct {
	rdb *redis.Client
}

func NewStore(rdb *redis.Client) *Store {
	return &Store{rdb: rdb}
}

func codeKey(phone string) string { return "otp:code:" + phone }
func rlKey(phone string) string   { return "otp:rl:" + phone }

// Request issues a fresh 6-digit code for the phone number.
// The caller is responsible for "sending" it (mock SMS logs it in the PoC).
func (s *Store) Request(ctx context.Context, phone string) (code string, err error) {
	count, err := s.rdb.Incr(ctx, rlKey(phone)).Result()
	if err != nil {
		return "", err
	}
	if count == 1 {
		s.rdb.Expire(ctx, rlKey(phone), RateLimitWindow)
	}
	if count > RateLimitMax {
		return "", ErrRateLimited
	}

	n, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	code = fmt.Sprintf("%06d", n.Int64())

	pipe := s.rdb.TxPipeline()
	pipe.HSet(ctx, codeKey(phone), "code", code, "attempts", 0)
	pipe.Expire(ctx, codeKey(phone), CodeTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return "", err
	}
	return code, nil
}

// Verify checks the code; the stored code is consumed on success and after
// too many failures.
func (s *Store) Verify(ctx context.Context, phone, code string) error {
	vals, err := s.rdb.HGetAll(ctx, codeKey(phone)).Result()
	if err != nil {
		return err
	}
	if len(vals) == 0 {
		return ErrInvalidCode // never requested or expired
	}

	if vals["code"] != code {
		attempts, err := s.rdb.HIncrBy(ctx, codeKey(phone), "attempts", 1).Result()
		if err != nil {
			return err
		}
		if attempts >= MaxAttempts {
			s.rdb.Del(ctx, codeKey(phone))
			return ErrTooManyAttempts
		}
		return ErrInvalidCode
	}

	return s.rdb.Del(ctx, codeKey(phone)).Err()
}
