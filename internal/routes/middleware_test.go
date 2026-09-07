package routes

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/labstack/echo/v4"

	"hylete/internal/models"
	"hylete/internal/utils"
)

func TestIsPublicRoute(t *testing.T) {
	// The static-asset cases hit the real public/ directory via
	// http.Dir, so the test must run from the project root for
	// the lookups to resolve.
	chdirToProjectRoot(t)

	tests := []struct {
		path     string
		expected bool
	}{
		{"/login", true},
		{"/register", true},
		{"/api/v1/auth/password-reset/request", true},
		// Static assets: public because they resolve to files in
		// public/, which anonymous visitors' pages load.
		{"/css/styles.css", true},
		{"/icons/favicon-96x96.png", true},
		{"/img/login.jpg", true},
		{"/js/basecoat.js", true},
		{"/manifest.json", true},
		{"/favicon.ico", true},
		// Not a file in public/ (the service worker was removed),
		// so it falls through to the protected-route default.
		{"/sw.js", false},
		{"/", true},
		{"/profile", false},
		{"/exercises/Squat", false},
		{"/export/weight", false},
		{"/admin/users", false},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			if got := isPublicRoute(tt.path); got != tt.expected {
				t.Errorf("isPublicRoute(%q) = %v, want %v", tt.path, got, tt.expected)
			}
		})
	}
}

func TestRedirectToLogin_NonHTMX(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := redirectToLogin(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected status %d, got %d", http.StatusSeeOther, rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestRedirectToLogin_HTMX(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := redirectToLogin(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
	if hx := rec.Header().Get("HX-Redirect"); hx != "/login" {
		t.Fatalf("expected HX-Redirect /login, got %q", hx)
	}
}

func TestAuthMiddleware_PublicRoute(t *testing.T) {
	jwtService := utils.NewJWTService("test-secret")
	middleware := AuthMiddleware(jwtService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/login", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	next := func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	}

	err := middleware(next)(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected next handler to be called for public route")
	}
}

func TestAuthMiddleware_MissingCookie(t *testing.T) {
	jwtService := utils.NewJWTService("test-secret")
	middleware := AuthMiddleware(jwtService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	next := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	err := middleware(next)(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %q", loc)
	}
}

func TestAuthMiddleware_InvalidToken(t *testing.T) {
	jwtService := utils.NewJWTService("test-secret")
	middleware := AuthMiddleware(jwtService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	req.AddCookie(&http.Cookie{
		Name:  utils.CookieName,
		Value: "invalid-token",
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	next := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	err := middleware(next)(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusSeeOther {
		t.Fatalf("expected redirect status, got %d", rec.Code)
	}
}

func TestAuthMiddleware_ValidToken(t *testing.T) {
	jwtService := utils.NewJWTService("test-secret")
	token, err := jwtService.GenerateToken("user-42", "alice@example.com", "Alice", false)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	middleware := AuthMiddleware(jwtService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	req.AddCookie(&http.Cookie{
		Name:  utils.CookieName,
		Value: token,
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	next := func(c echo.Context) error {
		called = true

		// Verify claims were injected
		claims := GetClaims(c)
		if claims == nil {
			t.Fatal("expected claims to be set in context")
		}
	if claims.UserID != "user-42" {
		t.Fatalf("expected user_id 'user-42', got %q", claims.UserID)
	}
		if claims.Email != "alice@example.com" {
			t.Fatalf("expected email 'alice@example.com', got %q", claims.Email)
		}
		if claims.Name != "Alice" {
			t.Fatalf("expected name 'Alice', got %q", claims.Name)
		}
		if claims.IsAdmin {
			t.Fatal("expected is_admin to be false")
		}

		return c.String(http.StatusOK, "ok")
	}

	err = middleware(next)(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected next handler to be called")
	}
}

func TestGetClaims_Missing(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	claims := GetClaims(c)
	if claims != nil {
		t.Fatalf("expected nil claims, got %+v", claims)
	}
}

func TestGetClaims_Present(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	expected := &utils.Claims{
		UserID:  "user-7",
		Email:   "bob@example.com",
		Name:    "Bob",
		IsAdmin: true,
	}
	c.Set(authContextKey, expected)

	claims := GetClaims(c)
	if claims == nil {
		t.Fatal("expected claims, got nil")
	}
	if claims.UserID != expected.UserID {
		t.Fatalf("expected user_id %q, got %q", expected.UserID, claims.UserID)
	}
	if claims.Email != expected.Email {
		t.Fatalf("expected email %q, got %q", expected.Email, claims.Email)
	}
	if claims.IsAdmin != expected.IsAdmin {
		t.Fatalf("expected is_admin %v, got %v", expected.IsAdmin, claims.IsAdmin)
	}
}

func TestAuthMiddleware_ValidToken_HTMX(t *testing.T) {
	jwtService := utils.NewJWTService("test-secret")
	token, err := jwtService.GenerateToken("user-1", "test@example.com", "Test", false)
	if err != nil {
		t.Fatalf("failed to generate token: %v", err)
	}

	middleware := AuthMiddleware(jwtService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	req.Header.Set("HX-Request", "true")
	req.AddCookie(&http.Cookie{
		Name:  utils.CookieName,
		Value: token,
	})
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	next := func(c echo.Context) error {
		called = true
		return c.String(http.StatusOK, "ok")
	}

	err = middleware(next)(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Fatal("expected next handler to be called for valid HTMX request")
	}
}

func TestAuthMiddleware_MissingCookie_HTMX(t *testing.T) {
	jwtService := utils.NewJWTService("test-secret")
	middleware := AuthMiddleware(jwtService)

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/exercise-entries", nil)
	req.Header.Set("HX-Request", "true")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	next := func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	}

	err := middleware(next)(c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d for HTMX, got %d", http.StatusUnauthorized, rec.Code)
	}
	if hx := rec.Header().Get("HX-Redirect"); hx != "/login" {
		t.Fatalf("expected HX-Redirect /login, got %q", hx)
	}
}

// countingUserRepo wraps a base UserRepo (nil-safe) and counts how many
// times GetUserByID is invoked. Used by TestGetUser_CachedOnSecondCall
// to assert the request-scoped cache works.
type countingUserRepo struct {
	base    models.UserRepo
	getCalls int32
	err     error
}

func (r *countingUserRepo) CreateUser(user *models.User) error { return r.base.CreateUser(user) }
func (r *countingUserRepo) GetUserByEmail(email string) (*models.User, error) {
	return r.base.GetUserByEmail(email)
}
func (r *countingUserRepo) GetUserByID(id string) (*models.User, error) {
	atomic.AddInt32(&r.getCalls, 1)
	if r.err != nil {
		return nil, r.err
	}
	return r.base.GetUserByID(id)
}
func (r *countingUserRepo) UpdateUser(user *models.User) error    { return r.base.UpdateUser(user) }
func (r *countingUserRepo) UpdateUserPassword(id, hash string) error {
	return r.base.UpdateUserPassword(id, hash)
}
func (r *countingUserRepo) UpdateUserReminder(userID string, prefs models.ReminderPreferences) error {
	return r.base.UpdateUserReminder(userID, prefs)
}

// TestGetUser_NoClaims asserts that GetUser returns nil when no auth
// claims are present in the context (e.g. on a public route or in a
// test that doesn't set them up).
func TestGetUser_NoClaims(t *testing.T) {
	h, _, _, e := setupHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if u := h.GetUser(c); u != nil {
		t.Fatalf("expected nil user when no claims, got %+v", u)
	}
}

// TestGetUser_DBError asserts that GetUser swallows DB errors (logs
// them) and returns nil rather than propagating. The caller is
// expected to handle a nil user, e.g. by falling back to the default
// "kg" unit.
func TestGetUser_DBError(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	mockUser.users = []models.User{
		{ID: "u1", Name: "Alice", Email: "a@b.c", PasswordHash: "x", WeightUnit: "kg"},
	}
	counting := &countingUserRepo{base: mockUser, err: errors.New("simulated db error")}
	h.userRepo = counting

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "u1", "a@b.c", "Alice", false)

	if u := h.GetUser(c); u != nil {
		t.Fatalf("expected nil user on db error, got %+v", u)
	}
	if got := atomic.LoadInt32(&counting.getCalls); got != 1 {
		t.Errorf("expected exactly 1 GetUserByID call, got %d", got)
	}
}

// TestGetUser_CachedOnSecondCall asserts that two GetUser calls in the
// same request only trigger one DB read. The second call should be
// served from the Echo context cache.
func TestGetUser_CachedOnSecondCall(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	mockUser.users = []models.User{
		{ID: "u1", Name: "Alice", Email: "a@b.c", PasswordHash: "x", WeightUnit: "lbs"},
	}
	counting := &countingUserRepo{base: mockUser}
	h.userRepo = counting

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "u1", "a@b.c", "Alice", false)

	u1 := h.GetUser(c)
	u2 := h.GetUser(c)
	if u1 == nil || u2 == nil {
		t.Fatal("expected both GetUser calls to return a non-nil user")
	}
	if u1 != u2 {
		t.Error("expected cached user pointer identity between calls")
	}
	if got := atomic.LoadInt32(&counting.getCalls); got != 1 {
		t.Errorf("expected exactly 1 GetUserByID call (second should be cached), got %d", got)
	}
	if u1.WeightUnitDisplay() != "lbs" {
		t.Errorf("expected cached user to expose lbs unit, got %q", u1.WeightUnitDisplay())
	}
}
