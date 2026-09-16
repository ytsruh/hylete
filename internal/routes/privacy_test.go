package routes

import (
	"net/http"
	"strings"
	"testing"
)

// TestPrivacy_AnonymousGetsPage confirms /privacy is publicly
// reachable: an unauthenticated request must render the policy
// (200) rather than bouncing to /login. Exercises the full
// middleware + router stack so the isPublicRoute entry and the
// route-table registration are both covered.
func TestPrivacy_AnonymousGetsPage(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := newRecorder()
	e.ServeHTTP(rec, req("GET", "/privacy"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Privacy, in plain English.",
		"AI Privacy",
		"entirely opt-in",
		"support@hyleteapp.com",
		"third-party cookies",
		`href="/privacy"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("privacy page missing %q", want)
		}
	}
}

// TestPrivacy_IsPublicRoute pins the middleware exemption so a
// future edit to the public-route list can't silently gate the
// page behind login.
func TestPrivacy_IsPublicRoute(t *testing.T) {
	if !isPublicRoute("/privacy") {
		t.Error("isPublicRoute(\"/privacy\") = false, want true")
	}
}
