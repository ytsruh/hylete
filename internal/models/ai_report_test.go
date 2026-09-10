package models

import (
	"context"
	"testing"
	"time"

	"hylete/internal/db"
)

// aiReportTestHarness returns an in-memory DB plus an AIReportRepository.
func aiReportTestHarness(t *testing.T) (*AIReportRepository, *db.DB) {
	t.Helper()
	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("in-memory db: %v", err)
	}
	return NewAIReportRepository(database), database
}

func seedAIUser(t *testing.T, database *db.DB) string {
	t.Helper()
	userRepo := NewUserRepository(database)
	u := &User{Name: "Coach User", Email: "coach@example.com", PasswordHash: "hash"}
	if err := userRepo.CreateUser(u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	return u.ID
}

func TestAIReportRepository_CreateGetLatestList(t *testing.T) {
	repo, database := aiReportTestHarness(t)
	defer database.Close()
	userID := seedAIUser(t, database)

	mk := func(day int) *AIReport {
		return &AIReport{
			UserID: userID, Type: AIReportTypeWeekly,
			PeriodStart:   time.Date(2026, 8, day, 0, 0, 0, 0, time.UTC),
			PeriodEnd:     time.Date(2026, 8, day+6, 0, 0, 0, 0, time.UTC),
			PromptVersion: "v1", Model: "test",
			PayloadJSON: `{"summary":"x","recommendations":["A"]}`,
		}
	}
	first, second := mk(3), mk(10)
	if err := repo.Create(first); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if first.ID == "" {
		t.Fatal("expected generated ID")
	}
	if err := repo.Create(second); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Get by composite key (cron idempotency read).
	got, err := repo.Get(userID, AIReportTypeWeekly, second.PeriodStart)
	if err != nil || got == nil || got.ID != second.ID {
		t.Fatalf("Get: %+v %v", got, err)
	}
	// Unknown key → nil, nil.
	missing, err := repo.Get(userID, AIReportTypeWeekly, time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil || missing != nil {
		t.Fatalf("expected nil: %+v %v", missing, err)
	}

	// Latest returns the newest week.
	latest, err := repo.Latest(userID, AIReportTypeWeekly)
	if err != nil || latest == nil || latest.ID != second.ID {
		t.Fatalf("Latest: %+v %v", latest, err)
	}

	// List newest-first with limit.
	list, err := repo.List(userID, AIReportTypeWeekly, 1)
	if err != nil || len(list) != 1 || list[0].ID != second.ID {
		t.Fatalf("List: %+v %v", list, err)
	}

	// GetByID scoping: wrong user → nil.
	other, err := repo.GetByID(second.ID, "someone-else")
	if err != nil || other != nil {
		t.Fatalf("expected scoped nil: %+v %v", other, err)
	}
}

func TestAIReportRepository_ReadDismiss(t *testing.T) {
	repo, database := aiReportTestHarness(t)
	defer database.Close()
	userID := seedAIUser(t, database)
	r := &AIReport{
		UserID: userID, Type: AIReportTypeWeekly,
		PeriodStart: time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		PeriodEnd:   time.Date(2026, 8, 9, 0, 0, 0, 0, time.UTC),
		PayloadJSON: `{}`,
	}
	if err := repo.Create(r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.MarkRead(r.ID, userID); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
	if err := repo.MarkDismissed(r.ID, userID); err != nil {
		t.Fatalf("MarkDismissed: %v", err)
	}
	got, err := repo.GetByID(r.ID, userID)
	if err != nil || got == nil || !got.IsRead() || !got.IsDismissed() {
		t.Fatalf("stamps: %+v %v", got, err)
	}
}

func TestUserRepository_AIPreferencesRoundTrip(t *testing.T) {
	repo, database := userTestHarness(t)
	defer database.Close()
	u := &User{Name: "AI Prefs", Email: "ai-prefs@example.com", PasswordHash: "hash"}
	if err := repo.CreateUser(u); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	// Defaults: opted out, empty aim.
	got, err := repo.GetUserByID(u.ID)
	if err != nil || got == nil {
		t.Fatalf("GetUserByID: %+v %v", got, err)
	}
	if got.AIOptIn || got.AIGoalText != "" {
		t.Fatalf("defaults: %+v", got)
	}
	// Write + round-trip.
	if err := repo.UpdateUserAIPreferences(u.ID, true, "Bench 100kg by December"); err != nil {
		t.Fatalf("UpdateUserAIPreferences: %v", err)
	}
	got, err = repo.GetUserByID(u.ID)
	if err != nil || got == nil || !got.AIOptIn || got.AIGoalText != "Bench 100kg by December" {
		t.Fatalf("round-trip: %+v %v", got, err)
	}
	// Over-length rejected.
	long := make([]byte, AIGoalTextMaxLength+1)
	for i := range long {
		long[i] = 'x'
	}
	if err := repo.UpdateUserAIPreferences(u.ID, true, string(long)); err == nil {
		t.Fatal("expected length error")
	}
	// Cron listing sees the opted-in user only.
	opted, err := repo.ListAIOptedInUsers(context.Background())
	if err != nil || len(opted) != 1 || opted[0].ID != u.ID {
		t.Fatalf("ListAIOptedInUsers: %+v %v", opted, err)
	}
}
