package export

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"
)

// TestBuildExerciseEntriesZip_EmptyInput ensures a user with no
// exercise entries still gets a valid zip containing the header CSV
// row and an empty manifest (photo_count 0 — exercise entries carry
// no photos).
func TestBuildExerciseEntriesZip_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	res, err := BuildExerciseEntriesZip(context.Background(), &buf, nil, "user-1", "kg", "km")
	if err != nil {
		t.Fatalf("BuildExerciseEntriesZip: %v", err)
	}
	if res.Entries != 0 {
		t.Errorf("Entries = %d, want 0", res.Entries)
	}
	if res.PhotosWritten != 0 {
		t.Errorf("PhotosWritten = %d, want 0", res.PhotosWritten)
	}
	if len(res.MissingPhotos) != 0 {
		t.Errorf("MissingPhotos = %v, want empty", res.MissingPhotos)
	}

	files := readZip(t, buf.Bytes())
	if _, ok := files["exercise_entries.csv"]; !ok {
		t.Fatal("expected exercise_entries.csv in zip")
	}
	if _, ok := files["manifest.json"]; !ok {
		t.Fatal("expected manifest.json in zip")
	}
	// Exercise entries have no photos, so no photos/ directory
	// should ever appear.
	for name := range files {
		if strings.HasPrefix(name, "photos/") {
			t.Errorf("unexpected photo file %q in exercise entries export", name)
		}
	}

	rows := mustReadCSV(t, files["exercise_entries.csv"])
	if len(rows) != 1 {
		t.Fatalf("expected 1 CSV row (header only), got %d", len(rows))
	}
	wantHeader := []string{
		"id", "created_at", "date", "exercise_id", "exercise_name", "exercise_type",
		"reps", "weight", "weight_unit", "rest_time_seconds",
		"duration_seconds", "distance_km", "avg_heart_rate_bpm", "calories_burned", "pace_sec_per_km",
		"notes",
	}
	if !equalRow(rows[0], wantHeader) {
		t.Errorf("csv header = %v, want %v", rows[0], wantHeader)
	}
}

// TestBuildExerciseEntriesZip_MultipleEntries covers the happy path:
// several exercise entries submitted out of order. Asserts CSV row
// count, date-ascending ordering regardless of input order, and the
// full column contents of a single row.
func TestBuildExerciseEntriesZip_MultipleEntries(t *testing.T) {
	day1 := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 10, 18, 30, 0, 0, time.UTC)

	entries := []models.ExerciseEntry{
		{ID: "e2", UserID: "u1", ExerciseID: "ex-1", ExerciseName: "Squat", ExerciseType: models.ExerciseTypeStrength, Reps: 5, Weight: 100.5, RestTime: 180, Notes: "second", CreatedAt: day2},
		{ID: "e1", UserID: "u1", ExerciseID: "ex-2", ExerciseName: "Bench Press", ExerciseType: models.ExerciseTypeStrength, Reps: 8, Weight: 60, RestTime: 90, Notes: "first", CreatedAt: day1},
	}

	var buf bytes.Buffer
	res, err := BuildExerciseEntriesZip(context.Background(), &buf, entries, "u1", "kg", "km")
	if err != nil {
		t.Fatalf("BuildExerciseEntriesZip: %v", err)
	}
	if res.Entries != 2 {
		t.Errorf("Entries = %d, want 2", res.Entries)
	}

	files := readZip(t, buf.Bytes())
	rows := mustReadCSV(t, files["exercise_entries.csv"])
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows (header + 2 entries), got %d", len(rows))
	}

	// Rows must be in date-ascending order regardless of input order.
	wantIDs := []string{"e1", "e2"}
	for i, want := range wantIDs {
		if rows[i+1][0] != want {
			t.Errorf("row %d id = %q, want %q", i+1, rows[i+1][0], want)
		}
	}

	// Full column check for the first data row.
	first := rows[1]
	wantRow := []string{
		"e1",
		"2026-01-09T08:00:00Z", // created_at (RFC3339 UTC)
		"2026-01-09",           // date
		"ex-2",                 // exercise_id
		"Bench Press",          // exercise_name
		"strength",             // exercise_type
		"8",                    // reps
		"60.0",                 // weight
		"kg",                   // weight_unit
		"90",                   // rest_time_seconds
		"0",                    // duration_seconds (cardio metrics zeroed)
		"0.000",                // distance_km
		"0",                    // avg_heart_rate_bpm
		"0.0",                  // calories_burned
		"",                     // pace_sec_per_km (empty when not derivable)
		"first",                // notes
	}
	if !equalRow(first, wantRow) {
		t.Errorf("first data row = %v, want %v", first, wantRow)
	}

	// weight_unit column is filled on every row with the value passed in.
	for i := 1; i < len(rows); i++ {
		if rows[i][8] != "kg" {
			t.Errorf("row %d weight_unit = %q, want kg", i, rows[i][8])
		}
	}

	// Manifest matches the result totals; photo fields stay empty.
	var mf struct {
		UserID        string   `json:"user_id"`
		EntryCount    int      `json:"entry_count"`
		PhotoCount    int      `json:"photo_count"`
		MissingPhotos []string `json:"missing_photos"`
	}
	if err := json.Unmarshal(files["manifest.json"], &mf); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if mf.UserID != "u1" {
		t.Errorf("manifest.user_id = %q, want u1", mf.UserID)
	}
	if mf.EntryCount != 2 || mf.PhotoCount != 0 {
		t.Errorf("manifest counts = (%d, %d), want (2, 0)", mf.EntryCount, mf.PhotoCount)
	}
	if len(mf.MissingPhotos) != 0 {
		t.Errorf("manifest.missing_photos = %v, want empty", mf.MissingPhotos)
	}
}

// TestBuildExerciseEntriesZip_CSVEscaping ensures notes containing
// commas, quotes and newlines are correctly escaped by encoding/csv.
func TestBuildExerciseEntriesZip_CSVEscaping(t *testing.T) {
	day := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	entries := []models.ExerciseEntry{
		{ID: "e1", UserID: "u1", ExerciseName: "Squat", ExerciseType: models.ExerciseTypeStrength, Reps: 5, Weight: 100, Notes: `has, comma and "quote" and` + "\nnewline", CreatedAt: day},
	}

	var buf bytes.Buffer
	if _, err := BuildExerciseEntriesZip(context.Background(), &buf, entries, "u1", "kg", "km"); err != nil {
		t.Fatalf("BuildExerciseEntriesZip: %v", err)
	}

	files := readZip(t, buf.Bytes())
	rows := mustReadCSV(t, files["exercise_entries.csv"])
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 row, got %d", len(rows))
	}
	want := `has, comma and "quote" and` + "\nnewline"
	if rows[1][15] != want {
		t.Errorf("csv notes = %q, want %q", rows[1][15], want)
	}
}

// TestBuildExerciseEntriesZip_CardioRow ensures a cardio exercise entry
// exports its duration/distance metrics with the derived pace column,
// while the strength-only columns stay zeroed — mirroring the server's
// normalization on write.
func TestBuildExerciseEntriesZip_CardioRow(t *testing.T) {
	// 25 minutes for 5 km → 300s pace (5:00/km).
	day := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	entries := []models.ExerciseEntry{
		{ID: "e1", UserID: "u1", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, AvgHeartRate: 152, CaloriesBurned: 320, Notes: "easy pace", CreatedAt: day},
	}

	var buf bytes.Buffer
	if _, err := BuildExerciseEntriesZip(context.Background(), &buf, entries, "u1", "kg", "km"); err != nil {
		t.Fatalf("BuildExerciseEntriesZip: %v", err)
	}

	files := readZip(t, buf.Bytes())
	rows := mustReadCSV(t, files["exercise_entries.csv"])
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 row, got %d", len(rows))
	}
	row := rows[1]
	if row[5] != "cardio" {
		t.Errorf("exercise_type = %q, want cardio", row[5])
	}
	if row[10] != "1500" {
		t.Errorf("duration_seconds = %q, want 1500", row[10])
	}
	if row[11] != "5.000" {
		t.Errorf("distance_km = %q, want 5.000", row[11])
	}
	if row[12] != "152" {
		t.Errorf("avg_heart_rate_bpm = %q, want 152", row[12])
	}
	if row[13] != "320.0" {
		t.Errorf("calories_burned = %q, want 320.0", row[13])
	}
	if row[14] != "300.00" {
		t.Errorf("pace_sec_per_km = %q, want 300.00 (5:00/km)", row[14])
	}
}
