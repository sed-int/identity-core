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
	if repo.devices[devKey(id, "dev-b")] {
		t.Fatal("failed verification must not register the device")
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

	for i := range devverify.MaxAttempts {
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
