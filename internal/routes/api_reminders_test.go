package routes

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"
)

// TestAPIReminders_GetDefaults asserts a never-touched row reads as
// reminders-off with the 09:00 default time, so the iOS editor always
// has a usable first-paint state.
func TestAPIReminders_GetDefaults(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "rem-default@example.com", "Rem")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/me/reminders", token, nil)
	got := decodeAPI[ReminderPreferencesDTO](t, rec, http.StatusOK)
	if got.Enabled {
		t.Error("Enabled = true, want false for a fresh row")
	}
	if got.Frequency != string(models.ReminderOff) {
		t.Errorf("Frequency = %q, want %q", got.Frequency, models.ReminderOff)
	}
	if got.Time != "09:00" {
		t.Errorf("Time = %q, want %q", got.Time, "09:00")
	}
	if got.DayOfWeek != nil {
		t.Errorf("DayOfWeek = %d, want nil for a fresh row", *got.DayOfWeek)
	}
}

// TestAPIReminders_GetStored asserts GET reflects previously stored
// preferences (written directly to the mock repo).
func TestAPIReminders_GetStored(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, _ := loginUser(t, h, mockUser, "rem-stored@example.com", "Rem")
	day := 1
	mockUser.users[len(mockUser.users)-1].ReminderEnabled = true
	mockUser.users[len(mockUser.users)-1].ReminderFrequency = models.ReminderWeekly
	mockUser.users[len(mockUser.users)-1].ReminderDayOfWeek = &day
	mockUser.users[len(mockUser.users)-1].ReminderTime = "07:00"

	rec := apiDo(t, e, http.MethodGet, "/api/v1/me/reminders", token, nil)
	got := decodeAPI[ReminderPreferencesDTO](t, rec, http.StatusOK)
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if got.Frequency != string(models.ReminderWeekly) {
		t.Errorf("Frequency = %q, want weekly", got.Frequency)
	}
	if got.DayOfWeek == nil || *got.DayOfWeek != 1 {
		t.Errorf("DayOfWeek = %v, want 1", got.DayOfWeek)
	}
	if got.Time != "07:00" {
		t.Errorf("Time = %q, want 07:00", got.Time)
	}
}

// TestAPIReminders_UpdateWeekly_ComputesNextFire asserts a weekly
// PUT persists the prefs and advances next_fire_at via ComputeNextFire
// (pinned clock keeps the assertion deterministic).
func TestAPIReminders_UpdateWeekly_ComputesNextFire(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, user := loginUser(t, h, mockUser, "rem-weekly@example.com", "Rem")
	// Anchor CreatedAt to a Sunday; pin "now" to a Wednesday so the
	// next Sunday 09:00 UTC is unambiguous.
	for i := range mockUser.users {
		if mockUser.users[i].ID == user.ID {
			mockUser.users[i].CreatedAt = time.Date(2024, 1, 7, 0, 0, 0, 0, time.UTC)
		}
	}
	h.clock = &fixedClock{t: time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)} // Wednesday

	day := 0
	rec := apiDo(t, e, http.MethodPut, "/api/v1/me/reminders", token, UpdateReminderPreferencesRequest{
		Enabled: true, Frequency: "weekly", DayOfWeek: &day, Time: "09:00",
	})
	got := decodeAPI[ReminderPreferencesDTO](t, rec, http.StatusOK)
	if !got.Enabled || got.Frequency != "weekly" || got.DayOfWeek == nil || *got.DayOfWeek != 0 || got.Time != "09:00" {
		t.Fatalf("response = %+v, want enabled weekly Sun 09:00", got)
	}

	stored, err := mockUser.GetUserByID(user.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	want := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	if stored.ReminderNextFireAt == nil || !stored.ReminderNextFireAt.Equal(want) {
		t.Errorf("ReminderNextFireAt = %v, want %v", stored.ReminderNextFireAt, want)
	}
}

// TestAPIReminders_UpdateDaily_NilsDayOfWeek asserts a daily PUT drops
// any submitted day-of-week so the row never carries a meaningless 0.
func TestAPIReminders_UpdateDaily_NilsDayOfWeek(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, user := loginUser(t, h, mockUser, "rem-daily@example.com", "Rem")

	day := 3
	rec := apiDo(t, e, http.MethodPut, "/api/v1/me/reminders", token, UpdateReminderPreferencesRequest{
		Enabled: true, Frequency: "daily", DayOfWeek: &day, Time: "07:00",
	})
	got := decodeAPI[ReminderPreferencesDTO](t, rec, http.StatusOK)
	if got.DayOfWeek != nil {
		t.Errorf("DayOfWeek = %d, want nil for daily", *got.DayOfWeek)
	}
	stored, _ := mockUser.GetUserByID(user.ID)
	if stored.ReminderDayOfWeek != nil {
		t.Errorf("stored DayOfWeek = %d, want nil for daily", *stored.ReminderDayOfWeek)
	}
}

// TestAPIReminders_UpdateOff_ClearsNextFire asserts disabling reminders
// clears next_fire_at (ComputeNextFire returns false for off).
func TestAPIReminders_UpdateOff_ClearsNextFire(t *testing.T) {
	h, _, mockUser, e := setupHandler(t)
	token, user := loginUser(t, h, mockUser, "rem-off@example.com", "Rem")

	rec := apiDo(t, e, http.MethodPut, "/api/v1/me/reminders", token, UpdateReminderPreferencesRequest{
		Enabled: false, Frequency: "off", Time: "09:00",
	})
	decodeAPI[ReminderPreferencesDTO](t, rec, http.StatusOK)
	stored, _ := mockUser.GetUserByID(user.ID)
	if stored.ReminderNextFireAt != nil {
		t.Errorf("ReminderNextFireAt = %v, want nil for off", stored.ReminderNextFireAt)
	}
}

// TestAPIReminders_UpdateRejectsBadInput table-drives the 400 paths:
// unknown frequency, out-of-range day, and non-hour time.
func TestAPIReminders_UpdateRejectsBadInput(t *testing.T) {
	badDay := 9
	goodDay := 1
	tests := []struct {
		name string
		body UpdateReminderPreferencesRequest
		want string
	}{
		{"bad frequency", UpdateReminderPreferencesRequest{Enabled: true, Frequency: "yearly", Time: "09:00"}, "frequency"},
		{"bad day", UpdateReminderPreferencesRequest{Enabled: true, Frequency: "weekly", DayOfWeek: &badDay, Time: "09:00"}, "dayofweek"},
		{"bad time", UpdateReminderPreferencesRequest{Enabled: true, Frequency: "daily", DayOfWeek: &goodDay, Time: "09:30"}, "on the hour"},
		{"empty time", UpdateReminderPreferencesRequest{Enabled: true, Frequency: "daily", Time: ""}, "required"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h, _, mockUser, e := setupHandler(t)
			token, _ := loginUser(t, h, mockUser, "rem-bad@example.com", "Rem")
			rec := apiDo(t, e, http.MethodPut, "/api/v1/me/reminders", token, tt.body)
			errBody := decodeAPIError(t, rec, http.StatusBadRequest)
			if errBody.Error == "" {
				t.Fatal("expected non-empty error message")
			}
			if !strings.Contains(strings.ToLower(errBody.Error), strings.ToLower(tt.want)) {
				t.Errorf("error = %q, want substring %q", errBody.Error, tt.want)
			}
		})
	}
}

// TestAPIReminders_Unauthorized asserts the endpoints are JWT-protected
// (JSON 401, not a redirect) when no token is supplied.
func TestAPIReminders_Unauthorized(t *testing.T) {
	_, _, _, e := setupHandler(t)
	rec := apiDo(t, e, http.MethodGet, "/api/v1/me/reminders", "", nil)
	decodeAPIError(t, rec, http.StatusUnauthorized)
	rec = apiDo(t, e, http.MethodPut, "/api/v1/me/reminders", "", UpdateReminderPreferencesRequest{
		Enabled: true, Frequency: "daily", Time: "09:00",
	})
	decodeAPIError(t, rec, http.StatusUnauthorized)
}
