package rtr

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	mr := miniredis.RunT(t)
	return NewStore(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
}

func TestConcurrentReuseAllowsOneRotationThenRevokesFamily(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()
	tok, err := s.Issue(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}

	const callers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var successes atomic.Int32
	var reuses atomic.Int32
	var winnerMu sync.Mutex
	var winner string
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			rotated, _, err := s.Rotate(ctx, tok)
			switch {
			case err == nil:
				successes.Add(1)
				winnerMu.Lock()
				winner = rotated
				winnerMu.Unlock()
			case errors.Is(err, ErrReuseDetected), errors.Is(err, ErrInvalidToken):
				reuses.Add(1)
			default:
				t.Errorf("unexpected rotate error: %v", err)
			}
		}()
	}
	close(start)
	wg.Wait()

	if got := successes.Load(); got != 1 {
		t.Fatalf("successful concurrent rotations = %d, want 1", got)
	}
	if got := reuses.Load(); got != callers-1 {
		t.Fatalf("rejected concurrent rotations = %d, want %d", got, callers-1)
	}
	if _, _, err := s.Rotate(ctx, winner); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("winning token survived family revocation: %v", err)
	}
}

func TestIssueAndRotate(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	tok1, err := s.Issue(ctx, 7)
	if err != nil {
		t.Fatal(err)
	}

	tok2, userID, err := s.Rotate(ctx, tok1)
	if err != nil {
		t.Fatal(err)
	}
	if userID != 7 {
		t.Fatalf("userID = %d, want 7", userID)
	}
	if tok2 == tok1 {
		t.Fatal("rotation must return a new token")
	}

	// The new token keeps working.
	if _, _, err := s.Rotate(ctx, tok2); err != nil {
		t.Fatalf("rotating current token: %v", err)
	}
}

func TestReuseDetectionRevokesFamily(t *testing.T) {
	s := testStore(t)
	ctx := context.Background()

	tok1, _ := s.Issue(ctx, 7)
	tok2, _, err := s.Rotate(ctx, tok1)
	if err != nil {
		t.Fatal(err)
	}

	// Replay the OLD token: reuse must be detected...
	if _, _, err := s.Rotate(ctx, tok1); !errors.Is(err, ErrReuseDetected) {
		t.Fatalf("expected ErrReuseDetected, got %v", err)
	}
	// ...and the whole family revoked, killing the current token too.
	if _, _, err := s.Rotate(ctx, tok2); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("current token must be dead after reuse, got %v", err)
	}
}

func TestUnknownTokenRejected(t *testing.T) {
	s := testStore(t)
	if _, _, err := s.Rotate(context.Background(), "no-such-token"); !errors.Is(err, ErrInvalidToken) {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}
