package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"sort"
	"strings"
	"testing"
	"time"

	"hylete/internal/models"
)

// fakePhotos is a PhotoGetter backed by an in-memory map. missing
// keys produce an error so the test can simulate a photo that's been
// deleted from R2 while still referenced in the DB.
type fakePhotos struct {
	data    map[string][]byte
	missing map[string]bool
}

func (f *fakePhotos) Get(_ context.Context, key string) (io.ReadCloser, error) {
	if f.missing[key] {
		return nil, errors.New("simulated R2 miss: " + key)
	}
	b, ok := f.data[key]
	if !ok {
		return nil, errors.New("simulated R2 miss: " + key)
	}
	return io.NopCloser(bytes.NewReader(b)), nil
}

// readZip extracts every file from b into a map keyed by zip entry
// name. Bodies are fully read into memory; fine for tests.
func readZip(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	out := make(map[string][]byte, len(zr.File))
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		out[f.Name] = body
	}
	return out
}

func mustReadCSV(t *testing.T, b []byte) [][]string {
	t.Helper()
	rdr := csv.NewReader(bytes.NewReader(b))
	rows, err := rdr.ReadAll()
	if err != nil {
		t.Fatalf("parse csv: %v", err)
	}
	return rows
}

// TestBuildWeightZip_EmptyInput ensures a user with no entries still
// gets a valid zip containing the header CSV row and an empty manifest.
func TestBuildWeightZip_EmptyInput(t *testing.T) {
	var buf bytes.Buffer
	res, err := BuildWeightZip(context.Background(), &buf, nil, &fakePhotos{}, "user-1", "kg")
	if err != nil {
		t.Fatalf("BuildWeightZip: %v", err)
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
	if _, ok := files["weight.csv"]; !ok {
		t.Fatal("expected weight.csv in zip")
	}
	if _, ok := files["manifest.json"]; !ok {
		t.Fatal("expected manifest.json in zip")
	}
	// No photos directory entries when there are no entries.
	for name := range files {
		if strings.HasPrefix(name, "photos/") {
			t.Errorf("unexpected photo file %q in empty export", name)
		}
	}

	rows := mustReadCSV(t, files["weight.csv"])
	if len(rows) != 1 {
		t.Fatalf("expected 1 CSV row (header only), got %d", len(rows))
	}
	wantHeader := []string{"id", "created_at", "date", "weight", "weight_unit", "notes", "photo_filename"}
	if !equalRow(rows[0], wantHeader) {
		t.Errorf("csv header = %v, want %v", rows[0], wantHeader)
	}
}

// TestBuildWeightZip_MixedEntries covers the happy path: some entries
// with photos, some without. Asserts CSV row count, photo file
// presence, and that the manifest reports the right totals.
func TestBuildWeightZip_MixedEntries(t *testing.T) {
	day1 := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 16, 8, 0, 0, 0, time.UTC)
	day3 := time.Date(2026, 1, 23, 8, 0, 0, 0, time.UTC)

	entries := []models.WeightEntry{
		{ID: "entry-2-photo", UserID: "u1", Weight: 80.5, Notes: "second", PhotoKey: "weight/u1/b.jpg", CreatedAt: day2},
		{ID: "entry-1", UserID: "u1", Weight: 81.0, Notes: "first", PhotoKey: "", CreatedAt: day1},
		{ID: "entry-3-photo", UserID: "u1", Weight: 80.0, Notes: "third", PhotoKey: "weight/u1/c.png", CreatedAt: day3},
	}
	photos := &fakePhotos{
		data: map[string][]byte{
			"weight/u1/b.jpg": {0xFF, 0xD8, 0xFF, 0xE0}, // jpeg magic
			"weight/u1/c.png": {0x89, 0x50, 0x4E, 0x47}, // png magic
		},
	}

	var buf bytes.Buffer
	res, err := BuildWeightZip(context.Background(), &buf, entries, photos, "u1", "kg")
	if err != nil {
		t.Fatalf("BuildWeightZip: %v", err)
	}

	if res.Entries != 3 {
		t.Errorf("Entries = %d, want 3", res.Entries)
	}
	if res.PhotosWritten != 2 {
		t.Errorf("PhotosWritten = %d, want 2", res.PhotosWritten)
	}
	if len(res.MissingPhotos) != 0 {
		t.Errorf("MissingPhotos = %v, want empty", res.MissingPhotos)
	}

	files := readZip(t, buf.Bytes())
	rows := mustReadCSV(t, files["weight.csv"])
	if len(rows) != 4 {
		t.Fatalf("expected 4 rows (header + 3 entries), got %d", len(rows))
	}

	// Rows must be in date-ascending order regardless of input order.
	wantIDs := []string{"entry-1", "entry-2-photo", "entry-3-photo"}
	for i, want := range wantIDs {
		if rows[i+1][0] != want {
			t.Errorf("row %d id = %q, want %q", i+1, rows[i+1][0], want)
		}
	}

	// weight_unit column is filled on every row with the value passed in.
	for i := 1; i < len(rows); i++ {
		if rows[i][4] != "kg" {
			t.Errorf("row %d weight_unit = %q, want kg", i, rows[i][4])
		}
	}

	// The middle entry has a photo; CSV should reference it.
	mid := rows[2]
	if mid[6] == "" {
		t.Errorf("expected photo_filename on the second entry, got empty")
	}
	// And that filename must exist inside the photos/ dir of the zip.
	photoPath := "photos/" + mid[6]
	if _, ok := files[photoPath]; !ok {
		t.Errorf("expected %q in zip, but it's missing", photoPath)
	}

	// Manifest matches the result totals.
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
	if mf.EntryCount != 3 || mf.PhotoCount != 2 {
		t.Errorf("manifest counts = (%d, %d), want (3, 2)", mf.EntryCount, mf.PhotoCount)
	}
	if len(mf.MissingPhotos) != 0 {
		t.Errorf("manifest.missing_photos = %v, want empty", mf.MissingPhotos)
	}
}

// TestBuildWeightZip_MissingPhotoInR2 ensures a photo that R2 cannot
// serve does NOT abort the export: the entry is still in the CSV
// (with an empty photo_filename) and the key is recorded in the
// manifest's missing_photos list.
func TestBuildWeightZip_MissingPhotoInR2(t *testing.T) {
	day := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	entries := []models.WeightEntry{
		{ID: "e1", UserID: "u1", Weight: 80, Notes: "ok", PhotoKey: "weight/u1/ok.jpg", CreatedAt: day},
		{ID: "e2", UserID: "u1", Weight: 79, Notes: "broken", PhotoKey: "weight/u1/gone.jpg", CreatedAt: day.Add(24 * time.Hour)},
	}
	photos := &fakePhotos{
		data: map[string][]byte{
			"weight/u1/ok.jpg": {0x01, 0x02},
		},
		missing: map[string]bool{
			"weight/u1/gone.jpg": true,
		},
	}

	var buf bytes.Buffer
	res, err := BuildWeightZip(context.Background(), &buf, entries, photos, "u1", "lbs")
	if err != nil {
		t.Fatalf("BuildWeightZip: %v", err)
	}
	if res.PhotosWritten != 1 {
		t.Errorf("PhotosWritten = %d, want 1", res.PhotosWritten)
	}
	if len(res.MissingPhotos) != 1 || res.MissingPhotos[0] != "weight/u1/gone.jpg" {
		t.Errorf("MissingPhotos = %v, want [weight/u1/gone.jpg]", res.MissingPhotos)
	}

	files := readZip(t, buf.Bytes())
	rows := mustReadCSV(t, files["weight.csv"])
	if len(rows) != 3 {
		t.Fatalf("expected header + 2 rows, got %d", len(rows))
	}
	if rows[1][6] == "" {
		t.Errorf("expected photo_filename on ok row, got empty")
	}
	if rows[2][6] != "" {
		t.Errorf("expected empty photo_filename on broken row, got %q", rows[2][6])
	}
	// weight_unit column is filled on every row with the value passed in.
	if rows[1][4] != "lbs" || rows[2][4] != "lbs" {
		t.Errorf("weight_unit column = (%q, %q), want (lbs, lbs)", rows[1][4], rows[2][4])
	}

	var mf struct {
		MissingPhotos []string `json:"missing_photos"`
		PhotoCount    int      `json:"photo_count"`
	}
	if err := json.Unmarshal(files["manifest.json"], &mf); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if mf.PhotoCount != 1 {
		t.Errorf("manifest.photo_count = %d, want 1", mf.PhotoCount)
	}
	if len(mf.MissingPhotos) != 1 || mf.MissingPhotos[0] != "weight/u1/gone.jpg" {
		t.Errorf("manifest.missing_photos = %v", mf.MissingPhotos)
	}
}

// TestBuildWeightZip_CSVEscaping ensures notes containing commas,
// quotes and newlines are correctly escaped by encoding/csv.
func TestBuildWeightZip_CSVEscaping(t *testing.T) {
	day := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	entries := []models.WeightEntry{
		{ID: "e1", UserID: "u1", Weight: 80, Notes: `has, comma and "quote" and` + "\nnewline", CreatedAt: day},
	}

	var buf bytes.Buffer
	if _, err := BuildWeightZip(context.Background(), &buf, entries, &fakePhotos{}, "u1", "kg"); err != nil {
		t.Fatalf("BuildWeightZip: %v", err)
	}

	files := readZip(t, buf.Bytes())
	rows := mustReadCSV(t, files["weight.csv"])
	if len(rows) != 2 {
		t.Fatalf("expected header + 1 row, got %d", len(rows))
	}
	want := `has, comma and "quote" and` + "\nnewline"
	if rows[1][5] != want {
		t.Errorf("csv notes = %q, want %q", rows[1][5], want)
	}
}

// TestBuildWeightZip_PhotosSortedByDate ensures the photo filenames
// inside the photos/ directory appear in date order, which is what a
// user sees when they extract the zip and open the folder in a file
// browser.
func TestBuildWeightZip_PhotosSortedByDate(t *testing.T) {
	day1 := time.Date(2026, 1, 9, 8, 0, 0, 0, time.UTC)
	day2 := time.Date(2026, 1, 16, 8, 0, 0, 0, time.UTC)
	entries := []models.WeightEntry{
		// Out of order on purpose.
		{ID: "second", UserID: "u1", Weight: 80, PhotoKey: "weight/u1/b.jpg", CreatedAt: day2},
		{ID: "first", UserID: "u1", Weight: 81, PhotoKey: "weight/u1/a.jpg", CreatedAt: day1},
	}
	photos := &fakePhotos{data: map[string][]byte{
		"weight/u1/a.jpg": {0xAA},
		"weight/u1/b.jpg": {0xBB},
	}}

	var buf bytes.Buffer
	if _, err := BuildWeightZip(context.Background(), &buf, entries, photos, "u1", "kg"); err != nil {
		t.Fatalf("BuildWeightZip: %v", err)
	}

	files := readZip(t, buf.Bytes())
	var names []string
	for n := range files {
		if strings.HasPrefix(n, "photos/") {
			names = append(names, n)
		}
	}
	if len(names) != 2 {
		t.Fatalf("expected 2 photo files, got %d (%v)", len(names), names)
	}
	sort.Strings(names)
	if !strings.HasPrefix(names[0], "photos/2026-01-09_") || !strings.HasPrefix(names[1], "photos/2026-01-16_") {
		t.Errorf("photos not in date-ascending order: %v", names)
	}
}

func equalRow(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
