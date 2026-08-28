# Phase 6 — Edge States & Security Hardening Implementation Plan

> Status: ✅ completed 2026-08-28 — all tasks executed, `make test` + m1/m2/m4/m6 smokes green
>
> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement the two remaining PRD §4.2 edge flows — DORMANT reactivation (re-consent → ACTIVE) and unknown-device verification (identity-owned knowledge check) — end to end: proto contract, usecase, delivery, smoke test, and minimal web demo screens.

**Architecture:** Both flows reuse the flow-token continuation pattern already shipped in Phase 2 (`token.FlowReactivation` / `token.FlowDeviceVerify` purposes exist; `NextStep` enum values 3/4 exist in the proto). Two new RPCs complete them: `CompleteReactivation` (consent → DORMANT→ACTIVE via the domain state machine) and `VerifyDevice` (account-creation-month check, Redis attempt cap, then device registration). The ACTIVE login branch gains a device gate: unknown fingerprint → no tokens, `DEVICE_VERIFICATION_REQUIRED` + flow token. Signup starts registering the signup device so a fresh user's next login is not flagged.

**Tech Stack:** Go, gRPC + grpc-gateway (buf), MySQL (`user_devices`), Redis (attempt counter, miniredis in tests), React/Vite demo client.

**Spec:** `prd.md` §4.2 (items 3–4), §5 (`user_devices`), §7. Roadmap: `plan/2026-07-17-build-order.md` Phase 6. OTP hardening (§7) already landed in Phase 2 — out of scope here.

## Global Constraints

- Branch: all work on `feat/edge-flows` off `dev`; Conventional Commits; leave branch unmerged for hcho's review (CLAUDE.md review gate).
- Layering: `domain` stays pure; `usecase` imports no infrastructure clients directly (Redis lives behind small stores, pattern: `internal/otp`, `internal/rtr`).
- Proto is SSoT: contract changes go through `api/proto/identity/v1/identity.proto` + `make proto`; generated code under `api/gen/` is committed.
- Existing field numbers/enum values in the proto must not change; only append.
- All tests run with `make test` (unit only, miniredis for Redis, no Docker needed).

## Design decisions (locked)

1. **Device-verification challenge = account creation month** (`"YYYY-MM"`, UTC), per PRD §4.2's identity-owned-data example. The user already passed phone OTP to reach this step, so a second OTP to the same number adds nothing; the month check is a distinct knowledge factor. Capped at **3 attempts per user per 10 min** (Redis counter) — only ~12–36 plausible values, so an attempt cap is mandatory.
2. **Check-then-register, never register-then-check.** `VerifyOtp` must NOT upsert the device before verification (today's `UpsertDevice`-first code would let an attacker "know" a device just by logging in twice). New repo method `IsDeviceKnown` does a read-only check; `UpsertDevice` is called only for already-known devices (refresh `last_seen_at`), at signup, after reactivation, and after successful device verification. `UpsertDevice` loses its now-unused `known` return.
3. **Empty fingerprint skips the device gate** (curl / non-browser clients keep working; smoke scripts and the web client always send one). Deliberate PoC ceiling, marked with a `ponytail:` comment.
4. **Signup and reactivation register the presented device** — both flows just proved control of the registered phone via OTP.
5. **Reactivation requires `privacy_consent=true`** (PRD: re-consent to privacy policy). Consent is not persisted (no consent-ledger table in §5 schema — YAGNI for the PoC). Replay of a used reactivation flow token fails naturally: ACTIVE→ACTIVE is not a legal transition.
6. **DORMANT is induced via SQL in the smoke test** (`UPDATE users SET status='DORMANT'`). No admin endpoint — nothing in the PRD asks for one.

## File map

| File | Change |
| :--- | :--- |
| `api/proto/identity/v1/identity.proto` | +2 RPCs, +4 messages, +2 fields on `CompleteSignupRequest` |
| `api/gen/**` | regenerated (`make proto`), committed |
| `services/identity/internal/devverify/limiter.go` (new) | Redis attempt cap for device verification |
| `services/identity/internal/devverify/limiter_test.go` (new) | miniredis test |
| `services/identity/internal/repo/mysql/user_repo.go` | +`IsDeviceKnown`; `UpsertDevice` drops `known` return |
| `services/identity/internal/usecase/auth.go` | device gate in `VerifyOtp`; +`CompleteReactivation`, +`VerifyDevice`; `CompleteSignup` registers device; interface grows `IsDeviceKnown`+`UpdateStatus` |
| `services/identity/internal/usecase/auth_test.go` (new) | fake repo + miniredis + throwaway RSA issuer |
| `services/identity/internal/delivery/grpc/handler.go` | +2 handlers, error mapping |
| `services/identity/internal/app/app.go` | wire `devverify.NewLimiter(rdb)` |
| `scripts/m6_smoke.sh` (new) | E2E: dormant→reactivate, unknown device→verify |
| `web/src/api.ts`, `web/src/screens/{Reactivate,DeviceVerify}.tsx` (new), `web/src/screens/Login.tsx`, `web/src/App.tsx` | demo UI for both flows |
| `README.md`, `plan/2026-07-17-build-order.md` | status updates (final task) |

---

### Task 1: Proto contract

**Files:**
- Modify: `api/proto/identity/v1/identity.proto`
- Generated: `api/gen/**` via `make proto`

**Interfaces:**
- Produces (Go, used by Task 5): `identityv1.CompleteReactivationRequest{FlowToken, PrivacyConsent, DeviceFingerprint, DeviceName}`, `identityv1.CompleteReactivationResponse{Tokens}`, `identityv1.VerifyDeviceRequest{FlowToken, RegistrationMonth, DeviceFingerprint, DeviceName}`, `identityv1.VerifyDeviceResponse{Tokens}`, `CompleteSignupRequest.GetDeviceFingerprint()/GetDeviceName()`.
- REST paths (grpc-gateway): `POST /auth/v1/reactivate`, `POST /auth/v1/device/verify`.

- [ ] **Step 1: Add RPCs + messages**

In the `service IdentityService` block, after `CompleteSignup`:

```proto
  // Completes DORMANT-account reactivation (PRD §4.2 item 3): the flow_token
  // from VerifyOtp plus explicit privacy re-consent roll the account back to
  // ACTIVE and issue tokens.
  rpc CompleteReactivation(CompleteReactivationRequest) returns (CompleteReactivationResponse) {
    option (google.api.http) = {
      post: "/auth/v1/reactivate"
      body: "*"
    };
  }

  // Completes unknown-device verification (PRD §4.2 item 4): the user proves
  // an Identity-owned fact (account creation month) before the new device is
  // registered and tokens are issued. 3 attempts per flow.
  rpc VerifyDevice(VerifyDeviceRequest) returns (VerifyDeviceResponse) {
    option (google.api.http) = {
      post: "/auth/v1/device/verify"
      body: "*"
    };
  }
```

Append to `CompleteSignupRequest` (numbers 4 and 5 — do not renumber existing fields):

```proto
  // The signup device is registered so the user's next login from it is not
  // flagged as a device change.
  string device_fingerprint = 4;
  string device_name = 5;
```

New messages at the end of the file:

```proto
message CompleteReactivationRequest {
  string flow_token = 1;      // purpose=reactivation, from VerifyOtp
  bool privacy_consent = 2;   // must be true (PRD §4.2: re-consent required)
  string device_fingerprint = 3;
  string device_name = 4;
}

message CompleteReactivationResponse {
  TokenPair tokens = 1;
}

message VerifyDeviceRequest {
  string flow_token = 1;         // purpose=device_verify, from VerifyOtp
  string registration_month = 2; // account creation month, "YYYY-MM" (UTC)
  string device_fingerprint = 3;
  string device_name = 4;
}

message VerifyDeviceResponse {
  TokenPair tokens = 1;
}
```

Also update the `NextStep` enum comments: drop the `(Phase 6)` markers on values 3/4 since the flows now exist.

- [ ] **Step 2: Regenerate + compile**

Run: `make proto && go build ./...`
Expected: buf lint passes, `api/gen/identity/v1/*` regenerated, build green (new RPCs are added to the service; the handler satisfies them via the embedded `UnimplementedIdentityServiceServer` until Task 5).

- [ ] **Step 3: Commit**

```bash
git add api/
git commit -m "feat(proto): add CompleteReactivation + VerifyDevice RPCs, signup device fields"
```

---

### Task 2: Device-verification attempt limiter

**Files:**
- Create: `services/identity/internal/devverify/limiter.go`
- Test: `services/identity/internal/devverify/limiter_test.go`

**Interfaces:**
- Produces: `devverify.NewLimiter(rdb *redis.Client) *Limiter`; `(*Limiter).Attempt(ctx, userID int64) error` (nil while ≤3 attempts in the window, `devverify.ErrTooManyAttempts` beyond); `(*Limiter).Reset(ctx, userID int64) error`; `devverify.MaxAttempts = 3`.

- [ ] **Step 1: Write the failing test**

`limiter_test.go` (mirror the miniredis setup style of `internal/otp/store_test.go`):

```go
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

	for i := 0; i < MaxAttempts; i++ {
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./services/identity/internal/devverify/`
Expected: FAIL (package does not compile — `Limiter` undefined).

- [ ] **Step 3: Write the implementation**

`limiter.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./services/identity/internal/devverify/`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add services/identity/internal/devverify/
git commit -m "feat(identity): add device-verification attempt limiter (redis, 3/10min)"
```

---

### Task 3: Repo — read-only device check

**Files:**
- Modify: `services/identity/internal/repo/mysql/user_repo.go` (UpsertDevice at :153)

**Interfaces:**
- Produces: `(*UserRepo).IsDeviceKnown(ctx, userID int64, fingerprint string) (bool, error)`; `(*UserRepo).UpsertDevice(ctx, userID int64, fingerprint, name string) error` (signature change: `known bool` return removed).
- Consumed by Task 4 via the usecase `UserRepo` interface.

No unit test here — repo layer has no MySQL tests yet (testcontainers is Phase 7 per the build-order plan); coverage comes from the fake-repo usecase tests (Task 4) and `scripts/m6_smoke.sh` (Task 6).

- [ ] **Step 1: Add `IsDeviceKnown`, simplify `UpsertDevice`**

Replace the existing `UpsertDevice` (which returns `known bool` — after Task 4 nothing consumes it, and computing it pre-write is exactly the register-before-check trap this phase removes):

```go
// IsDeviceKnown reports whether the fingerprint is already registered for the
// user. Read-only: the unknown-device flow (PRD §4.2) must not register a
// device before it has been verified.
func (r *UserRepo) IsDeviceKnown(ctx context.Context, userID int64, fingerprint string) (bool, error) {
	var one int
	err := r.db.QueryRowContext(ctx,
		`SELECT 1 FROM user_devices WHERE user_id = ? AND device_fingerprint = ?`,
		userID, fingerprint).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, nil
}

// UpsertDevice registers the device or refreshes last_seen_at/device_name.
func (r *UserRepo) UpsertDevice(ctx context.Context, userID int64, fingerprint, name string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO user_devices (user_id, device_fingerprint, device_name, last_seen_at)
		VALUES (?, ?, ?, NOW(6))
		ON DUPLICATE KEY UPDATE last_seen_at = NOW(6), device_name = VALUES(device_name)`,
		userID, fingerprint, name)
	return err
}
```

Keep the body of the existing upsert SQL exactly as it is today — only the signature and any `known` computation go.

- [ ] **Step 2: Build (expected red)**

Run: `go build ./...`
Expected: FAIL — `usecase/auth.go` still uses the two-value `UpsertDevice`. That's the Task 4 seam; fix it there. (If executing tasks in order in one session, it is fine to commit Task 3 together with Task 4's first green build; otherwise stage this task and continue.)

---

### Task 4: Usecase — device gate, reactivation, device verification

**Files:**
- Modify: `services/identity/internal/usecase/auth.go`
- Test: `services/identity/internal/usecase/auth_test.go` (new)

**Interfaces:**
- Consumes: `devverify.Limiter` (Task 2), repo methods (Task 3), existing `token.FlowReactivation` / `token.FlowDeviceVerify` purposes, `domain.User.TransitionTo`.
- Produces (used by Task 5):
  - `NewAuth(repo UserRepo, otpStore *otp.Store, rtrStore *rtr.Store, limiter *devverify.Limiter, issuer *token.Issuer, log zerolog.Logger, devMode bool) *Auth` (limiter param added)
  - `(*Auth).CompleteSignup(ctx, flowToken, nickname, profileImageURL, fingerprint, deviceName string) (*TokenPair, error)` (two params added)
  - `(*Auth).CompleteReactivation(ctx, flowToken string, privacyConsent bool, fingerprint, deviceName string) (*TokenPair, error)`
  - `(*Auth).VerifyDevice(ctx, flowToken, registrationMonth, fingerprint, deviceName string) (*TokenPair, error)`
  - Errors: `usecase.ErrConsentRequired`, `usecase.ErrDeviceVerifyFailed`
  - `UserRepo` interface: `UpsertDevice(...) error` (no bool), plus `IsDeviceKnown(ctx, userID int64, fingerprint string) (bool, error)` and `UpdateStatus(ctx, userID int64, status domain.Status) error`

- [ ] **Step 1: Write the failing tests**

`auth_test.go`. Fakes + real OTP/RTR stores on miniredis + throwaway RSA issuer — the flow-token round trip is the core of both flows, so don't fake the issuer.

```go
package usecase

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"

	"identity-service/services/identity/internal/devverify"
	"identity-service/services/identity/internal/domain"
	"identity-service/services/identity/internal/otp"
	"identity-service/services/identity/internal/rtr"
	"identity-service/services/identity/internal/token"
)

// fakeRepo is an in-memory UserRepo.
type fakeRepo struct {
	users   map[int64]*domain.User
	phones  map[string]int64 // phone → user id
	devices map[string]bool  // "userID:fingerprint" → known
	nextID  int64
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{users: map[int64]*domain.User{}, phones: map[string]int64{}, devices: map[string]bool{}, nextID: 1}
}

func devKey(userID int64, fp string) string { return strconv.FormatInt(userID, 10) + ":" + fp }

func (f *fakeRepo) CreateUser(_ context.Context, phone string, _ domain.Profile, status domain.Status) (*domain.User, error) {
	id := f.nextID
	f.nextID++
	u := &domain.User{ID: id, Status: status, CreatedAt: time.Now(), UpdatedAt: time.Now()}
	f.users[id] = u
	f.phones[phone] = id
	return u, nil
}

func (f *fakeRepo) FindByCredential(_ context.Context, _ domain.AuthType, phone string) (*domain.User, error) {
	id, ok := f.phones[phone]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return f.users[id], nil
}

func (f *fakeRepo) FindByID(_ context.Context, userID int64) (*domain.User, error) {
	u, ok := f.users[userID]
	if !ok {
		return nil, domain.ErrUserNotFound
	}
	return u, nil
}

func (f *fakeRepo) GetProfile(_ context.Context, userID int64) (*domain.Profile, error) {
	return &domain.Profile{UserID: userID, Nickname: "nick"}, nil
}

func (f *fakeRepo) TouchLastLogin(context.Context, int64) error { return nil }

func (f *fakeRepo) UpdateStatus(_ context.Context, userID int64, status domain.Status) error {
	f.users[userID].Status = status
	return nil
}

func (f *fakeRepo) UpsertDevice(_ context.Context, userID int64, fp, _ string) error {
	f.devices[devKey(userID, fp)] = true
	return nil
}

func (f *fakeRepo) IsDeviceKnown(_ context.Context, userID int64, fp string) (bool, error) {
	return f.devices[devKey(userID, fp)], nil
}

func newTestAuth(t *testing.T) (*Auth, *fakeRepo) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	repo := newFakeRepo()
	auth := NewAuth(repo, otp.NewStore(rdb), rtr.NewStore(rdb),
		devverify.NewLimiter(rdb), token.NewIssuer(key, "http://test"), zerolog.Nop(), true)
	return auth, repo
}

// login drives phone OTP end to end and returns the VerifyOtp result.
func login(t *testing.T, a *Auth, phone, fp, name string) *VerifyResult {
	t.Helper()
	_, code, err := a.RequestOtp(context.Background(), phone)
	if err != nil {
		t.Fatal(err)
	}
	res, err := a.VerifyOtp(context.Background(), phone, code, fp, name)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

// signup creates an ACTIVE user with a registered device and returns its id.
func signup(t *testing.T, a *Auth, repo *fakeRepo, phone, fp string) int64 {
	t.Helper()
	res := login(t, a, phone, fp, "test-device")
	if res.NextStep != StepSignupRequired {
		t.Fatalf("want signup step, got %v", res.NextStep)
	}
	if _, err := a.CompleteSignup(context.Background(), res.FlowToken, "nick", "", fp, "test-device"); err != nil {
		t.Fatal(err)
	}
	return repo.phones[phone]
}

func TestSignupRegistersDevice(t *testing.T) {
	a, repo := newTestAuth(t)
	id := signup(t, a, repo, "+82100", "dev-a")
	if !repo.devices[devKey(id, "dev-a")] {
		t.Fatal("signup did not register the device")
	}
	// Next login from the same device: straight to tokens.
	res := login(t, a, "+82100", "dev-a", "test-device")
	if res.NextStep != StepTokensIssued || res.Tokens == nil {
		t.Fatalf("known device: want tokens, got step %v", res.NextStep)
	}
}

func TestUnknownDeviceRequiresVerification(t *testing.T) {
	a, repo := newTestAuth(t)
	id := signup(t, a, repo, "+82100", "dev-a")

	res := login(t, a, "+82100", "dev-b", "other-device")
	if res.NextStep != StepDeviceVerificationRequired {
		t.Fatalf("want device verification step, got %v", res.NextStep)
	}
	if res.Tokens != nil {
		t.Fatal("unknown device must not receive tokens")
	}
	if repo.devices[devKey(id, "dev-b")] {
		t.Fatal("unknown device must not be registered before verification")
	}

	// Wrong month: rejected, still unregistered.
	if _, err := a.VerifyDevice(context.Background(), res.FlowToken, "1999-01", "dev-b", "other-device"); !errors.Is(err, ErrDeviceVerifyFailed) {
		t.Fatalf("want ErrDeviceVerifyFailed, got %v", err)
	}

	// Right month: tokens issued, device registered.
	month := repo.users[id].CreatedAt.UTC().Format("2006-01")
	pair, err := a.VerifyDevice(context.Background(), res.FlowToken, month, "dev-b", "other-device")
	if err != nil || pair == nil || pair.AccessToken == "" {
		t.Fatalf("verify device: %v", err)
	}
	if !repo.devices[devKey(id, "dev-b")] {
		t.Fatal("verified device was not registered")
	}
}

func TestDeviceVerificationAttemptCap(t *testing.T) {
	a, repo := newTestAuth(t)
	id := signup(t, a, repo, "+82100", "dev-a")
	res := login(t, a, "+82100", "dev-b", "other")

	for i := 0; i < devverify.MaxAttempts; i++ {
		if _, err := a.VerifyDevice(context.Background(), res.FlowToken, "1999-01", "dev-b", "other"); !errors.Is(err, ErrDeviceVerifyFailed) {
			t.Fatalf("attempt %d: want ErrDeviceVerifyFailed, got %v", i+1, err)
		}
	}
	// Cap exhausted: even the RIGHT answer is rejected.
	month := repo.users[id].CreatedAt.UTC().Format("2006-01")
	if _, err := a.VerifyDevice(context.Background(), res.FlowToken, month, "dev-b", "other"); !errors.Is(err, devverify.ErrTooManyAttempts) {
		t.Fatalf("want ErrTooManyAttempts, got %v", err)
	}
}

func TestEmptyFingerprintSkipsDeviceGate(t *testing.T) {
	a, repo := newTestAuth(t)
	signup(t, a, repo, "+82100", "dev-a")
	res := login(t, a, "+82100", "", "")
	if res.NextStep != StepTokensIssued {
		t.Fatalf("empty fingerprint: want tokens, got step %v", res.NextStep)
	}
}

func TestDormantReactivation(t *testing.T) {
	a, repo := newTestAuth(t)
	id := signup(t, a, repo, "+82100", "dev-a")
	repo.users[id].Status = domain.StatusDormant

	res := login(t, a, "+82100", "dev-a", "test-device")
	if res.NextStep != StepReactivationRequired {
		t.Fatalf("want reactivation step, got %v", res.NextStep)
	}

	// Without consent: rejected, still DORMANT.
	if _, err := a.CompleteReactivation(context.Background(), res.FlowToken, false, "dev-a", "test-device"); !errors.Is(err, ErrConsentRequired) {
		t.Fatalf("want ErrConsentRequired, got %v", err)
	}
	if repo.users[id].Status != domain.StatusDormant {
		t.Fatal("status must stay DORMANT without consent")
	}

	pair, err := a.CompleteReactivation(context.Background(), res.FlowToken, true, "dev-a", "test-device")
	if err != nil || pair == nil || pair.AccessToken == "" {
		t.Fatalf("reactivation: %v", err)
	}
	if repo.users[id].Status != domain.StatusActive {
		t.Fatalf("want ACTIVE after reactivation, got %s", repo.users[id].Status)
	}

	// Replay of the used flow token: ACTIVE→ACTIVE is not a legal transition.
	if _, err := a.CompleteReactivation(context.Background(), res.FlowToken, true, "dev-a", "test-device"); !errors.Is(err, domain.ErrInvalidTransition) {
		t.Fatalf("replay: want ErrInvalidTransition, got %v", err)
	}
}

func TestFlowTokenPurposeIsEnforced(t *testing.T) {
	a, repo := newTestAuth(t)
	signup(t, a, repo, "+82100", "dev-a")
	res := login(t, a, "+82100", "dev-b", "other") // device_verify flow token

	// A device_verify token must not complete a reactivation, and vice versa.
	if _, err := a.CompleteReactivation(context.Background(), res.FlowToken, true, "dev-b", "other"); !errors.Is(err, token.ErrInvalidFlowToken) {
		t.Fatalf("cross-purpose: want ErrInvalidFlowToken, got %v", err)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./services/identity/internal/usecase/`
Expected: FAIL — compile errors (`NewAuth` arity, missing methods/errors).

- [ ] **Step 3: Implement**

In `auth.go`:

Errors and struct/constructor:

```go
var (
	ErrUnsupportedGrant   = errors.New("unsupported grant_type")
	ErrConsentRequired    = errors.New("privacy consent is required for reactivation")
	ErrDeviceVerifyFailed = errors.New("device verification failed")
)
```

Interface gains (and `UpsertDevice` loses the bool):

```go
	UpdateStatus(ctx context.Context, userID int64, status domain.Status) error
	UpsertDevice(ctx context.Context, userID int64, fingerprint, name string) error
	IsDeviceKnown(ctx context.Context, userID int64, fingerprint string) (bool, error)
```

`Auth` gains `limiter *devverify.Limiter`; `NewAuth` takes it after `rtrStore`:

```go
func NewAuth(repo UserRepo, otpStore *otp.Store, rtrStore *rtr.Store, limiter *devverify.Limiter, issuer *token.Issuer, log zerolog.Logger, devMode bool) *Auth {
	return &Auth{repo: repo, otp: otpStore, rtr: rtrStore, limiter: limiter, issuer: issuer, log: log, devMode: devMode}
}
```

`VerifyOtp` ACTIVE branch becomes:

```go
	case domain.StatusActive:
		// ponytail: empty fingerprint skips the device gate so curl/demo
		// clients keep working; a real IdP would reject fingerprint-less logins.
		if fingerprint != "" {
			known, err := a.repo.IsDeviceKnown(ctx, user.ID, fingerprint)
			if err != nil {
				return nil, err
			}
			if !known {
				// Unknown device (PRD §4.2 item 4): no tokens until the user
				// passes the identity-owned knowledge check. The device is
				// deliberately NOT registered here.
				flowToken, err := a.issuer.FlowToken(token.FlowDeviceVerify, strconv.FormatInt(user.ID, 10))
				if err != nil {
					return nil, err
				}
				return &VerifyResult{NextStep: StepDeviceVerificationRequired, FlowToken: flowToken}, nil
			}
			if err := a.repo.UpsertDevice(ctx, user.ID, fingerprint, deviceName); err != nil {
				return nil, err
			}
		}
		if err := a.repo.TouchLastLogin(ctx, user.ID); err != nil {
			return nil, err
		}
		tokens, err := a.issueTokens(ctx, user)
		if err != nil {
			return nil, err
		}
		return &VerifyResult{NextStep: StepTokensIssued, Tokens: tokens}, nil
```

`CompleteSignup` signature gains `fingerprint, deviceName string`; after `CreateUser` succeeds:

```go
	if fingerprint != "" {
		if err := a.repo.UpsertDevice(ctx, user.ID, fingerprint, deviceName); err != nil {
			return nil, err
		}
	}
```

New methods:

```go
// CompleteReactivation rolls a DORMANT account back to ACTIVE after explicit
// privacy re-consent (PRD §4.2 item 3). Consent itself is not persisted — the
// §5 schema has no consent ledger, and the PoC doesn't need one.
func (a *Auth) CompleteReactivation(ctx context.Context, flowToken string, privacyConsent bool, fingerprint, deviceName string) (*TokenPair, error) {
	if !privacyConsent {
		return nil, ErrConsentRequired
	}
	userID, err := a.flowUserID(flowToken, token.FlowReactivation)
	if err != nil {
		return nil, err
	}
	user, err := a.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// The domain state machine is the gate: replaying a used flow token finds
	// the account already ACTIVE and fails here.
	if err := user.TransitionTo(domain.StatusActive); err != nil {
		return nil, err
	}
	if err := a.repo.UpdateStatus(ctx, user.ID, user.Status); err != nil {
		return nil, err
	}
	return a.finishLogin(ctx, user, fingerprint, deviceName)
}

// VerifyDevice completes the unknown-device flow (PRD §4.2 item 4): the user
// proves the account creation month ("YYYY-MM", UTC) — Identity-owned data
// only. Attempts are capped; success registers the device.
func (a *Auth) VerifyDevice(ctx context.Context, flowToken, registrationMonth, fingerprint, deviceName string) (*TokenPair, error) {
	userID, err := a.flowUserID(flowToken, token.FlowDeviceVerify)
	if err != nil {
		return nil, err
	}
	if err := a.limiter.Attempt(ctx, userID); err != nil {
		return nil, err
	}
	user, err := a.repo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// ponytail: UTC month; a signup near midnight KST on the 1st can answer
	// "wrong" — acceptable for the PoC, localize per-market if it ever ships.
	if user.CreatedAt.UTC().Format("2006-01") != registrationMonth {
		return nil, ErrDeviceVerifyFailed
	}
	if err := a.limiter.Reset(ctx, userID); err != nil {
		return nil, err
	}
	return a.finishLogin(ctx, user, fingerprint, deviceName)
}

// flowUserID verifies a flow token whose subject is a user ID.
func (a *Auth) flowUserID(flowToken, purpose string) (int64, error) {
	sub, err := a.issuer.VerifyFlowToken(flowToken, purpose)
	if err != nil {
		return 0, err
	}
	userID, err := strconv.ParseInt(sub, 10, 64)
	if err != nil {
		return 0, token.ErrInvalidFlowToken
	}
	return userID, nil
}

// finishLogin registers the device (both callers just proved control of the
// account), touches last_login, and issues the token set.
func (a *Auth) finishLogin(ctx context.Context, user *domain.User, fingerprint, deviceName string) (*TokenPair, error) {
	if fingerprint != "" {
		if err := a.repo.UpsertDevice(ctx, user.ID, fingerprint, deviceName); err != nil {
			return nil, err
		}
	}
	if err := a.repo.TouchLastLogin(ctx, user.ID); err != nil {
		return nil, err
	}
	return a.issueTokens(ctx, user)
}
```

Add `"identity-service/services/identity/internal/devverify"` to imports. Also update the two Phase-6 breadcrumb comments now stale (`// Device tracking only in Phase 2; ... lands in Phase 6.` — delete).

- [ ] **Step 4: Fix remaining callers so the build is green**

`services/identity/internal/app/app.go:55` — add the limiter arg:

```go
	auth := usecase.NewAuth(
		userRepo,
		otp.NewStore(rdb),
		rtr.NewStore(rdb),
		devverify.NewLimiter(rdb),
		...
```

(import `identity-service/services/identity/internal/devverify`).

`services/identity/internal/delivery/grpc/handler.go:59` — pass the new args through:

```go
	tokens, err := h.auth.CompleteSignup(ctx, req.GetFlowToken(), req.GetNickname(), req.GetProfileImageUrl(),
		req.GetDeviceFingerprint(), req.GetDeviceName())
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go build ./... && go test ./services/identity/...`
Expected: all green, including the new usecase tests.

- [ ] **Step 6: Commit (includes Task 3's staged repo change)**

```bash
git add services/identity/internal/
git commit -m "feat(identity): unknown-device gate + reactivation and device-verify usecases"
```

---

### Task 5: Delivery — gRPC handlers + error mapping

**Files:**
- Modify: `services/identity/internal/delivery/grpc/handler.go`

**Interfaces:**
- Consumes: Task 1 generated types, Task 4 usecase methods/errors.
- Produces: REST `POST /auth/v1/reactivate`, `POST /auth/v1/device/verify` via the existing gateway (no HTTP-layer change needed — routes come from proto annotations).

- [ ] **Step 1: Add the two handlers**

```go
func (h *Handler) CompleteReactivation(ctx context.Context, req *identityv1.CompleteReactivationRequest) (*identityv1.CompleteReactivationResponse, error) {
	if req.GetFlowToken() == "" {
		return nil, status.Error(codes.InvalidArgument, "flow_token is required")
	}
	tokens, err := h.auth.CompleteReactivation(ctx, req.GetFlowToken(), req.GetPrivacyConsent(),
		req.GetDeviceFingerprint(), req.GetDeviceName())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.CompleteReactivationResponse{Tokens: toProtoTokens(tokens)}, nil
}

func (h *Handler) VerifyDevice(ctx context.Context, req *identityv1.VerifyDeviceRequest) (*identityv1.VerifyDeviceResponse, error) {
	if req.GetFlowToken() == "" || req.GetRegistrationMonth() == "" {
		return nil, status.Error(codes.InvalidArgument, "flow_token and registration_month are required")
	}
	tokens, err := h.auth.VerifyDevice(ctx, req.GetFlowToken(), req.GetRegistrationMonth(),
		req.GetDeviceFingerprint(), req.GetDeviceName())
	if err != nil {
		return nil, mapErr(err)
	}
	return &identityv1.VerifyDeviceResponse{Tokens: toProtoTokens(tokens)}, nil
}
```

- [ ] **Step 2: Extend `mapErr`**

Add cases (import `devverify`):

```go
	case errors.Is(err, usecase.ErrConsentRequired):
		return status.Error(codes.InvalidArgument, err.Error())
	case errors.Is(err, usecase.ErrDeviceVerifyFailed),
		errors.Is(err, devverify.ErrTooManyAttempts):
		// Same code as the OTP failures: an auth challenge was not met.
		return status.Error(codes.Unauthenticated, err.Error())
	case errors.Is(err, domain.ErrInvalidTransition):
		// e.g. replaying a used reactivation flow token on an ACTIVE account.
		return status.Error(codes.FailedPrecondition, err.Error())
	case errors.Is(err, domain.ErrUserNotFound):
		// Flow token for a since-deleted user; don't leak which part failed.
		return status.Error(codes.Unauthenticated, "invalid flow")
```

- [ ] **Step 3: Build + full test sweep**

Run: `go build ./... && make test`
Expected: green (handlers are thin; behavior is covered by Task 4's tests).

- [ ] **Step 4: Commit**

```bash
git add services/identity/internal/delivery/
git commit -m "feat(identity): expose reactivation + device-verify endpoints"
```

---

### Task 6: m6 smoke test

**Files:**
- Create: `scripts/m6_smoke.sh` (mode 755, follow the style of `scripts/m1_smoke.sh`: `set -euo pipefail`, `jq` assertions, numbered steps)

**Interfaces:**
- Consumes: running compose stack (`make run` + `make migrate-up`), REST endpoints from Task 5, MySQL container name from `docker-compose.yml` (verify with `docker compose ps` — use `docker compose exec -T mysql mysql ...`).

- [ ] **Step 1: Write the script**

```bash
#!/usr/bin/env bash
# m6 smoke: phase 6 edge flows (PRD §4.2 items 3–4).
#   A. DORMANT reactivation: dormant user → REACTIVATION_REQUIRED → consent → tokens.
#   B. Unknown device: new fingerprint → DEVICE_VERIFICATION_REQUIRED →
#      wrong month rejected → right month → tokens → board post works.
set -euo pipefail

IDENTITY=${IDENTITY:-http://localhost:8090}
BOARD=${BOARD:-http://localhost:8091}
PHONE="+8210$(date +%s | tail -c 8)" # unique per run
# Same access pattern as m4_smoke.sh (container idsvc-mysql, root:root — PoC only).
MYSQL=(docker exec idsvc-mysql mysql -N -uroot -proot identity)

step() { echo; echo "== $1"; }
fail() { echo "SMOKE FAIL: $1" >&2; exit 1; }

otp_login() { # $1=fingerprint $2=device_name → verify response JSON
  local code
  code=$(curl -sf "$IDENTITY/auth/v1/otp/request" -d "{\"phone_number\":\"$PHONE\"}" | jq -r .debugCode)
  curl -sf "$IDENTITY/auth/v1/otp/verify" \
    -d "{\"phone_number\":\"$PHONE\",\"code\":\"$code\",\"device_fingerprint\":\"$1\",\"device_name\":\"$2\"}"
}

step "signup (registers device dev-a)"
V=$(otp_login dev-a m6-primary)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_SIGNUP_REQUIRED" ] || fail "expected signup step"
FLOW=$(jq -r .flowToken <<<"$V")
curl -sf "$IDENTITY/auth/v1/signup" \
  -d "{\"flow_token\":\"$FLOW\",\"nickname\":\"m6user\",\"device_fingerprint\":\"dev-a\",\"device_name\":\"m6-primary\"}" \
  | jq -e '.tokens.accessToken' >/dev/null || fail "signup issued no tokens"
USER_ID=$("${MYSQL[@]}" -e "SELECT user_id FROM user_credentials WHERE identifier='$PHONE'")
echo "user id: $USER_ID"

step "A. force DORMANT via SQL, login → REACTIVATION_REQUIRED"
"${MYSQL[@]}" -e "UPDATE users SET status='DORMANT' WHERE id=$USER_ID"
V=$(otp_login dev-a m6-primary)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_REACTIVATION_REQUIRED" ] || fail "expected reactivation step"
FLOW=$(jq -r .flowToken <<<"$V")

step "A. reactivate without consent → 4xx"
curl -sf "$IDENTITY/auth/v1/reactivate" \
  -d "{\"flow_token\":\"$FLOW\",\"privacy_consent\":false}" >/dev/null && fail "consentless reactivation accepted"

step "A. reactivate with consent → tokens, status ACTIVE"
R=$(curl -sf "$IDENTITY/auth/v1/reactivate" \
  -d "{\"flow_token\":\"$FLOW\",\"privacy_consent\":true,\"device_fingerprint\":\"dev-a\",\"device_name\":\"m6-primary\"}")
[ "$(jq -r '.tokens.accessToken' <<<"$R")" != "null" ] || fail "no tokens after reactivation"
[ "$("${MYSQL[@]}" -N -e "SELECT status FROM users WHERE id=$USER_ID")" = "ACTIVE" ] || fail "status not ACTIVE"

step "B. login from unknown device dev-b → DEVICE_VERIFICATION_REQUIRED, no tokens"
V=$(otp_login dev-b m6-laptop)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_DEVICE_VERIFICATION_REQUIRED" ] || fail "expected device verification step"
[ "$(jq -r .tokens <<<"$V")" = "null" ] || fail "tokens leaked before device verification"
FLOW=$(jq -r .flowToken <<<"$V")

step "B. wrong creation month → 401"
curl -sf "$IDENTITY/auth/v1/device/verify" \
  -d "{\"flow_token\":\"$FLOW\",\"registration_month\":\"1999-01\",\"device_fingerprint\":\"dev-b\",\"device_name\":\"m6-laptop\"}" \
  >/dev/null && fail "wrong month accepted"

step "B. right creation month → tokens; board accepts them"
MONTH=$("${MYSQL[@]}" -N -e "SELECT DATE_FORMAT(created_at,'%Y-%m') FROM users WHERE id=$USER_ID")
R=$(curl -sf "$IDENTITY/auth/v1/device/verify" \
  -d "{\"flow_token\":\"$FLOW\",\"registration_month\":\"$MONTH\",\"device_fingerprint\":\"dev-b\",\"device_name\":\"m6-laptop\"}")
AT=$(jq -r '.tokens.accessToken' <<<"$R")
[ "$AT" != "null" ] || fail "no tokens after device verification"
curl -sf "$BOARD/board/v1/posts" -H "Authorization: Bearer $AT" \
  -d '{"title":"m6","content":"edge flows work"}' >/dev/null || fail "board rejected verified-device token"

step "B. dev-b now known: fresh login goes straight to tokens"
V=$(otp_login dev-b m6-laptop)
[ "$(jq -r .nextStep <<<"$V")" = "NEXT_STEP_TOKENS_ISSUED" ] || fail "re-login from verified device not trusted"

echo; echo "M6 SMOKE PASSED ✅"
```

Note for the implementer: OTP rate limit is 5/hour/phone and this script makes 5 OTP requests (signup, dormant login, unknown-device login, known-device re-login = 4; recount after any edit) — the unique `$PHONE` per run keeps it under the cap.

- [ ] **Step 2: Run it against the live stack**

Run: `make run && make migrate-up && ./scripts/m6_smoke.sh`
Expected: `M6 SMOKE PASSED ✅`. Also re-run `./scripts/m1_smoke.sh` and `./scripts/m2_smoke.sh` — m1 logs in twice with the same fingerprint (`smoke-device`), which now exercises the known-device path; both must still pass.

- [ ] **Step 3: Commit**

```bash
git add scripts/m6_smoke.sh
git commit -m "test: add m6 smoke for reactivation + device-verification flows"
```

---

### Task 7: Web demo — reactivation & device-verify screens

**Files:**
- Modify: `web/src/api.ts`, `web/src/screens/Login.tsx`, `web/src/App.tsx`
- Create: `web/src/screens/Reactivate.tsx`, `web/src/screens/DeviceVerify.tsx`

**Interfaces:**
- Consumes: REST endpoints from Task 5; existing helpers `setTokens`, `deviceFingerprint()`, `deviceName()`, `apiFetch`, `TokenPair`.
- Produces: `completeReactivation(flowToken)`, `verifyDevice(flowToken, registrationMonth)` in `api.ts`; `Login` gains `onReactivationRequired`/`onDeviceVerifyRequired` props; `App` gains `'reactivate' | 'deviceVerify'` screens.

- [ ] **Step 1: api.ts — two client calls + signup sends device**

```ts
export const completeReactivation = (flowToken: string) =>
  apiFetch<{ tokens: TokenPair }>('/auth/v1/reactivate', {
    method: 'POST',
    body: JSON.stringify({
      flow_token: flowToken,
      privacy_consent: true, // the screen's button IS the consent action
      device_fingerprint: deviceFingerprint(),
      device_name: deviceName(),
    }),
  })

export const verifyDevice = (flowToken: string, registrationMonth: string) =>
  apiFetch<{ tokens: TokenPair }>('/auth/v1/device/verify', {
    method: 'POST',
    body: JSON.stringify({
      flow_token: flowToken,
      registration_month: registrationMonth,
      device_fingerprint: deviceFingerprint(),
      device_name: deviceName(),
    }),
  })
```

And in `completeSignup`, add the device fields to the body:

```ts
export const completeSignup = (flowToken: string, nickname: string) =>
  apiFetch<{ tokens: TokenPair }>('/auth/v1/signup', {
    method: 'POST',
    body: JSON.stringify({
      flow_token: flowToken,
      nickname,
      device_fingerprint: deviceFingerprint(),
      device_name: deviceName(),
    }),
  })
```

- [ ] **Step 2: Screens**

`web/src/screens/Reactivate.tsx`:

```tsx
import { useState } from 'react'
import { completeReactivation } from '../api'
import { setTokens } from '../auth'

interface Props {
  flowToken: string
  onDone: () => void
}

// DORMANT-account reactivation: agreeing to the privacy policy rolls the
// account back to ACTIVE (PRD §4.2 item 3).
export default function Reactivate({ flowToken, onDone }: Props) {
  const [agreed, setAgreed] = useState(false)
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit() {
    setBusy(true)
    setError('')
    try {
      const res = await completeReactivation(flowToken)
      setTokens(res.tokens)
      onDone()
    } catch (err) {
      setError(String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="card">
      <h1>Welcome back</h1>
      <p>This account was dormant. Re-agree to the privacy policy to reactivate it.</p>
      <label>
        <input type="checkbox" checked={agreed} onChange={(e) => setAgreed(e.target.checked)} /> I
        agree to the privacy policy
      </label>
      <button disabled={!agreed || busy} onClick={submit}>
        Reactivate
      </button>
      {error && <p className="error">{error}</p>}
    </main>
  )
}
```

`web/src/screens/DeviceVerify.tsx`:

```tsx
import { useState, type FormEvent } from 'react'
import { ApiError, verifyDevice } from '../api'
import { setTokens } from '../auth'

interface Props {
  flowToken: string
  onDone: () => void
  onRestart: () => void // attempts/flow token exhausted → back to login
}

// Unknown-device check: prove the account creation month (PRD §4.2 item 4).
export default function DeviceVerify({ flowToken, onDone, onRestart }: Props) {
  const [month, setMonth] = useState('')
  const [error, setError] = useState('')
  const [busy, setBusy] = useState(false)

  async function submit(e: FormEvent) {
    e.preventDefault()
    setBusy(true)
    setError('')
    try {
      const res = await verifyDevice(flowToken, month)
      setTokens(res.tokens)
      onDone()
    } catch (err) {
      setError(err instanceof ApiError && err.status === 401 ? 'wrong answer — try again' : String(err))
    } finally {
      setBusy(false)
    }
  }

  return (
    <main className="card">
      <h1>New device</h1>
      <p>This device isn’t registered to your account. When did you sign up?</p>
      <form onSubmit={submit}>
        <label htmlFor="month">Signup month</label>
        <input id="month" type="month" value={month} onChange={(e) => setMonth(e.target.value)} required />
        <button disabled={busy}>Verify</button>
        <button type="button" onClick={onRestart}>
          Back to login
        </button>
      </form>
      {error && <p className="error">{error}</p>}
    </main>
  )
}
```

(`<input type="month">` yields exactly `"YYYY-MM"` — no parsing code needed.)

- [ ] **Step 3: Wire Login + App**

`Login.tsx`: extend `Props` and the switch:

```tsx
interface Props {
  onTokens: () => void
  onSignupRequired: (flowToken: string) => void
  onReactivationRequired: (flowToken: string) => void
  onDeviceVerifyRequired: (flowToken: string) => void
}
```

```tsx
        case 'NEXT_STEP_REACTIVATION_REQUIRED':
          onReactivationRequired(res.flowToken!)
          break
        case 'NEXT_STEP_DEVICE_VERIFICATION_REQUIRED':
          onDeviceVerifyRequired(res.flowToken!)
          break
        default:
          setError(`unexpected next step: ${res.nextStep}`)
```

`App.tsx`:

```tsx
type Screen = 'loading' | 'login' | 'signup' | 'reactivate' | 'deviceVerify' | 'board'
```

```tsx
    case 'login':
      return (
        <Login
          onTokens={() => setScreen('board')}
          onSignupRequired={(ft) => {
            setFlowToken(ft)
            setScreen('signup')
          }}
          onReactivationRequired={(ft) => {
            setFlowToken(ft)
            setScreen('reactivate')
          }}
          onDeviceVerifyRequired={(ft) => {
            setFlowToken(ft)
            setScreen('deviceVerify')
          }}
        />
      )
    case 'reactivate':
      return <Reactivate flowToken={flowToken} onDone={() => setScreen('board')} />
    case 'deviceVerify':
      return (
        <DeviceVerify
          flowToken={flowToken}
          onDone={() => setScreen('board')}
          onRestart={() => setScreen('login')}
        />
      )
```

(imports: `import Reactivate from './screens/Reactivate'`, `import DeviceVerify from './screens/DeviceVerify'`.)

- [ ] **Step 4: Verify**

Run: `make web-test && (cd web && npx tsc --noEmit)`
Expected: existing vitest suite green, no type errors. Manual check (optional but cheap since the stack is already up from Task 6): `make web-dev`, sign up, then in devtools `localStorage.removeItem('idsvc.device_id')` + logout → login again → device-verify screen appears; correct month lands on the board.

- [ ] **Step 5: Commit**

```bash
git add web/src/
git commit -m "feat(web): reactivation + device-verification screens"
```

---

### Task 8: Docs + status flip

**Files:**
- Modify: `README.md`, `plan/2026-07-17-build-order.md`, `plan/2026-08-28-phase6-edge-states.md`

- [ ] **Step 1: Update docs**

- `README.md` roadmap: phase 6 row → `✅ 2026-08-28`.
- `README.md` auth-flow mermaid: the DORMANT branch note `(Phase 6)` → drop marker; add the two endpoints to the endpoint table (`POST /auth/v1/reactivate`, `POST /auth/v1/device/verify`) and `./scripts/m6_smoke.sh` to the smoke-test list.
- `plan/2026-07-17-build-order.md` header: Phases 0–6 done, Phase 7 next.
- This plan file: status → ✅ completed.

- [ ] **Step 2: Commit**

```bash
git add README.md plan/
git commit -m "docs: mark phase 6 complete"
```

Then stop: leave `feat/edge-flows` unmerged for hcho's review (CLAUDE.md review gate).

## Skipped on purpose

- Consent ledger table — no §5 schema for it; add if consent history ever matters.
- Admin endpoint to force DORMANT — SQL in the smoke script covers the demo; a dormancy sweep job is a real-product concern.
- Binding the device_verify flow token to the fingerprint seen at login — client re-sends it; fingerprints are client-chosen either way, so binding adds ceremony, not security, in this PoC.
- Device list / revoke UI — nothing in the PRD.
