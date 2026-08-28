package devverify

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func newTestLimiter(t *testing.T) *Limiter {
	t.Helper()
	mr := miniredis.RunT(t)
	return NewLimiter(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
}

func TestAttemptCapAndReset(t *testing.T) {
	ctx := context.Background()
	l := newTestLimiter(t)

	for i := range MaxAttempts {
		if err := l.Attempt(ctx, 42); err != nil {
			t.Fatalf("attempt %d: unexpected error %v", i+1, err)
		}
	}
	if err := l.Attempt(ctx, 42); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("attempt %d: want ErrTooManyAttempts, got %v", MaxAttempts+1, err)
	}

	// Other users are unaffected.
	if err := l.Attempt(ctx, 7); err != nil {
		t.Fatalf("other user: unexpected error %v", err)
	}

	// Reset clears the counter (called after successful verification).
	if err := l.Reset(ctx, 42); err != nil {
		t.Fatalf("reset: %v", err)
	}
	if err := l.Attempt(ctx, 42); err != nil {
		t.Fatalf("post-reset attempt: unexpected error %v", err)
	}
}
