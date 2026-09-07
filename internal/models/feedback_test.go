package models

import (
	"testing"
	"time"

	"hylete/internal/db"
)

func setupFeedbackTestRepo(t *testing.T) (*FeedbackRepo, *UserRepository, *db.DB, string) {
	t.Helper()

	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory database: %v", err)
	}

	userRepo := NewUserRepository(database)
	feedbackRepo := NewFeedbackRepository(database)

	user := &User{
		Name:         "Test User",
		Email:        "test@example.com",
		PasswordHash: "hash",
	}
	if err := userRepo.CreateUser(user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	return feedbackRepo, userRepo, database, user.ID
}

func TestFeedbackRepository_Create(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	feedback := &Feedback{
		UserID:  userID,
		Title:   "Test Feedback",
		Message: "This is a test feedback message",
	}

	err := repo.Create(feedback)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if feedback.ID == "" {
		t.Fatal("expected non-empty feedback ID after creation")
	}
	if feedback.Title != "Test Feedback" {
		t.Fatalf("expected title 'Test Feedback', got %q", feedback.Title)
	}
	if feedback.Message != "This is a test feedback message" {
		t.Fatalf("expected message 'This is a test feedback message', got %q", feedback.Message)
	}
}

func TestFeedbackRepository_GetAll_Empty(t *testing.T) {
	repo, _, database, _ := setupFeedbackTestRepo(t)
	defer database.Close()

	feedback, err := repo.GetAll("")
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(feedback) != 0 {
		t.Fatalf("expected 0 feedback items, got %d", len(feedback))
	}
}

func TestFeedbackRepository_GetAll_OpenOnly(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	openFeedback := &Feedback{UserID: userID, Title: "Open", Message: "Open message"}
	closedFeedback := &Feedback{UserID: userID, Title: "Closed", Message: "Closed message"}

	if err := repo.Create(openFeedback); err != nil {
		t.Fatalf("Create open failed: %v", err)
	}
	if err := repo.Create(closedFeedback); err != nil {
		t.Fatalf("Create closed failed: %v", err)
	}
	if err := repo.UpdateStatus(closedFeedback.ID, true); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	feedback, err := repo.GetAll("open")
	if err != nil {
		t.Fatalf("GetAll open failed: %v", err)
	}
	if len(feedback) != 1 {
		t.Fatalf("expected 1 open feedback, got %d", len(feedback))
	}
	if feedback[0].Title != "Open" {
		t.Fatalf("expected open feedback title 'Open', got %q", feedback[0].Title)
	}
}

func TestFeedbackRepository_GetAll_ClosedOnly(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	openFeedback := &Feedback{UserID: userID, Title: "Open", Message: "Open message"}
	closedFeedback := &Feedback{UserID: userID, Title: "Closed", Message: "Closed message"}

	if err := repo.Create(openFeedback); err != nil {
		t.Fatalf("Create open failed: %v", err)
	}
	if err := repo.Create(closedFeedback); err != nil {
		t.Fatalf("Create closed failed: %v", err)
	}
	if err := repo.UpdateStatus(closedFeedback.ID, true); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	feedback, err := repo.GetAll("closed")
	if err != nil {
		t.Fatalf("GetAll closed failed: %v", err)
	}
	if len(feedback) != 1 {
		t.Fatalf("expected 1 closed feedback, got %d", len(feedback))
	}
	if feedback[0].Title != "Closed" {
		t.Fatalf("expected closed feedback title 'Closed', got %q", feedback[0].Title)
	}
}

func TestFeedbackRepository_GetAll_All(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	feedback1 := &Feedback{UserID: userID, Title: "First", Message: "First message"}
	feedback2 := &Feedback{UserID: userID, Title: "Second", Message: "Second message"}

	if err := repo.Create(feedback1); err != nil {
		t.Fatalf("Create first failed: %v", err)
	}
	if err := repo.Create(feedback2); err != nil {
		t.Fatalf("Create second failed: %v", err)
	}
	if err := repo.UpdateStatus(feedback2.ID, true); err != nil {
		t.Fatalf("UpdateStatus failed: %v", err)
	}

	feedback, err := repo.GetAll("")
	if err != nil {
		t.Fatalf("GetAll all failed: %v", err)
	}
	if len(feedback) != 2 {
		t.Fatalf("expected 2 feedback items, got %d", len(feedback))
	}
}

func TestFeedbackRepository_GetByID_Found(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	feedback := &Feedback{UserID: userID, Title: "Find Me", Message: "Found message"}
	if err := repo.Create(feedback); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	found, err := repo.GetByID(feedback.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found == nil {
		t.Fatal("expected feedback, got nil")
	}
	if found.Title != "Find Me" {
		t.Fatalf("expected title 'Find Me', got %q", found.Title)
	}
}

func TestFeedbackRepository_GetByID_NotFound(t *testing.T) {
	repo, _, database, _ := setupFeedbackTestRepo(t)
	defer database.Close()

	found, err := repo.GetByID("non-existent-id")
	if err != nil {
		t.Fatalf("GetByID unexpected error: %v", err)
	}
	if found != nil {
		t.Fatalf("expected nil for non-existing feedback, got %+v", found)
	}
}

func TestFeedbackRepository_UpdateStatus_Close(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	feedback := &Feedback{UserID: userID, Title: "To Close", Message: "Closing message"}
	if err := repo.Create(feedback); err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if err := repo.UpdateStatus(feedback.ID, true); err != nil {
		t.Fatalf("UpdateStatus close failed: %v", err)
	}

	found, err := repo.GetByID(feedback.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if !found.IsClosed {
		t.Fatal("expected IsClosed to be true")
	}
}

func TestFeedbackRepository_UpdateStatus_Reopen(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	feedback := &Feedback{UserID: userID, Title: "To Reopen", Message: "Reopen message"}
	if err := repo.Create(feedback); err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if err := repo.UpdateStatus(feedback.ID, true); err != nil {
		t.Fatalf("UpdateStatus close failed: %v", err)
	}

	if err := repo.UpdateStatus(feedback.ID, false); err != nil {
		t.Fatalf("UpdateStatus reopen failed: %v", err)
	}

	found, err := repo.GetByID(feedback.ID)
	if err != nil {
		t.Fatalf("GetByID failed: %v", err)
	}
	if found.IsClosed {
		t.Fatal("expected IsClosed to be false after reopen")
	}
}

func TestFeedbackRepository_GetAll_OrderedByCreatedAtDesc(t *testing.T) {
	repo, _, database, userID := setupFeedbackTestRepo(t)
	defer database.Close()

	older := &Feedback{UserID: userID, Title: "Older", Message: "Older message", CreatedAt: time.Now().Add(-time.Hour)}
	newer := &Feedback{UserID: userID, Title: "Newer", Message: "Newer message", CreatedAt: time.Now()}

	if err := repo.Create(older); err != nil {
		t.Fatalf("Create older failed: %v", err)
	}
	if err := repo.Create(newer); err != nil {
		t.Fatalf("Create newer failed: %v", err)
	}

	feedback, err := repo.GetAll("")
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(feedback) != 2 {
		t.Fatalf("expected 2 feedback items, got %d", len(feedback))
	}
	if feedback[0].Title != "Newer" || feedback[1].Title != "Older" {
		t.Fatal("expected feedback ordered by created_at descending")
	}
}
