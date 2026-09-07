// Package emailtmpl holds the email-specific Templ components and
// their render helpers. It is a sibling of the email package
// (internal/email) by Go's directory = package rule; the parent
// package imports this one to render emails.
//
// All HTML in this package is designed for email-client
// compatibility. The rules are stricter than for web pages:
//
//   - Layout uses <table> with role="presentation", cellpadding,
//     cellspacing, and border attributes. CSS flex/grid does not
//     work in Outlook (Word rendering engine) and is unreliable in
//     older mobile clients.
//   - Every visible style is inline (style="..."). <style> blocks
//     are stripped by Gmail and most other webmail clients.
//   - No JavaScript. No htmx. No external CSS or fonts loaded from
//     the network — recipients may have images off by default, and
//     any third-party request also leaks their open to the
//     tracker.
//   - MSO conditional comments wrap the table layout so Outlook
//     falls back to a fixed-width rendering when its CSS engine
//     is not used.
//   - Colors are hard-coded hex values. oklch() (used by the web
//     app) is not supported in most email clients.
//
// The plaintext part of each email is produced by sibling
// Go functions in render.go, not by templ — the plaintext path
// has none of the above constraints and templ would be overkill.
package emailtmpl

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"time"

	"hylete/internal/models"
)

// PasswordResetTTL is how long a freshly-minted password-reset
// token remains valid. 1 hour is the conventional value: long
// enough that a user can find the email, short enough that a
// leaked inbox does not stay exploitable indefinitely. The value
// is enforced server-side by the SQL "expires_at > datetime('now')"
// check in the ConsumeAuthToken query, not by trusting the
// client's clock.
const PasswordResetTTL = time.Hour

// CurrentYear returns the current UTC year. A single helper used
// by every templ component that needs a copyright year. Kept in
// Go (not in a templ file) because templ would force it to
// produce HTML, and "2024" as a string is fine.
func CurrentYear() int {
	return time.Now().UTC().Year()
}

// humanizeTTL turns a duration into a short English phrase used in
// the password-reset email body, e.g. "1 hour", "30 minutes",
// "24 hours". Only the cases the app actually uses are handled
// (1h, 30m, 24h) — any other duration falls back to a generic
// "a short time" so we never accidentally print "1h0m0s" to a
// user. Adding more cases is a one-line change.
func humanizeTTL(ttl time.Duration) string {
	switch {
	case ttl >= 24*time.Hour && ttl%(24*time.Hour) == 0:
		hours := int(ttl / time.Hour)
		return fmt.Sprintf("%d hours", hours)
	case ttl >= time.Hour && ttl%time.Hour == 0:
		hours := int(ttl / time.Hour)
		if hours == 1 {
			return "1 hour"
		}
		return fmt.Sprintf("%d hours", hours)
	case ttl >= time.Minute && ttl%time.Minute == 0:
		minutes := int(ttl / time.Minute)
		if minutes == 1 {
			return "1 minute"
		}
		return fmt.Sprintf("%d minutes", minutes)
	default:
		return "a short time"
	}
}

// RenderWelcome renders the welcome email to its HTML and
// plaintext bodies. The baseURL is the absolute origin the
// email is being sent from (typically the value of the
// PUBLIC_URL env var) and is threaded into the dashboard
// button and the footer's "view on the web" link.
//
// Returns the HTML and text bodies as strings. On a templ
// render error (which is unreachable for a bytes.Buffer
// target but kept as a defensive fallback) the function
// returns the plaintext body and an empty HTML body so the
// caller can still send a usable email.
func RenderWelcome(name, baseURL string) (html, text string) {
	var buf bytes.Buffer
	if err := WelcomeEmail(name, baseURL).Render(context.Background(), &buf); err != nil {
		return "", welcomeText(name, baseURL)
	}
	return buf.String(), welcomeText(name, baseURL)
}

// RenderWeightReminder renders the per-user "log your weight"
// reminder email. The cadence is one of models.ReminderDaily /
// Weekly / Biweekly and is used to pick the email's subject
// line and a short cadence-aware blurb in the body so the user
// gets a "Time to log today's weight" email on a daily cadence
// and a "Weekly weigh-in" email on a weekly cadence. The copy
// is deliberately day-agnostic: a weekly reminder can fire on
// whichever weekday the user picked (and drift for operational
// reasons), so no email copy names a day or a time.
//
// The baseURL is threaded into the dashboard button (which
// points at /weight/new) and the footer's "view on the web"
// link. Same fallback behavior as RenderWelcome: a templ
// render error returns the plaintext body and an empty HTML
// body so the caller can still send a usable email.
func RenderWeightReminder(name, baseURL string, cadence models.ReminderFrequency) (html, text string) {
	subject, header := reminderCadenceCopy(cadence)
	var buf bytes.Buffer
	if err := WeightReminderEmail(name, baseURL, subject, header).Render(context.Background(), &buf); err != nil {
		return "", weightReminderText(name, baseURL, subject, header)
	}
	return buf.String(), weightReminderText(name, baseURL, subject, header)
}

// reminderCadenceCopy returns the (subject, header) pair for
// the reminder email given the user's chosen cadence. The two
// strings are split so the templ component can use the same
// pair (and the plaintext body too) — there's no need to
// recompute the cadence label in three places.
func reminderCadenceCopy(cadence models.ReminderFrequency) (subject, header string) {
	switch cadence {
	case models.ReminderDaily:
		return "Time to log your weight", "Today's weigh-in"
	case models.ReminderWeekly:
		return "Weekly weigh-in reminder", "Time to log this week's weight."
	case models.ReminderBiweekly:
		return "Time to log your weight", "Time to log this week's weight."
	}
	// Off / unknown: should not happen (the orchestrator
	// skips off rows), but fall back to a generic pair so
	// the email still renders.
	return "Time to log your weight", "Time to log your weight."
}

// RenderPasswordReset renders the password-reset email. Takes
// the raw token (not the hashed one) and the baseURL, and
// builds the URL internally — the URL is the only place the
// raw token appears, so keeping the URL building here next to
// the templ components makes the data flow obvious. Same
// fallback behavior as RenderWelcome.
func RenderPasswordReset(name, rawToken, baseURL string, ttl time.Duration) (html, text string) {
	resetURL := buildResetURL(rawToken, baseURL)
	var buf bytes.Buffer
	if err := PasswordResetEmail(name, resetURL, baseURL, ttl).Render(context.Background(), &buf); err != nil {
		return "", passwordResetText(name, resetURL, ttl)
	}
	return buf.String(), passwordResetText(name, resetURL, ttl)
}

// buildResetURL composes the absolute URL the user clicks in the
// email. The token is the only secret material; everything else
// is the public path that the route handlers in
// internal/routes/auth_recovery.go serve.
func buildResetURL(rawToken, baseURL string) string {
	v := url.Values{}
	v.Set("token", rawToken)
	return baseURL + "/reset?" + v.Encode()
}

// welcomeText is the plaintext body of the welcome email. Kept
// in Go (not a .templ file) because there is no layout to inherit
// and the body is short enough that templ would be overkill.
// Lines are wrapped at 78 cols by the recipient's mail client
// (RFC 3676); we keep the source under that limit to avoid
// mid-paragraph wraps that look bad in clients that don't
// reflow.
func welcomeText(name, baseURL string) string {
	return fmt.Sprintf(
		"Hi %s,\n\n"+
			"Welcome to Hylete. Hylete helps you log every set, watch your numbers climb, and keep your training consistent.\n\n"+
			"Head to your dashboard to log your first set:\n%s\n\n"+
			"— The Hylete team\n",
		name, baseURL+"/dashboard",
	)
}

// weightReminderText is the plaintext body of the per-user
// "log your weight" reminder email. Kept in Go (not a .templ file)
// because the body is short and templ would be
// overkill. The dashboard link points at /weight/new so the
// recipient lands directly on the new-entry form. The cadence
// label ("Weekly weigh-in" / "Today's weigh-in" / generic) is
// threaded in from reminderCadenceCopy so the same function
// is the single source of truth for the cadence-specific
// copy across HTML and plaintext.
func weightReminderText(name, baseURL, subject, header string) string {
	body := ""
	switch subject {
	case "Weekly weigh-in reminder":
		body = "Time to log your weight for the week. " +
			"A single entry a week is the most reliable way to spot a trend without making the scale a daily event. " +
			"Tap below to add this week's reading, and feel free to add a photo if you want to see your progress over time:"
	case "Time to log your weight":
		body = "Time to log your weight. " +
			"A quick reading keeps your trend line honest. " +
			"Tap below to add today's entry:"
	}
	_ = header
	return fmt.Sprintf(
		"Hi %s,\n\n"+
			"%s\n\n"+
			"%s\n%s\n\n"+
			"— The Hylete team\n",
		name, header, body, baseURL+"/weight/new",
	)
}

// passwordResetText is the plaintext body of the password-reset
// email. Includes the URL on its own line so the recipient can
// copy-paste it if their mail client does not render HTML.
func passwordResetText(name, resetURL string, ttl time.Duration) string {
	return fmt.Sprintf(
		"Hi %s,\n\n"+
			"We received a request to reset the password on your Hylete account.\n\n"+
			"Open the link below within %s to choose a new password:\n%s\n\n"+
			"If you didn't ask for this, you can safely ignore this email — your password will not change until you open the link and pick a new one.\n\n"+
			"— The Hylete team\n",
		name, humanizeTTL(ttl), resetURL,
	)
}
