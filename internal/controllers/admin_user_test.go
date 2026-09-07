package controllers

import (
	"context"
	"errors"
	"sync"
	"testing"

	"hylete/internal/models"
)

// mockAdminUserRepository is an in-memory AdminUserRepo. GetUserByID
// returns copies so tests can't accidentally mutate the stored rows;
// SetUserAdmin errors on unknown IDs so a silent no-op cannot hide a
// broken expectation.
type mockAdminUserRepository struct {
	mu    sync.Mutex
	users []models.User
}

func newMockAdminUserRepository() *mockAdminUserRepository {
	return &mockAdminUserRepository{}
}

func (m *mockAdminUserRepository) ListUsers(_ context.Context) ([]models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]models.User, len(m.users))
	copy(result, m.users)
	return result, nil
}

func (m *mockAdminUserRepository) GetUserByID(_ context.Context, id string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.users {
		if m.users[i].ID == id {
			u := m.users[i]
			return &u, nil
		}
	}
	return nil, nil
}

func (m *mockAdminUserRepository) SetUserAdmin(_ context.Context, userID string, isAdmin bool) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.users {
		if m.users[i].ID == userID {
			m.users[i].IsAdmin = isAdmin
			return nil
		}
	}
	return errors.New("user not found")
}

// newAdminUserControllerWithUsers wires an AdminUserController to an
// in-memory repo pre-seeded with users and a shared mock reset sender.
func newAdminUserControllerWithUsers(t *testing.T, users []models.User) (*AdminUserController, *mockAdminUserRepository, *mockResetSender) {
	t.Helper()
	mock := newMockAdminUserRepository()
	mock.users = users
	sender := newMockResetSender()
	ctrl := NewAdminUserController(mock, nil, sender)
	return ctrl, mock, sender
}

func TestAdminUserController_ListUsers(t *testing.T) {
	ctrl, _, _ := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "user-1", Name: "Alice", Email: "alice@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com", IsAdmin: false},
	})
	users, err := ctrl.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(users) != 2 {
		t.Fatalf("expected 2 users, got %d", len(users))
	}

	if users[0].Name != "Alice" {
		t.Errorf("expected first user to be 'Alice', got %q", users[0].Name)
	}
}

func TestAdminUserController_ListUsers_Empty(t *testing.T) {
	ctrl, _, _ := newAdminUserControllerWithUsers(t, nil)

	users, err := ctrl.ListUsers(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(users) != 0 {
		t.Fatalf("expected 0 users, got %d", len(users))
	}
}

// --- SetAdmin ---------------------------------------------------

func TestAdminUserController_SetAdmin_Grants(t *testing.T) {
	ctrl, mock, _ := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com", IsAdmin: false},
	})

	if err := ctrl.SetAdmin(context.Background(), "admin-1", "user-2", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := mock.GetUserByID(context.Background(), "user-2")
	if user == nil || !user.IsAdmin {
		t.Errorf("expected user-2 to be admin, got %+v", user)
	}
}

func TestAdminUserController_SetAdmin_Revokes(t *testing.T) {
	ctrl, mock, _ := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "admin-2", Name: "Other", Email: "other@example.com", IsAdmin: true},
	})

	if err := ctrl.SetAdmin(context.Background(), "admin-1", "admin-2", false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	user, _ := mock.GetUserByID(context.Background(), "admin-2")
	if user == nil || user.IsAdmin {
		t.Errorf("expected admin-2 to no longer be admin, got %+v", user)
	}
}

func TestAdminUserController_SetAdmin_UserNotFound(t *testing.T) {
	ctrl, _, _ := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
	})

	err := ctrl.SetAdmin(context.Background(), "admin-1", "ghost", true)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}
}

func TestAdminUserController_SetAdmin_SelfDemotionBlocked(t *testing.T) {
	ctrl, mock, _ := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
	})

	err := ctrl.SetAdmin(context.Background(), "admin-1", "admin-1", false)
	if !errors.Is(err, ErrCannotDemoteSelf) {
		t.Errorf("expected ErrCannotDemoteSelf, got %v", err)
	}

	// The repo must be untouched — the guard runs before the write.
	user, _ := mock.GetUserByID(context.Background(), "admin-1")
	if user == nil || !user.IsAdmin {
		t.Errorf("expected admin-1 to keep admin status, got %+v", user)
	}
}

func TestAdminUserController_SetAdmin_SelfPromotionAllowed(t *testing.T) {
	// Granting admin to yourself is a no-op write but must not be
	// treated as self-demotion — only the revoke path is blocked.
	ctrl, _, _ := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "user-1", Name: "User", Email: "user@example.com", IsAdmin: false},
	})

	if err := ctrl.SetAdmin(context.Background(), "user-1", "user-1", true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// --- SendPasswordReset -------------------------------------------

func TestAdminUserController_SendPasswordReset_SendsToTargetUser(t *testing.T) {
	target := models.User{ID: "user-2", Name: "Bob", Email: "bob@example.com"}
	ctrl, _, sender := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		target,
	})

	if err := ctrl.SendPasswordReset(context.Background(), "user-2"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.calls != 1 {
		t.Fatalf("expected 1 send, got %d", sender.calls)
	}
	if sender.user == nil || sender.user.ID != "user-2" || sender.user.Email != "bob@example.com" {
		t.Errorf("expected send to target user-2, got %+v", sender.user)
	}
}

func TestAdminUserController_SendPasswordReset_UserNotFound(t *testing.T) {
	ctrl, _, sender := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
	})

	err := ctrl.SendPasswordReset(context.Background(), "ghost")
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("expected ErrUserNotFound, got %v", err)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.calls != 0 {
		t.Errorf("sender called %d times for unknown user, want 0", sender.calls)
	}
}

func TestAdminUserController_SendPasswordReset_SMTPFailure(t *testing.T) {
	ctrl, _, sender := newAdminUserControllerWithUsers(t, []models.User{
		{ID: "user-1", Name: "Bob", Email: "bob@example.com", IsAdmin: false},
	})
	sender.mu.Lock()
	sender.sendErr = errors.New("smtp down")
	sender.mu.Unlock()

	err := ctrl.SendPasswordReset(context.Background(), "user-1")
	if err == nil {
		t.Fatal("expected error when the send fails")
	}
	if errors.Is(err, ErrUserNotFound) {
		t.Errorf("send failure must not surface as ErrUserNotFound, got %v", err)
	}
}
