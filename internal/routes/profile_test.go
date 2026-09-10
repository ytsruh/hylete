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
		controllers.NewHealthSnapshotController(newMockHealthSnapshotRepository()),
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

// TestProfileUpdate_ProfileFields asserts height/gender/date-of-birth
// round-trip through the /profile form into the user row. All three are
// optional; this posts every field set.
func TestProfileUpdate_ProfileFields(t *testing.T) {
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
	form.Set("height_cm", "180.5")
	form.Set("gender", "female")
	form.Set("date_of_birth", "1996-03-04")
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
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}

	got, err := mockUser.GetUserByID("user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.HeightCm == nil || *got.HeightCm != 180.5 {
		t.Errorf("HeightCm = %v, want 180.5", got.HeightCm)
	}
	if got.Gender != "female" {
		t.Errorf("Gender = %q, want female", got.Gender)
	}
	if got.DateOfBirth == nil || *got.DateOfBirth != "1996-03-04" {
		t.Errorf("DateOfBirth = %v, want 1996-03-04", got.DateOfBirth)
	}
}

// TestProfileUpdate_ClearProfileFields asserts empty height/DOB inputs
// clear the stored values (NULL) and an empty gender clears to unset.
func TestProfileUpdate_ClearProfileFields(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	height := 170.0
	dob := "1986-05-06"
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", HeightCm: &height, Gender: "male", DateOfBirth: &dob,
			CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	// height_cm / date_of_birth omitted → cleared; gender omitted → unset.
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
	got, err := mockUser.GetUserByID("user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.HeightCm != nil {
		t.Errorf("HeightCm = %v, want nil after clear", *got.HeightCm)
	}
	if got.Gender != "" {
		t.Errorf("Gender = %q, want empty after clear", got.Gender)
	}
	if got.DateOfBirth != nil {
		t.Errorf("DateOfBirth = %v, want nil after clear", *got.DateOfBirth)
	}
}

// TestProfileUpdate_RejectsBadProfileFields asserts malformed
// height/DOB values are rejected with a friendly error. The clock is
// pinned so the future-date and min-age cases are deterministic.
func TestProfileUpdate_RejectsBadProfileFields(t *testing.T) {
	for _, tc := range []struct {
		name  string
		field string
		value string
		want  string
	}{
		{"height too high", "height_cm", "400", "at most 300"},
		{"height not a number", "height_cm", "tall", "Height must be a valid positive number"},
		{"dob malformed", "date_of_birth", "old", "not a valid YYYY-MM-DD date"},
		{"dob impossible", "date_of_birth", "1996-02-30", "not a valid YYYY-MM-DD date"},
		{"dob too early", "date_of_birth", "1899-12-31", "before 1900-01-01"},
		{"dob in future", "date_of_birth", "2026-09-11", "in the future"},
		{"dob too young", "date_of_birth", "2020-01-01", "at least 10 years old"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h, mockUser2, e := profileTestHarness(t)
			// Pin the clock: the future-date and min-age DOB
			// cases are relative to "today".
			h.clock = &fixedClock{t: time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)}
			mockUser2.users = []models.User{
				{
					ID: "user-1", Name: "Test User", Email: "test@example.com",
					PasswordHash: "hash", CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
				},
			}
			form := url.Values{}
			form.Set("name", "Test User")
			form.Set("weight_unit", "kg")
			form.Set(tc.field, tc.value)
			form.Set("reminder_frequency", "off")
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
			if !strings.Contains(body, tc.want) {
				t.Errorf("expected %q in body, got: %q", tc.want, body)
			}
		})
	}
}

// TestProfileUpdate_UnknownGenderNormalizesToUnset asserts an
// unrecognised gender form value is normalised to unset rather than
// rejected (the form normalises before validation by design; the
// JSON API still rejects unknown values via its oneof tag).
func TestProfileUpdate_UnknownGenderNormalizesToUnset(t *testing.T) {
	h, mockUser, e := profileTestHarness(t)
	mockUser.users = []models.User{
		{
			ID: "user-1", Name: "Test User", Email: "test@example.com",
			PasswordHash: "hash", Gender: "male",
			CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	form := url.Values{}
	form.Set("name", "Test User")
	form.Set("weight_unit", "kg")
	form.Set("gender", "unknown")
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
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body.String())
	}
	got, err := mockUser.GetUserByID("user-1")
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.Gender != "" {
		t.Errorf("Gender = %q, want empty (normalised)", got.Gender)
	}
}

// fixedClock is a tiny test-only time source for the
// profile route's "now" computation. Same shape as the
// reminders package's helper; declared in this file so the
// test does not depend on the reminders package.
type fixedClock struct{ t time.Time }

func (f *fixedClock) Now() time.Time { return f.t }

