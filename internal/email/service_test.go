package email

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	emailtmpl "hylete/internal/email/templates"
	"hylete/internal/models"
)

// testBaseURL is the base URL the Service tests thread into
// every NewService call. Picked once here so the test bodies
// stay short and the URL format is consistent across the
// suite.
const testBaseURL = "https://hylete.test.local"

// recordingSender captures every Message that the Service hands
// to it, for the test to assert on. Thread-safe so a parallel
// Service can record into a single instance.
type recordingSender struct {
	mu       sync.Mutex
	messages []Message
	err      error // returned by Send (overrides default nil)
}

func (r *recordingSender) Send(ctx context.Context, msg Message) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.messages = append(r.messages, msg)
	return r.err
}

func (r *recordingSender) last() *Message {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.messages) == 0 {
		return nil
	}
	m := r.messages[len(r.messages)-1]
	return &m
}

// fakeAuthTokenRepo is a minimal AuthTokenRepo for the
// SendPasswordReset tests. It records the token it is asked to
// create and lets the test dictate the create-side error.
type fakeAuthTokenRepo struct {
	mu        sync.Mutex
	created   []createdToken
	createErr error
	consumeOK bool
}

type createdToken struct {
	userID string
	raw    string
	ttl    time.Duration
}

func (f *fakeAuthTokenRepo) CreatePasswordResetToken(ctx context.Context, userID, raw string, ttl time.Duration) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.createErr != nil {
		return "", f.createErr
	}
	f.created = append(f.created, createdToken{userID: userID, raw: raw, ttl: ttl})
	return "row-id", nil
}

func (f *fakeAuthTokenRepo) ConsumePasswordResetToken(ctx context.Context, raw string) (string, error) {
	return "", nil
}

// newTestService builds a Service with the recording sender and
// the test base URL. Returns (*Service, error) so the test
// can also assert on NewService validation failures.
func newTestService(t *testing.T) (*Service, *recordingSender) {
	t.Helper()
	sender := &recordingSender{}
	svc, err := NewService(sender, testBaseURL)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	return svc, sender
}

func TestNewService_ValidatesBaseURL(t *testing.T) {
	// A misconfigured deployment should fail at startup, not
	// surface as broken links in every email. The validation
	// runs in NewService; a bad URL returns a non-nil error.
	sender := &recordingSender{}
	cases := []struct {
		name    string
		baseURL string
	}{
		{"empty", ""},
		{"no scheme", "hylete.test.local"},
		{"ftp scheme", "ftp://hylete.test.local"},
		{"no host", "https://"},
		{"trailing slash", "https://hylete.test.local/"},
		{"garbage", "https://not a url"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewService(sender, tc.baseURL)
			if err == nil {
				t.Errorf("expected error for baseURL %q, got nil", tc.baseURL)
			}
		})
	}
}

func TestNewService_RejectsNilSender(t *testing.T) {
	// A nil sender would crash on the first Send call. Better
	// to catch it at construction time.
	if _, err := NewService(nil, testBaseURL); err == nil {
		t.Error("expected error for nil sender, got nil")
	}
}

func TestService_SendWelcome_HappyPath(t *testing.T) {
	svc, sender := newTestService(t)

	err := svc.SendWelcome(context.Background(), &models.User{
		Name:  "Alice",
		Email: "alice@example.com",
	})
	if err != nil {
		t.Fatalf("SendWelcome: %v", err)
	}
	got := sender.last()
	if got == nil {
		t.Fatal("sender received no message")
	}
	if got.To != "alice@example.com" {
		t.Errorf("To = %q, want alice@example.com", got.To)
	}
	if got.Subject != "Welcome to Hylete" {
		t.Errorf("Subject = %q, want %q", got.Subject, "Welcome to Hylete")
	}
	if !strings.Contains(got.HTML, "Alice") {
		t.Errorf("HTML missing user name: %q", got.HTML)
	}
	// Templ lowercases "<!DOCTYPE html>" in its output; both
	// are valid HTML5. Check the case-insensitive form so a
	// future templ version change does not break this test.
	if !strings.Contains(strings.ToLower(got.HTML), "<!doctype html>") {
		t.Errorf("HTML missing doctype: %q", got.HTML)
	}
	if !strings.Contains(got.Text, "Alice") {
		t.Errorf("text missing user name: %q", got.Text)
	}
	// The dashboard link and the footer link must point at
	// the configured baseURL, not at a hard-coded production
	// URL. This is the whole point of the baseURL knob.
	if !strings.Contains(got.HTML, testBaseURL) {
		t.Errorf("HTML missing baseURL %q", testBaseURL)
	}
}

func TestService_SendWelcome_ValidatesInputs(t *testing.T) {
	// Both nil and empty-email cases must short-circuit before
	// the sender is touched. The sender would be expensive
	// (SMTP, network) and is irrelevant for these errors.
	svc, sender := newTestService(t)

	if err := svc.SendWelcome(context.Background(), nil); err == nil {
		t.Error("expected error for nil user")
	}
	if err := svc.SendWelcome(context.Background(), &models.User{Name: "n"}); err == nil {
		t.Error("expected error for empty email")
	}
	if len(sender.messages) != 0 {
		t.Errorf("sender received %d messages, want 0", len(sender.messages))
	}
}

func TestService_SendWelcome_PropagatesSenderError(t *testing.T) {
	// The Sender's error must surface verbatim so the caller
	// (the auth controller's goroutine) can log it. Swallowing
	// the error would hide SMTP outages.
	wantErr := errors.New("smtp connection refused")
	sender := &recordingSender{err: wantErr}
	svc, err := NewService(sender, testBaseURL)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	err = svc.SendWelcome(context.Background(), &models.User{
		Name: "Alice", Email: "alice@example.com",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestService_SendWeightReminder_HappyPath(t *testing.T) {
	svc, sender := newTestService(t)

	err := svc.SendWeightReminder(context.Background(), &models.User{
		Name:              "Alice",
		Email:             "alice@example.com",
		ReminderFrequency: models.ReminderWeekly,
	})
	if err != nil {
		t.Fatalf("SendWeightReminder: %v", err)
	}
	got := sender.last()
	if got == nil {
		t.Fatal("sender received no message")
	}
	if got.To != "alice@example.com" {
		t.Errorf("To = %q, want alice@example.com", got.To)
	}
	if got.Subject != "Weekly weigh-in reminder" {
		t.Errorf("Subject = %q, want %q", got.Subject, "Weekly weigh-in reminder")
	}
	if !strings.Contains(got.HTML, "Alice") {
		t.Errorf("HTML missing user name: %q", got.HTML)
	}
	// The CTA must point at the configured baseURL + the new
	// weight entry route so the recipient lands directly on the
	// new-entry form (where the photo upload already lives).
	if !strings.Contains(got.HTML, testBaseURL+"/weight/new") {
		t.Errorf("HTML missing dashboard link to %q/weight/new: %q", testBaseURL, got.HTML)
	}
	if !strings.Contains(got.Text, testBaseURL+"/weight/new") {
		t.Errorf("text missing dashboard link to %q/weight/new: %q", testBaseURL, got.Text)
	}
	// The footer link must still point at the baseURL, not at a
	// hard-coded production value.
	if !strings.Contains(got.HTML, testBaseURL) {
		t.Errorf("HTML missing baseURL %q", testBaseURL)
	}
}

func TestService_SendWeightReminder_CadenceSubjects(t *testing.T) {
	// The subject and body must change with the cadence. A
	// daily user gets "Time to log your weight" + "Today's
	// weigh-in"; a weekly user gets the day-agnostic weekly
	// copy (the reminder can fire on whichever weekday the
	// user picked, so no email text names a day).
	// The orchestrator picks the cadence from the user's row,
	// so this is the only place to assert that the mapping is
	// right end-to-end (cadence → subject → body).
	cases := []struct {
		cadence       models.ReminderFrequency
		wantSubject   string
		wantInText    string
		wantInHTML    string
	}{
		{
			cadence:     models.ReminderDaily,
			wantSubject: "Time to log your weight",
			wantInText:  "Today's weigh-in",
			wantInHTML:  "Log today&#39;s weight",
		},
		{
			cadence:     models.ReminderWeekly,
			wantSubject: "Weekly weigh-in reminder",
			wantInText:  "Time to log this week's weight",
			wantInHTML:  "Log this week&#39;s weight",
		},
		{
			cadence:     models.ReminderBiweekly,
			wantSubject: "Time to log your weight",
			wantInText:  "Time to log this week's weight",
			wantInHTML:  "Log today&#39;s weight",
		},
	}
	for _, tc := range cases {
		t.Run(string(tc.cadence), func(t *testing.T) {
			svc, sender := newTestService(t)
			if err := svc.SendWeightReminder(context.Background(), &models.User{
				Name:              "Alice",
				Email:             "alice@example.com",
				ReminderFrequency: tc.cadence,
			}); err != nil {
				t.Fatalf("SendWeightReminder: %v", err)
			}
			got := sender.last()
			if got == nil {
				t.Fatal("sender received no message")
			}
			if got.Subject != tc.wantSubject {
				t.Errorf("Subject = %q, want %q", got.Subject, tc.wantSubject)
			}
			if !strings.Contains(got.Text, tc.wantInText) {
				t.Errorf("text missing %q: %q", tc.wantInText, got.Text)
			}
			if !strings.Contains(got.HTML, tc.wantInHTML) {
				t.Errorf("HTML missing %q: %q", tc.wantInHTML, got.HTML)
			}
		})
	}
}

func TestService_SendWeightReminder_ValidatesInputs(t *testing.T) {
	// Both nil and empty-email cases must short-circuit before
	// the sender is touched. The sender would be expensive
	// (SMTP, network) and is irrelevant for these errors.
	svc, sender := newTestService(t)

	if err := svc.SendWeightReminder(context.Background(), nil); err == nil {
		t.Error("expected error for nil user")
	}
	if err := svc.SendWeightReminder(context.Background(), &models.User{Name: "n"}); err == nil {
		t.Error("expected error for empty email")
	}
	if len(sender.messages) != 0 {
		t.Errorf("sender received %d messages, want 0", len(sender.messages))
	}
}

func TestService_SendWeightReminder_PropagatesSenderError(t *testing.T) {
	// The Sender's error must surface verbatim so the
	// reminders package's goroutine can log it. Swallowing
	// the error would hide SMTP outages.
	wantErr := errors.New("smtp connection refused")
	sender := &recordingSender{err: wantErr}
	svc, err := NewService(sender, testBaseURL)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	err = svc.SendWeightReminder(context.Background(), &models.User{
		Name: "Alice", Email: "alice@example.com",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
}

func TestService_SendPasswordReset_HappyPath(t *testing.T) {
	// The Service must: mint a token, persist its hash via the
	// repo, build the URL, render the email, and send it. We
	// assert on each step except the SMTP wire (covered by the
	// Client tests).
	svc, sender := newTestService(t)
	repo := &fakeAuthTokenRepo{}

	raw, err := svc.SendPasswordReset(context.Background(), repo, &models.User{
		ID: "user-bob", Name: "Bob", Email: "bob@example.com",
	})
	if err != nil {
		t.Fatalf("SendPasswordReset: %v", err)
	}
	if raw == "" {
		t.Error("expected non-empty raw token")
	}
	if len(raw) != 43 {
		t.Errorf("len(raw) = %d, want 43 (32 bytes base64url)", len(raw))
	}
	if len(repo.created) != 1 {
		t.Fatalf("repo.CreatePasswordResetToken called %d times, want 1", len(repo.created))
	}
	created := repo.created[0]
	if created.userID == "" {
		t.Error("CreatePasswordResetToken called with empty userID")
	}
	if created.raw != raw {
		t.Error("CreatePasswordResetToken called with a different token than the one returned")
	}
	if created.ttl != emailtmpl.PasswordResetTTL {
		t.Errorf("ttl = %v, want %v", created.ttl, emailtmpl.PasswordResetTTL)
	}

	got := sender.last()
	if got == nil {
		t.Fatal("sender received no message")
	}
	if got.To != "bob@example.com" {
		t.Errorf("To = %q, want bob@example.com", got.To)
	}
	if !strings.Contains(got.Subject, "Reset") {
		t.Errorf("Subject = %q, want something with 'Reset'", got.Subject)
	}
	// The URL must use the configured baseURL (not a
	// hard-coded production value) and the token parameter
	// must equal the raw token returned to the caller. The
	// link is the only secret material; if the token in the
	// URL does not match, the recipient will click and the
	// controller will reject the attempt.
	wantURL := testBaseURL + "/reset?token=" + raw
	if !strings.Contains(got.HTML, wantURL) {
		t.Errorf("HTML missing reset URL with token: want %q in %q", wantURL, got.HTML)
	}
	if !strings.Contains(got.Text, wantURL) {
		t.Errorf("text missing reset URL with token: want %q in %q", wantURL, got.Text)
	}
}

func TestService_SendPasswordReset_RepoErrorBubblesUp(t *testing.T) {
	// If the token repo fails (e.g. DB down), the Service
	// must return the error and not call the sender. Calling
	// the sender would result in an email with no token in
	// the database, which is worse than no email at all.
	wantErr := errors.New("db unavailable")
	repo := &fakeAuthTokenRepo{createErr: wantErr}
	sender := &recordingSender{}
	svc, err := NewService(sender, testBaseURL)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	raw, err := svc.SendPasswordReset(context.Background(), repo, &models.User{
		ID: "user-bob", Name: "Bob", Email: "bob@example.com",
	})
	if !errors.Is(err, wantErr) {
		t.Errorf("err = %v, want %v", err, wantErr)
	}
	if raw != "" {
		t.Errorf("raw = %q, want empty on error", raw)
	}
	if len(sender.messages) != 0 {
		t.Errorf("sender received %d messages, want 0 on repo error", len(sender.messages))
	}
}

func TestService_SendPasswordReset_SenderErrorLeavesTokenInDB(t *testing.T) {
	// The documented design choice: if the SMTP send fails,
	// the token stays in the DB and expires naturally. The
	// caller can retry the email without invalidating
	// outstanding tokens. The test asserts both halves: the
	// token was created, and the sender error is surfaced.
	sendErr := errors.New("smtp timeout")
	repo := &fakeAuthTokenRepo{}
	sender := &recordingSender{err: sendErr}
	svc, err := NewService(sender, testBaseURL)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}

	raw, err := svc.SendPasswordReset(context.Background(), repo, &models.User{
		ID: "user-bob", Name: "Bob", Email: "bob@example.com",
	})
	if !errors.Is(err, sendErr) {
		t.Errorf("err = %v, want %v", err, sendErr)
	}
	if raw == "" {
		t.Error("expected raw token to be returned even on send error")
	}
	if len(repo.created) != 1 {
		t.Errorf("repo.CreatePasswordResetToken called %d times, want 1 (token must persist)", len(repo.created))
	}
}

func TestService_SendPasswordReset_ValidatesInputs(t *testing.T) {
	svc, _ := newTestService(t)
	cases := []struct {
		name string
		user *models.User
		repo models.AuthTokenRepo
	}{
		{"nil user", nil, &fakeAuthTokenRepo{}},
		{"empty email", &models.User{Name: "n"}, &fakeAuthTokenRepo{}},
		{"nil repo", &models.User{Name: "n", Email: "a@b.c"}, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.SendPasswordReset(context.Background(), tc.repo, tc.user)
			if err == nil {
				t.Errorf("expected error for %s, got nil", tc.name)
			}
		})
	}
}
