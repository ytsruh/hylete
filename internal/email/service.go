package email

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"strings"

	emailtmpl "hylete/internal/email/templates"
	"hylete/internal/models"
)

// Sender is the narrow contract Service depends on for delivering
// one message over SMTP. Defined as an interface (rather than
// depending on *Client directly) so the service can be unit-tested
// with a fake — see the fakes in service_test.go.
type Sender interface {
	Send(ctx context.Context, msg Message) error
}

// Service is the high-level email entry point used by the
// controllers. It composes templated messages and hands them to a
// Sender. The service itself is stateless and safe for concurrent
// use; each method call constructs its own Message and returns the
// underlying Sender's error (or nil) verbatim.
//
// baseURL is the absolute origin the email is being sent from
// (typically the value of the PUBLIC_URL env var, e.g.
// "https://www.hyleteapp.com" in production). It is threaded into
// every link the email contains (dashboard button, password-reset
// link, footer "view on the web" link) so a staging deployment
// sends email that points at itself, not at production.
//
// Two methods are exported today: SendWelcome (used by the
// register flow) and SendPasswordReset (used by the
// forgot-password flow). Adding a new email type is a templ
// addition in internal/email/templates + a new method here.
type Service struct {
	sender  Sender
	baseURL string
}

// NewService returns a Service ready to send. sender is the
// (possibly faked) SMTP client. baseURL is validated up front so
// a misconfigured deployment fails fast at startup rather than
// surfacing as broken links in every outbound email.
//
// baseURL must be a non-empty http or https URL with no trailing
// slash. The trailing-slash rule keeps the buildResetURL /
// welcomeButton concatenations in the templates trivially safe
// (we can always do baseURL + "/path" without doubling up).
func NewService(sender Sender, baseURL string) (*Service, error) {
	if sender == nil {
		return nil, errors.New("email: NewService: sender is nil")
	}
	if err := validateBaseURL(baseURL); err != nil {
		return nil, fmt.Errorf("email: NewService: %w", err)
	}
	return &Service{sender: sender, baseURL: baseURL}, nil
}

// validateBaseURL enforces the rules documented on NewService.
// Split out so the tests can exercise the corner cases without
// going through NewService.
func validateBaseURL(baseURL string) error {
	if baseURL == "" {
		return errors.New("baseURL is required")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("baseURL is not a valid URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("baseURL must use http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return errors.New("baseURL must include a host")
	}
	if strings.HasSuffix(baseURL, "/") {
		return errors.New("baseURL must not have a trailing slash")
	}
	return nil
}

// SendWelcome delivers the "welcome to Hylete" email to the user.
// Render is deterministic and the only inputs are the user's
// name and email address, so this method has no error path other
// than the underlying SMTP send.
//
// The caller (controllers.AuthController.Register) is expected to
// fire this from a goroutine with a recover so a transient SMTP
// failure does not fail user registration.
func (s *Service) SendWelcome(ctx context.Context, user *models.User) error {
	if user == nil {
		return errors.New("email: SendWelcome: user is nil")
	}
	if user.Email == "" {
		return errors.New("email: SendWelcome: user.Email is empty")
	}

	html, text := emailtmpl.RenderWelcome(user.Name, s.baseURL)
	return s.sender.Send(ctx, Message{
		To:      user.Email,
		Subject: "Welcome to Hylete",
		HTML:    html,
		Text:    text,
	})
}

// SendWeightReminder delivers the per-user "log your weight"
// reminder email. The cadence is one of models.ReminderDaily /
// Weekly / Biweekly and is threaded into the templ component so
// a daily user gets a "Time to log your weight" subject and a
// weekly user gets "Weekly weigh-in reminder". The copy is
// deliberately day-agnostic: a weekly reminder fires on
// whichever weekday the user picked, so no email text names a
// day or a time.
//
// The caller (the reminders package) is expected to fire this
// from a goroutine with a recover so a single SMTP failure does
// not take the whole batch down.
func (s *Service) SendWeightReminder(ctx context.Context, user *models.User) error {
	if user == nil {
		return errors.New("email: SendWeightReminder: user is nil")
	}
	if user.Email == "" {
		return errors.New("email: SendWeightReminder: user.Email is empty")
	}

	cadence := user.ReminderFrequency
	html, text := emailtmpl.RenderWeightReminder(user.Name, s.baseURL, cadence)
	subject, _ := weightReminderSubject(cadence)
	return s.sender.Send(ctx, Message{
		To:      user.Email,
		Subject: subject,
		HTML:    html,
		Text:    text,
	})
}

// weightReminderSubject returns the email subject for the given
// cadence. Pulled into a helper so the EmailSender can stamp the
// right subject on the Message without re-running the templ
// renderer's branching — the templ component already encodes the
// subject into the email body, but the SMTP layer needs the bare
// subject string in the Message envelope.
func weightReminderSubject(cadence models.ReminderFrequency) (string, string) {
	switch cadence {
	case models.ReminderDaily:
		return "Time to log your weight", "Today's weigh-in"
	case models.ReminderWeekly:
		return "Weekly weigh-in reminder", "Time to log this week's weight."
	case models.ReminderBiweekly:
		return "Time to log your weight", "Time to log this week's weight."
	}
	return "Time to log your weight", "Time to log your weight."
}

// SendPasswordReset mints a fresh password-reset token via the
// tokenRepo and delivers the email. The raw token only ever lives
// in the email link — the database stores the sha256 hash.
//
// The token is generated here (not in the controller) so the
// service is the single source of truth for the secret material.
// The controller's only job is to look up the user and call this
// method; if the user does not exist the controller short-circuits
// before reaching here, so this method can assume a non-nil user.
//
// rawToken is returned to the caller for the (admittedly niche)
// case where the caller wants to log it or include it in a
// developer-mode preview. In production the raw token is never
// logged, never returned in an HTTP response, and only ever sent
// over SMTP. The return value is the second slot, not the first,
// to keep the call site readable: `token, err := svc.SendPasswordReset(...)`.
func (s *Service) SendPasswordReset(ctx context.Context, tokenRepo models.AuthTokenRepo, user *models.User) (rawToken string, err error) {
	if user == nil {
		return "", errors.New("email: SendPasswordReset: user is nil")
	}
	if user.Email == "" {
		return "", errors.New("email: SendPasswordReset: user.Email is empty")
	}
	if tokenRepo == nil {
		return "", errors.New("email: SendPasswordReset: tokenRepo is nil")
	}

	rawToken, err = newRawToken()
	if err != nil {
		return "", fmt.Errorf("email: generate token: %w", err)
	}

	if _, err := tokenRepo.CreatePasswordResetToken(ctx, user.ID, rawToken, emailtmpl.PasswordResetTTL); err != nil {
		return "", fmt.Errorf("email: persist reset token: %w", err)
	}

	html, text := emailtmpl.RenderPasswordReset(user.Name, rawToken, s.baseURL, emailtmpl.PasswordResetTTL)
	if err := s.sender.Send(ctx, Message{
		To:      user.Email,
		Subject: "Reset your Hylete password",
		HTML:    html,
		Text:    text,
	}); err != nil {
		// The token was persisted but the email failed. The
		// token will eventually expire on its own (1h TTL);
		// the caller logs the SMTP error and moves on. We do
		// NOT delete the token here — a retry path that
		// re-issues the email would otherwise need to wait
		// for the old token to expire.
		return rawToken, fmt.Errorf("email: send reset email: %w", err)
	}
	return rawToken, nil
}

// newRawToken returns 32 cryptographically-random bytes,
// base64url-encoded (no padding) per RFC 4648 §5. The encoding
// produces a 43-character ASCII string safe for use in a URL
// query parameter without further escaping.
func newRawToken() (string, error) {
	const n = 32
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
