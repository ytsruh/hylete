package routes

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/utils"
)

// mockAdminResetSender is a PasswordResetSender fake for the admin
// user action tests. It records the target user of each send and can
// be made to fail so the SMTP error path can be exercised without a
// real SMTP endpoint.
type mockAdminResetSender struct {
	mu       sync.Mutex
	calls    int
	lastUser *models.User
	sendErr  error
}

func (m *mockAdminResetSender) SendPasswordReset(_ context.Context, _ models.AuthTokenRepo, user *models.User) (string, error) {
	m.mu.Lock()
	m.calls++
	u := *user
	m.lastUser = &u
	sendErr := m.sendErr
	m.mu.Unlock()
	if sendErr != nil {
		return "", sendErr
	}
	return "raw-token", nil
}

// setupAdminUserHandler builds a Handler whose admin user controller
// is wired to an in-memory AdminUserRepo seeded with users and the
// supplied reset sender. Routes are registered for consistency with
// the other setups, but the tests invoke the handlers directly so
// setAuthContext can supply the admin claims.
func setupAdminUserHandler(t *testing.T, users []models.User, sender *mockAdminResetSender) (*Handler, *mockAdminUserRepository, *echo.Echo) {
	t.Helper()
	e := echo.New()
	mockUser := newMockUserRepository()
	mockAdminUser := newMockAdminUserRepository()
	mockAdminUser.users = users
	jwtService := utils.NewJWTService("test-secret")
	proc, upl := newFakeImagePipeline()
	h := NewHandler(
		controllers.NewAuthController(mockUser, jwtService, nil),
		controllers.NewAuthRecoveryController(mockUser, newMockAuthTokenRepo(), nil),
		controllers.NewExerciseEntryController(newMockRepository()),
		controllers.NewAdminController(newMockRepository()),
		controllers.NewAdminUserController(mockAdminUser, newMockAuthTokenRepo(), sender),
		controllers.NewFeedbackController(newMockFeedbackRepository()),
		controllers.NewWeightController(newMockWeightRepository(), nil),
		controllers.NewGoalsController(newMockGoalRepository()),
		mockUser, jwtService, utils.NewValidator(),
		proc, upl, DefaultExerciseImageConfig,
	)
	h.RegisterRoutes(e)
	return h, mockAdminUser, e
}

// adminUserActionPost invokes an admin user action handler directly,
// simulating a form-urlencoded POST to /admin/users/:id/... with the
// acting admin's claims. htmx toggles the HX-Request header the
// handlers branch on.
func adminUserActionPost(t *testing.T, h *Handler, e *echo.Echo, handler func(echo.Context) error, actingUserID, targetUserID string, form url.Values, htmx bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/admin/users/"+targetUserID, strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	if htmx {
		req.Header.Set("HX-Request", "true")
	}
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.SetParamNames("id")
	c.SetParamValues(targetUserID)
	setAuthContext(c, actingUserID, "admin@example.com", "Admin", true)
	if err := handler(c); err != nil {
		t.Fatalf("admin user action handler returned error: %v", err)
	}
	return rec
}

// --- POST /admin/users/:id/admin ----------------------------------

func TestAdminSetUserAdmin_Grant_HTMX(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, mockAdminUser, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com"},
	}, sender)

	form := url.Values{}
	form.Set("is_admin", "true")
	rec := adminUserActionPost(t, h, e, h.AdminSetUserAdmin, "admin-1", "user-2", form, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("HX-Trigger"); !strings.Contains(got, "triggerRedirect") {
		t.Errorf("expected HX-Trigger triggerRedirect header, got %q", got)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Admin access granted") {
		t.Errorf("expected success toast in body, got %q", body)
	}

	user, _ := mockAdminUser.GetUserByID(context.Background(), "user-2")
	if user == nil || !user.IsAdmin {
		t.Errorf("expected user-2 to be admin after grant, got %+v", user)
	}
}

func TestAdminSetUserAdmin_Revoke_HTMX(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, mockAdminUser, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "admin-2", Name: "Other", Email: "other@example.com", IsAdmin: true},
	}, sender)

	form := url.Values{}
	form.Set("is_admin", "false")
	rec := adminUserActionPost(t, h, e, h.AdminSetUserAdmin, "admin-1", "admin-2", form, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Admin access removed") {
		t.Errorf("expected success toast in body, got %q", body)
	}

	user, _ := mockAdminUser.GetUserByID(context.Background(), "admin-2")
	if user == nil || user.IsAdmin {
		t.Errorf("expected admin-2 to lose admin status, got %+v", user)
	}
}

func TestAdminSetUserAdmin_SelfDemotionBlocked(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, mockAdminUser, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
	}, sender)

	form := url.Values{}
	form.Set("is_admin", "false")
	rec := adminUserActionPost(t, h, e, h.AdminSetUserAdmin, "admin-1", "admin-1", form, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with error toast, got %d", rec.Code)
	}
	// templ HTML-escapes the apostrophe in the toast copy, so assert
	// on the apostrophe-free tail of the message.
	if body := rec.Body.String(); !strings.Contains(body, "remove your own admin access") {
		t.Errorf("expected self-demotion error toast, got %q", body)
	}

	// The acting admin's row must be untouched.
	user, _ := mockAdminUser.GetUserByID(context.Background(), "admin-1")
	if user == nil || !user.IsAdmin {
		t.Errorf("expected admin-1 to keep admin status, got %+v", user)
	}
}

func TestAdminSetUserAdmin_UserNotFound(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, _, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
	}, sender)

	form := url.Values{}
	form.Set("is_admin", "true")
	rec := adminUserActionPost(t, h, e, h.AdminSetUserAdmin, "admin-1", "ghost", form, true)

	if body := rec.Body.String(); !strings.Contains(body, "User not found") {
		t.Errorf("expected user-not-found toast, got %q", body)
	}
}

func TestAdminSetUserAdmin_PlainPostRedirects(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, _, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com"},
	}, sender)

	form := url.Values{}
	form.Set("is_admin", "true")
	rec := adminUserActionPost(t, h, e, h.AdminSetUserAdmin, "admin-1", "user-2", form, false)

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected 303 redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/users" {
		t.Errorf("expected redirect to /admin/users, got %q", loc)
	}
}

// --- POST /admin/users/:id/send-password-reset --------------------

func TestAdminSendUserPasswordReset_HTMX(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, _, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com"},
	}, sender)

	rec := adminUserActionPost(t, h, e, h.AdminSendUserPasswordReset, "admin-1", "user-2", url.Values{}, true)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if body := rec.Body.String(); !strings.Contains(body, "Password reset email sent") {
		t.Errorf("expected success toast in body, got %q", body)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.calls != 1 {
		t.Fatalf("expected 1 send, got %d", sender.calls)
	}
	if sender.lastUser == nil || sender.lastUser.ID != "user-2" || sender.lastUser.Email != "bob@example.com" {
		t.Errorf("expected send to user-2, got %+v", sender.lastUser)
	}
}

func TestAdminSendUserPasswordReset_UserNotFound(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, _, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
	}, sender)

	rec := adminUserActionPost(t, h, e, h.AdminSendUserPasswordReset, "admin-1", "ghost", url.Values{}, true)

	if body := rec.Body.String(); !strings.Contains(body, "User not found") {
		t.Errorf("expected user-not-found toast, got %q", body)
	}

	sender.mu.Lock()
	defer sender.mu.Unlock()
	if sender.calls != 0 {
		t.Errorf("sender called %d times for unknown user, want 0", sender.calls)
	}
}

func TestAdminSendUserPasswordReset_SMTPFailure(t *testing.T) {
	sender := &mockAdminResetSender{sendErr: errors.New("smtp down")}
	h, _, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com"},
	}, sender)

	rec := adminUserActionPost(t, h, e, h.AdminSendUserPasswordReset, "admin-1", "user-2", url.Values{}, true)

	if body := rec.Body.String(); !strings.Contains(body, "Failed to send password reset email") {
		t.Errorf("expected SMTP failure toast, got %q", body)
	}
}

// --- GET /admin/users (list renders the new actions) --------------

func TestAdminListUsers_RendersActionButtons(t *testing.T) {
	sender := &mockAdminResetSender{}
	h, _, e := setupAdminUserHandler(t, []models.User{
		{ID: "admin-1", Name: "Admin", Email: "admin@example.com", IsAdmin: true},
		{ID: "user-2", Name: "Bob", Email: "bob@example.com"},
	}, sender)

	req := httptest.NewRequest(http.MethodGet, "/admin/users", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "admin-1", "admin@example.com", "Admin", true)
	if err := h.AdminListUsers(c); err != nil {
		t.Fatalf("admin list users handler returned error: %v", err)
	}

	body := rec.Body.String()
	// Non-admin row gets Make Admin + Send Reset.
	if !strings.Contains(body, "/admin/users/user-2/admin") {
		t.Errorf("expected toggle endpoint for user-2 in body")
	}
	if !strings.Contains(body, "/admin/users/user-2/send-password-reset") {
		t.Errorf("expected reset endpoint for user-2 in body")
	}
	if !strings.Contains(body, "Make Admin") {
		t.Errorf("expected Make Admin button in body")
	}
	// The confirm dialog's OK button is destructive-red by default;
	// these workflow actions opt into the brand-orange variant.
	if !strings.Contains(body, `data-confirm-variant="primary"`) {
		t.Errorf("expected data-confirm-variant=primary on the user action buttons")
	}
	// templ escapes the inner quotes of the hx-vals JSON attribute.
	if !strings.Contains(body, "is_admin") || !strings.Contains(body, "true") {
		t.Errorf("expected is_admin=true hx-vals for user-2, got %q", body)
	}
	// The acting admin's own row must not offer self-demotion.
	if strings.Contains(body, "/admin/users/admin-1/admin") {
		t.Errorf("expected no toggle endpoint for the acting admin's own row")
	}
}
