package controllers

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"hylete/internal/models"
	"hylete/internal/utils"
)

// timeAfter is a one-line helper to keep the test above readable.
// 2_000_000_000 ns == 2s, which is generous for an in-process
// goroutine handoff that just appends to a slice.
func timeAfter(d time.Duration) <-chan time.Time { return time.After(d) }

type mockUserRepository struct {
	mu    sync.Mutex
	users []models.User
}

func newMockUserRepository() *mockUserRepository {
	return &mockUserRepository{}
}

func (m *mockUserRepository) CreateUser(user *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == user.Email {
			return errors.New("email already exists")
		}
	}
	user.ID = "user-" + user.Email
	m.users = append(m.users, *user)
	return nil
}

func (m *mockUserRepository) GetUserByEmail(email string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.Email == email {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepository) GetUserByID(id string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if u.ID == id {
			cp := u
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockUserRepository) UpdateUser(user *models.User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, u := range m.users {
		if u.ID == user.ID {
			m.users[i] = *user
			return nil
		}
	}
	return errors.New("user not found")
}

func (m *mockUserRepository) UpdateUserPassword(userID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if passwordHash == "" {
		return errors.New("password hash is empty")
	}
	for i, u := range m.users {
		if u.ID == userID {
			m.users[i].PasswordHash = passwordHash
			return nil
		}
	}
	return errors.New("user not found")
}

// UpdateUserReminder is a no-op in the auth-test mock; the
// auth flows never write reminder state, but the interface
// requires the method.
func (m *mockUserRepository) UpdateUserReminder(userID string, prefs models.ReminderPreferences) error {
	return nil
}

func setupAuthController(t *testing.T) (*AuthController, *mockUserRepository) {
	t.Helper()
	mockUser := newMockUserRepository()
	jwtService := utils.NewJWTService("test-secret")
	return NewAuthController(mockUser, jwtService, nil), mockUser
}

// mockWelcomeSender records the user the email would be sent to.
// Used by the new "welcome is fired on register" test below.
type mockWelcomeSender struct {
	mu      sync.Mutex
	users   []*models.User
	err     error // returned by SendWelcome
	gotCall chan struct{}
}

func newMockWelcomeSender() *mockWelcomeSender {
	return &mockWelcomeSender{gotCall: make(chan struct{}, 1)}
}

func (m *mockWelcomeSender) SendWelcome(ctx context.Context, user *models.User) error {
	m.mu.Lock()
	m.users = append(m.users, user)
	m.mu.Unlock()
	select {
	case m.gotCall <- struct{}{}:
	default:
	}
	return m.err
}

func (m *mockWelcomeSender) lastUser() *models.User {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.users) == 0 {
		return nil
	}
	u := m.users[len(m.users)-1]
	return u
}

func TestAuthController_Login_Success(t *testing.T) {
	ac, mockUser := setupAuthController(t)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	mockUser.users = append(mockUser.users, models.User{
		ID:           "user-1",
		Name:         "Alice",
		Email:        "alice@example.com",
		PasswordHash: string(hash),
	})

	user, token, err := ac.Login("alice@example.com", "correct")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.Email != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %q", user.Email)
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}
}

func TestAuthController_Login_InvalidPassword(t *testing.T) {
	ac, mockUser := setupAuthController(t)

	hash, _ := bcrypt.GenerateFromPassword([]byte("correct"), bcrypt.DefaultCost)
	mockUser.users = append(mockUser.users, models.User{
		ID:           "user-1",
		Name:         "Bob",
		Email:        "bob@example.com",
		PasswordHash: string(hash),
	})

	_, _, err := ac.Login("bob@example.com", "wrong")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthController_Login_UserNotFound(t *testing.T) {
	ac, _ := setupAuthController(t)

	_, _, err := ac.Login("nobody@example.com", "password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("expected ErrInvalidCredentials, got %v", err)
	}
}

func TestAuthController_Login_EmptyFields(t *testing.T) {
	ac, _ := setupAuthController(t)

	tests := []struct {
		email    string
		password string
	}{
		{"", "password"},
		{"email@example.com", ""},
		{"", ""},
	}

	for _, tt := range tests {
		_, _, err := ac.Login(tt.email, tt.password)
		if !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("expected ErrInvalidCredentials for email=%q password=%q, got %v", tt.email, tt.password, err)
		}
	}
}

func TestAuthController_Register_Success(t *testing.T) {
	ac, mockUser := setupAuthController(t)

	user, token, err := ac.Register("Alice", "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if user == nil {
		t.Fatal("expected user, got nil")
	}
	if user.ID == "" {
		t.Fatalf("expected non-empty user ID")
	}
	if token == "" {
		t.Fatal("expected non-empty token")
	}

	stored := mockUser.users[0]
	if err := bcrypt.CompareHashAndPassword([]byte(stored.PasswordHash), []byte("secret123")); err != nil {
		t.Fatal("expected stored password to be a valid bcrypt hash")
	}
}

func TestAuthController_Register_EmailExists(t *testing.T) {
	ac, mockUser := setupAuthController(t)

	mockUser.users = append(mockUser.users, models.User{
		ID:    "user-1",
		Name:  "Alice",
		Email: "alice@example.com",
	})

	_, _, err := ac.Register("Alice2", "alice@example.com", "secret123")
	if !errors.Is(err, ErrEmailExists) {
		t.Fatalf("expected ErrEmailExists, got %v", err)
	}
}

func TestAuthController_Register_EmptyFields(t *testing.T) {
	ac, _ := setupAuthController(t)

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{"", "email@example.com", "password"},
		{"Name", "", "password"},
		{"Name", "email@example.com", ""},
		{"", "", ""},
	}

	for _, tt := range tests {
		_, _, err := ac.Register(tt.name, tt.email, tt.password)
		if err == nil {
			t.Fatalf("expected error for name=%q email=%q", tt.name, tt.email)
		}
		if err.Error() != "all fields are required" {
			t.Fatalf("expected 'all fields are required', got %q", err.Error())
		}
	}
}

func TestAuthController_Register_ShortPassword(t *testing.T) {
	ac, _ := setupAuthController(t)

	_, _, err := ac.Register("Alice", "alice@example.com", "short")
	if err == nil {
		t.Fatal("expected error for short password")
	}
	if err.Error() != "password must be at least 6 characters" {
		t.Fatalf("expected password length error, got %q", err.Error())
	}
}

func TestAuthController_RegisterAndLogin(t *testing.T) {
	ac, _ := setupAuthController(t)

	user, token, err := ac.Register("Alice", "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if user == nil || token == "" {
		t.Fatal("expected user and token after registration")
	}

	loggedInUser, loginToken, err := ac.Login("alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}
	if loggedInUser == nil || loginToken == "" {
		t.Fatal("expected user and token after login")
	}
	if loggedInUser.Email != "alice@example.com" {
		t.Fatalf("expected email alice@example.com, got %q", loggedInUser.Email)
	}
}

func TestAuthController_Register_FiresWelcome(t *testing.T) {
	// A successful Register must hand the user off to the
	// welcomeSender. The send happens in a goroutine, so the
	// test waits on the gotCall channel up to a small
	// timeout — a missing handoff would mean the goroutine
	// never ran, not that it ran slowly.
	mockUser := newMockUserRepository()
	sender := newMockWelcomeSender()
	ac := NewAuthController(mockUser, utils.NewJWTService("test-secret"), sender)

	_, _, err := ac.Register("Alice", "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("Register: %v", err)
	}

	select {
	case <-sender.gotCall:
	case <-timeAfter(2_000_000_000): // 2 seconds in ns
		t.Fatal("welcome email was not sent within 2s")
	}

	got := sender.lastUser()
	if got == nil {
		t.Fatal("sender received no user")
	}
	if got.Email != "alice@example.com" {
		t.Errorf("welcome user email = %q, want alice@example.com", got.Email)
	}
}

func TestAuthController_Register_EmailFailureDoesNotFailRegister(t *testing.T) {
	// A sender that errors must not propagate the error
	// back to the caller. The user is created and
	// authenticated regardless; the email is best-effort.
	mockUser := newMockUserRepository()
	sender := &mockWelcomeSender{err: errors.New("smtp down"), gotCall: make(chan struct{}, 1)}
	ac := NewAuthController(mockUser, utils.NewJWTService("test-secret"), sender)

	user, token, err := ac.Register("Alice", "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("Register should not fail on email error, got %v", err)
	}
	if user == nil || token == "" {
		t.Fatal("expected user and token despite email error")
	}
}

func TestAuthController_Register_NilSenderSkipsWelcome(t *testing.T) {
	// A nil sender must not panic. Register with a nil
	// welcomeSender is supported so the existing tests that
	// pre-date the email feature still work.
	mockUser := newMockUserRepository()
	ac := NewAuthController(mockUser, utils.NewJWTService("test-secret"), nil)

	_, _, err := ac.Register("Alice", "alice@example.com", "secret123")
	if err != nil {
		t.Fatalf("Register with nil sender: %v", err)
	}
}
