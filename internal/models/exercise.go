package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// ExerciseRepository provides CRUD operations for exercises using sqlc-generated queries.
type ExerciseRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewExerciseRepository creates a new exercise repository backed by sqlc.
func NewExerciseRepository(dbConn *db.DB) *ExerciseRepository {
	return &ExerciseRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Create creates a new exercise or returns the existing ID.
func (r *ExerciseRepository) Create(tx *sql.Tx, name string) (string, error) {
	ctx := context.Background()
	id := uuid.New().String()
	if tx != nil {
		return r.queries.WithTx(tx).Create(ctx, db.CreateParams{ID: id, Name: name})
	}
	return r.queries.Create(ctx, db.CreateParams{ID: id, Name: name})
}

// GetByName retrieves an exercise by its normalized name.
func (r *ExerciseRepository) GetByName(name string) (*Exercise, error) {
	ctx := context.Background()
	row, err := r.queries.GetByName(ctx, name)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise: %w", err)
	}
	return &Exercise{
		ID:             row.ID,
		Name:           row.Name,
		Description:    nullStringToString(row.Description),
		VideoURL:       nullStringToString(row.VideoUrl),
		ImgURL:         nullStringToString(row.ImgUrl),
		ImgURLOriginal: nullStringToString(row.ImgUrlOriginal),
		Type:           ExerciseType(row.Type),
	}, nil
}

// List returns all exercises ordered by name.
func (r *ExerciseRepository) List() ([]Exercise, error) {
	ctx := context.Background()
	rows, err := r.queries.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list exercises: %w", err)
	}

	exercises := make([]Exercise, len(rows))
	for i, row := range rows {
		exercises[i] = Exercise{
			ID:             row.ID,
			Name:           row.Name,
			Description:    nullStringToString(row.Description),
			VideoURL:       nullStringToString(row.VideoUrl),
			ImgURL:         nullStringToString(row.ImgUrl),
			ImgURLOriginal: nullStringToString(row.ImgUrlOriginal),
			Type:           ExerciseType(row.Type),
		}
	}
	return exercises, nil
}

// GetByID retrieves an exercise by its ID.
func (r *ExerciseRepository) GetByID(id string) (*Exercise, error) {
	ctx := context.Background()
	row, err := r.queries.GetByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise: %w", err)
	}
	return &Exercise{
		ID:             row.ID,
		Name:           row.Name,
		Description:    nullStringToString(row.Description),
		VideoURL:       nullStringToString(row.VideoUrl),
		ImgURL:         nullStringToString(row.ImgUrl),
		ImgURLOriginal: nullStringToString(row.ImgUrlOriginal),
		Type:           ExerciseType(row.Type),
	}, nil
}

// GetExerciseByID retrieves an exercise by its ID, scoped to a user.
// Returns nil if not found.
func (r *ExerciseRepository) GetExerciseByID(id string, userID string) (*Exercise, error) {
	ctx := context.Background()
	row, err := r.queries.GetByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise: %w", err)
	}
	return &Exercise{
		ID:             row.ID,
		Name:           row.Name,
		Description:    nullStringToString(row.Description),
		VideoURL:       nullStringToString(row.VideoUrl),
		ImgURL:         nullStringToString(row.ImgUrl),
		ImgURLOriginal: nullStringToString(row.ImgUrlOriginal),
		Type:           ExerciseType(row.Type),
	}, nil
}

// CreateNoTx creates a new exercise without a transaction.
func (r *ExerciseRepository) CreateNoTx(params CreateExerciseParams) (string, error) {
	ctx := context.Background()
	id := uuid.New().String()
	return r.queries.Create(ctx, db.CreateParams{
		ID:             id,
		Name:           params.Name,
		Description:    stringToNullString(params.Description),
		VideoUrl:       stringToNullString(params.VideoURL),
		ImgUrl:         stringToNullString(params.ImgURL),
		ImgUrlOriginal: stringToNullString(params.ImgURLOriginal),
		Type:           string(params.Type),
	})
}

// Update updates an exercise's metadata.
func (r *ExerciseRepository) Update(id string, params UpdateExerciseParams) (*Exercise, error) {
	ctx := context.Background()
	row, err := r.queries.Update(ctx, db.UpdateParams{
		Name:           params.Name,
		Description:    stringToNullString(params.Description),
		VideoUrl:       stringToNullString(params.VideoURL),
		ImgUrl:         stringToNullString(params.ImgURL),
		ImgUrlOriginal: stringToNullString(params.ImgURLOriginal),
		Type:           string(params.Type),
		ID:             id,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to update exercise: %w", err)
	}
	return &Exercise{
		ID:             row.ID,
		Name:           row.Name,
		Description:    nullStringToString(row.Description),
		VideoURL:       nullStringToString(row.VideoUrl),
		ImgURL:         nullStringToString(row.ImgUrl),
		ImgURLOriginal: nullStringToString(row.ImgUrlOriginal),
		Type:           ExerciseType(row.Type),
	}, nil
}

// CreateExerciseEntry persists a new exercise entry and links it to its exercise.
func (r *ExerciseRepository) CreateExerciseEntry(exerciseEntry *ExerciseEntry) error {
	return r.db.Transaction(func(tx *sql.Tx) error {
		ctx := context.Background()
		qtx := r.queries.WithTx(tx)

		entryUUID := uuid.New().String()
		_, err := qtx.CreateExerciseEntry(ctx, db.CreateExerciseEntryParams{
			ID:              entryUUID,
			ExerciseID:      exerciseEntry.ExerciseID,
			UserID:          sql.NullString{String: exerciseEntry.UserID, Valid: true},
			Reps:            int64(exerciseEntry.Reps),
			Weight:          exerciseEntry.Weight,
			Notes:           stringToNullString(exerciseEntry.Notes),
			RestTime:        int64(exerciseEntry.RestTime),
			DurationSeconds: int64(exerciseEntry.DurationSeconds),
			DistanceMeters:  exerciseEntry.DistanceMeters,
			AvgHeartRate:    int64(exerciseEntry.AvgHeartRate),
			CaloriesBurned:  exerciseEntry.CaloriesBurned,
			CreatedAt:       timeToNullTime(exerciseEntry.CreatedAt),
		})
		if err != nil {
			return fmt.Errorf("failed to create exercise entry: %w", err)
		}

		exerciseEntry.ID = entryUUID
		return nil
	})
}

// GetExerciseEntry retrieves a single exercise entry by ID with its exercise name.
// Returns nil if not found. Scopes to the given user ID.
func (r *ExerciseRepository) GetExerciseEntry(id string, userID string) (*ExerciseEntry, error) {
	ctx := context.Background()
	row, err := r.queries.GetExerciseEntry(ctx, db.GetExerciseEntryParams{
		ID:     id,
		UserID: sql.NullString{String: userID, Valid: true},
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise entry: %w", err)
	}
	return mapGetExerciseEntryRow(row), nil
}

// UpdateExerciseEntry updates an existing exercise entry without changing its created_at date.
// Scopes to the given user ID.
func (r *ExerciseRepository) UpdateExerciseEntry(exerciseEntry *ExerciseEntry, userID string) error {
	return r.db.Transaction(func(tx *sql.Tx) error {
		ctx := context.Background()
		qtx := r.queries.WithTx(tx)

		err := qtx.UpdateExerciseEntry(ctx, db.UpdateExerciseEntryParams{
			ExerciseID:      exerciseEntry.ExerciseID,
			Reps:            int64(exerciseEntry.Reps),
			Weight:          exerciseEntry.Weight,
			Notes:           stringToNullString(exerciseEntry.Notes),
			RestTime:        int64(exerciseEntry.RestTime),
			DurationSeconds: int64(exerciseEntry.DurationSeconds),
			DistanceMeters:  exerciseEntry.DistanceMeters,
			AvgHeartRate:    int64(exerciseEntry.AvgHeartRate),
			CaloriesBurned:  exerciseEntry.CaloriesBurned,
			ID:              exerciseEntry.ID,
			UserID:          sql.NullString{String: userID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update exercise entry: %w", err)
		}

		return nil
	})
}

// UpdateExerciseEntryWithDate updates an existing exercise entry including its created_at date.
// Scopes to the given user ID.
func (r *ExerciseRepository) UpdateExerciseEntryWithDate(exerciseEntry *ExerciseEntry, userID string) error {
	return r.db.Transaction(func(tx *sql.Tx) error {
		ctx := context.Background()
		qtx := r.queries.WithTx(tx)

		err := qtx.UpdateExerciseEntryWithDate(ctx, db.UpdateExerciseEntryWithDateParams{
			ExerciseID:      exerciseEntry.ExerciseID,
			Reps:            int64(exerciseEntry.Reps),
			Weight:          exerciseEntry.Weight,
			Notes:           stringToNullString(exerciseEntry.Notes),
			RestTime:        int64(exerciseEntry.RestTime),
			DurationSeconds: int64(exerciseEntry.DurationSeconds),
			DistanceMeters:  exerciseEntry.DistanceMeters,
			AvgHeartRate:    int64(exerciseEntry.AvgHeartRate),
			CaloriesBurned:  exerciseEntry.CaloriesBurned,
			CreatedAt:       timeToNullTime(exerciseEntry.CreatedAt),
			ID:              exerciseEntry.ID,
			UserID:          sql.NullString{String: userID, Valid: true},
		})
		if err != nil {
			return fmt.Errorf("failed to update exercise entry: %w", err)
		}

		return nil
	})
}

// DeleteExerciseEntry removes an exercise entry by ID. Scopes to the given user ID.
func (r *ExerciseRepository) DeleteExerciseEntry(id string, userID string) error {
	ctx := context.Background()
	err := r.queries.DeleteExerciseEntry(ctx, db.DeleteExerciseEntryParams{
		ID:     id,
		UserID: sql.NullString{String: userID, Valid: true},
	})
	if err != nil {
		return fmt.Errorf("failed to delete exercise entry: %w", err)
	}
	return nil
}

// ListExerciseEntries returns exercise entries ordered by created_at descending.
// If limit > 0, results are capped at that count. Scopes to the given user ID.
func (r *ExerciseRepository) ListExerciseEntries(userID string, limit int) ([]ExerciseEntry, error) {
	ctx := context.Background()

	var rows []db.ListExerciseEntriesRow
	var err error
	uid := sql.NullString{String: userID, Valid: true}
	if limit > 0 {
		var limited []db.ListExerciseEntriesWithLimitRow
		limited, err = r.queries.ListExerciseEntriesWithLimit(ctx, db.ListExerciseEntriesWithLimitParams{
			UserID: uid,
			Limit:  int64(limit),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list exercise entries: %w", err)
		}
		return mapListExerciseEntriesWithLimitRows(limited), nil
	}

	rows, err = r.queries.ListExerciseEntries(ctx, uid)
	if err != nil {
		return nil, fmt.Errorf("failed to list exercise entries: %w", err)
	}
	return mapListExerciseEntriesRows(rows), nil
}

// GetExerciseEntriesByExercisePaginated returns a page of exercise entries for a specific
// exercise ID, ordered by created_at descending. Scopes to the given user ID.
func (r *ExerciseRepository) GetExerciseEntriesByExercisePaginated(exerciseID string, userID string, limit, offset int) ([]ExerciseEntry, error) {
	ctx := context.Background()
	rows, err := r.queries.GetExerciseEntriesByExercisePaginated(ctx, db.GetExerciseEntriesByExercisePaginatedParams{
		ExerciseID: exerciseID,
		UserID:     sql.NullString{String: userID, Valid: true},
		Limit:      int64(limit),
		Offset:     int64(offset),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise entries by exercise: %w", err)
	}
	return mapGetExerciseEntriesByExercisePaginatedRows(rows), nil
}

// GetMaxWeightByExercise returns the heaviest weight logged for the given exercise by
// the given user, or 0 when no exercise entries exist. Scopes to the given user ID.
func (r *ExerciseRepository) GetMaxWeightByExercise(exerciseID string, userID string) (float64, error) {
	ctx := context.Background()
	max, err := r.queries.GetMaxWeightByExercise(ctx, db.GetMaxWeightByExerciseParams{
		ExerciseID: exerciseID,
		UserID:     sql.NullString{String: userID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get max weight by exercise: %w", err)
	}
	return max, nil
}

// GetBestPaceByExercise returns the fastest pace (seconds per kilometre) logged for
// the given exercise by the given user, or 0 when no qualifying exercise entries exist.
// Scopes to the given user ID.
func (r *ExerciseRepository) GetBestPaceByExercise(exerciseID string, userID string) (float64, error) {
	ctx := context.Background()
	best, err := r.queries.GetBestPaceByExercise(ctx, db.GetBestPaceByExerciseParams{
		ExerciseID: exerciseID,
		UserID:     sql.NullString{String: userID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get best pace by exercise: %w", err)
	}
	return best, nil
}

// GetLongestDistanceByExercise returns the longest distance (metres) logged for the
// given exercise by the given user, or 0 when no exercise entries exist.
// Scopes to the given user ID.
func (r *ExerciseRepository) GetLongestDistanceByExercise(exerciseID string, userID string) (float64, error) {
	ctx := context.Background()
	longest, err := r.queries.GetLongestDistanceByExercise(ctx, db.GetLongestDistanceByExerciseParams{
		ExerciseID: exerciseID,
		UserID:     sql.NullString{String: userID, Valid: true},
	})
	if err != nil {
		return 0, fmt.Errorf("failed to get longest distance by exercise: %w", err)
	}
	return longest, nil
}

// GetLastSetByExercise returns the most recent exercise entry for the given exercise by the
// given user. Returns (nil, nil) when no exercise entries exist. Scopes to the given user ID.
func (r *ExerciseRepository) GetLastSetByExercise(exerciseID string, userID string) (*ExerciseEntry, error) {
	ctx := context.Background()
	row, err := r.queries.GetLastSetByExercise(ctx, db.GetLastSetByExerciseParams{
		ExerciseID: exerciseID,
		UserID:     sql.NullString{String: userID, Valid: true},
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get last set by exercise: %w", err)
	}
	return mapGetLastSetByExerciseRow(row), nil
}

// GetExerciseEntriesByDateRange returns exercise entries within an inclusive date range.
// Scopes to the given user ID.
func (r *ExerciseRepository) GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]ExerciseEntry, error) {
	ctx := context.Background()
	rows, err := r.queries.GetExerciseEntriesByDateRange(ctx, db.GetExerciseEntriesByDateRangeParams{
		CreatedAt:   timeToNullTime(start),
		CreatedAt_2: timeToNullTime(end),
		UserID:      sql.NullString{String: userID, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get exercise entries by date range: %w", err)
	}
	return mapGetExerciseEntriesByDateRangeRows(rows), nil
}

// ListExerciseEntriesLast7Days returns exercise entries from the last 7 days ordered by
// created_at descending. Scopes to the given user ID.
func (r *ExerciseRepository) ListExerciseEntriesLast7Days(userID string) ([]ExerciseEntry, error) {
	ctx := context.Background()
	rows, err := r.queries.ListExerciseEntriesLast7Days(ctx, sql.NullString{String: userID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to list exercise entries last 7 days: %w", err)
	}
	return mapListExerciseEntriesLast7DaysRows(rows), nil
}

// --- Mapping helpers ---

// mapExerciseEntryFields copies the shared exercise entry columns from a
// sqlc row into the domain model. Every SELECT in the exercise entries
// queries returns the same column list, so one helper keeps all six
// per-query mapping helpers in sync.
func mapExerciseEntryFields(target *ExerciseEntry, id, exerciseID, exerciseName, exerciseType, userID string, reps int64, weight float64, notes string, restTime int64, durationSeconds int64, distanceMeters float64, avgHeartRate int64, caloriesBurned float64, createdAt sql.NullTime) {
	target.ID = id
	target.ExerciseID = exerciseID
	target.ExerciseName = exerciseName
	target.ExerciseType = ExerciseType(exerciseType)
	target.UserID = userID
	target.Reps = int(reps)
	target.Weight = weight
	target.Notes = notes
	target.RestTime = int(restTime)
	target.DurationSeconds = int(durationSeconds)
	target.DistanceMeters = distanceMeters
	target.AvgHeartRate = int(avgHeartRate)
	target.CaloriesBurned = caloriesBurned
	target.CreatedAt = nullTimeToTime(createdAt)
}

func mapGetExerciseEntryRow(row db.GetExerciseEntryRow) *ExerciseEntry {
	e := &ExerciseEntry{}
	mapExerciseEntryFields(e, row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	return e
}

func mapListExerciseEntriesRows(rows []db.ListExerciseEntriesRow) []ExerciseEntry {
	exerciseEntries := make([]ExerciseEntry, len(rows))
	for i, row := range rows {
		mapExerciseEntryFields(&exerciseEntries[i], row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	}
	return exerciseEntries
}

func mapListExerciseEntriesWithLimitRows(rows []db.ListExerciseEntriesWithLimitRow) []ExerciseEntry {
	exerciseEntries := make([]ExerciseEntry, len(rows))
	for i, row := range rows {
		mapExerciseEntryFields(&exerciseEntries[i], row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	}
	return exerciseEntries
}

func mapGetExerciseEntriesByExercisePaginatedRows(rows []db.GetExerciseEntriesByExercisePaginatedRow) []ExerciseEntry {
	exerciseEntries := make([]ExerciseEntry, len(rows))
	for i, row := range rows {
		mapExerciseEntryFields(&exerciseEntries[i], row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	}
	return exerciseEntries
}

func mapGetLastSetByExerciseRow(row db.GetLastSetByExerciseRow) *ExerciseEntry {
	e := &ExerciseEntry{}
	mapExerciseEntryFields(e, row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	return e
}

func mapGetExerciseEntriesByDateRangeRows(rows []db.GetExerciseEntriesByDateRangeRow) []ExerciseEntry {
	exerciseEntries := make([]ExerciseEntry, len(rows))
	for i, row := range rows {
		mapExerciseEntryFields(&exerciseEntries[i], row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	}
	return exerciseEntries
}

func mapListExerciseEntriesLast7DaysRows(rows []db.ListExerciseEntriesLast7DaysRow) []ExerciseEntry {
	exerciseEntries := make([]ExerciseEntry, len(rows))
	for i, row := range rows {
		mapExerciseEntryFields(&exerciseEntries[i], row.ID, row.ExerciseID, row.ExerciseName, row.ExerciseType, nullStringToString(row.UserID), row.Reps, row.Weight, nullStringToString(row.Notes), row.RestTime, row.DurationSeconds, row.DistanceMeters, row.AvgHeartRate, row.CaloriesBurned, row.CreatedAt)
	}
	return exerciseEntries
}

func nullStringToString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

func stringToNullString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: true}
}

func nullTimeToTime(nt sql.NullTime) time.Time {
	if nt.Valid {
		return nt.Time
	}
	return time.Time{}
}

func timeToNullTime(t time.Time) sql.NullTime {
	return sql.NullTime{Time: t, Valid: !t.IsZero()}
}
