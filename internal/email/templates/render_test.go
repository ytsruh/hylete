package emailtmpl

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"
)

// testBaseURL is the base URL the render tests thread into
// every Render call. Picked once here so the test bodies
// stay short and the URL format is consistent across the
// suite. Distinct from the value in service tests so a
// failure caused by accidentally hard-coding the production
// URL (e.g. an old `PublicBaseURL` constant) would show up
// in both packages.
const testBaseURL = "https://hylete.test.local"

// TestRenderWelcome_ContainsEssentials guards the basic shape
// of the welcome email. It must contain the user's name
// (rendered into both the HTML and the text), the dashboard
// link, and the Hylete wordmark.
func TestRenderWelcome_ContainsEssentials(t *testing.T) {
	html, text := RenderWelcome("Alice", testBaseURL)

	for _, want := range []string{
		"Alice",
		"Welcome to Hylete",
		testBaseURL + "/",
		"Hylete",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
	}

	for _, want := range []string{
		"Alice",
		"Welcome to Hylete",
		testBaseURL + "/",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q", want)
		}
	}
}

func TestRenderWelcome_UsesProvidedBaseURL(t *testing.T) {
	// A staging deployment must send email that points at
	// itself, not at production. Render with two different
	// base URLs and assert each one ends up in the body.
	cases := []string{
		"https://www.hyleteapp.com",
		"https://staging.hylete.example",
		"http://localhost:8080",
	}
	for _, baseURL := range cases {
		t.Run(baseURL, func(t *testing.T) {
			html, text := RenderWelcome("Alice", baseURL)
			// The baseURL must appear in the dashboard
			// link (HTML + text) and the footer link.
			dashboardLink := baseURL + "/dashboard"
			if !strings.Contains(html, dashboardLink) {
				t.Errorf("HTML missing dashboard link %q", dashboardLink)
			}
			if !strings.Contains(text, dashboardLink) {
				t.Errorf("text missing dashboard link %q", dashboardLink)
			}
			if !strings.Contains(html, baseURL) {
				t.Errorf("HTML missing baseURL %q (footer)", baseURL)
			}
		})
	}
}

func TestRenderWelcome_NoWebFootprint(t *testing.T) {
	// A few "the web app should not leak into the email"
	// guards. The web UI uses htmx, Tailwind utility classes,
	// and the dark-mode `dark` class on <html>; none of those
	// belong in an email. If any of these assertions break,
	// the design boundary between the web views and the email
	// views has been crossed.
	html, _ := RenderWelcome("Alice", testBaseURL)
	htmlLower := strings.ToLower(html)

	for _, banned := range []string{
		"hx-",      // htmx attributes
		"class=\"", // Tailwind/Basecoat utility classes
		"dark",     // dark-mode class
		"<script",  // any JavaScript
	} {
		if strings.Contains(htmlLower, banned) {
			t.Errorf("email contains forbidden web-only token %q", banned)
		}
	}
}

func TestRenderPasswordReset_ContainsEssentials(t *testing.T) {
	const rawToken = "abc123"
	html, text := RenderPasswordReset("Bob", rawToken, testBaseURL, time.Hour)

	// The user name goes in the greeting; the URL is the
	// only secret material and must appear in both bodies.
	for _, want := range []string{
		"Bob",
		"/reset?token=abc123",
		"1 hour",
		"Hylete",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML missing %q", want)
		}
		if !strings.Contains(text, want) {
			t.Errorf("text missing %q", want)
		}
	}

	// The full URL must use the configured baseURL.
	wantURL := testBaseURL + "/reset?token=abc123"
	if !strings.Contains(html, wantURL) {
		t.Errorf("HTML missing reset URL %q", wantURL)
	}
	if !strings.Contains(text, wantURL) {
		t.Errorf("text missing reset URL %q", wantURL)
	}
}

func TestRenderPasswordReset_HumanizesCommonTTLs(t *testing.T) {
	// The TTL phrase is user-facing copy. Lock the four
	// cases the app actually uses, so a future refactor
	// can't silently change "1 hour" to "1h0m0s".
	cases := []struct {
		ttl  time.Duration
		want string
	}{
		{time.Hour, "1 hour"},
		{2 * time.Hour, "2 hours"},
		{24 * time.Hour, "24 hours"},
		{30 * time.Minute, "30 minutes"},
	}
	for _, tc := range cases {
		t.Run(tc.ttl.String(), func(t *testing.T) {
			if got := humanizeTTL(tc.ttl); got != tc.want {
				t.Errorf("humanizeTTL(%v) = %q, want %q", tc.ttl, got, tc.want)
			}
		})
	}
}

func TestRenderPasswordReset_HasSafeLinkAttributes(t *testing.T) {
	// The reset link is the only thing a recipient can click.
	// It must open in a new tab (target=_blank) and have the
	// rel="noopener noreferrer" combination so a malicious
	// reset page cannot navigate the opener window. Lock the
	// attributes so a future refactor can't drop them
	// silently.
	html, _ := RenderPasswordReset("Bob", "abc123", testBaseURL, time.Hour)
	if !strings.Contains(html, `target="_blank"`) {
		t.Error("reset link missing target=_blank")
	}
	if !strings.Contains(html, "noopener") {
		t.Error("reset link missing rel=noopener")
	}
	if !strings.Contains(html, "noreferrer") {
		t.Error("reset link missing rel=noreferrer")
	}
	// And the templ SafeURL wrapper should have rejected any
	// javascript: URLs at compile time. Belt-and-braces:
	// confirm no javascript: pseudo-protocol leaked into the
	// output.
	if strings.Contains(strings.ToLower(html), "javascript:") {
		t.Error("email contains javascript: link")
	}
}

func TestRenderLayout_IncludesPreheader(t *testing.T) {
	// The preheader is a hidden div at the very top of the
	// body; it must be present so Gmail's inbox preview
	// shows a useful sentence. The CSS that hides it is
	// inline (display:none) so no <style> block is required.
	html, _ := RenderWelcome("Alice", testBaseURL)
	// Templ HTML-escapes the apostrophe in the preheader
	// ("let's" → "let&#39;s"). Accept either form so the
	// test does not break if templ changes its escape
	// strategy in the future.
	preheader := "let&#39;s get you started"
	if !strings.Contains(html, preheader) && !strings.Contains(html, "let's get you started") {
		t.Errorf("preheader text not found in welcome email: %q", html)
	}
	// And the wrapper around the preheader must hide it
	// (display:none).
	if matched := regexp.MustCompile(`display:\s*none`).FindStringIndex(html); matched == nil {
		t.Error("preheader div missing display:none hiding rule")
	}
}

func TestCurrentYear_IsReasonable(t *testing.T) {
	// The current year is rendered into the email's footer.
	// The test guards against a clock-rollover bug (e.g. a
	// future refactor that accidentally returns 0) by
	// asserting the year is in the 2020-2099 range.
	y := CurrentYear()
	if y < 2020 || y > 2099 {
		t.Errorf("CurrentYear = %d, outside 2020-2099", y)
	}
}

func TestRenderWelcome_DirectRenderEqualsViaRenderFn(t *testing.T) {
	// RenderWelcome should be a thin wrapper around
	// WelcomeEmail.Render(...). Render the templ directly
	// and compare to confirm no transformation (other than
	// the bytes.Buffer target) happens in between.
	var buf directBuffer
	if err := WelcomeEmail("Alice", testBaseURL).Render(context.Background(), &buf); err != nil {
		t.Fatalf("direct render: %v", err)
	}
	html, _ := RenderWelcome("Alice", testBaseURL)
	if buf.String() != html {
		t.Errorf("RenderWelcome output differs from direct templ render.\ndirect: %q\nvia:    %q",
			buf.String(), html)
	}
}

func TestRenderWeightReminder_CadenceSpecificCopy(t *testing.T) {
	// The email must be cadence-specific: a daily subscriber
	// gets "Today's weigh-in", a weekly subscriber gets the
	// day-agnostic weekly copy (no email text names a day,
	// since the reminder fires on whichever weekday the user
	// picked). The button label also changes ("Log today's
	// weight" vs "Log this week's weight") so the
	// call-to-action matches the cadence the user picked.
	cases := []struct {
		cadence     models.ReminderFrequency
		wantInHTML  []string
		wantInText  []string
	}{
		{
			cadence:    models.ReminderDaily,
			wantInHTML: []string{"Today&#39;s weigh-in", "Log today&#39;s weight"},
			wantInText: []string{"Today's weigh-in", "today's entry"},
		},
		{
			cadence:    models.ReminderWeekly,
			wantInHTML: []string{"Weekly weigh-in", "Log this week&#39;s weight"},
			wantInText: []string{"this week's weight", "this week's reading"},
		},
		{
			cadence:    models.ReminderBiweekly,
			wantInHTML: []string{"Time to log", "Log today&#39;s weight"},
			wantInText: []string{"Time to log this week's weight", "today's entry"},
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.cadence), func(t *testing.T) {
			html, text := RenderWeightReminder("Alice", testBaseURL, tc.cadence)
			for _, want := range tc.wantInHTML {
				if !strings.Contains(html, want) {
					t.Errorf("HTML missing %q for cadence=%q: %q", want, tc.cadence, html)
				}
			}
			for _, want := range tc.wantInText {
				if !strings.Contains(text, want) {
					t.Errorf("text missing %q for cadence=%q: %q", want, tc.cadence, text)
				}
			}
		})
	}
}

// directBuffer is a tiny wrapper around strings.Builder that
// satisfies io.Writer, so we can render a templ.Component into
// it without importing bytes in this test file.
type directBuffer struct{ strings.Builder }

func (b *directBuffer) Write(p []byte) (int, error) { return b.Builder.Write(p) }
