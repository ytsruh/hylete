package models

import (
	"path/filepath"
	"testing"

	"hylete/internal/db"
)

// seedWorkoutFixture inserts the minimal rows a workout needs:
// a user, an exercise, and a block with two items. Returned IDs are
// the block's.
func seedWorkoutFixture(t *testing.T, database *db.DB) (userID, blockID string) {
	t.Helper()
	conn := database.Conn()
	if _, err := conn.Exec(`INSERT INTO users (id, name, email, password_hash) VALUES ('u1', 'Test', 't@example.com', 'x')`); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO exercises (id, name) VALUES ('ex-1', 'Squat'), ('ex-2', 'Bench')`); err != nil {
		t.Fatalf("seed exercises: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO blocks (id, user_id, name, block_type) VALUES ('blk-1', 'u1', 'Push', 'standard')`); err != nil {
		t.Fatalf("seed block: %v", err)
	}
	if _, err := conn.Exec(`INSERT INTO block_items (id, block_id, exercise_id, position) VALUES ('bi-1', 'blk-1', 'ex-1', 0), ('bi-2', 'blk-1', 'ex-2', 1)`); err != nil {
		t.Fatalf("seed block items: %v", err)
	}
	return "u1", "blk-1"
}

// TestWorkoutRepository_EndToEnd exercises the real sqlc queries
// against a migrated SQLite database: create with live block refs,
// detail read with resolved names/counts, list + range with
// done counts, per-block status, usage counting, update, delete.
// This is the only coverage that executes the workouts SQL itself.
func TestWorkoutRepository_EndToEnd(t *testing.T) {
	database, err := db.NewLocalConnection(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()
	userID, blockID := seedWorkoutFixture(t, database)
	repo := NewWorkoutRepository(database)

	w := &Workout{
		UserID:        userID,
		Name:          "Monday Strength",
		Description:   "Heavy day",
		ScheduledDate: "2026-09-14",
		Status:        WorkoutStatusPlanned,
		Blocks:        []WorkoutBlock{{BlockID: blockID}},
	}
	if err := repo.Create(w); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if w.ID == "" || len(w.Blocks) != 1 || w.Blocks[0].ID == "" {
		t.Fatalf("Create did not assign IDs: %+v", w)
	}

	got, err := repo.GetByID(w.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("GetByID returned nil")
	}
	if got.Blocks[0].BlockName != "Push" || got.Blocks[0].ItemCount != 2 {
		t.Errorf("block not resolved: %+v", got.Blocks[0])
	}

	if other, err := repo.GetByID(w.ID, "someone-else"); err != nil || other != nil {
		t.Errorf("GetByID cross-user = %+v, %v; want nil, nil", other, err)
	}

	summaries, err := repo.List(userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(summaries) != 1 || summaries[0].BlockCount != 1 || summaries[0].DoneCount != 0 {
		t.Errorf("List = %+v, want 1 summary with 1 block / 0 done", summaries)
	}

	inRange, err := repo.ListRange(userID, "2026-09-14", "2026-09-14")
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(inRange) != 1 {
		t.Errorf("ListRange in-window len = %d, want 1", len(inRange))
	}
	outRange, err := repo.ListRange(userID, "2026-09-15", "2026-09-21")
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(outRange) != 0 {
		t.Errorf("ListRange out-of-window len = %d, want 0", len(outRange))
	}

	if err := repo.SetBlockStatus(w.ID, w.Blocks[0].ID, WorkoutBlockDone); err != nil {
		t.Fatalf("SetBlockStatus: %v", err)
	}
	row, err := repo.GetWorkoutBlock(w.ID, w.Blocks[0].ID)
	if err != nil || row == nil || row.Status != WorkoutBlockDone {
		t.Errorf("GetWorkoutBlock = %+v, %v; want done", row, err)
	}
	summaries, err = repo.List(userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if summaries[0].DoneCount != 1 {
		t.Errorf("DoneCount = %d, want 1", summaries[0].DoneCount)
	}

	uses, err := repo.CountBlockUsage(blockID, userID)
	if err != nil {
		t.Fatalf("CountBlockUsage: %v", err)
	}
	if uses != 1 {
		t.Errorf("CountBlockUsage = %d, want 1", uses)
	}

	w.Name = "Renamed"
	w.Status = WorkoutStatusInProgress
	if err := repo.Update(w, userID); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.GetByID(w.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if updated.Name != "Renamed" || updated.Status != WorkoutStatusInProgress {
		t.Errorf("Update not persisted: %+v", updated)
	}

	if err := repo.Delete(w.ID, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	deleted, err := repo.GetByID(w.ID, userID)
	if err != nil || deleted != nil {
		t.Errorf("GetByID after delete = %+v, %v; want nil, nil", deleted, err)
	}
	uses, err = repo.CountBlockUsage(blockID, userID)
	if err != nil {
		t.Fatalf("CountBlockUsage: %v", err)
	}
	if uses != 0 {
		t.Errorf("CountBlockUsage after delete = %d, want 0", uses)
	}
}

// TestWorkoutRepository_CreateBatch persists several workouts
// atomically against real SQLite: all rows (plus their blocks) land
// together, IDs come back on every value, and the per-copy block
// slices are independent (no aliased backing arrays).
func TestWorkoutRepository_CreateBatch(t *testing.T) {
	database, err := db.NewLocalConnection(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()
	userID, blockID := seedWorkoutFixture(t, database)
	repo := NewWorkoutRepository(database)

	mk := func(date string) *Workout {
		return &Workout{
			UserID: userID, Name: "Monday", ScheduledDate: date,
			Status: WorkoutStatusPlanned,
			Blocks: []WorkoutBlock{{BlockID: blockID}},
		}
	}
	ws := []*Workout{mk("2026-09-21"), mk("2026-09-28")}
	if err := repo.CreateBatch(ws); err != nil {
		t.Fatalf("CreateBatch: %v", err)
	}
	for i, w := range ws {
		if w.ID == "" || len(w.Blocks) != 1 || w.Blocks[0].ID == "" {
			t.Errorf("copy %d missing IDs: %+v", i, w)
		}
	}
	if ws[0].ID == ws[1].ID || ws[0].Blocks[0].ID == ws[1].Blocks[0].ID {
		t.Error("batch copies share IDs")
	}
	inRange, err := repo.ListRange(userID, "2026-09-21", "2026-09-28")
	if err != nil {
		t.Fatalf("ListRange: %v", err)
	}
	if len(inRange) != 2 {
		t.Errorf("ListRange len = %d, want 2", len(inRange))
	}
}
