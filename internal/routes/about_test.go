package routes

import (
	"net/http"
	"strings"
	"testing"
)

// TestAbout_AnonymousGetsPage confirms /about is publicly
// reachable: an unauthenticated request must render the page
// (200) rather than bouncing to /login. Exercises the full
// middleware + router stack so the isPublicRoute entry and the
// route-table registration are both covered.
func TestAbout_AnonymousGetsPage(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := newRecorder()
	e.ServeHTTP(rec, req("GET", "/about"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Built for people who train.",
		"Frequently asked questions",
		`class="accordion"`,
		"data-multiple",
		"What is your AI policy?",
		"Is Hylete really free?",
		"How does it make money?",
		"Why iOS only?",
		"How safe is my data?",
		"Data Export page",
		"support@hyleteapp.com",
		`href="/about"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("about page missing %q", want)
		}
	}
}

// TestAbout_IsPublicRoute pins the middleware exemption so a
// future edit to the public-route list can't silently gate the
// page behind login.
func TestAbout_IsPublicRoute(t *testing.T) {
	if !isPublicRoute("/about") {
		t.Error("isPublicRoute(\"/about\") = false, want true")
	}
}
