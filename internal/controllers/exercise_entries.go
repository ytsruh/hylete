// Package controllers provides business logic for the strength tracker application.
package controllers

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"time"

	"hylete/internal/export"
	"hylete/internal/models"
)

// Sentinel errors returned by ValidateExerciseSetInput so callers (the JSON
// API handlers) can map them to human-readable 400 responses while unit
// tests can assert on identity with errors.Is.
var (
	// ErrRepsRequired is returned when a strength exercise entry is submitted without repetitions.
	ErrRepsRequired = errors.New("reps must be at least 1")
	// ErrDurationRequired is returned when a cardio exercise entry is submitted without a duration.
	ErrDurationRequired = errors.New("duration is required for cardio exercises")
	// ErrDistanceRequired is returned when a cardio exercise entry is submitted without a distance.
	ErrDistanceRequired = errors.New("distance is required for cardio exercises")
)

// ExerciseEntryController handles exercise entry business logic.
type ExerciseEntryController struct {
	repo models.Repository
}

// NewExerciseEntryController creates a new ExerciseEntryController instance.
func NewExerciseEntryController(repo models.Repository) *ExerciseEntryController {
	return &ExerciseEntryController{repo: repo}
}

// ListExerciseEntriesLast7Days returns exercise entries from the last 7 days for a user.
func (ec *ExerciseEntryController) ListExerciseEntriesLast7Days(userID string) ([]models.ExerciseEntry, error) {
	return ec.repo.ListExerciseEntriesLast7Days(userID)
}

// GetExerciseEntriesByDateRange returns the user's exercise entries
// within the inclusive [start, end] range, ordered by created_at
// descending. Thin wrapper over the repository method so the JSON
// API route handler can ask for a custom date window (e.g. "last
// 30 days") without going around the controller layer.
func (ec *ExerciseEntryController) GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]models.ExerciseEntry, error) {
	return ec.repo.GetExerciseEntriesByDateRange(start, end, userID)
}

// GetExerciseEntry retrieves a single exercise entry by ID, scoped to the user.
func (ec *ExerciseEntryController) GetExerciseEntry(id, userID string) (*models.ExerciseEntry, error) {
	return ec.repo.GetExerciseEntry(id, userID)
}

// ExerciseSetInput describes a single set to be persisted as part of a multi-set
// exercise entry submission. All sets within a submission share an exercise,
// user, notes and timestamp; only per-set values differ. Strength sets carry
// Reps/Weight/RestTime, cardio sets carry DurationSeconds/DistanceMeters plus
// optional AvgHeartRate/CaloriesBurned — which pair applies is decided by the
// exercise's type via ValidateExerciseSetInput and normalizeForExerciseType.
type ExerciseSetInput struct {
	Reps            int
	Weight          float64
	RestTime        int
	DurationSeconds int
	DistanceMeters  float64
	AvgHeartRate    int
	CaloriesBurned  float64
}

// ValidateExerciseSetInput checks one set against the requirements for its
// exercise's type: strength entries need at least one rep, cardio entries need
// both a positive duration and a positive distance. Numeric range limits
// (max weight, max duration…) are enforced earlier by the request validators;
// this covers the type-conditional rules they cannot express.
func ValidateExerciseSetInput(exerciseType models.ExerciseType, in ExerciseSetInput) error {
	switch exerciseType {
	case models.ExerciseTypeCardio:
		if in.DurationSeconds <= 0 {
			return ErrDurationRequired
		}
		if in.DistanceMeters <= 0 {
			return ErrDistanceRequired
		}
	default:
		if in.Reps < 1 {
			return ErrRepsRequired
		}
	}
	return nil
}

// normalizeForExerciseType zeroes the metric pair that does not apply to the
// given exercise type so exactly one pair is ever non-zero on disk: strength
// entries never keep cardio metrics and vice versa. Rest time is also dropped
// from cardio entries because there is no per-set rest to rest between.
func (in ExerciseSetInput) normalizeForExerciseType(exerciseType models.ExerciseType) ExerciseSetInput {
	if exerciseType == models.ExerciseTypeCardio {
		in.Reps = 0
		in.Weight = 0
		in.RestTime = 0
		return in
	}
	in.DurationSeconds = 0
	in.DistanceMeters = 0
	in.AvgHeartRate = 0
	in.CaloriesBurned = 0
	return in
}

// CreateExerciseEntries persists a group of sets in a single submission, all
// sharing the same exercise, user, notes and the supplied createdAt timestamp.
// The timestamp comes from the caller (parsed from the client's created_at
// field, or time.Now() when absent) so a multi-set submission can be
// back-dated as a single unit. Each set is validated against the exercise's
// type and normalized (the non-applicable metric pair is zeroed) before being
// written. On the first repository error the loop aborts and the error is
// returned; partial-success semantics aren't worth the complexity for a
// workout log.
func (ec *ExerciseEntryController) CreateExerciseEntries(userID, exerciseID string, exerciseType models.ExerciseType, notes string, createdAt time.Time, sets []ExerciseSetInput) ([]models.ExerciseEntry, error) {
	for _, s := range sets {
		if err := ValidateExerciseSetInput(exerciseType, s); err != nil {
			return nil, err
		}
	}

	created := make([]models.ExerciseEntry, 0, len(sets))
	for _, s := range sets {
		s = s.normalizeForExerciseType(exerciseType)
		exerciseEntry := &models.ExerciseEntry{
			ExerciseID:      exerciseID,
			Notes:           notes,
			Reps:            s.Reps,
			Weight:          s.Weight,
			RestTime:        s.RestTime,
			DurationSeconds: s.DurationSeconds,
			DistanceMeters:  s.DistanceMeters,
			AvgHeartRate:    s.AvgHeartRate,
			CaloriesBurned:  s.CaloriesBurned,
			UserID:          userID,
			CreatedAt:       createdAt,
		}
		if err := ec.repo.CreateExerciseEntry(exerciseEntry); err != nil {
			return nil, err
		}
		created = append(created, *exerciseEntry)
	}
	return created, nil
}

// UpdateExerciseEntry updates an existing exercise entry, including its timestamp.
// The entry is validated and normalized against the supplied exercise type (the
// caller resolves it from the existing row or the newly linked exercise) so an
// edit cannot leave both metric pairs populated.
func (ec *ExerciseEntryController) UpdateExerciseEntry(id, userID string, exerciseID string, exerciseType models.ExerciseType, notes string, in ExerciseSetInput, createdAt time.Time) (*models.ExerciseEntry, error) {
	if err := ValidateExerciseSetInput(exerciseType, in); err != nil {
		return nil, err
	}
	in = in.normalizeForExerciseType(exerciseType)
	exerciseEntry := &models.ExerciseEntry{
		ID:              id,
		ExerciseID:      exerciseID,
		Notes:           notes,
		Reps:            in.Reps,
		Weight:          in.Weight,
		RestTime:        in.RestTime,
		DurationSeconds: in.DurationSeconds,
		DistanceMeters:  in.DistanceMeters,
		AvgHeartRate:    in.AvgHeartRate,
		CaloriesBurned:  in.CaloriesBurned,
		UserID:          userID,
		CreatedAt:       createdAt,
	}
	if err := ec.repo.UpdateExerciseEntryWithDate(exerciseEntry, userID); err != nil {
		return nil, err
	}
	return exerciseEntry, nil
}

// DeleteExerciseEntry removes an exercise entry by ID, scoped to the user.
func (ec *ExerciseEntryController) DeleteExerciseEntry(id, userID string) error {
	return ec.repo.DeleteExerciseEntry(id, userID)
}

// List returns all exercises.
func (ec *ExerciseEntryController) List() ([]models.Exercise, error) {
	return ec.repo.List()
}

// ExerciseHistoryPageSize is the number of exercise entries shown per page on
// the exercise history view. Exposed as a constant so views and tests can rely on it.
const ExerciseHistoryPageSize = 25

// ExerciseHistoryChartSize is the number of most-recent exercise entries fetched
// to drive the line chart on the exercise history page. Kept relatively small
// because the chart sits in a narrow two-column grid cell on desktop, and
// only one point is plotted per calendar day (max weight) so beyond this
// size the additional points are usually redundant.
const ExerciseHistoryChartSize = 30

// GetExerciseEntriesByExercise returns a paginated page of exercise entries for
// a specific exercise ID along with the user's lifetime stats for that
// exercise. Pages are 1-indexed; invalid pages are clamped to 1. Scopes to
// the given user ID.
func (ec *ExerciseEntryController) GetExerciseEntriesByExercise(exerciseID string, userID string, page int) (*models.ExerciseHistoryPage, error) {
	if page < 1 {
		page = 1
	}
	offset := (page - 1) * ExerciseHistoryPageSize

	// Fetch one extra row to detect whether a next page exists without a COUNT(*).
	rows, err := ec.repo.GetExerciseEntriesByExercisePaginated(exerciseID, userID, ExerciseHistoryPageSize+1, offset)
	if err != nil {
		return nil, err
	}
	hasNext := len(rows) > ExerciseHistoryPageSize
	if hasNext {
		rows = rows[:ExerciseHistoryPageSize]
	}

	stats, err := ec.loadHistoryStats(exerciseID, userID)
	if err != nil {
		return nil, err
	}

	return &models.ExerciseHistoryPage{
		ExerciseEntries: rows,
		Stats:           stats,
		Page:            page,
		HasPrev:         page > 1,
		HasNext:         hasNext,
	}, nil
}

// loadHistoryStats fetches the personal best and most recent set for the header
// stat cards, plus the cardio personal bests (fastest pace, longest distance).
// Strength callers read MaxWeight; cardio callers read BestPaceSecPerKm /
// LongestDistanceMeters — everything is always loaded so the caller only needs
// the exercise's type to pick. Always reflects the user's full history, not
// just the current page.
func (ec *ExerciseEntryController) loadHistoryStats(exerciseID string, userID string) (models.HistoryStats, error) {
	maxWeight, err := ec.repo.GetMaxWeightByExercise(exerciseID, userID)
	if err != nil {
		return models.HistoryStats{}, err
	}
	lastSet, err := ec.repo.GetLastSetByExercise(exerciseID, userID)
	if err != nil {
		return models.HistoryStats{}, err
	}
	bestPace, err := ec.repo.GetBestPaceByExercise(exerciseID, userID)
	if err != nil {
		return models.HistoryStats{}, err
	}
	longestDistance, err := ec.repo.GetLongestDistanceByExercise(exerciseID, userID)
	if err != nil {
		return models.HistoryStats{}, err
	}
	stats := models.HistoryStats{
		MaxWeight:             maxWeight,
		BestPaceSecPerKm:      bestPace,
		LongestDistanceMeters: longestDistance,
	}
	if lastSet != nil {
		stats.LastSet = *lastSet
	}
	return stats, nil
}

// GetExerciseByID returns an exercise by its UUID.
func (ec *ExerciseEntryController) GetExerciseByID(id, userID string) (*models.Exercise, error) {
	return ec.repo.GetExerciseByID(id, userID)
}

// GetRecentExerciseEntriesForChart returns the most recent ExerciseHistoryChartSize
// exercise entries for the given exercise, scoped to the user. It feeds the
// line chart rendered on the exercise history page; the view groups the
// returned exercise entries by day and plots the heaviest set of each day.
func (ec *ExerciseEntryController) GetRecentExerciseEntriesForChart(exerciseID, userID string) ([]models.ExerciseEntry, error) {
	return ec.repo.GetExerciseEntriesByExercisePaginated(exerciseID, userID, ExerciseHistoryChartSize, 0)
}

// GetAllExerciseEntriesForChart returns every exercise entry the user has
// logged for the given exercise, scoped to the user. It feeds the dedicated
// /chart view, which renders a full-width line chart of the user's full
// workout history for that exercise. The view is responsible for aggregating
// to a daily series (heaviest weight per calendar day) before plotting.
// math.MaxInt32 is used as a sentinel for "no limit" — the underlying
// paginated repo method is reused to avoid introducing a new SQL query.
func (ec *ExerciseEntryController) GetAllExerciseEntriesForChart(exerciseID, userID string) ([]models.ExerciseEntry, error) {
	return ec.repo.GetExerciseEntriesByExercisePaginated(exerciseID, userID, math.MaxInt32, 0)
}

// SuggestedExerciseEntriesExportFilename returns the recommended filename
// for a user's exercise entries export zip. Exposed so the route layer
// (and tests) don't need to know the format.
func SuggestedExerciseEntriesExportFilename(now time.Time) string {
	return "hylete-exercise-entries-export-" + now.UTC().Format("2006-01-02") + ".zip"
}

// ExportExerciseEntriesZip builds a streaming zip archive of the user's
// exercise entries and returns a reader the caller can pipe straight to
// the HTTP response. The string returned alongside is the suggested
// filename for the Content-Disposition header.
//
// The export is built in a background goroutine writing into an io.Pipe,
// mirroring WeightController.ExportWeightZip so the HTTP layer can start
// sending bytes as soon as the first zip entry is written — no in-memory
// buffering of the full archive.
//
// weightUnit and distanceUnit are the user's preferred display units and
// are recorded in the CSV so the file is self-describing.
//
// The error returned covers failures up to the point of returning the
// pipe reader; once streaming has started, errors are surfaced by the
// goroutine closing the pipe with the error, which io.Copy propagates to
// the response writer.
func (ec *ExerciseEntryController) ExportExerciseEntriesZip(ctx context.Context, userID string, weightUnit string, distanceUnit string) (io.Reader, string, error) {
	entries, err := ec.repo.ListExerciseEntries(userID, 0)
	if err != nil {
		return nil, "", fmt.Errorf("load exercise entries: %w", err)
	}

	pr, pw := io.Pipe()
	filename := SuggestedExerciseEntriesExportFilename(time.Now())

	go func() {
		// Use a detached context for the actual build so an
		// in-flight export isn't cancelled when the request
		// context is. The pipe closure is the only way to
		// signal a mid-stream error to io.Copy.
		buildCtx := context.Background()
		_, buildErr := export.BuildExerciseEntriesZip(buildCtx, pw, entries, userID, weightUnit, distanceUnit)
		if buildErr != nil {
			_ = pw.CloseWithError(buildErr)
			return
		}
		_ = pw.Close()
	}()

	return pr, filename, nil
}
