package ai_coach

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"hylete/internal/models"
	"hylete/internal/trainingstats"
)

// Sentinel errors the routes map to status codes.
var (
	// ErrDisabled means the service was constructed disabled (nil
	// client). Unreachable in production — credentials are
	// required EnvVar fields — but the API still maps it to 503
	// so tests and future wiring fail precisely, not vaguely.
	ErrDisabled = errors.New("ai: service disabled (missing Cloudflare credentials)")
	// ErrOptedOut means the user has ai_opt_in = 0. The API maps
	// this to 403. The cron never calls the service for such
	// users, so this only fires on direct API hits.
	ErrOptedOut = errors.New("ai: user has not opted in")
	// ErrNoReport means no report exists yet for the requested scope.
	ErrNoReport = errors.New("ai: no report yet")
)

// Narrow data-access interfaces the service accepts (per repo
// convention: packages take interfaces, concrete repos satisfy
// them without modification).
type (
	// ExerciseEntryStore loads entries in a date range.
	ExerciseEntryStore interface {
		GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]models.ExerciseEntry, error)
	}
	// WeightStore lists body-weight entries (service filters by window).
	WeightStore interface {
		List(userID string) ([]models.WeightEntry, error)
	}
	// HealthStore lists snapshots in a YYYY-MM-DD range, newest first.
	HealthStore interface {
		ListRange(userID, startDate, endDate string) ([]models.HealthSnapshot, error)
	}
	// GoalStore lists goals, active first then completed.
	GoalStore interface {
		List(userID string) ([]models.Goal, error)
	}
	// UserStore reads the Coach consent fields.
	UserStore interface {
		GetUserByID(id string) (*models.User, error)
		ListAIOptedInUsers(ctx context.Context) ([]models.User, error)
	}
)

// ReportPayload is the validated shape of a weekly report. The LLM
// must return exactly this; ValidateReport rejects anything else so
// a malformed completion never reaches storage or the API.
type ReportPayload struct {
	Summary         string `json:"summary"`
	ProgressPerGoal []struct {
		Goal   string `json:"goal"`
		Status string `json:"status"`
	} `json:"progress_per_goal"`
	PRs             []string `json:"prs"`
	Stalling        []string `json:"stalling"`
	Trends          struct {
		Volume      string `json:"volume"`
		Frequency   string `json:"frequency"`
		Bodyweight  string `json:"bodyweight"`
	} `json:"trends"`
	Adherence       string   `json:"adherence"`
	RecoverySignals []string `json:"recovery_signals"`
	Recommendations []string `json:"recommendations"`
}

// Service orchestrates weekly Coach reports.
type Service struct {
	entries ExerciseEntryStore
	weights WeightStore
	health  HealthStore
	goals   GoalStore
	users   UserStore
	reports models.AIReportRepo
	client  Client
	model   string
	enabled bool
	now     func() time.Time
}

// NewService wires the orchestrator. client may be nil (e.g. in
// tests that only exercise thin-data paths); enabled=false marks
// the whole feature unavailable regardless of per-user opt-in.
func NewService(
	entries ExerciseEntryStore,
	weights WeightStore,
	health HealthStore,
	goals GoalStore,
	users UserStore,
	reports models.AIReportRepo,
	client Client,
	model string,
	enabled bool,
) (*Service, error) {
	if err := ValidatePromptVersion(); err != nil {
		return nil, err
	}
	return &Service{
		entries: entries, weights: weights, health: health,
		goals: goals, users: users, reports: reports,
		client: client, model: model, enabled: enabled,
		now: time.Now,
	}, nil
}

// Enabled reports whether the service may call the LLM.
func (s *Service) Enabled() bool { return s.enabled && s.client != nil }

// WeekBounds returns the last full Mon–Sun week (UTC) before now:
// periodStart is the previous Monday 00:00, periodEnd the most
// recent Monday 00:00. Stable for any now within the same calendar
// week, which makes the cron idempotent on periodStart.
func WeekBounds(now time.Time) (periodStart, periodEnd time.Time) {
	now = now.UTC()
	y, m, d := now.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
	mondayOffset := (int(midnight.Weekday()) + 6) % 7 // Mon=0 … Sun=6
	thisMonday := midnight.AddDate(0, 0, -mondayOffset)
	return thisMonday.AddDate(0, 0, -7), thisMonday
}

// BuildWeeklyReport generates (or reuses) the weekly report for one
// user. Idempotent: a stored row for (user, weekly, periodStart) is
// returned without any LLM call.
func (s *Service) BuildWeeklyReport(ctx context.Context, userID string, now time.Time) (*models.AIReport, error) {
	if !s.Enabled() {
		return nil, ErrDisabled
	}
	user, err := s.users.GetUserByID(userID)
	if err != nil {
		return nil, fmt.Errorf("ai: failed to load user: %w", err)
	}
	if user == nil {
		return nil, fmt.Errorf("ai: user not found")
	}
	if !user.AIOptIn {
		return nil, ErrOptedOut
	}

	periodStart, periodEnd := WeekBounds(now)
	if existing, err := s.reports.Get(userID, models.AIReportTypeWeekly, periodStart); err != nil {
		return nil, err
	} else if existing != nil {
		return existing, nil
	}

	entries, err := s.entries.GetExerciseEntriesByDateRange(periodEnd.Add(-12*7*24*time.Hour), periodEnd, userID)
	if err != nil {
		return nil, fmt.Errorf("ai: failed to load entries: %w", err)
	}
	weights, err := s.weights.List(userID)
	if err != nil {
		return nil, fmt.Errorf("ai: failed to load weights: %w", err)
	}
	health, err := s.health.ListRange(userID,
		periodEnd.Add(-5*7*24*time.Hour).Format("2006-01-02"),
		periodEnd.Format("2006-01-02"))
	if err != nil {
		return nil, fmt.Errorf("ai: failed to load health: %w", err)
	}
	goals, err := s.goals.List(userID)
	if err != nil {
		return nil, fmt.Errorf("ai: failed to load goals: %w", err)
	}

	stats := trainingstats.Build(trainingstats.Input{
		Entries: entries, Weights: weights, Health: health, Now: periodEnd,
	})

	var payload string
	var in, out int
	if stats.ThinData {
		payload = thinDataPayload(stats)
	} else {
		statsJSON, err := json.Marshal(stats)
		if err != nil {
			return nil, fmt.Errorf("ai: failed to encode stats: %w", err)
		}
		prompt := RenderWeekly(string(statsJSON), user.AIGoalText, goalsList(goals), prefsLine(user))
		raw, tokensIn, tokensOut, err := s.client.Chat(ctx, prompt)
		if err != nil {
			return nil, err
		}
		in, out = tokensIn, tokensOut
		payload, err = ValidateReport(raw)
		if err != nil {
			return nil, err
		}
	}

	report := &models.AIReport{
		UserID: userID, Type: models.AIReportTypeWeekly,
		PeriodStart: periodStart, PeriodEnd: periodEnd,
		PromptVersion: PromptVersion, Model: s.model,
		PayloadJSON: payload, TokensIn: in, TokensOut: out,
	}
	if err := s.reports.Create(report); err != nil {
		return nil, err
	}
	return report, nil
}

// TickResult summarises one cron run for the server log.
type TickResult struct {
	UsersSeen  int
	Generated  int
	Reused     int
	Failures   int
	TokensIn   int
	TokensOut  int
}

// RunWeekly iterates every opted-in user and builds their weekly
// report. Per-user failures are counted (not returned) so one bad
// row cannot abort the whole tick.
func (s *Service) RunWeekly(ctx context.Context, now time.Time) TickResult {
	var res TickResult
	if !s.Enabled() {
		return res
	}
	users, err := s.users.ListAIOptedInUsers(ctx)
	if err != nil {
		res.Failures++
		return res
	}
	for _, u := range users {
		res.UsersSeen++
		before, _ := s.reports.Get(u.ID, models.AIReportTypeWeekly, weekStartOf(now))
		report, err := s.BuildWeeklyReport(ctx, u.ID, now)
		if err != nil {
			res.Failures++
			continue
		}
		res.TokensIn += report.TokensIn
		res.TokensOut += report.TokensOut
		if before != nil {
			res.Reused++
		} else {
			res.Generated++
		}
	}
	return res
}

// weekStartOf mirrors the periodStart WeekBounds would compute, for
// the RunWeekly reuse counter.
func weekStartOf(now time.Time) time.Time {
	ps, _ := WeekBounds(now)
	return ps
}

// ValidateReport parses raw LLM output into ReportPayload, enforces
// the contract (non-empty summary, 1–3 recommendations), and returns
// canonical JSON. Fences (```json … ```) are stripped tolerantly;
// anything else malformed is an error and the report is not stored.
func ValidateReport(raw string) (string, error) {
	cleaned := strings.TrimSpace(raw)
	cleaned = strings.TrimPrefix(cleaned, "```json")
	cleaned = strings.TrimPrefix(cleaned, "```")
	cleaned = strings.TrimSuffix(cleaned, "```")
	cleaned = strings.TrimSpace(cleaned)
	var p ReportPayload
	if err := json.Unmarshal([]byte(cleaned), &p); err != nil {
		return "", fmt.Errorf("ai: report is not valid JSON: %w", err)
	}
	if strings.TrimSpace(p.Summary) == "" {
		return "", fmt.Errorf("ai: report has empty summary")
	}
	// Trim whitespace-only entries; require 1–3 recommendations.
	recs := p.Recommendations[:0]
	for _, r := range p.Recommendations {
		if strings.TrimSpace(r) != "" {
			recs = append(recs, strings.TrimSpace(r))
		}
	}
	if len(recs) == 0 {
		return "", fmt.Errorf("ai: report has no recommendations")
	}
	if len(recs) > 3 {
		recs = recs[:3]
	}
	p.Recommendations = recs
	if p.ProgressPerGoal == nil {
		p.ProgressPerGoal = []struct {
			Goal   string `json:"goal"`
			Status string `json:"status"`
		}{}
	}
	if p.PRs == nil {
		p.PRs = []string{}
	}
	if p.Stalling == nil {
		p.Stalling = []string{}
	}
	if p.RecoverySignals == nil {
		p.RecoverySignals = []string{}
	}
	out, err := json.Marshal(p)
	if err != nil {
		return "", fmt.Errorf("ai: failed to re-encode report: %w", err)
	}
	return string(out), nil
}

// thinDataPayload stores a deterministic template with no LLM spend.
func thinDataPayload(stats trainingstats.WeeklyStats) string {
	p := ReportPayload{
		Summary: "Not enough training data this week to write a review. Log at least 3 sessions and check back Monday.",
		Adherence: fmt.Sprintf("%.0f%% of recent average (%d sessions)",
			stats.AdherencePct, stats.Sessions),
		Recommendations: []string{"Log at least 3 sessions next week so Coach has data to work with."},
	}
	p.ProgressPerGoal = []struct {
		Goal   string `json:"goal"`
		Status string `json:"status"`
	}{}
	p.PRs = []string{}
	p.Stalling = []string{}
	p.RecoverySignals = []string{}
	out, _ := json.Marshal(p)
	return string(out)
}

// goalsList renders goals as plain context lines. The service never
// parses titles — the LLM references them by name only.
func goalsList(goals []models.Goal) string {
	if len(goals) == 0 {
		return ""
	}
	var b strings.Builder
	for _, g := range goals {
		status := "active"
		if g.IsComplete() {
			status = "completed"
		}
		fmt.Fprintf(&b, "- %s (%s)\n", g.Title, status)
	}
	return b.String()
}

// prefsLine carries display units so narration labels weights and
// paces the way the user sees them elsewhere in the app.
func prefsLine(u *models.User) string {
	return "weight_unit=" + u.WeightUnitDisplay() + " distance_unit=" + u.DistanceUnitDisplay()
}
