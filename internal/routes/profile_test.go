package routes

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
	"hylete/internal/utils"
)

// profileTestHarness wires a Handler with the minimum
// dependencies the profile route needs (user repo, image
// pipeline). Returns the handler and the user repo so tests can
// inspect what was stored.
func profileTestHarness(t *testing.T) (*Handler, *mockUserRepository, *echo.Echo) {
	t.Helper()
	e := echo.New()
	mockUser := newMockUserRepository()
	mockAdminUser := newMockAdminUserRepository()
	mockFeedback := newMockFeedbackRepository()
	mockWeight := newMockWeightRepository()
	mockRepo := newMockRepository()
	jwtService := utils.NewJWTService("test-secret")
	authCtrl := controllers.NewAuthController(mockUser, jwtService, nil)
	authRecoveryCtrl := controllers.NewAuthRecoveryController(mockUser, newMockAuthTokenRepo(), nil)
	entryCtrl := controllers.NewExerciseEntryController(mockRepo)
	adminCtrl := controllers.NewAdminController(mockRepo)
	adminUserCtrl := controllers.NewAdminUserController(mockAdminUser, newMockAuthTokenRepo(), nil)
	feedbackCtrl := controllers.NewFeedbackController(mockFeedback)
	weightCtrl := controllers.NewWeightController(mockWeight, nil)
	goalsCtrl := controllers.NewGoalsController(newMockGoalRepository())
	validator := utils.NewValidator()
	proc, upl := newFakeImagePipeline()
	h := NewHandler(
		authCtrl, authRecoveryCtrl, entryCtrl, adminCtrl, adminUserCtrl,
		feedbackCtrl, weightCtrl,
		goalsCtrl,
		mockUser, jwtService, validator,
		proc, upl, DefaultExerciseImageConfig,
	)
	return h, mockUser, e
}

// TestProfileUpdate_ReminderOff_StoresPreferences asserts that
// the off-frequency path round-trips through the repo with
// ReminderEnabled = false. The /profile form is the only
// place the user changes these prefs, so a future regression
// that drops the field on the route is the tripwire.
func TestProfileUpdate_ReminderOff_StoresPreferences(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	form.Set("reminder_frequency", "off")
	form.Set("reminder_time", "09:00")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	got, err := mockUser.GetUserByID("user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.ReminderEnabled {
		t.Error("ReminderEnabled = true, want false (no master-switch submitted)")
	}
	if got.ReminderFrequency != models.ReminderOff {
		t.Errorf("ReminderFrequency = %q, want %q", got.ReminderFrequency, models.ReminderOff)
	}
	if got.ReminderTime != "09:00" {
		t.Errorf("ReminderTime = %q, want %q", got.ReminderTime, "09:00")
	}
}

// TestProfileUpdate_WeeklyReminder_ComputesNextFire asserts
// that a weekly Sunday 09:00 reminder saves a next_fire_at
// that matches the user's ComputeNextFire for that
// configuration. The route is the only place next_fire_at is
// written outside the orchestrator, so this is the
// tripwire for a regression that drops the advance.
func TestProfileUpdate_WeeklyReminder_ComputesNextFire(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	createdAt := time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC) // Sunday
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", CreatedAt: createdAt,
		},
	}
	// Pin the clock so the next-fire math is deterministic.
	pinned := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC) // Wednesday
	h.clock = &fixedClock{t: pinned}

	day := 0
	_ = day
	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	form.Set("reminder_enabled", "1")
	form.Set("reminder_frequency", "weekly")
	form.Set("reminder_day_of_week", "0")
	form.Set("reminder_time", "09:00")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}

	got, err := mockUser.GetUserByID("user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !got.ReminderEnabled {
		t.Error("ReminderEnabled = false, want true")
	}
	if got.ReminderFrequency != models.ReminderWeekly {
		t.Errorf("ReminderFrequency = %q, want %q", got.ReminderFrequency, models.ReminderWeekly)
	}
	if got.ReminderDayOfWeek == nil || *got.ReminderDayOfWeek != 0 {
		t.Errorf("ReminderDayOfWeek = %v, want 0", got.ReminderDayOfWeek)
	}
	if got.ReminderTime != "09:00" {
		t.Errorf("ReminderTime = %q, want %q", got.ReminderTime, "09:00")
	}
	// ComputeNextFire for weekly Sunday 09:00 with
	// now=Wednesday 10:00 returns the upcoming Sunday at
	// 09:00 UTC, which is the very next Sunday 4 days
	// later.
	want := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	if got.ReminderNextFireAt == nil || !got.ReminderNextFireAt.Equal(want) {
		t.Errorf("ReminderNextFireAt = %v, want %v", got.ReminderNextFireAt, want)
	}
}

// TestProfileUpdate_RejectsBadTimeFormat asserts the route
// rejects a malformed reminder_time with a friendly error
// (so a hand-rolled POST cannot smuggle a non-hour value
// into the column).
func TestProfileUpdate_RejectsBadTimeFormat(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	form.Set("reminder_enabled", "1")
	form.Set("reminder_frequency", "daily")
	form.Set("reminder_time", "09:30") // minute precision is not accepted
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("UpdateProfile returned unexpected error: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Reminder time must be on the hour") {
		t.Errorf("expected 'Reminder time must be on the hour' error in body, got: %q", body)
	}
}

// TestProfileUpdate_RejectsInvalidFrequency asserts the
// route rejects an unknown frequency value (e.g. a
// hand-rolled POST with frequency=yearly).
func TestProfileUpdate_RejectsInvalidFrequency(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	form.Set("reminder_enabled", "1")
	form.Set("reminder_frequency", "yearly") // not in the enum
	form.Set("reminder_time", "09:00")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("UpdateProfile returned unexpected error: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "failed validation") {
		t.Errorf("expected validation failure in body, got: %q", body)
	}
}

// TestProfileUpdate_DailyReminder_NoDayOfWeek asserts that
// a daily reminder round-trips with a nil DayOfWeek. The
// form hides the picker for daily, so the user never
// submits a value; the route must not store a meaningless 0.
func TestProfileUpdate_DailyReminder_NoDayOfWeek(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	form.Set("reminder_enabled", "1")
	form.Set("reminder_frequency", "daily")
	form.Set("reminder_time", "07:00")
	req := httptest.NewRequest(http.MethodPost, "/profile", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "user-1", "test@example.com", "Test User", false)

	if err := h.UpdateProfile(c); err != nil {
		t.Fatalf("UpdateProfile: %v", err)
	}
	got, err := mockUser.GetUserByID("user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.ReminderDayOfWeek != nil {
		t.Errorf("ReminderDayOfWeek = %d, want nil for daily", *got.ReminderDayOfWeek)
	}
}

// fixedClock is a tiny test-only time source for the
// profile route's "now" computation. Same shape as the
// reminders package's helper; declared in this file so the
// test does not depend on the reminders package.
type fixedClock struct{ t time.Time }

func (f *fixedClock) Now() time.Time { return f.t }

