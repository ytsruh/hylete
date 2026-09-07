package controllers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"hylete/internal/models"
)

// memoryAuthTokenRepo is a fully in-memory AuthTokenRepo used
// by the recovery controller tests. It implements atomic
// consumption with a sync.Mutex so a second Consume on the same
// raw token returns models.ErrAuthTokenInvalid (matching the
// SQL behavior the production repo relies on).
type memoryAuthTokenRepo struct {
	mu      sync.Mutex
	tokens  map[string]memoryToken // key = sha256(raw)
	counter int
}

type memoryToken struct {
	id        string
	userID    string
	raw       string
	ttl       time.Duration
	createdAt time.Time
	used      bool
}

func newMemoryAuthTokenRepo() *memoryAuthTokenRepo {
	return &memoryAuthTokenRepo{tokens: map[string]memoryToken{}}
}

func (r *memoryAuthTokenRepo) CreatePasswordResetToken(ctx context.Context, userID, raw string, ttl time.Duration) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if userID == "" || raw == "" || ttl <= 0 {
		return "", errors.New("invalid input")
	}
	r.counter++
	id := "mem-row-" + raw
	r.tokens[hashForTest(raw)] = memoryToken{
		id:        id,
		userID:    userID,
		raw:       raw,
		ttl:       ttl,
		createdAt: time.Now(),
	}
	return id, nil
}

func (r *memoryAuthTokenRepo) ConsumePasswordResetToken(ctx context.Context, raw string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if raw == "" {
		return "", models.ErrAuthTokenInvalid
	}
	key := hashForTest(raw)
	tok, ok := r.tokens[key]
	if !ok || tok.used {
		return "", models.ErrAuthTokenInvalid
	}
	if time.Since(tok.createdAt) > tok.ttl {
		return "", models.ErrAuthTokenInvalid
	}
	tok.used = true
	r.tokens[key] = tok
	return tok.userID, nil
}

// hashForTest is a stand-in for the production sha256 hash. We
// use a simple XOR so the map key is deterministic without
// pulling crypto into the test file; the production code is
// tested in models/auth_token_test.go.
func hashForTest(s string) string {
	const mask = byte(0x5a)
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		out[i] = s[i] ^ mask
	}
	return string(out)
}

// mockResetSender records the (tokenRepo, user) pairs handed
// to it. The test controls whether the send returns an error.
type mockResetSender struct {
	mu       sync.Mutex
	calls    int
	user     *models.User
	repo     models.AuthTokenRepo
	sendErr  error
	calledCh chan struct{}
}

func newMockResetSender() *mockResetSender {
	return &mockResetSender{calledCh: make(chan struct{}, 1)}
}

func (m *mockResetSender) SendPasswordReset(ctx context.Context, repo models.AuthTokenRepo, user *models.User) (string, error) {
	m.mu.Lock()
	m.calls++
	m.user = user
	m.repo = repo
	m.mu.Unlock()
	select {
	case m.calledCh <- struct{}{}:
	default:
	}
	if m.sendErr != nil {
		return "", m.sendErr
	}
	return "raw-token", nil
}

func setupRecoveryController(t *testing.T) (*AuthRecoveryController, *mockUserRepository, *memoryAuthTokenRepo, *mockResetSender) {
	t.Helper()
	mockUser := newMockUserRepository()
	repo := newMemoryAuthTokenRepo()
	sender := newMockResetSender()
	return NewAuthRecoveryController(mockUser, repo, sender), mockUser, repo, sender
}

// --- RequestPasswordReset --------------------------------------

func TestAuthRecovery_RequestPasswordReset_UnknownUserIsSilent(t *testing.T) {
	// A request for an email that does not exist in the
	// store must return nil — the route renders the same
	// "we sent you an email" page as on success, so an
	// attacker cannot enumerate accounts.
	ctrl, _, _, sender := setupRecoveryController(t)
	err := ctrl.RequestPasswordReset(context.Background(), "ghost@example.com")
	if err != nil {
		t.Fatalf("expected nil error for unknown user, got %v", err)
	}
	// Sender must not be called — there is no user to send
	// to. If the controller accidentally called the sender
	// with a nil user, the test would crash inside the
	// goroutine rather than here, but the calls counter is
	// the cleanest signal.
	if sender.calls != 0 {
		t.Errorf("sender called %d times, want 0", sender.calls)
	}
}

func TestAuthRecovery_RequestPasswordReset_ExistingUserSendsEmail(t *testing.T) {
	ctrl, mockUser, _, sender := setupRecoveryController(t)
	mockUser.users = append(mockUser.users, models.User{
		ID: "user-1", Name: "Alice", Email: "alice@example.com",
	})

	err := ctrl.RequestPasswordReset(context.Background(), "alice@example.com")
	if err != nil {
		t.Fatalf("RequestPasswordReset: %v", err)
	}
	select {
	case <-sender.calledCh:
	case <-time.After(2 * time.Second):
		t.Fatal("sender was not called within 2s")
	}
	if sender.user == nil || sender.user.Email != "alice@example.com" {
		t.Errorf("sender received wrong user: %+v", sender.user)
	}
}

func TestAuthRecovery_RequestPasswordReset_TrimsAndIgnoresEmpty(t *testing.T) {
	// A blank email (or whitespace-only) is treated like a
	// missing email: return nil, do not look up the user.
	ctrl, mockUser, _, sender := setupRecoveryController(t)
	mockUser.users = append(mockUser.users, models.User{
		ID: "u", Name: "X", Email: "x@example.com",
	})
	if err := ctrl.RequestPasswordReset(context.Background(), "   "); err != nil {
		t.Errorf("expected nil for blank email, got %v", err)
	}
	if sender.calls != 0 {
		t.Errorf("sender called %d times, want 0 for blank email", sender.calls)
	}
}

func TestAuthRecovery_RequestPasswordReset_SenderErrorSurfaces(t *testing.T) {
	// An SMTP error must surface to the caller (which renders
	// a generic error toast). The token may or may not have
	// been persisted depending on the order of operations
	// inside the service — we do not assert on that here, the
	// service-level test in email/service_test.go does.
	ctrl, mockUser, _, sender := setupRecoveryController(t)
	sender.sendErr = errors.New("smtp down")
	mockUser.users = append(mockUser.users, models.User{
		ID: "u", Name: "X", Email: "x@example.com",
	})
	err := ctrl.RequestPasswordReset(context.Background(), "x@example.com")
	if err == nil {
		t.Fatal("expected error when sender fails")
	}
}

// --- ResetPassword ---------------------------------------------

func TestAuthRecovery_ResetPassword_HappyPath(t *testing.T) {
	ctrl, mockUser, repo, _ := setupRecoveryController(t)
	mockUser.users = append(mockUser.users, models.User{
		ID: "user-1", Name: "Alice", Email: "alice@example.com",
		PasswordHash: "old-hash",
	})
	if _, err := repo.CreatePasswordResetToken(context.Background(), "user-1", "raw-tok", time.Hour); err != nil {
		t.Fatalf("seed: %v", err)
	}

	if err := ctrl.ResetPassword(context.Background(), "raw-tok", "newpass1"); err != nil {
		t.Fatalf("ResetPassword: %v", err)
	}

	// The user is updated.
	got, _ := mockUser.GetUserByID("user-1")
	if got == nil {
		t.Fatal("user not found after reset")
	}
	if got.PasswordHash == "old-hash" {
		t.Error("password hash was not updated")
	}

	// The token is consumed — a second reset on the same
	// token must fail.
	if err := ctrl.ResetPassword(context.Background(), "raw-tok", "another1"); !errors.Is(err, models.ErrAuthTokenInvalid) {
		t.Errorf("second reset err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthRecovery_ResetPassword_RejectsShortPassword(t *testing.T) {
	ctrl, _, _, _ := setupRecoveryController(t)
	err := ctrl.ResetPassword(context.Background(), "any-token", "short")
	if !errors.Is(err, ErrInvalidPassword) {
		t.Errorf("err = %v, want ErrInvalidPassword", err)
	}
}

func TestAuthRecovery_ResetPassword_RejectsEmptyToken(t *testing.T) {
	ctrl, _, _, _ := setupRecoveryController(t)
	err := ctrl.ResetPassword(context.Background(), "", "longenough")
	if !errors.Is(err, models.ErrAuthTokenInvalid) {
		t.Errorf("err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthRecovery_ResetPassword_RejectsUnknownToken(t *testing.T) {
	ctrl, _, _, _ := setupRecoveryController(t)
	err := ctrl.ResetPassword(context.Background(), "never-issued", "longenough")
	if !errors.Is(err, models.ErrAuthTokenInvalid) {
		t.Errorf("err = %v, want ErrAuthTokenInvalid", err)
	}
}

func TestAuthRecovery_ResetPassword_RejectsExpiredToken(t *testing.T) {
	ctrl, mockUser, repo, _ := setupRecoveryController(t)
	mockUser.users = append(mockUser.users, models.User{
		ID: "user-1", Name: "Alice", Email: "alice@example.com",
	})
	// Seed with a negative TTL (in-memory repo's
	// CreatePasswordResetToken rejects that, so we
	// back-date the row directly).
	repo.mu.Lock()
	repo.tokens[hashForTest("raw-tok")] = memoryToken{
		id:        "row-1",
		userID:    "user-1",
		raw:       "raw-tok",
		ttl:       time.Hour,
		createdAt: time.Now().Add(-2 * time.Hour),
	}
	repo.mu.Unlock()

	err := ctrl.ResetPassword(context.Background(), "raw-tok", "longenough")
	if !errors.Is(err, models.ErrAuthTokenInvalid) {
		t.Errorf("err = %v, want ErrAuthTokenInvalid", err)
	}
}
