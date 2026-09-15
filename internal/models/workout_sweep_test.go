package models

import (
	"path/filepath"
	"testing"

	"hylete/internal/db"
)

// TestWorkoutRepository_SweepStalePlannedWorkouts exercises the
// nightly auto-skip sweep against real SQLite: only stale
// (scheduled_date < today), still-planned workouts with zero linked
// exercise entries flip to skipped, plus their pending blocks.
// in_progress / completed / skipped workouts, workouts with linked
// entries, and today-or-later workouts are untouched; done blocks
// stay done; block-less stale planned workouts are skipped too.
func TestWorkoutRepository_SweepStalePlannedWorkouts(t *testing.T) {
	database, err := db.NewLocalConnection(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()
	userID, blockID := seedWorkoutFixture(t, database)
	if _, err := database.Conn().Exec(`INSERT INTO blocks (id, user_id, name, block_type) VALUES ('blk-2', 'u1', 'Pull', 'standard')`); err != nil {
		t.Fatalf("seed second block: %v", err)
	}
	repo := NewWorkoutRepository(database)
	erepo := NewExerciseRepository(database)

	const today, stale = "2026-09-15", "2026-09-14"

	// Swept: stale planned, two blocks (one flipped to done first),
	// no exercise entries.
	swept := &Workout{
		UserID: userID, Name: "Stale", ScheduledDate: stale,
		Status: WorkoutStatusPlanned,
		Blocks: []WorkoutBlock{{BlockID: blockID}, {BlockID: "blk-2"}},
	}
	if err := repo.Create(swept); err != nil {
		t.Fatalf("Create swept: %v", err)
	}
	if err := repo.SetBlockStatus(swept.ID, swept.Blocks[0].ID, WorkoutBlockDone); err != nil {
		t.Fatalf("SetBlockStatus: %v", err)
	}

	// Untouched: stale planned but has a linked exercise entry.
	withEntry := &Workout{
		UserID: userID, Name: "Logged", ScheduledDate: stale,
		Status: WorkoutStatusPlanned,
		Blocks: []WorkoutBlock{{BlockID: blockID}},
	}
	if err := repo.Create(withEntry); err != nil {
		t.Fatalf("Create withEntry: %v", err)
	}
	joinID := withEntry.Blocks[0].ID
	if err := erepo.CreateExerciseEntry(&ExerciseEntry{
		ExerciseID: "ex-1", UserID: userID, Reps: 5, Weight: 100,
		WorkoutID: &withEntry.ID, BlockID: &blockID, WorkoutBlockID: &joinID,
	}); err != nil {
		t.Fatalf("CreateExerciseEntry: %v", err)
	}

	// Untouched: stale in_progress with no entries.
	inProgress := &Workout{
		UserID: userID, Name: "Started", ScheduledDate: stale,
		Status: WorkoutStatusInProgress,
		Blocks: []WorkoutBlock{{BlockID: blockID}},
	}
	if err := repo.Create(inProgress); err != nil {
		t.Fatalf("Create inProgress: %v", err)
	}

	// Untouched: scheduled today, no entries.
	fresh := &Workout{
		UserID: userID, Name: "Today", ScheduledDate: today,
		Status: WorkoutStatusPlanned,
		Blocks: []WorkoutBlock{{BlockID: blockID}},
	}
	if err := repo.Create(fresh); err != nil {
		t.Fatalf("Create fresh: %v", err)
	}

	// Swept: stale planned with no blocks and no entries.
	empty := &Workout{
		UserID: userID, Name: "Empty", ScheduledDate: stale,
		Status: WorkoutStatusPlanned,
	}
	if err := repo.Create(empty); err != nil {
		t.Fatalf("Create empty: %v", err)
	}

	// Untouched: already skipped.
	already := &Workout{
		UserID: userID, Name: "Skipped", ScheduledDate: stale,
		Status: WorkoutStatusSkipped,
		Blocks: []WorkoutBlock{{BlockID: blockID}},
	}
	if err := repo.Create(already); err != nil {
		t.Fatalf("Create already: %v", err)
	}

	wCount, bCount, err := repo.SweepStalePlannedWorkouts(today)
	if err != nil {
		t.Fatalf("SweepStalePlannedWorkouts: %v", err)
	}
	if wCount != 2 {
		t.Errorf("workouts skipped = %d, want 2 (stale + empty)", wCount)
	}
	if bCount != 1 {
		t.Errorf("blocks skipped = %d, want 1 (only the pending block)", bCount)
	}

	statusOf := func(id string) (WorkoutStatus, WorkoutBlockStatus, WorkoutBlockStatus) {
		t.Helper()
		got, err := repo.GetByID(id, userID)
		if err != nil || got == nil {
			t.Fatalf("GetByID(%s) = %+v, %v", id, got, err)
		}
		if len(got.Blocks) == 2 {
			return got.Status, got.Blocks[0].Status, got.Blocks[1].Status
		}
		return got.Status, "", ""
	}

	if st, b0, b1 := statusOf(swept.ID); st != WorkoutStatusSkipped || b0 != WorkoutBlockDone || b1 != WorkoutBlockSkipped {
		t.Errorf("swept = %q/%q/%q, want skipped/done/skipped", st, b0, b1)
	}

	for name, id := range map[string]string{
		"withEntry":  withEntry.ID,
		"inProgress": inProgress.ID,
		"fresh":      fresh.ID,
		"already":    already.ID,
	} {
		got, err := repo.GetByID(id, userID)
		if err != nil || got == nil {
			t.Fatalf("GetByID(%s) = %+v, %v", name, got, err)
		}
		want := map[string]WorkoutStatus{
			"withEntry": WorkoutStatusPlanned, "inProgress": WorkoutStatusInProgress,
			"fresh": WorkoutStatusPlanned, "already": WorkoutStatusSkipped,
		}[name]
		if got.Status != want {
			t.Errorf("%s status = %q, want %q", name, got.Status, want)
		}
		for _, b := range got.Blocks {
			if b.Status != WorkoutBlockPending {
				t.Errorf("%s block %s = %q, want pending", name, b.ID, b.Status)
			}
		}
	}

	got, err := repo.GetByID(empty.ID, userID)
	if err != nil || got == nil {
		t.Fatalf("GetByID(empty) = %+v, %v", got, err)
	}
	if got.Status != WorkoutStatusSkipped {
		t.Errorf("empty status = %q, want skipped", got.Status)
	}

	// Second run is a no-op: the sweep is idempotent.
	wCount, bCount, err = repo.SweepStalePlannedWorkouts(today)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if wCount != 0 || bCount != 0 {
		t.Errorf("second sweep = %d/%d, want 0/0", wCount, bCount)
	}
}
