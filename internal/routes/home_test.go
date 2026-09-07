package routes

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"hylete/internal/utils"
)

// TestHome_AnonymousGetsLandingPage confirms the root path serves
// the public marketing page to visitors without a session: a 200
// with hero copy and Login/Register calls to action. This is the
// route that used to be the (auth-gated) dashboard, so the test
// also pins the public accessibility — an unauthenticated request
// must not be bounced to /login.
func TestHome_AnonymousGetsLandingPage(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := newRecorder()
	e.ServeHTTP(rec, req("GET", "/"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Log every set", `href="/register"`, `href="/login"`} {
		if !strings.Contains(body, want) {
			t.Errorf("landing page missing %q", want)
		}
	}
	if strings.Contains(body, "7 Day History") {
		t.Error("landing page must not render dashboard content")
	}
}

// TestHome_AuthenticatedRedirectsToDashboard confirms signed-in
// users hitting "/" are forwarded to the app under /dashboard
// instead of being shown the sales pitch. "/" is a public route,
// so the auth middleware never populates claims here — the Home
// handler's own token check is what produces this redirect.
func TestHome_AuthenticatedRedirectsToDashboard(t *testing.T) {
	h, _, _, e := setupHandler(t)

	token, err := h.jwtService.GenerateToken("user-1", "test@example.com", "Test User", false)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	r := req("GET", "/")
	r.AddCookie(&http.Cookie{Name: utils.CookieName, Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != dashboardPath {
		t.Errorf("expected redirect to %s, got %q", dashboardPath, loc)
	}
}

// TestDashboardRoute_RequiresAuth guards the route move itself:
// the web app moved from "/" to /dashboard, so an unauthenticated
// GET /dashboard must still be redirected to /login by the auth
// middleware rather than rendering anything.
func TestDashboardRoute_RequiresAuth(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := newRecorder()
	e.ServeHTTP(rec, req("GET", "/dashboard"))
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusSeeOther)
	}
	if loc := rec.Header().Get("Location"); loc != "/login" {
		t.Errorf("expected redirect to /login, got %q", loc)
	}
}

// TestDashboardRoute_RendersForAuthenticatedUser exercises GET
// /dashboard through the full middleware + router stack (the other
// dashboard tests call the handler directly), proving the route
// table maps /dashboard to the Dashboard handler.
func TestDashboardRoute_RendersForAuthenticatedUser(t *testing.T) {
	h, _, _, e := setupHandler(t)

	token, err := h.jwtService.GenerateToken("user-1", "test@example.com", "Test User", false)
	if err != nil {
		t.Fatalf("GenerateToken failed: %v", err)
	}

	r := req("GET", "/dashboard")
	r.AddCookie(&http.Cookie{Name: utils.CookieName, Value: token})
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, r)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Hylete") || !strings.Contains(body, "Logout") {
		t.Error("expected authenticated app chrome on /dashboard")
	}
}
