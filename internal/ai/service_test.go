package ai_coach

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"
)

func TestValidatePromptVersion(t *testing.T) {
	if err := ValidatePromptVersion(); err != nil {
		t.Fatal(err)
	}
}

func TestRenderWeeklySubstitutes(t *testing.T) {
	out := RenderWeekly(`{"thin_data":false}`, "Bench 100kg", "- Bench 100 (active)", "weight_unit=kg distance_unit=km", "height=180.0 cm gender=male age=30")
	for _, want := range []string{"Bench 100kg", "Bench 100 (active)", "weight_unit=kg", `{"thin_data":false}`, "height=180.0 cm gender=male age=30"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q", want)
		}
	}
	if strings.Contains(out, "{{") {
		t.Fatal("unsubstituted placeholder remains")
	}
}

func TestRenderWeeklyEmptyAim(t *testing.T) {
	out := RenderWeekly(`{}`, "  ", "", "", "")
	if !strings.Contains(out, "No stated aim.") || !strings.Contains(out, "(no goals recorded)") {
		t.Fatal("defaults missing")
	}
	if !strings.Contains(out, "No profile details provided.") {
		t.Fatal("profile default missing")
	}
}

func TestRenderUserProfile(t *testing.T) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	// All unset → empty (caller falls back to the default line).
	if got := RenderUserProfile(nil, "", nil, now); got != "" {
		t.Fatalf("empty profile = %q, want empty", got)
	}
	height := 180.0
	dob := "1996-03-04"
	got := RenderUserProfile(&height, "female", &dob, now)
	for _, want := range []string{"height=180.0 cm", "gender=female", "age=30"} {
		if !strings.Contains(got, want) {
			t.Fatalf("profile %q missing %q", got, want)
		}
	}
	// The raw birth date is never echoed — only the derived age.
	if strings.Contains(got, "1996-03-04") {
		t.Fatalf("profile must not contain the raw DOB: %q", got)
	}
	// Unknown gender is dropped, never echoed.
	if got := RenderUserProfile(nil, "unknown", nil, now); got != "" {
		t.Fatalf("unknown gender must be omitted, got %q", got)
	}
	// Malformed DOB is dropped, never echoed.
	bad := "not-a-date"
	if got := RenderUserProfile(nil, "", &bad, now); got != "" {
		t.Fatalf("malformed DOB must be omitted, got %q", got)
	}
	// Partial profile renders only the set fields.
	if got := RenderUserProfile(nil, "male", nil, now); got != "gender=male" {
		t.Fatalf("partial profile = %q, want %q", got, "gender=male")
	}
}

func TestValidateReportOK(t *testing.T) {
	raw := `{"summary":"Good week.","progress_per_goal":[],"prs":[],"stalling":[],
		"trends":{"volume":"up","frequency":"steady","bodyweight":"stable"},
		"adherence":"100%","recovery_signals":[],"recommendations":["A","B","C","D"]}`
	clean, err := ValidateReport("```json\n" + raw + "\n```")
	if err != nil {
		t.Fatal(err)
	}
	var p ReportPayload
	if err := json.Unmarshal([]byte(clean), &p); err != nil {
		t.Fatal(err)
	}
	if len(p.Recommendations) != 3 {
		t.Fatalf("recommendations truncated to 3: %+v", p)
	}
}

func TestValidateReportRejects(t *testing.T) {
	for _, raw := range []string{
		`not json`,
		`{"summary":"","recommendations":["A"]}`,
		`{"summary":"x","recommendations":[]}`,
	} {
		if _, err := ValidateReport(raw); err == nil {
			t.Fatalf("expected error for %q", raw)
		}
	}
}

func TestWeekBoundsStableWithinWeek(t *testing.T) {
	// Wednesday and Sunday of the same week share bounds; Monday 06:00
	// (cron time) falls in the new week with a fresh period.
	wed := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC) // Wednesday
	sun := time.Date(2026, 9, 13, 23, 0, 0, 0, time.UTC)
	mon := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC) // Monday cron
	psW, peW := WeekBounds(wed)
	psS, peS := WeekBounds(sun)
	if !psW.Equal(psS) || !peW.Equal(peS) {
		t.Fatal("bounds unstable within week")
	}
	psM, _ := WeekBounds(mon)
	if !psM.Equal(peW) {
		t.Fatalf("Monday period should start where last week ended: %v vs %v", psM, peW)
	}
	if peW.Weekday() != time.Monday || psW.Weekday() != time.Monday {
		t.Fatal("bounds must be Mondays")
	}
}

// --- Service thin-data path (no LLM) ---

type fakeEntries struct{ entries []models.ExerciseEntry }

func (f fakeEntries) GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]models.ExerciseEntry, error) {
	return f.entries, nil
}

type fakeWeights struct{}

func (fakeWeights) List(userID string) ([]models.WeightEntry, error) { return nil, nil }

type fakeHealth struct{}

func (fakeHealth) ListRange(userID, startDate, endDate string) ([]models.HealthSnapshot, error) {
	return nil, nil
}

type fakeGoals struct{}

func (fakeGoals) List(userID string) ([]models.Goal, error) { return nil, nil }

type fakeUsers struct{ user *models.User }

func (f fakeUsers) GetUserByID(id string) (*models.User, error) { return f.user, nil }
func (f fakeUsers) ListAIOptedInUsers(ctx context.Context) ([]models.User, error) {
	return []models.User{*f.user}, nil
}

type fakeReports struct{ stored *models.AIReport }

func (f *fakeReports) Create(r *models.AIReport) error { f.stored = r; return nil }
func (f *fakeReports) Get(userID, typ string, ps time.Time) (*models.AIReport, error) {
	return nil, nil
}
func (f *fakeReports) GetByID(id, userID string) (*models.AIReport, error) { return nil, nil }
func (f *fakeReports) Latest(userID, typ string) (*models.AIReport, error) { return nil, nil }
func (f *fakeReports) List(userID, typ string, limit int) ([]models.AIReport, error) {
	return nil, nil
}
func (f *fakeReports) MarkDismissed(id, userID string) error { return nil }
func (f *fakeReports) Reopen(id, userID string) error             { return nil }

type fakeClient struct{ calls int }

func (f *fakeClient) Chat(ctx context.Context, prompt string) (string, int, int, error) {
	f.calls++
	return `{"summary":"x","progress_per_goal":[],"prs":[],"stalling":[],
		"trends":{"volume":"v","frequency":"f","bodyweight":"b"},"adherence":"a",
		"recovery_signals":[],"recommendations":["A","B","C"]}`, 10, 20, nil
}

func testService(user *models.User, entries []models.ExerciseEntry, client Client) (*Service, *fakeReports, *fakeClient) {
	reports := &fakeReports{}
	fc, _ := client.(*fakeClient)
	svc, err := NewService(fakeEntries{entries}, fakeWeights{}, fakeHealth{}, fakeGoals{},
		fakeUsers{user}, reports, client, "test-model", true)
	if err != nil {
		panic(err)
	}
	return svc, reports, fc
}

func TestBuildWeeklyThinDataSkipsLLM(t *testing.T) {
	user := &models.User{ID: "u1", AIOptIn: true, WeightUnit: "kg", DistanceUnit: "km"}
	fc := &fakeClient{}
	svc, reports, _ := testService(user, nil, fc)
	now := time.Date(2026, 9, 14, 6, 0, 0, 0, time.UTC)
	report, err := svc.BuildWeeklyReport(context.Background(), "u1", now)
	if err != nil {
		t.Fatal(err)
	}
	if fc.calls != 0 {
		t.Fatal("thin data must not call the LLM")
	}
	if report.Model != "test-model" || report.Type != models.AIReportTypeWeekly {
		t.Fatalf("%+v", report)
	}
	var p ReportPayload
	if err := json.Unmarshal([]byte(reports.stored.PayloadJSON), &p); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Summary, "Not enough training data") {
		t.Fatalf("%+v", p)
	}
}

func TestBuildWeeklyOptedOut(t *testing.T) {
	user := &models.User{ID: "u1", AIOptIn: false}
	svc, _, _ := testService(user, nil, &fakeClient{})
	_, err := svc.BuildWeeklyReport(context.Background(), "u1", time.Now())
	if err != ErrOptedOut {
		t.Fatalf("expected ErrOptedOut, got %v", err)
	}
}

func TestBuildWeeklyDisabled(t *testing.T) {
	user := &models.User{ID: "u1", AIOptIn: true}
	reports := &fakeReports{}
	svc, err := NewService(fakeEntries{}, fakeWeights{}, fakeHealth{}, fakeGoals{},
		fakeUsers{user}, reports, nil, "", false)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.BuildWeeklyReport(context.Background(), "u1", time.Now())
	if err != ErrDisabled {
		t.Fatalf("expected ErrDisabled, got %v", err)
	}
}
