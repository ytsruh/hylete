package routes

import (
	"net/http"
	"strings"
	"testing"
)

// TestFeatures_AnonymousGetsFeaturesPage confirms /features serves
// the public marketing page to visitors without a session: a 200
// with stable feature copy plus the Beta section (Coach, HealthKit,
// Workouts). Like /showcase, /privacy and /about it must not bounce
// unauthenticated visitors to /login.
func TestFeatures_AnonymousGetsFeaturesPage(t *testing.T) {
	_, _, _, e := setupHandler(t)

	rec := newRecorder()
	e.ServeHTTP(rec, req("GET", "/features"))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Everything you need",
		"Log every set",
		"Beta",
		"Coach",
		"HealthKit",
		"Workouts",
		`href="/register"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("features page missing %q", want)
		}
	}
}
