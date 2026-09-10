package trainingstats

import (
	"testing"
	"time"

	"hylete/internal/models"
)

func TestEpley1RM(t *testing.T) {
	if got := Epley1RM(1, 100); got != 100 {
		t.Fatalf("single: got %v", got)
	}
	if got := Epley1RM(10, 100); got < 133 || got > 134 {
		t.Fatalf("10x100: got %v", got)
	}
	if Epley1RM(0, 100) != 0 || Epley1RM(5, 0) != 0 {
		t.Fatal("non-positive input must be 0")
	}
}

func TestBuildThinData(t *testing.T) {
	now := time.Now()
	stats := Build(Input{Now: now})
	if !stats.ThinData {
		t.Fatal("empty input must be thin")
	}
	if stats.Sessions != 0 || stats.AdherencePct != 100 {
		t.Fatalf("unexpected %+v", stats)
	}
}

func TestBuildVolumeAndAdherence(t *testing.T) {
	now := time.Now()
	entry := func(daysAgo int, exID, name string, reps int, w float64) models.ExerciseEntry {
		return models.ExerciseEntry{
			ExerciseID: exID, ExerciseName: name, Reps: reps, Weight: w,
			CreatedAt: now.AddDate(0, 0, -daysAgo),
		}
	}
	// Baseline: 4 sessions over 4 weeks (1/wk), recent: 2 sessions this week.
	entries := []models.ExerciseEntry{
		entry(28, "ex1", "Bench", 5, 100),
		entry(21, "ex1", "Bench", 5, 100),
		entry(14, "ex1", "Bench", 5, 100),
		entry(8, "ex1", "Bench", 5, 100),
		entry(2, "ex1", "Bench", 5, 102.5),
		entry(1, "ex1", "Bench", 5, 102.5),
	}
	stats := Build(Input{Entries: entries, Now: now})
	if stats.Sessions != 2 {
		t.Fatalf("sessions: got %d", stats.Sessions)
	}
	if len(stats.Exercises) != 1 {
		t.Fatalf("exercises: %+v", stats.Exercises)
	}
	if stats.Exercises[0].TotalVolume != 2*5*102.5 {
		t.Fatalf("volume: %+v", stats.Exercises[0])
	}
	// Baseline avg 1/wk, recent 2 sessions → 200%.
	if stats.AdherencePct != 200 {
		t.Fatalf("adherence: got %v", stats.AdherencePct)
	}
}

func TestBuildCardio(t *testing.T) {
	now := time.Now()
	entries := []models.ExerciseEntry{
		{ExerciseID: "run", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: now.AddDate(0, 0, -1)},
		{ExerciseID: "run", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: now.AddDate(0, 0, -2)},
		{ExerciseID: "run", ExerciseName: "Run", ExerciseType: models.ExerciseTypeCardio, DurationSeconds: 1500, DistanceMeters: 5000, CreatedAt: now.AddDate(0, 0, -3)},
	}
	stats := Build(Input{Entries: entries, Now: now})
	if stats.ThinData {
		t.Fatal("3 sessions must not be thin")
	}
	ex := stats.Exercises[0]
	if !ex.IsCardio || ex.TotalDistanceM != 15000 || ex.TotalDurationS != 4500 {
		t.Fatalf("cardio: %+v", ex)
	}
	if ex.BestPaceSecPerKm != 300 {
		t.Fatalf("pace: %+v", ex)
	}
}

func TestPlateauFlag(t *testing.T) {
	now := time.Now()
	var entries []models.ExerciseEntry
	// Same 5x100 every week for 8 weeks: no PR after week 8-ago → plateaued.
	for w := 0; w < 8; w++ {
		entries = append(entries, models.ExerciseEntry{
			ExerciseID: "ex1", ExerciseName: "Bench", Reps: 5, Weight: 100,
			CreatedAt: now.AddDate(0, 0, -(w*7 + 1)),
		})
	}
	stats := Build(Input{Entries: entries, Now: now})
	if len(stats.Exercises) != 1 {
		t.Fatalf("exercises: %+v", stats)
	}
	if !stats.Exercises[0].Plateaued {
		t.Fatalf("expected plateau: %+v", stats.Exercises[0])
	}
}

func TestRecoveryNotesSparseIsSilent(t *testing.T) {
	now := time.Now()
	stats := Build(Input{Now: now})
	if len(stats.RecoveryNotes) != 0 {
		t.Fatalf("sparse health must be silent: %+v", stats.RecoveryNotes)
	}
}
