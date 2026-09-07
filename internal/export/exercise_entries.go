package export

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"time"

	"hylete/internal/models"
)

// BuildExerciseEntriesZip writes a zip archive to w containing the
// user's exercise entries (one row per set).
//
// Layout of the archive:
//   - exercise_entries.csv CSV with one row per entry (header included)
//   - manifest.json        export metadata
//
// Exercise entries have no binary assets of their own (unlike weight
// entries, whose photos live in R2), so there is no photos/ phase and
// the manifest always reports photo_count = 0 with an empty
// missing_photos list.
//
// Entries are sorted ascending by CreatedAt on a copy of the input so
// the resulting files are deterministic regardless of repository
// order, mirroring BuildWeightZip.
//
// weightUnit and distanceUnit are the user's preferred display units
// ("kg"/"lbs" and "km"/"mi"). The DB stores weights as a unit-agnostic
// number and distances in metres, so the CSV's `weight` column is
// written verbatim with `weight_unit` recording the passed value, while
// `distance_km` converts metres to kilometres for readability. Cardio
// metric columns are 0 on strength rows and vice versa, matching the
// server-side normalization; `exercise_type` tells the two apart.
//
// ctx is unused today but is kept first in the signature for parity
// with BuildWeightZip (whose photo fetches take it) so callers can
// treat both builders identically.
func BuildExerciseEntriesZip(ctx context.Context, w io.Writer, entries []models.ExerciseEntry, userID string, weightUnit string, distanceUnit string) (Result, error) {
	// Sort a copy so the caller's slice is untouched.
	sorted := make([]models.ExerciseEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	result := Result{Entries: len(sorted), MissingPhotos: []string{}}

	zw := zip.NewWriter(w)

	csvBody, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "exercise_entries.csv",
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: create csv entry: %w", err)
	}
	cw := csv.NewWriter(csvBody)
	if err := cw.Write([]string{
		"id", "created_at", "date", "exercise_id", "exercise_name", "exercise_type",
		"reps", "weight", "weight_unit", "rest_time_seconds",
		"duration_seconds", "distance_km", "avg_heart_rate_bpm", "calories_burned", "pace_sec_per_km",
		"notes",
	}); err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: write csv header: %w", err)
	}
	for _, e := range sorted {
		pace := ""
		if p := e.PaceSecPerKm(); p > 0 {
			pace = strconv.FormatFloat(p, 'f', 2, 64)
		}
		if err := cw.Write([]string{
			e.ID,
			e.CreatedAt.UTC().Format(time.RFC3339),
			e.CreatedAt.UTC().Format("2006-01-02"),
			e.ExerciseID,
			e.ExerciseName,
			string(e.ExerciseType),
			strconv.Itoa(e.Reps),
			fmt.Sprintf("%.1f", e.Weight),
			weightUnit,
			strconv.Itoa(e.RestTime),
			strconv.Itoa(e.DurationSeconds),
			fmt.Sprintf("%.3f", e.DistanceMeters/1000.0),
			strconv.Itoa(e.AvgHeartRate),
			fmt.Sprintf("%.1f", e.CaloriesBurned),
			pace,
			e.Notes,
		}); err != nil {
			_ = zw.Close()
			return result, fmt.Errorf("export: write csv row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: flush csv: %w", err)
	}

	mf, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "manifest.json",
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: create manifest: %w", err)
	}
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest{
		UserID:        userID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		EntryCount:    result.Entries,
		PhotoCount:    0,
		MissingPhotos: result.MissingPhotos,
	}); err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: encode manifest: %w", err)
	}

	if err := zw.Close(); err != nil {
		return result, fmt.Errorf("export: close zip: %w", err)
	}
	return result, nil
}
