// Package devverify rate-limits the unknown-device verification challenge
// (PRD §4.2 item 4). The challenge answer space is tiny (account creation
// month), so a hard attempt cap is required; state lives in Redis with a TTL,
// same as the OTP counters (PRD §5).
package devverify

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ErrTooManyAttempts = errors.New("too many device verification attempts")

const (
	MaxAttempts = 3
	// Window matches token.FlowTokenTTL: once the flow token is dead the
	// counter no longer matters.
	Window = 10 * time.Minute
)

type Limiter struct {
	rdb *redis.Client
}

func NewLimiter(rdb *redis.Client) *Limiter { return &Limiter{rdb: rdb} }

func key(userID int64) string { return fmt.Sprintf("devverify:attempts:%d", userID) }

// Attempt counts one verification attempt, failing once the cap is exceeded.
func (l *Limiter) Attempt(ctx context.Context, userID int64) error {
	count, err := l.rdb.Incr(ctx, key(userID)).Result()
	if err != nil {
		return err
	}
	if count == 1 {
		l.rdb.Expire(ctx, key(userID), Window)
	}
	if count > MaxAttempts {
		return ErrTooManyAttempts
	}
	return nil
}

// Reset clears the counter after a successful verification.
func (l *Limiter) Reset(ctx context.Context, userID int64) error {
	return l.rdb.Del(ctx, key(userID)).Err()
}
