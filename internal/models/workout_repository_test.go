package models

import (
	"testing"

	"hylete/internal/db"
)

// setupWorkoutTestRepo boots an in-memory database (running every
// embedded migration), a user, and two blocks, and returns the
// workout repository plus the IDs needed to build links.
// Mirrors setupBlockTestRepo in block_test.go.
func setupWorkoutTestRepo(t *testing.T) (*WorkoutRepository, *BlockRepository, *db.DB, string, string, string) {
	t.Helper()

	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory database: %v", err)
	}

	userRepo := NewUserRepository(database)
	user := &User{Name: "Workout User", Email: "workouts@example.com", PasswordHash: "hash"}
	if err := userRepo.CreateUser(user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	exerciseRepo := NewExerciseRepository(database)
	squatID, err := exerciseRepo.Create(nil, "Back Squat")
	if err != nil {
		t.Fatalf("failed to create squat: %v", err)
	}

	blockRepo := NewBlockRepository(database)
	mkBlock := func(name string) string {
		b := &Block{
			UserID: user.ID,
			Name:   name,
			Type:   BlockTypeStandard,
			Items:  []BlockItem{{ExerciseID: squatID}},
		}
		if err := blockRepo.Create(b); err != nil {
			t.Fatalf("failed to create block %s: %v", name, err)
		}
		return b.ID
	}

	return NewWorkoutRepository(database), blockRepo, database, user.ID, mkBlock("Push"), mkBlock("Pull")
}

// TestWorkoutRepository_RoundTrip covers create (links stored in
// order with generated IDs), GetByID (block names resolved),
// List (block counts), Update (full link replace), Delete
// (links + assignments removed), and cross-user isolation -
// all against real SQL.
func TestWorkoutRepository_RoundTrip(t *testing.T) {
	repo, _, database, userID, pushID, pullID := setupWorkoutTestRepo(t)
	defer database.Close()

	created := &Workout{
		UserID:      userID,
		Title:       "Push Day",
		Description: "chest + shoulders",
		Blocks: []WorkoutBlock{
			{BlockID: pushID},
			{BlockID: pullID},
		},
	}
	if err := repo.Create(created); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated workout ID")
	}

	got, err := repo.GetByID(created.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected workout, got nil")
	}
	if len(got.Blocks) != 2 {
		t.Fatalf("blocks len = %d, want 2", len(got.Blocks))
	}
	if got.Blocks[0].Position != 0 || got.Blocks[1].Position != 1 {
		t.Errorf("unexpected positions: %+v", got.Blocks)
	}
	if got.Blocks[0].BlockName != "Push" {
		t.Errorf("block name = %q, want Push", got.Blocks[0].BlockName)
	}

	list, err := repo.List(userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].BlockCount != 2 {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Cross-user isolation.
	if other, err := repo.GetByID(created.ID, "someone-else"); err != nil || other != nil {
		t.Fatalf("cross-user GetByID = %v, %v; want nil, nil", other, err)
	}

	// Update replaces links.
	got.Title = "Push Day v2"
	got.Blocks = []WorkoutBlock{{BlockID: pullID}}
	if err := repo.Update(got, userID); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.GetByID(created.ID, userID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if updated.Title != "Push Day v2" || len(updated.Blocks) != 1 || updated.Blocks[0].BlockID != pullID {
		t.Fatalf("unexpected updated workout: %+v", updated)
	}

	// Schedule then delete: assignments go with the workout.
	if _, err := repo.AddAssignments(created.ID, userID, []string{"2026-09-14"}); err != nil {
		t.Fatalf("AddAssignments: %v", err)
	}
	if err := repo.Delete(created.ID, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gone, err := repo.GetByID(created.ID, userID); err != nil || gone != nil {
		t.Fatalf("after delete GetByID = %v, %v; want nil, nil", gone, err)
	}
	if left, err := repo.ListAssignmentsByDateRange(userID, "2026-09-01", "2026-09-30"); err != nil || len(left) != 0 {
		t.Fatalf("after delete schedule = %+v, %v; want empty", left, err)
	}
}

// TestWorkoutRepository_Schedule covers the assignment flow:
// add (ordered back), idempotent re-add of the same day,
// per-day delete, and the calendar range query with resolved
// workout titles.
func TestWorkoutRepository_Schedule(t *testing.T) {
	repo, _, database, userID, pushID, _ := setupWorkoutTestRepo(t)
	defer database.Close()

	w := &Workout{UserID: userID, Title: "Base", Blocks: []WorkoutBlock{{BlockID: pushID}}}
	if err := repo.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	added, err := repo.AddAssignments(w.ID, userID, []string{"2026-09-21", "2026-09-14"})
	if err != nil {
		t.Fatalf("AddAssignments: %v", err)
	}
	if len(added) != 2 {
		t.Fatalf("added len = %d, want 2", len(added))
	}

	// Re-adding an existing day is a skip, not an error.
	resend, err := repo.AddAssignments(w.ID, userID, []string{"2026-09-14"})
	if err != nil {
		t.Fatalf("re-add: %v", err)
	}
	if len(resend) != 0 {
		t.Fatalf("re-add len = %d, want 0", len(resend))
	}

	sched, err := repo.ListAssignmentsForWorkout(w.ID)
	if err != nil {
		t.Fatalf("ListAssignmentsForWorkout: %v", err)
	}
	if len(sched) != 2 || sched[0].ScheduledDate != "2026-09-14" {
		t.Fatalf("unexpected schedule: %+v", sched)
	}

	ranged, err := repo.ListAssignmentsByDateRange(userID, "2026-09-14", "2026-09-21")
	if err != nil {
		t.Fatalf("range: %v", err)
	}
	if len(ranged) != 2 || ranged[0].WorkoutTitle != "Base" {
		t.Fatalf("unexpected range: %+v", ranged)
	}
	narrow, err := repo.ListAssignmentsByDateRange(userID, "2026-09-15", "2026-09-20")
	if err != nil {
		t.Fatalf("narrow range: %v", err)
	}
	if len(narrow) != 0 {
		t.Fatalf("narrow len = %d, want 0", len(narrow))
	}

	if err := repo.DeleteAssignment(sched[0].ID, userID); err != nil {
		t.Fatalf("DeleteAssignment: %v", err)
	}
	if gone, err := repo.GetAssignment(sched[0].ID, userID); err != nil || gone != nil {
		t.Fatalf("after delete GetAssignment = %v, %v; want nil, nil", gone, err)
	}
}

// TestWorkoutRepository_Duplicate covers server-side copy:
// "<title> copy" with the same links, schedule copied only on
// request.
func TestWorkoutRepository_Duplicate(t *testing.T) {
	repo, _, database, userID, pushID, pullID := setupWorkoutTestRepo(t)
	defer database.Close()

	w := &Workout{
		UserID: userID,
		Title:  "Base",
		Blocks: []WorkoutBlock{{BlockID: pushID}, {BlockID: pullID}},
	}
	if err := repo.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.AddAssignments(w.ID, userID, []string{"2026-09-14"}); err != nil {
		t.Fatalf("AddAssignments: %v", err)
	}

	dup, err := repo.Duplicate(w.ID, userID, false)
	if err != nil {
		t.Fatalf("Duplicate: %v", err)
	}
	if dup == nil || dup.Title != "Base copy" {
		t.Fatalf("unexpected duplicate: %+v", dup)
	}
	if len(dup.Blocks) != 2 {
		t.Fatalf("dup blocks len = %d, want 2", len(dup.Blocks))
	}
	if sched, err := repo.ListAssignmentsForWorkout(dup.ID); err != nil || len(sched) != 0 {
		t.Fatalf("dup schedule = %+v, %v; want empty", sched, err)
	}

	withSched, err := repo.Duplicate(w.ID, userID, true)
	if err != nil {
		t.Fatalf("Duplicate with schedule: %v", err)
	}
	sched, err := repo.ListAssignmentsForWorkout(withSched.ID)
	if err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if len(sched) != 1 || sched[0].ScheduledDate != "2026-09-14" {
		t.Fatalf("unexpected copied schedule: %+v", sched)
	}

	if missing, err := repo.Duplicate("nope", userID, false); err != nil || missing != nil {
		t.Fatalf("missing Duplicate = %v, %v; want nil, nil", missing, err)
	}
}

// TestWorkoutRepository_BlockDeleteKeepsWorkout covers the
// agreed delete semantic: removing a block linked into a
// workout removes the link, never the workout.
func TestWorkoutRepository_BlockDeleteKeepsWorkout(t *testing.T) {
	repo, blockRepo, database, userID, pushID, pullID := setupWorkoutTestRepo(t)
	defer database.Close()

	w := &Workout{UserID: userID, Title: "Mixed", Blocks: []WorkoutBlock{{BlockID: pushID}, {BlockID: pullID}}}
	if err := repo.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := blockRepo.Delete(pushID, userID); err != nil {
		t.Fatalf("block Delete: %v", err)
	}
	got, err := repo.GetByID(w.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("workout must survive its block being deleted")
	}
	if len(got.Blocks) != 1 || got.Blocks[0].BlockID != pullID {
		t.Fatalf("unexpected surviving links: %+v", got.Blocks)
	}
}
