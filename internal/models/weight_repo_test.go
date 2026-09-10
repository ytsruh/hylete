package models

import (
	"path/filepath"
	"testing"
	"time"

	"hylete/internal/db"
)

// TestWeightRepository_RealDB exercises the weight repository against a
// real (local, migrated) SQLite database rather than a mock. This is
// the only coverage that runs the sqlc-generated weight queries
// against the post-00018 schema (front/side/back_photo_key): a column
// rename/add that compiles but doesn't match the database would
// otherwise surface only at runtime as a 500 on GET /api/v1/weight.
func TestWeightRepository_RealDB(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "weight-it.db")
	database, err := db.NewLocalConnection(dbPath)
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()
	repo := NewWeightRepository(database)

	createdAt := time.Date(2026, 3, 4, 8, 0, 0, 0, time.UTC)
	entry := &WeightEntry{
		UserID:        "u1",
		Weight:        80.5,
		Notes:         "morning",
		FrontPhotoKey: "weight/u1/front.jpg",
		SidePhotoKey:  "weight/u1/side.jpg",
		BackPhotoKey:  "weight/u1/back.jpg",
		CreatedAt:     createdAt,
	}
	if err := repo.Create(entry); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if entry.ID == "" {
		t.Fatal("expected generated ID after Create")
	}

	got, err := repo.GetByID(entry.ID, "u1")
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got == nil {
		t.Fatal("expected entry back from GetByID")
	}
	if got.FrontPhotoKey != "weight/u1/front.jpg" || got.SidePhotoKey != "weight/u1/side.jpg" || got.BackPhotoKey != "weight/u1/back.jpg" {
		t.Errorf("photo keys = %q/%q/%q, want all three back", got.FrontPhotoKey, got.SidePhotoKey, got.BackPhotoKey)
	}
	if got.PhotoCount() != 3 {
		t.Errorf("PhotoCount = %d, want 3", got.PhotoCount())
	}

	// A second entry sharing only the front angle, for the
	// compare-fetch path.
	other := &WeightEntry{
		UserID:        "u1",
		Weight:        79,
		FrontPhotoKey: "weight/u1/other-front.jpg",
		CreatedAt:     createdAt.Add(24 * time.Hour),
	}
	if err := repo.Create(other); err != nil {
		t.Fatalf("Create other: %v", err)
	}

	listed, err := repo.List("u1")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) != 2 {
		t.Fatalf("List len = %d, want 2", len(listed))
	}

	pair, err := repo.GetByIDs(entry.ID, other.ID, "u1")
	if err != nil {
		t.Fatalf("GetByIDs: %v", err)
	}
	if len(pair) != 2 {
		t.Fatalf("GetByIDs len = %d, want 2", len(pair))
	}

	// Clearing one slot persists as empty.
	entry.SidePhotoKey = ""
	if err := repo.Update(entry, "u1"); err != nil {
		t.Fatalf("Update: %v", err)
	}
	updated, err := repo.GetByID(entry.ID, "u1")
	if err != nil {
		t.Fatalf("GetByID after update: %v", err)
	}
	if updated.SidePhotoKey != "" || updated.PhotoCount() != 2 {
		t.Errorf("after clear: side = %q count = %d, want empty/2", updated.SidePhotoKey, updated.PhotoCount())
	}

	if err := repo.Delete(other.ID, "u1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	remaining, err := repo.List("u1")
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(remaining) != 1 {
		t.Fatalf("List len after delete = %d, want 1", len(remaining))
	}
}
