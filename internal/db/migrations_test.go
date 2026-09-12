package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// TestMigrate_CreatesHealthSnapshotsTable boots a fresh local
// database (running every embedded goose migration) and asserts
// the health_snapshots table, its uniqueness contract, and its
// indexes exist. This is the only coverage that executes the
// migration SQL itself — a syntax error here would otherwise
// surface only at first boot.
func TestMigrate_CreatesHealthSnapshotsTable(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := NewLocalConnection(dbPath)
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()

	var tableName string
	err = database.Conn().QueryRow(
		"SELECT name FROM sqlite_master WHERE type='table' AND name='health_snapshots'",
	).Scan(&tableName)
	if err != nil {
		t.Fatalf("health_snapshots table missing after migrate: %v", err)
	}

	// The (user_id, snapshot_date) upsert contract depends on
	// this uniqueness — assert it, not just the table. Note:
	// SQLite records constraint auto-indexes with NULL sql, so
	// read PRAGMA index_list and confirm the unique composite
	// index covers exactly (user_id, snapshot_date).
	rows, err := database.Conn().Query(`PRAGMA index_list('health_snapshots')`)
	if err != nil {
		t.Fatalf("PRAGMA index_list: %v", err)
	}
	defer rows.Close()
	foundUnique := false
	for rows.Next() {
		var seq int
		var name string
		var unique bool
		var origin, partial string
		if err := rows.Scan(&seq, &name, &unique, &origin, &partial); err != nil {
			t.Fatalf("scan index_list: %v", err)
		}
		if !unique || origin != "u" {
			continue
		}
		colRows, err := database.Conn().Query(`PRAGMA index_info('` + name + `')`)
		if err != nil {
			t.Fatalf("PRAGMA index_info: %v", err)
		}
		var cols []string
		for colRows.Next() {
			var seqno, cid int
			var col string
			if err := colRows.Scan(&seqno, &cid, &col); err != nil {
				colRows.Close()
				t.Fatalf("scan index_info: %v", err)
			}
			cols = append(cols, col)
		}
		colRows.Close()
		if len(cols) == 2 && cols[0] == "user_id" && cols[1] == "snapshot_date" {
			foundUnique = true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("index_list rows: %v", err)
	}
	if !foundUnique {
		t.Error("UNIQUE(user_id, snapshot_date) missing after migrate")
	}

	for _, want := range []string{"idx_health_snapshots_user", "idx_health_snapshots_date"} {
		var name string
		if err := database.Conn().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", want,
		).Scan(&name); err != nil {
			t.Errorf("index %s missing after migrate: %v", want, err)
		}
	}
}

// TestMigrate_WeightThreePhotoColumns boots a fresh local
// database (running every embedded goose migration) and asserts
// the weight_entries photo slots exist. Migration 00018 renames
// photo_key to front_photo_key (all legacy photos are treated as
// front-facing) and adds the side/back slots. This executes the
// migration SQL itself — a syntax error there would otherwise
// surface only at first boot.
func TestMigrate_WeightThreePhotoColumns(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := NewLocalConnection(dbPath)
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()

	rows, err := database.Conn().Query(`PRAGMA table_info('weight_entries')`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	found := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		found[name] = true
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info rows: %v", err)
	}
	for _, want := range []string{"front_photo_key", "side_photo_key", "back_photo_key"} {
		if !found[want] {
			t.Errorf("weight_entries.%s column missing after migrate", want)
		}
	}
	if found["photo_key"] {
		t.Error("weight_entries.photo_key should have been renamed to front_photo_key")
	}
}

// TestMigrate_CreatesBlocksTables boots a fresh local database
// (running every embedded goose migration) and asserts the blocks
// and block_items tables plus their indexes exist. This executes
// the 00019 migration SQL itself — a syntax error there would
// otherwise surface only at first boot.
func TestMigrate_CreatesBlocksTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := NewLocalConnection(dbPath)
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()

	for _, table := range []string{"blocks", "block_items"} {
		var tableName string
		if err := database.Conn().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&tableName); err != nil {
			t.Errorf("%s table missing after migrate: %v", table, err)
		}
	}
	for _, want := range []string{"idx_blocks_user", "idx_block_items_block"} {
		var name string
		if err := database.Conn().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", want,
		).Scan(&name); err != nil {
			t.Errorf("index %s missing after migrate: %v", want, err)
		}
	}
}

// TestMigrate_CreatesWorkoutsTables boots a fresh local database
// (running every embedded goose migration) and asserts the
// workouts, workout_blocks, and workout_assignments tables plus
// their indexes exist. This executes the 00020 migration SQL
// itself - a syntax error there would otherwise surface only at
// first boot.
func TestMigrate_CreatesWorkoutsTables(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := NewLocalConnection(dbPath)
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()

	for _, table := range []string{"workouts", "workout_blocks", "workout_assignments"} {
		var tableName string
		if err := database.Conn().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='table' AND name=?", table,
		).Scan(&tableName); err != nil {
			t.Errorf("%s table missing after migrate: %v", table, err)
		}
	}
	for _, want := range []string{"idx_workouts_user", "idx_workout_blocks_workout", "idx_workout_assignments_user_date"} {
		var name string
		if err := database.Conn().QueryRow(
			"SELECT name FROM sqlite_master WHERE type='index' AND name=?", want,
		).Scan(&name); err != nil {
			t.Errorf("index %s missing after migrate: %v", want, err)
		}
	}
}

// TestMigrate_AddsExerciseAliasesColumn boots a fresh local
// database (running every embedded goose migration) and asserts
// the exercises.aliases column exists. This executes the
// 00012 migration SQL itself — a syntax error there would
// otherwise surface only at first boot.
func TestMigrate_AddsExerciseAliasesColumn(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	database, err := NewLocalConnection(dbPath)
	if err != nil {
		t.Fatalf("NewLocalConnection: %v", err)
	}
	defer database.Close()

	rows, err := database.Conn().Query(`PRAGMA table_info('exercises')`)
	if err != nil {
		t.Fatalf("PRAGMA table_info: %v", err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var cid int
		var name, colType string
		var notNull int
		var dflt sql.NullString
		var pk int
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info: %v", err)
		}
		if name == "aliases" {
			found = true
			if notNull != 1 {
				t.Errorf("aliases NOT NULL = %d, want 1", notNull)
			}
			if !dflt.Valid || dflt.String != "''" {
				t.Errorf("aliases default = %v, want ''", dflt)
			}
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info rows: %v", err)
	}
	if !found {
		t.Error("exercises.aliases column missing after migrate")
	}
}
