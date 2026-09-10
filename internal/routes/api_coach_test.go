package routes

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	aicoach "hylete/internal/ai"
	"hylete/internal/models"
)

// stubCoachClient never fires in these tests (no build path); it
// exists so the attached service reports Enabled().
type stubCoachClient struct{}

func (stubCoachClient) Chat(ctx context.Context, prompt string) (string, int, int, error) {
	return "", 0, 0, errors.New("stub client: no LLM in route tests")
}

// enabledTestCoachService attaches a service that passes the
// enabled gate without touching the network.
func enabledTestCoachService(t *testing.T, h *Handler) *aicoach.Service {
	t.Helper()
	svc, err := aicoach.NewService(nil, nil, nil, nil, nil, nil, stubCoachClient{}, "test-model", true)
	if err != nil {
		t.Fatal(err)
	}
	return svc
}

// fakeAIReportRepo is an in-memory models.AIReportRepo for Coach
// route tests. No DB, no sqlc — just a map keyed by ID.
type fakeAIReportRepo struct {
	reports map[string]*models.AIReport
}

func newFakeAIReportRepo() *fakeAIReportRepo {
	return &fakeAIReportRepo{reports: map[string]*models.AIReport{}}
}

func (f *fakeAIReportRepo) Create(r *models.AIReport) error {
	if r.ID == "" {
		r.ID = "report-" + r.PeriodStart.Format("20060102")
	}
	cp := *r
	f.reports[r.ID] = &cp
	return nil
}

func (f *fakeAIReportRepo) Get(userID, typ string, ps time.Time) (*models.AIReport, error) {
	for _, r := range f.reports {
		if r.UserID == userID && r.Type == typ && r.PeriodStart.Equal(ps) {
			return r, nil
		}
	}
	return nil, nil
}

func (f *fakeAIReportRepo) GetByID(id, userID string) (*models.AIReport, error) {
	r, ok := f.reports[id]
	if !ok || r.UserID != userID {
		return nil, nil
	}
	return r, nil
}

func (f *fakeAIReportRepo) Latest(userID, typ string) (*models.AIReport, error) {
	var best *models.AIReport
	for _, r := range f.reports {
		if r.UserID != userID || r.Type != typ {
			continue
		}
		if best == nil || r.PeriodStart.After(best.PeriodStart) {
			best = r
		}
	}
	return best, nil
}

func (f *fakeAIReportRepo) List(userID, typ string, limit int) ([]models.AIReport, error) {
	var out []models.AIReport
	for _, r := range f.reports {
		if r.UserID == userID && r.Type == typ {
			out = append(out, *r)
		}
	}
	return out, nil
}

func (f *fakeAIReportRepo) MarkDismissed(id, userID string) error {
	if r, ok := f.reports[id]; ok && r.UserID == userID {
		now := time.Now()
		r.DismissedAt = &now
	}
	return nil
}

func (f *fakeAIReportRepo) Reopen(id, userID string) error {
	if r, ok := f.reports[id]; ok && r.UserID == userID {
		r.DismissedAt = nil
	}
	return nil
}

// optInCoach flips the mock user into the Coach beta: server-side
// opt-in on, with an aim. Mirrors what the iOS Profile screen PUTs.
func optInCoach(t *testing.T, h *Handler, mockUser *mockUserRepository, email string) {
	t.Helper()
	for i, u := range mockUser.users {
		if u.Email == email {
			mockUser.users[i].AIOptIn = true
			mockUser.users[i].AIGoalText = "Bench 100kg"
			return
		}
	}
	t.Fatalf("user %s not found", email)
}

func TestAPICoachPreferences_RoundTrip(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "coach-pref@example.com", "Coach")

	// Defaults: opted out, empty aim (works with no service attached).
	got := decodeAPI[CoachPreferencesDTO](t,
		apiDo(t, e, http.MethodGet, "/api/v1/me/coach-preferences", token, nil), http.StatusOK)
	if got.OptIn || got.GoalText != "" {
		t.Fatalf("defaults: %+v", got)
	}

	// Save opt-in + aim.
	saved := decodeAPI[CoachPreferencesDTO](t,
		apiDo(t, e, http.MethodPut, "/api/v1/me/coach-preferences", token,
			UpdateCoachPreferencesRequest{OptIn: true, GoalText: "  Bench 100kg by December  "}), http.StatusOK)
	if !saved.OptIn || saved.GoalText != "Bench 100kg by December" {
		t.Fatalf("trim: %+v", saved)
	}

	// Over-length aim is a 400.
	long := strings.Repeat("x", models.AIGoalTextMaxLength+1)
	rec := apiDo(t, e, http.MethodPut, "/api/v1/me/coach-preferences", token,
		UpdateCoachPreferencesRequest{OptIn: true, GoalText: long})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestAPICoachLatest_OptedOut403(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "coach-out@example.com", "Coach")
	h.SetCoachService(nil, newFakeAIReportRepo())

	rec := apiDo(t, e, http.MethodGet, "/api/v1/coach/weekly:latest", token, nil)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPICoachLatest_Disabled503(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "coach-dis@example.com", "Coach")
	optInCoach(t, h, mockUser, "coach-dis@example.com")
	h.SetCoachService(nil, newFakeAIReportRepo())

	rec := apiDo(t, e, http.MethodGet, "/api/v1/coach/weekly:latest", token, nil)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPICoachLatest_NoReport404(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "coach-empty@example.com", "Coach")
	optInCoach(t, h, mockUser, "coach-empty@example.com")
	h.SetCoachService(enabledTestCoachService(t, h), newFakeAIReportRepo())

	rec := apiDo(t, e, http.MethodGet, "/api/v1/coach/weekly:latest", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404, body=%s", rec.Code, rec.Body.String())
	}
}

func TestAPICoachLatestHistoryDismiss(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, user := loginUser(t, h, mockUser, "coach-full@example.com", "Coach")
	optInCoach(t, h, mockUser, "coach-full@example.com")
	fake := newFakeAIReportRepo()
	h.SetCoachService(enabledTestCoachService(t, h), fake)

	seed := &models.AIReport{
		ID: "r1", UserID: user.ID, Type: models.AIReportTypeWeekly,
		PeriodStart: time.Date(2026, 9, 7, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
		PromptVersion: "v1", Model: "test",
		PayloadJSON:   `{"summary":"Good week.","recommendations":["A"]}`,
	}
	if err := fake.Create(seed); err != nil {
		t.Fatal(err)
	}

	latest := decodeAPI[CoachReportDTO](t,
		apiDo(t, e, http.MethodGet, "/api/v1/coach/weekly:latest", token, nil), http.StatusOK)
	if latest.ID != "r1" || string(latest.Payload) != seed.PayloadJSON {
		t.Fatalf("latest: %+v", latest)
	}

	hist := decodeAPI[map[string][]CoachReportDTO](t,
		apiDo(t, e, http.MethodGet, "/api/v1/coach/weekly?limit=5", token, nil), http.StatusOK)
	if len(hist["reports"]) != 1 {
		t.Fatalf("history: %+v", hist)
	}

	if rec := apiDo(t, e, http.MethodPost, "/api/v1/coach/weekly/r1/dismiss", token, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("dismiss status = %d", rec.Code)
	}
	if rec := apiDo(t, e, http.MethodPost, "/api/v1/coach/weekly/nope/dismiss", token, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d, want 404", rec.Code)
	}

	// Restore returns the dismissed card to the list; unknown ids 404.
	if rec := apiDo(t, e, http.MethodPost, "/api/v1/coach/weekly/r1/restore", token, nil); rec.Code != http.StatusNoContent {
		t.Fatalf("restore status = %d", rec.Code)
	}
	if got, _ := fake.GetByID("r1", user.ID); got == nil || got.IsDismissed() {
		t.Fatalf("restore did not clear dismissed_at: %+v", got)
	}
	if rec := apiDo(t, e, http.MethodPost, "/api/v1/coach/weekly/nope/restore", token, nil); rec.Code != http.StatusNotFound {
		t.Fatalf("unknown id status = %d, want 404", rec.Code)
	}
}
