package models

import (
	"testing"

	"hylete/internal/db"
)

// setupBlockTestRepo boots an in-memory database (running every
// embedded migration), a user, and two catalog exercises, and
// returns the block repository plus the IDs needed to build
// items. Mirrors setupTestRepo in exercise_test.go.
func setupBlockTestRepo(t *testing.T) (*BlockRepository, *db.DB, string, string, string) {
	t.Helper()

	database, err := db.NewLocalConnection(":memory:")
	if err != nil {
		t.Fatalf("failed to create in-memory database: %v", err)
	}

	userRepo := NewUserRepository(database)
	user := &User{Name: "Block User", Email: "blocks@example.com", PasswordHash: "hash"}
	if err := userRepo.CreateUser(user); err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}

	exerciseRepo := NewExerciseRepository(database)
	squatID, err := exerciseRepo.Create(nil, "Back Squat")
	if err != nil {
		t.Fatalf("failed to create squat: %v", err)
	}
	benchID, err := exerciseRepo.Create(nil, "Bench Press")
	if err != nil {
		t.Fatalf("failed to create bench: %v", err)
	}

	return NewBlockRepository(database), database, user.ID, squatID, benchID
}

// TestBlockRepository_RoundTrip covers create (items stored in
// order with generated IDs), GetByID (exercise names resolved),
// List (item counts, newest first), Update (full item replace),
// Delete, and cross-user isolation.
func TestBlockRepository_RoundTrip(t *testing.T) {
	repo, database, userID, squatID, benchID := setupBlockTestRepo(t)
	defer database.Close()

	created := &Block{
		UserID:      userID,
		Name:        "Leg Day",
		Description: "heavy",
		Type:        BlockTypeCircuit,
		Rounds:      4,
		RestSeconds: 90,
		Items: []BlockItem{
			{ExerciseID: squatID, TargetText: "5x5 @ 100"},
			{ExerciseID: benchID},
		},
	}
	if err := repo.Create(created); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ID == "" {
		t.Fatal("expected generated block ID")
	}

	got, err := repo.GetByID(created.ID, userID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected block, got nil")
	}
	if len(got.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(got.Items))
	}
	if got.Items[0].Position != 0 || got.Items[1].Position != 1 {
		t.Errorf("unexpected positions: %+v", got.Items)
	}
	if got.Items[0].ExerciseName != "Back Squat" || got.Items[0].TargetText != "5x5 @ 100" {
		t.Errorf("unexpected first item: %+v", got.Items[0])
	}
	if got.Items[1].ExerciseName != "Bench Press" {
		t.Errorf("unexpected second item: %+v", got.Items[1])
	}

	// Cross-user reads see nothing.
	if other, err := repo.GetByID(created.ID, "someone-else"); err != nil || other != nil {
		t.Errorf("cross-user GetByID = %v, %v; want nil, nil", other, err)
	}

	list, err := repo.List(userID)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ItemCount != 2 || list[0].Name != "Leg Day" {
		t.Fatalf("unexpected list: %+v", list)
	}

	// Update replaces the items wholesale.
	got.Name = "Leg Day v2"
	got.Type = BlockTypeStandard
	got.Rounds = 0
	got.RestSeconds = 0
	got.Items = []BlockItem{{ExerciseID: benchID, TargetText: "3x10"}}
	if err := repo.Update(got, userID); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.GetByID(created.ID, userID)
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if updated.Name != "Leg Day v2" || len(updated.Items) != 1 {
		t.Fatalf("unexpected updated block: %+v", updated)
	}
	if updated.Items[0].ExerciseID != benchID || updated.Items[0].TargetText != "3x10" {
		t.Errorf("unexpected replaced item: %+v", updated.Items[0])
	}

	if err := repo.Delete(created.ID, "someone-else"); err != nil {
		t.Fatalf("cross-user Delete: %v", err)
	}
	if still, _ := repo.GetByID(created.ID, userID); still == nil {
		t.Fatal("cross-user Delete removed another user's block")
	}
	if err := repo.Delete(created.ID, userID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if gone, _ := repo.GetByID(created.ID, userID); gone != nil {
		t.Error("expected block gone after Delete")
	}
	if list, _ := repo.List(userID); len(list) != 0 {
		t.Errorf("expected empty list after Delete, got %+v", list)
	}
}

// TestBlock_TypeDisplayName locks the user-facing kind labels the
// iOS list falls back to (the DTO carries its own displayName;
// this covers the server-side helper).
func TestBlock_TypeDisplayName(t *testing.T) {
	cases := map[BlockType]string{
		BlockTypeStandard:  "Standard",
		BlockTypeCircuit:   "Circuit",
		BlockTypeAmrap:     "AMRAP",
		BlockTypeEmom:      "EMOM",
		BlockType("bogus"): "Standard",
	}
	for typ, want := range cases {
		b := &Block{Type: typ}
		if got := b.TypeDisplayName(); got != want {
			t.Errorf("TypeDisplayName(%q) = %q, want %q", typ, got, want)
		}
	}
}
