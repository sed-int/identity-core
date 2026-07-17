package otp

import (
	"context"
	"errors"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T) (*Store, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	return NewStore(redis.NewClient(&redis.Options{Addr: mr.Addr()})), mr
}

func TestRequestAndVerify(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()

	code, err := s.Request(ctx, "+821011112222")
	if err != nil {
		t.Fatal(err)
	}
	if len(code) != 6 {
		t.Fatalf("code %q is not 6 digits", code)
	}
	if err := s.Verify(ctx, "+821011112222", code); err != nil {
		t.Fatalf("valid code rejected: %v", err)
	}
	// Codes are single-use.
	if err := s.Verify(ctx, "+821011112222", code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("consumed code must be rejected, got %v", err)
	}
}

func TestAttemptLimit(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	phone := "+821011112222"

	code, _ := s.Request(ctx, phone)
	for i := 0; i < MaxAttempts-1; i++ {
		if err := s.Verify(ctx, phone, "000000"); !errors.Is(err, ErrInvalidCode) {
			t.Fatalf("attempt %d: got %v", i, err)
		}
	}
	if err := s.Verify(ctx, phone, "000000"); !errors.Is(err, ErrTooManyAttempts) {
		t.Fatalf("expected ErrTooManyAttempts, got %v", err)
	}
	// Even the right code is dead now.
	if err := s.Verify(ctx, phone, code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("code must be consumed after attempt cap, got %v", err)
	}
}

func TestRateLimit(t *testing.T) {
	s, _ := testStore(t)
	ctx := context.Background()
	phone := "+821011112222"

	for i := 0; i < RateLimitMax; i++ {
		if _, err := s.Request(ctx, phone); err != nil {
			t.Fatalf("request %d: %v", i, err)
		}
	}
	if _, err := s.Request(ctx, phone); !errors.Is(err, ErrRateLimited) {
		t.Fatalf("expected ErrRateLimited, got %v", err)
	}
}

func TestCodeExpires(t *testing.T) {
	s, mr := testStore(t)
	ctx := context.Background()
	phone := "+821011112222"

	code, _ := s.Request(ctx, phone)
	mr.FastForward(CodeTTL + 1)
	if err := s.Verify(ctx, phone, code); !errors.Is(err, ErrInvalidCode) {
		t.Fatalf("expired code must be rejected, got %v", err)
	}
}
