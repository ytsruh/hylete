package models

import (
	"context"
	"testing"
	"time"

	"hylete/internal/db"
)

// userTestHarness returns a connected in-memory DB plus a UserRepository.
// Closing the returned database is the caller's responsibility.
func userTestHarness(t *testing.T) (*UserRepository, *db.DB) {
	t.Helper()

	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("in-memory db: %v", err)
	}

	return NewUserRepository(database), database
}

func TestUserRepository_CreateAndGet_RoundTripsWeightFields(t *testing.T) {
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{
		Name:         "Round Trip",
		Email:        "round-trip@example.com",
		PasswordHash: "hash",
	}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// CreateUser doesn't write the goal/unit columns — those are set via
	// the profile form (UpdateUser). Set them now and assert round-trip.
	target := 75.0
	created.TargetWeight = &target
	created.WeightUnit = "lbs"
	if err := repo.UpdateUser(created); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	// GetUserByEmail round-trips both new fields.
	got, err := repo.GetUserByEmail(created.Email)
	if err != nil {
		t.Fatalf("GetUserByEmail: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.TargetWeight == nil {
		t.Fatal("expected TargetWeight to be non-nil after round-trip")
	}
	if *got.TargetWeight != 75.0 {
		t.Errorf("TargetWeight = %v, want 75.0", *got.TargetWeight)
	}
	if got.WeightUnit != "lbs" {
		t.Errorf("WeightUnit = %q, want %q", got.WeightUnit, "lbs")
	}
	if !got.HasWeightGoal() {
		t.Error("expected HasWeightGoal() to be true after round-trip")
	}

	// GetUserByID returns the same shape.
	byID, err := repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if byID == nil || byID.TargetWeight == nil || *byID.TargetWeight != 75.0 || byID.WeightUnit != "lbs" {
		t.Errorf("GetUserByID lost weight fields: %+v", byID)
	}
}

func TestUserRepository_DefaultsWeightFields(t *testing.T) {
	// A user created with no explicit goal/unit should come back with a nil
	// target and the default "kg" unit (matches the SQL DEFAULT and the
	// route's defaultWeightUnit).
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{
		Name:         "Defaults",
		Email:        "defaults@example.com",
		PasswordHash: "hash",
	}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	got, err := repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.TargetWeight != nil {
		t.Errorf("expected nil TargetWeight, got %v", *got.TargetWeight)
	}
	if got.HasWeightGoal() {
		t.Error("expected HasWeightGoal() to be false when target is nil")
	}
	if got.WeightUnit != "kg" {
		t.Errorf("WeightUnit = %q, want %q (SQL default)", got.WeightUnit, "kg")
	}
}

func TestUserRepository_UpdateUser_PersistsWeightFields(t *testing.T) {
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{
		Name:         "Updater",
		Email:        "updater@example.com",
		PasswordHash: "hash",
	}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	// Set a goal + unit.
	first := 80.0
	created.TargetWeight = &first
	created.WeightUnit = "lbs"
	if err := repo.UpdateUser(created); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err := repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got == nil || got.TargetWeight == nil || *got.TargetWeight != 80.0 || got.WeightUnit != "lbs" {
		t.Fatalf("after first update: %+v", got)
	}

	// Clear the goal and switch unit back to kg. The route submits a
	// nil *float64 when the user empties the field, so the repo must
	// write SQL NULL in that case (not 0.0).
	created.TargetWeight = nil
	created.WeightUnit = "kg"
	if err := repo.UpdateUser(created); err != nil {
		t.Fatalf("UpdateUser: %v", err)
	}

	got, err = repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected user, got nil")
	}
	if got.TargetWeight != nil {
		t.Errorf("expected nil TargetWeight after clearing, got %v", *got.TargetWeight)
	}
	if got.HasWeightGoal() {
		t.Error("expected HasWeightGoal() to be false after clearing the goal")
	}
	if got.WeightUnit != "kg" {
		t.Errorf("WeightUnit = %q, want %q", got.WeightUnit, "kg")
	}
}

func TestUser_HasWeightGoal(t *testing.T) {
	target := 70.0
	cases := []struct {
		name string
		user User
		want bool
	}{
		{name: "nil target", user: User{}, want: false},
		{name: "set target", user: User{TargetWeight: &target}, want: true},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.user.HasWeightGoal(); got != tt.want {
				t.Errorf("HasWeightGoal() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- Reminder preferences ---

func TestUserRepository_UpdateUserReminder_RoundTrips(t *testing.T) {
	// A full prefs update followed by GetUserByID must return the
	// exact same values. The mapping has to survive the round-trip
	// because the /profile form re-renders from the same row.
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{
		Name:         "Reminder Round Trip",
		Email:        "reminder-rt@example.com",
		PasswordHash: "hash",
	}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	day := 3 // Wednesday
	next := time.Date(2026, 8, 12, 18, 0, 0, 0, time.UTC)
	prefs := ReminderPreferences{
		Enabled:      true,
		Frequency:    ReminderBiweekly,
		DayOfWeek:    &day,
		Time:         "18:00",
		NextFireAt:   &next,
	}
	if err := repo.UpdateUserReminder(created.ID, prefs); err != nil {
		t.Fatalf("UpdateUserReminder: %v", err)
	}

	got, err := repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if !got.ReminderEnabled {
		t.Error("ReminderEnabled = false, want true")
	}
	if got.ReminderFrequency != ReminderBiweekly {
		t.Errorf("ReminderFrequency = %q, want %q", got.ReminderFrequency, ReminderBiweekly)
	}
	if got.ReminderDayOfWeek == nil || *got.ReminderDayOfWeek != 3 {
		t.Errorf("ReminderDayOfWeek = %v, want 3", got.ReminderDayOfWeek)
	}
	if got.ReminderTime != "18:00" {
		t.Errorf("ReminderTime = %q, want %q", got.ReminderTime, "18:00")
	}
	if got.ReminderNextFireAt == nil || !got.ReminderNextFireAt.Equal(next) {
		t.Errorf("ReminderNextFireAt = %v, want %v", got.ReminderNextFireAt, next)
	}
}

func TestUserRepository_UpdateUserReminder_RejectsBadFrequency(t *testing.T) {
	// A malformed frequency must fail before hitting the DB. This
	// is the trust-boundary check: the form picker can only emit the
	// four known values, but a hand-rolled POST should not silently
	// write a typo.
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{Name: "x", Email: "x@example.com", PasswordHash: "h"}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	err := repo.UpdateUserReminder(created.ID, ReminderPreferences{
		Enabled:   true,
		Frequency: ReminderFrequency("yearly"),
		Time:      "09:00",
	})
	if err == nil {
		t.Error("expected error for invalid frequency, got nil")
	}
}

func TestUserRepository_UpdateUserReminder_DayOfWeekNullable(t *testing.T) {
	// Daily reminders should round-trip with a nil DayOfWeek
	// (the form does not show the picker, so it does not submit a
	// value). The repo must write SQL NULL, not 0 (which the
	// picker would re-render as Sunday).
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{Name: "x", Email: "x@example.com", PasswordHash: "h"}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if err := repo.UpdateUserReminder(created.ID, ReminderPreferences{
		Enabled:      true,
		Frequency:    ReminderDaily,
		Time:         "07:00",
	}); err != nil {
		t.Fatalf("UpdateUserReminder: %v", err)
	}

	got, err := repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.ReminderDayOfWeek != nil {
		t.Errorf("ReminderDayOfWeek = %d, want nil for daily", *got.ReminderDayOfWeek)
	}
}

func TestUserRepository_ListUsersDueForReminder(t *testing.T) {
	// Three users: one due, one not due, one off. The query must
	// return only the due row, regardless of the off row's NULL
	// next_fire_at or the future-dated row.
	repo, database := userTestHarness(t)
	defer database.Close()

	now := time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC)
	past := now.Add(-1 * time.Hour)
	future := now.Add(24 * time.Hour)
	day := 3

	mk := func(name, email string, prefs ReminderPreferences) string {
		u := &User{Name: name, Email: email, PasswordHash: "h"}
		if err := repo.CreateUser(u); err != nil {
			t.Fatalf("CreateUser %s: %v", name, err)
		}
		if err := repo.UpdateUserReminder(u.ID, prefs); err != nil {
			t.Fatalf("UpdateUserReminder %s: %v", name, err)
		}
		return u.ID
	}
	dueID := mk("Due", "due@example.com", ReminderPreferences{
		Enabled: true, Frequency: ReminderWeekly, DayOfWeek: &day,
		Time: "10:00", NextFireAt: &past,
	})
	_ = mk("Future", "future@example.com", ReminderPreferences{
		Enabled: true, Frequency: ReminderWeekly, DayOfWeek: &day,
		Time: "10:00", NextFireAt: &future,
	})
	_ = mk("Off", "off@example.com", ReminderPreferences{
		Enabled: false, Frequency: ReminderOff, Time: "10:00",
	})

	users, err := repo.ListUsersDueForReminder(context.Background(), now)
	if err != nil {
		t.Fatalf("ListUsersDueForReminder: %v", err)
	}
	if len(users) != 1 {
		t.Fatalf("len(users) = %d, want 1", len(users))
	}
	if users[0].ID != dueID {
		t.Errorf("users[0].ID = %q, want %q", users[0].ID, dueID)
	}
}

func TestUserRepository_MarkUserReminderFired_AdvancesNextFire(t *testing.T) {
	// After the orchestrator fires, the tick must not pick the
	// user up again until the new next_fire_at passes. The repo's
	// MarkUserReminderFired is the only path that advances it.
	repo, database := userTestHarness(t)
	defer database.Close()

	created := &User{Name: "Fired", Email: "fired@example.com", PasswordHash: "h"}
	if err := repo.CreateUser(created); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	day := 0
	first := time.Date(2026, 8, 9, 9, 0, 0, 0, time.UTC)
	if err := repo.UpdateUserReminder(created.ID, ReminderPreferences{
		Enabled: true, Frequency: ReminderWeekly, DayOfWeek: &day,
		Time: "09:00", NextFireAt: &first,
	}); err != nil {
		t.Fatalf("UpdateUserReminder: %v", err)
	}

	second := time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC)
	firedAt := time.Date(2026, 8, 9, 9, 1, 0, 0, time.UTC)
	if err := repo.MarkUserReminderFired(context.Background(), created.ID, firedAt, second); err != nil {
		t.Fatalf("MarkUserReminderFired: %v", err)
	}

	got, err := repo.GetUserByID(created.ID)
	if err != nil {
		t.Fatalf("GetUserByID: %v", err)
	}
	if got.ReminderNextFireAt == nil || !got.ReminderNextFireAt.Equal(second) {
		t.Errorf("ReminderNextFireAt = %v, want %v", got.ReminderNextFireAt, second)
	}
	if got.ReminderLastFiredAt == nil || !got.ReminderLastFiredAt.Equal(firedAt) {
		t.Errorf("ReminderLastFiredAt = %v, want %v", got.ReminderLastFiredAt, firedAt)
	}

	// And the "due now" query at the original fire time must no
	// longer pick this user up.
	due, err := repo.ListUsersDueForReminder(context.Background(), firedAt)
	if err != nil {
		t.Fatalf("ListUsersDueForReminder: %v", err)
	}
	for _, u := range due {
		if u.ID == created.ID {
			t.Error("user still appears due after MarkUserReminderFired advanced next_fire_at")
		}
	}
}
