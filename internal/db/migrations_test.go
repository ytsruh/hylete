package db

import (
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
