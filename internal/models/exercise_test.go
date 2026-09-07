package models

import (
	"database/sql"
	"testing"
	"time"

	"hylete/internal/db"
)

func setupTestRepo(t *testing.T) (*ExerciseRepository, *UserRepository, *db.DB, string) {
	t.Helper()

	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory database: %v", err)
	}

	userRepo := NewUserRepository(database)
	exerciseRepo := NewExerciseRepository(database)

	user := &User{
		Name:         "Test User",
		Email:        "test@example.com",
		PasswordHash: "hash",
	}
	if err := userRepo.CreateUser(user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	return exerciseRepo, userRepo, database, user.ID
}

func TestExerciseRepository_Create(t *testing.T) {
	repo, _, database, _ := setupTestRepo(t)
	defer database.Close()

	id, err := repo.Create(nil, "Test Create")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	_, err = repo.Create(nil, "Test Create")
	if err == nil {
		t.Fatal("expected error for duplicate name, got nil")
	}
}

func TestExerciseRepository_Create_WithTx(t *testing.T) {
	repo, _, database, _ := setupTestRepo(t)
	defer database.Close()

	var createdID string
	err := database.Transaction(func(tx *sql.Tx) error {
		id, err := repo.Create(tx, "Tx Exercise")
		createdID = id
		return err
	})
	if err != nil {
		t.Fatalf("transaction failed: %v", err)
	}
	if createdID == "" {
		t.Fatal("expected non-empty id from transaction")
	}
}

func TestExerciseRepository_GetByName(t *testing.T) {
	repo, _, database, _ := setupTestRepo(t)
	defer database.Close()

	_, err := repo.Create(nil, "Test Exercise")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	et, err := repo.GetByName("Test Exercise")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if et == nil {
		t.Fatal("expected exercise, got nil")
	}
	if et.Name != "Test Exercise" {
		t.Fatalf("expected name 'Test Exercise', got %q", et.Name)
	}

	et, err = repo.GetByName("NonExistentExercise")
	if err != nil {
		t.Fatalf("GetByName failed: %v", err)
	}
	if et != nil {
		t.Fatalf("expected nil for non-existing exercise, got %+v", et)
	}
}

func TestExerciseRepository_List(t *testing.T) {
	repo, _, database, _ := setupTestRepo(t)
	defer database.Close()

	_, err := repo.Create(nil, "Alpha")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	_, err = repo.Create(nil, "Beta")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	exercises, err := repo.List()
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}

	if len(exercises) < 2 {
		t.Fatalf("expected at least 2 exercises, got %d", len(exercises))
	}

	if exercises[0].Name > exercises[1].Name {
		t.Fatalf("expected exercises ordered by name, got %q then %q", exercises[0].Name, exercises[1].Name)
	}
}

func TestExerciseRepository_CreateNoTx_RoundTripsImageKeys(t *testing.T) {
	// The admin image upload flow stores two storage keys per
	// exercise (display + original). Both columns must round-trip
	// through CreateNoTx, GetByID, GetByName, List, and Update.
	repo, _, database, _ := setupTestRepo(t)
	defer database.Close()

	id, err := repo.CreateNoTx(CreateExerciseParams{
		Name:           "Image Test",
		Description:    "Has an image",
		VideoURL:       "https://example.com/v.mp4",
		ImgURL:         "exercises/abc.jpg",
		ImgURLOriginal: "exercises/abc_original.jpg",
		Type:           ExerciseTypeStrength,
	})
	if err != nil {
		t.Fatalf("CreateNoTx: %v", err)
	}
	if id == "" {
		t.Fatal("expected non-empty id")
	}

	// GetByID round-trip
	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected exercise, got nil")
	}
	if got.ImgURL != "exercises/abc.jpg" {
		t.Errorf("ImgURL = %q, want exercises/abc.jpg", got.ImgURL)
	}
	if got.ImgURLOriginal != "exercises/abc_original.jpg" {
		t.Errorf("ImgURLOriginal = %q, want exercises/abc_original.jpg", got.ImgURLOriginal)
	}

	// GetByName round-trip (uses the same row -> same select)
	byName, err := repo.GetByName("Image Test")
	if err != nil {
		t.Fatalf("GetByName: %v", err)
	}
	if byName == nil || byName.ImgURL != "exercises/abc.jpg" || byName.ImgURLOriginal != "exercises/abc_original.jpg" {
		t.Errorf("GetByName returned %+v, want ImgURL and ImgURLOriginal set", byName)
	}

	// Update replaces both keys
	updated, err := repo.Update(id, UpdateExerciseParams{
		Name:           "Image Test",
		Description:    "Has an image",
		VideoURL:       "https://example.com/v.mp4",
		ImgURL:         "exercises/xyz.jpg",
		ImgURLOriginal: "exercises/xyz_original.jpg",
		Type:           ExerciseTypeStrength,
	})
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ImgURL != "exercises/xyz.jpg" || updated.ImgURLOriginal != "exercises/xyz_original.jpg" {
		t.Errorf("Update returned %+v, want updated keys", updated)
	}

	// List sees the new keys.
	list, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	found := false
	for _, e := range list {
		if e.ID == id {
			found = true
			if e.ImgURL != "exercises/xyz.jpg" || e.ImgURLOriginal != "exercises/xyz_original.jpg" {
				t.Errorf("List row %+v, want updated keys", e)
			}
		}
	}
	if !found {
		t.Error("updated exercise not in List() result")
	}
}

func TestExerciseRepository_CreateNoTx_EmptyImageKeys(t *testing.T) {
	// An exercise created without an image must round-trip
	// empty (NULL) for both columns, not "" strings or zero
	// values. Guards against a future migration accidentally
	// adding a NOT NULL constraint or default.
	repo, _, database, _ := setupTestRepo(t)
	defer database.Close()

	id, err := repo.CreateNoTx(CreateExerciseParams{
		Name: "Plain Exercise",
		Type: ExerciseTypeOther,
	})
	if err != nil {
		t.Fatalf("CreateNoTx: %v", err)
	}
	got, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.ImgURL != "" {
		t.Errorf("ImgURL = %q, want empty", got.ImgURL)
	}
	if got.ImgURLOriginal != "" {
		t.Errorf("ImgURLOriginal = %q, want empty", got.ImgURLOriginal)
	}
}

func TestExerciseRepository_CreateExerciseEntry(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Squat")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	exerciseEntry := &ExerciseEntry{
		ExerciseID: exercise,
		UserID:     userID,
		Reps:       5,
		Weight:     140.0,
		Notes:      "Test exercise entry",
		CreatedAt:  time.Now(),
	}

	err = repo.CreateExerciseEntry(exerciseEntry)
	if err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}
	if exerciseEntry.ID == "" {
		t.Fatal("expected non-empty exercise entry ID after creation")
	}
}

func TestExerciseRepository_GetExerciseEntry(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Bench Press")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	created := &ExerciseEntry{
		ExerciseID: exercise,
		UserID:     userID,
		Reps:       8,
		Weight:     80.0,
		Notes:      "",
		CreatedAt:  time.Now(),
	}
	if err := repo.CreateExerciseEntry(created); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	exerciseEntry, err := repo.GetExerciseEntry(created.ID, userID)
	if err != nil {
		t.Fatalf("GetExerciseEntry failed: %v", err)
	}
	if exerciseEntry == nil {
		t.Fatal("expected exercise entry, got nil")
	}
	if exerciseEntry.ExerciseName != "Bench Press" {
		t.Fatalf("expected exercise name 'Bench Press', got %q", exerciseEntry.ExerciseName)
	}
	if exerciseEntry.Reps != 8 {
		t.Fatalf("expected reps 8, got %d", exerciseEntry.Reps)
	}
	if exerciseEntry.Weight != 80.0 {
		t.Fatalf("expected weight 80.0, got %f", exerciseEntry.Weight)
	}
}

func TestExerciseRepository_UpdateExerciseEntry(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Deadlift")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	created := &ExerciseEntry{
		ExerciseID: exercise,
		UserID:     userID,
		Reps:       5,
		Weight:     180.0,
		Notes:      "heavy",
		CreatedAt:  time.Now(),
	}
	if err := repo.CreateExerciseEntry(created); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	created.Reps = 3
	created.Weight = 200.0
	created.Notes = "pr"

	if err := repo.UpdateExerciseEntry(created, userID); err != nil {
		t.Fatalf("UpdateExerciseEntry failed: %v", err)
	}

	updated, err := repo.GetExerciseEntry(created.ID, userID)
	if err != nil {
		t.Fatalf("GetExerciseEntry failed: %v", err)
	}
	if updated.Reps != 3 {
		t.Fatalf("expected reps 3, got %d", updated.Reps)
	}
	if updated.Weight != 200.0 {
		t.Fatalf("expected weight 200.0, got %f", updated.Weight)
	}
	if updated.Notes != "pr" {
		t.Fatalf("expected notes 'pr', got %q", updated.Notes)
	}
}

func TestExerciseRepository_UpdateExerciseEntryWithDate(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Overhead Press")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	originalTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	created := &ExerciseEntry{
		ExerciseID: exercise,
		UserID:     userID,
		Reps:       5,
		Weight:     60.0,
		Notes:      "",
		CreatedAt:  originalTime,
	}
	if err := repo.CreateExerciseEntry(created); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	newTime := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	created.Reps = 6
	created.CreatedAt = newTime

	if err := repo.UpdateExerciseEntryWithDate(created, userID); err != nil {
		t.Fatalf("UpdateExerciseEntryWithDate failed: %v", err)
	}

	updated, err := repo.GetExerciseEntry(created.ID, userID)
	if err != nil {
		t.Fatalf("GetExerciseEntry failed: %v", err)
	}
	if updated.Reps != 6 {
		t.Fatalf("expected reps 6, got %d", updated.Reps)
	}
	if !updated.CreatedAt.Equal(newTime) {
		t.Fatalf("expected created_at %v, got %v", newTime, updated.CreatedAt)
	}
}

func TestExerciseRepository_DeleteExerciseEntry(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Pull Up")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	created := &ExerciseEntry{
		ExerciseID: exercise,
		UserID:     userID,
		Reps:       10,
		Weight:     0,
		Notes:      "bw",
		CreatedAt:  time.Now(),
	}
	if err := repo.CreateExerciseEntry(created); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	if err := repo.DeleteExerciseEntry(created.ID, userID); err != nil {
		t.Fatalf("DeleteExerciseEntry failed: %v", err)
	}

	exerciseEntry, err := repo.GetExerciseEntry(created.ID, userID)
	if err != nil {
		t.Fatalf("GetExerciseEntry failed: %v", err)
	}
	if exerciseEntry != nil {
		t.Fatalf("expected nil after delete, got %+v", exerciseEntry)
	}
}

func TestExerciseRepository_ListExerciseEntries(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Dips")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	for i := 0; i < 5; i++ {
		exerciseEntry := &ExerciseEntry{
			ExerciseID: exercise,
			UserID:     userID,
			Reps:       i + 1,
			Weight:     float64(i * 10),
			CreatedAt:  time.Now().Add(time.Duration(i) * time.Hour),
		}
		if err := repo.CreateExerciseEntry(exerciseEntry); err != nil {
			t.Fatalf("CreateExerciseEntry failed: %v", err)
		}
	}

	exerciseEntries, err := repo.ListExerciseEntries(userID, 3)
	if err != nil {
		t.Fatalf("ListExerciseEntries failed: %v", err)
	}
	if len(exerciseEntries) != 3 {
		t.Fatalf("expected 3 exercise entries with limit, got %d", len(exerciseEntries))
	}

	exerciseEntries, err = repo.ListExerciseEntries(userID, 0)
	if err != nil {
		t.Fatalf("ListExerciseEntries failed: %v", err)
	}
	if len(exerciseEntries) != 5 {
		t.Fatalf("expected 5 exercise entries without limit, got %d", len(exerciseEntries))
	}

	if exerciseEntries[0].CreatedAt.Before(exerciseEntries[1].CreatedAt) {
		t.Fatal("expected exercise entries in descending order by created_at")
	}
}

func TestExerciseRepository_GetExerciseEntriesByExercisePaginated(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	squat, err := repo.Create(nil, "Squat")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}
	bench, err := repo.Create(nil, "Bench Press")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	now := time.Now()
	for i := 0; i < 7; i++ {
		if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: squat, UserID: userID, Reps: 5, Weight: float64(100 + i), CreatedAt: now.Add(time.Duration(i) * time.Minute)}); err != nil {
			t.Fatalf("CreateExerciseEntry failed: %v", err)
		}
	}
	// An exercise entry for a different exercise — must not leak into squat pages.
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: bench, UserID: userID, Reps: 8, Weight: 80, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	first, err := repo.GetExerciseEntriesByExercisePaginated(squat, userID, 5, 0)
	if err != nil {
		t.Fatalf("GetExerciseEntriesByExercisePaginated failed: %v", err)
	}
	if len(first) != 5 {
		t.Fatalf("page 1: expected 5 exercise entries, got %d", len(first))
	}
	// Newest first — weight 106 should be the first row.
	if first[0].Weight != 106 {
		t.Errorf("expected newest exercise entry (weight 106) first, got %v", first[0].Weight)
	}
	for _, e := range first {
		if e.ExerciseName != "Squat" {
			t.Errorf("expected exercise name 'Squat', got %q", e.ExerciseName)
		}
		if e.ExerciseID != squat {
			t.Errorf("expected exercise ID %q, got %q", squat, e.ExerciseID)
		}
	}

	second, err := repo.GetExerciseEntriesByExercisePaginated(squat, userID, 5, 5)
	if err != nil {
		t.Fatalf("GetExerciseEntriesByExercisePaginated (page 2) failed: %v", err)
	}
	if len(second) != 2 {
		t.Fatalf("page 2: expected 2 exercise entries, got %d", len(second))
	}
	if second[0].Weight != 101 {
		t.Errorf("expected second page first exercise entry weight 101, got %v", second[0].Weight)
	}

	beyond, err := repo.GetExerciseEntriesByExercisePaginated(squat, userID, 5, 100)
	if err != nil {
		t.Fatalf("GetExerciseEntriesByExercisePaginated (beyond) failed: %v", err)
	}
	if len(beyond) != 0 {
		t.Fatalf("page beyond data: expected 0 exercise entries, got %d", len(beyond))
	}
}

func TestExerciseRepository_GetMaxWeightByExercise(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	squat, err := repo.Create(nil, "Squat")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: squat, UserID: userID, Reps: 5, Weight: 100, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: squat, UserID: userID, Reps: 3, Weight: 140, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: squat, UserID: userID, Reps: 5, Weight: 120, CreatedAt: time.Now()}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	max, err := repo.GetMaxWeightByExercise(squat, userID)
	if err != nil {
		t.Fatalf("GetMaxWeightByExercise failed: %v", err)
	}
	if max != 140 {
		t.Fatalf("expected max weight 140, got %v", max)
	}
}

func TestExerciseRepository_GetLastSetByExercise(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	squat, err := repo.Create(nil, "Squat")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	earlier := time.Now().Add(-2 * time.Hour)
	later := time.Now().Add(-1 * time.Hour)
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: squat, UserID: userID, Reps: 5, Weight: 100, CreatedAt: earlier}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: squat, UserID: userID, Reps: 3, Weight: 110, CreatedAt: later}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	last, err := repo.GetLastSetByExercise(squat, userID)
	if err != nil {
		t.Fatalf("GetLastSetByExercise failed: %v", err)
	}
	if last == nil {
		t.Fatal("expected an exercise entry, got nil")
	}
	if last.Weight != 110 {
		t.Errorf("expected most recent weight 110, got %v", last.Weight)
	}
}

func TestExerciseRepository_GetLastSetByExercise_Empty(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	squat, err := repo.Create(nil, "Squat")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	last, err := repo.GetLastSetByExercise(squat, userID)
	if err != nil {
		t.Fatalf("GetLastSetByExercise failed: %v", err)
	}
	if last != nil {
		t.Fatalf("expected nil for empty history, got %+v", last)
	}

	max, err := repo.GetMaxWeightByExercise(squat, userID)
	if err != nil {
		t.Fatalf("GetMaxWeightByExercise failed: %v", err)
	}
	if max != 0 {
		t.Errorf("expected max weight 0 for empty history, got %v", max)
	}
}

func TestExerciseRepository_GetExerciseEntriesByDateRange(t *testing.T) {
	repo, _, database, userID := setupTestRepo(t)
	defer database.Close()

	exercise, err := repo.Create(nil, "Squat")
	if err != nil {
		t.Fatalf("Create exercise failed: %v", err)
	}

	loc := time.UTC
	day1 := time.Date(2024, 1, 1, 10, 0, 0, 0, loc)
	day2 := time.Date(2024, 1, 2, 10, 0, 0, 0, loc)
	day3 := time.Date(2024, 1, 3, 10, 0, 0, 0, loc)

	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: exercise, UserID: userID, Reps: 5, Weight: 100, CreatedAt: day1}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: exercise, UserID: userID, Reps: 5, Weight: 105, CreatedAt: day2}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}
	if err := repo.CreateExerciseEntry(&ExerciseEntry{ExerciseID: exercise, UserID: userID, Reps: 5, Weight: 110, CreatedAt: day3}); err != nil {
		t.Fatalf("CreateExerciseEntry failed: %v", err)
	}

	exerciseEntries, err := repo.GetExerciseEntriesByDateRange(day1, day2, userID)
	if err != nil {
		t.Fatalf("GetExerciseEntriesByDateRange failed: %v", err)
	}
	if len(exerciseEntries) != 2 {
		t.Fatalf("expected 2 exercise entries in range, got %d", len(exerciseEntries))
	}
}
