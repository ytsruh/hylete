package controllers

import (
	"database/sql"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"hylete/internal/models"
)

type mockRepository struct {
	mu              sync.Mutex
	exercises       []models.Exercise
	exerciseEntries []models.ExerciseEntry

	errCreate                              error
	errGetByName                           error
	errList                                error
	errCreateExerciseEntry                 error
	errGetExerciseEntry                    error
	errUpdateExerciseEntry                 error
	errUpdateExerciseEntryWithDate         error
	errDeleteExerciseEntry                 error
	errListExerciseEntries                 error
	errGetExerciseEntriesByExercisePaginated error
	errGetExerciseEntriesByDateRange       error
	errGetExerciseByID                     error
	errGetMaxWeightByExercise              error
	errGetBestPaceByExercise               error
	errGetLongestDistanceByExercise        error
	errGetLastSetByExercise                error
}

func newMockRepository() *mockRepository {
	return &mockRepository{}
}

func (m *mockRepository) Create(_ *sql.Tx, name string) (string, error) {
	if m.errCreate != nil {
		return "", m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == name {
			return e.ID, nil
		}
	}
	id := "ex-" + name
	m.exercises = append(m.exercises, models.Exercise{ID: id, Name: name})
	return id, nil
}

func (m *mockRepository) GetByName(name string) (*models.Exercise, error) {
	if m.errGetByName != nil {
		return nil, m.errGetByName
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == name {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) List() ([]models.Exercise, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]models.Exercise, len(m.exercises))
	copy(result, m.exercises)
	return result, nil
}

func (m *mockRepository) CreateExerciseEntry(exerciseEntry *models.ExerciseEntry) error {
	if m.errCreateExerciseEntry != nil {
		return m.errCreateExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, e := range m.exercises {
		if e.ID == exerciseEntry.ExerciseID {
			exerciseEntry.ExerciseName = e.Name
			exerciseEntry.ExerciseType = e.Type
			break
		}
	}
	exerciseEntry.ID = "entry-" + exerciseEntry.ExerciseID
	m.exerciseEntries = append(m.exerciseEntries, *exerciseEntry)
	return nil
}

func (m *mockRepository) GetExerciseEntry(id string, userID string) (*models.ExerciseEntry, error) {
	if m.errGetExerciseEntry != nil {
		return nil, m.errGetExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exerciseEntries {
		if e.ID == id && e.UserID == userID {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockRepository) UpdateExerciseEntry(exerciseEntry *models.ExerciseEntry, userID string) error {
	if m.errUpdateExerciseEntry != nil {
		return m.errUpdateExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exerciseEntries {
		if e.ID == exerciseEntry.ID && e.UserID == userID {
			m.exerciseEntries[i] = *exerciseEntry
			return nil
		}
	}
	return nil
}

func (m *mockRepository) UpdateExerciseEntryWithDate(exerciseEntry *models.ExerciseEntry, userID string) error {
	if m.errUpdateExerciseEntryWithDate != nil {
		return m.errUpdateExerciseEntryWithDate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exerciseEntries {
		if e.ID == exerciseEntry.ID && e.UserID == userID {
			m.exerciseEntries[i] = *exerciseEntry
			return nil
		}
	}
	return nil
}

func (m *mockRepository) DeleteExerciseEntry(id string, userID string) error {
	if m.errDeleteExerciseEntry != nil {
		return m.errDeleteExerciseEntry
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exerciseEntries {
		if e.ID == id && e.UserID == userID {
			m.exerciseEntries = append(m.exerciseEntries[:i], m.exerciseEntries[i+1:]...)
			return nil
		}
	}
	return nil
}

func (m *mockRepository) ListExerciseEntries(userID string, limit int) ([]models.ExerciseEntry, error) {
	if m.errListExerciseEntries != nil {
		return nil, m.errListExerciseEntries
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if e.UserID == userID {
			result = append(result, e)
		}
	}
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}
	return result, nil
}

func (m *mockRepository) GetExerciseEntriesByExercisePaginated(exerciseID string, userID string, limit, offset int) ([]models.ExerciseEntry, error) {
	if m.errGetExerciseEntriesByExercisePaginated != nil {
		return nil, m.errGetExerciseEntriesByExercisePaginated
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var all []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if e.ExerciseID == exerciseID && e.UserID == userID {
			all = append(all, e)
		}
	}
	if offset >= len(all) {
		return []models.ExerciseEntry{}, nil
	}
	end := offset + limit
	if end > len(all) {
		end = len(all)
	}
	return all[offset:end], nil
}

func (m *mockRepository) GetMaxWeightByExercise(exerciseID string, userID string) (float64, error) {
	if m.errGetMaxWeightByExercise != nil {
		return 0, m.errGetMaxWeightByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var max float64
	for _, e := range m.exerciseEntries {
		if e.ExerciseID == exerciseID && e.UserID == userID && e.Weight > max {
			max = e.Weight
		}
	}
	return max, nil
}

func (m *mockRepository) GetBestPaceByExercise(exerciseID string, userID string) (float64, error) {
	if m.errGetBestPaceByExercise != nil {
		return 0, m.errGetBestPaceByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	best := 0.0
	for _, e := range m.exerciseEntries {
		if e.ExerciseID != exerciseID || e.UserID != userID {
			continue
		}
		if pace := e.PaceSecPerKm(); pace > 0 && (best == 0 || pace < best) {
			best = pace
		}
	}
	return best, nil
}

func (m *mockRepository) GetLongestDistanceByExercise(exerciseID string, userID string) (float64, error) {
	if m.errGetLongestDistanceByExercise != nil {
		return 0, m.errGetLongestDistanceByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var longest float64
	for _, e := range m.exerciseEntries {
		if e.ExerciseID == exerciseID && e.UserID == userID && e.DistanceMeters > longest {
			longest = e.DistanceMeters
		}
	}
	return longest, nil
}

func (m *mockRepository) GetLastSetByExercise(exerciseID string, userID string) (*models.ExerciseEntry, error) {
	if m.errGetLastSetByExercise != nil {
		return nil, m.errGetLastSetByExercise
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var last *models.ExerciseEntry
	for i, e := range m.exerciseEntries {
		if e.ExerciseID != exerciseID || e.UserID != userID {
			continue
		}
		if last == nil || e.CreatedAt.After(last.CreatedAt) {
			cp := m.exerciseEntries[i]
			last = &cp
		}
	}
	return last, nil
}

func (m *mockRepository) GetExerciseEntriesByDateRange(start, end time.Time, userID string) ([]models.ExerciseEntry, error) {
	if m.errGetExerciseEntriesByDateRange != nil {
		return nil, m.errGetExerciseEntriesByDateRange
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if !e.CreatedAt.Before(start) && !e.CreatedAt.After(end) && e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockRepository) ListExerciseEntriesLast7Days(userID string) ([]models.ExerciseEntry, error) {
	if m.errListExerciseEntries != nil {
		return nil, m.errListExerciseEntries
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	sevenDaysAgo := time.Now().AddDate(0, 0, -7)
	var result []models.ExerciseEntry
	for _, e := range m.exerciseEntries {
		if e.CreatedAt.After(sevenDaysAgo) && e.UserID == userID {
			result = append(result, e)
		}
	}
	return result, nil
}

func (m *mockRepository) GetExerciseByID(id string, userID string) (*models.Exercise, error) {
	if m.errGetExerciseByID != nil {
		return nil, m.errGetExerciseByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.ID == id {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func setupExerciseEntryController(t *testing.T) (*ExerciseEntryController, *mockRepository) {
	t.Helper()
	mock := newMockRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
		{ID: "ex-2", Name: "Bench Press"},
	}
	return NewExerciseEntryController(mock), mock
}

func TestExerciseEntryController_GetExerciseEntry(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)
	mock := ec.repo.(*mockRepository)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
	}

	exerciseEntry, err := ec.GetExerciseEntry("entry-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exerciseEntry.ExerciseName != "Squat" {
		t.Errorf("expected 'Squat', got %q", exerciseEntry.ExerciseName)
	}
}

func TestExerciseEntryController_GetExerciseEntry_WrongUser(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)
	mock := ec.repo.(*mockRepository)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
	}

	exerciseEntry, err := ec.GetExerciseEntry("entry-1", "user-999")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exerciseEntry != nil {
		t.Error("expected nil exercise entry for wrong user")
	}
}

func TestExerciseEntryController_CreateExerciseEntries_Success(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)

	sets := []ExerciseSetInput{
		{Reps: 5, Weight: 100, RestTime: 60},
		{Reps: 5, Weight: 100, RestTime: 60},
		{Reps: 5, Weight: 95, RestTime: 90},
	}

	created, err := ec.CreateExerciseEntries("user-1", "ex-1", models.ExerciseTypeStrength, "felt good", time.Now(), sets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 3 {
		t.Fatalf("expected 3 exercise entries, got %d", len(created))
	}
	if len(mock.exerciseEntries) != 3 {
		t.Fatalf("expected 3 exercise entries in mock, got %d", len(mock.exerciseEntries))
	}

	// All exercise entries share the same exercise, user, notes and timestamp.
	first := created[0]
	for i, e := range created {
		if e.ExerciseID != "ex-1" {
			t.Errorf("exercise entry %d: expected exercise 'ex-1', got %q", i, e.ExerciseID)
		}
		if e.UserID != "user-1" {
			t.Errorf("exercise entry %d: expected user 'user-1', got %q", i, e.UserID)
		}
		if e.Notes != "felt good" {
			t.Errorf("exercise entry %d: expected notes 'felt good', got %q", i, e.Notes)
		}
		if !e.CreatedAt.Equal(first.CreatedAt) {
			t.Errorf("exercise entry %d: expected shared timestamp %v, got %v", i, first.CreatedAt, e.CreatedAt)
		}
	}

	// Per-set values are preserved in submission order.
	if created[2].Weight != 95 || created[2].RestTime != 90 {
		t.Errorf("third set values wrong: %+v", created[2])
	}
}

func TestExerciseEntryController_CreateExerciseEntries_EmptySets(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)

	created, err := ec.CreateExerciseEntries("user-1", "ex-1", models.ExerciseTypeStrength, "", time.Now(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 0 {
		t.Errorf("expected 0 exercise entries, got %d", len(created))
	}
	if len(mock.exerciseEntries) != 0 {
		t.Errorf("expected mock to be empty, got %d exercise entries", len(mock.exerciseEntries))
	}
}

func TestExerciseEntryController_CreateExerciseEntries_RepositoryErrorShortCircuits(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	mock.errCreateExerciseEntry = errors.New("db error")

	sets := []ExerciseSetInput{
		{Reps: 5, Weight: 100, RestTime: 0},
		{Reps: 5, Weight: 100, RestTime: 0},
	}

	_, err := ec.CreateExerciseEntries("user-1", "ex-1", models.ExerciseTypeStrength, "", time.Now(), sets)
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "db error") {
		t.Errorf("expected db error to bubble up, got %v", err)
	}
}

// TestExerciseEntryController_CreateExerciseEntries_PassesCreatedAt verifies
// that the caller-supplied createdAt is the exact value persisted on every
// row, including a back-dated timestamp that is clearly not "now".
func TestExerciseEntryController_CreateExerciseEntries_PassesCreatedAt(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	want := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)

	sets := []ExerciseSetInput{
		{Reps: 5, Weight: 100, RestTime: 60},
		{Reps: 5, Weight: 95, RestTime: 90},
	}

	created, err := ec.CreateExerciseEntries("user-1", "ex-1", models.ExerciseTypeStrength, "back-dated", want, sets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("expected 2 exercise entries, got %d", len(created))
	}
	for i, e := range created {
		if !e.CreatedAt.Equal(want) {
			t.Errorf("exercise entry %d: expected CreatedAt %v, got %v", i, want, e.CreatedAt)
		}
	}
	if len(mock.exerciseEntries) != 2 {
		t.Fatalf("expected 2 exercise entries in mock, got %d", len(mock.exerciseEntries))
	}
	for i, e := range mock.exerciseEntries {
		if !e.CreatedAt.Equal(want) {
			t.Errorf("mock exercise entry %d: expected CreatedAt %v, got %v", i, want, e.CreatedAt)
		}
	}
}

func TestExerciseEntryController_UpdateExerciseEntry(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)
	mock := ec.repo.(*mockRepository)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
	}

	exerciseEntry, err := ec.UpdateExerciseEntry("entry-1", "user-1", "ex-1", models.ExerciseTypeStrength, "even better", ExerciseSetInput{Reps: 6, Weight: 110, RestTime: 90}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exerciseEntry.Reps != 6 {
		t.Errorf("expected 6 reps, got %d", exerciseEntry.Reps)
	}
}

// --- Cardio validation & normalization ---

func TestValidateExerciseSetInput(t *testing.T) {
	tests := []struct {
		name        string
		exerciseTyp models.ExerciseType
		input       ExerciseSetInput
		wantErr     error
	}{
		{
			name:        "strength with reps passes",
			exerciseTyp: models.ExerciseTypeStrength,
			input:       ExerciseSetInput{Reps: 5},
			wantErr:     nil,
		},
		{
			name:        "strength without reps fails",
			exerciseTyp: models.ExerciseTypeStrength,
			input:       ExerciseSetInput{Reps: 0, Weight: 100},
			wantErr:     ErrRepsRequired,
		},
		{
			name:        "cardio with duration and distance passes",
			exerciseTyp: models.ExerciseTypeCardio,
			input:       ExerciseSetInput{DurationSeconds: 1500, DistanceMeters: 5000},
			wantErr:     nil,
		},
		{
			name:        "cardio without duration fails",
			exerciseTyp: models.ExerciseTypeCardio,
			input:       ExerciseSetInput{DistanceMeters: 5000},
			wantErr:     ErrDurationRequired,
		},
		{
			name:        "cardio without distance fails",
			exerciseTyp: models.ExerciseTypeCardio,
			input:       ExerciseSetInput{DurationSeconds: 1500},
			wantErr:     ErrDistanceRequired,
		},
		{
			name:        "other type follows strength rules",
			exerciseTyp: models.ExerciseTypeOther,
			input:       ExerciseSetInput{Reps: 3},
			wantErr:     nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateExerciseSetInput(tt.exerciseTyp, tt.input)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected error %v, got %v", tt.wantErr, err)
			}
		})
	}
}

// TestExerciseEntryController_CreateExerciseEntries_CardioNormalizesMetrics
// verifies that cardio sets keep their duration/distance metrics while the
// server zeroes reps/weight/rest — a client cannot sneak strength metrics
// onto a cardio entry.
func TestExerciseEntryController_CreateExerciseEntries_CardioNormalizesMetrics(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)

	sets := []ExerciseSetInput{
		{Reps: 99, Weight: 999, RestTime: 60, DurationSeconds: 1500, DistanceMeters: 5000, AvgHeartRate: 152, CaloriesBurned: 320},
	}

	created, err := ec.CreateExerciseEntries("user-1", "ex-2", models.ExerciseTypeCardio, "easy run", time.Now(), sets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := created[0]
	if got.Reps != 0 || got.Weight != 0 || got.RestTime != 0 {
		t.Errorf("expected strength metrics zeroed on cardio entry, got %+v", got)
	}
	if got.DurationSeconds != 1500 || got.DistanceMeters != 5000 || got.AvgHeartRate != 152 || got.CaloriesBurned != 320 {
		t.Errorf("expected cardio metrics preserved, got %+v", got)
	}
	if len(mock.exerciseEntries) != 1 {
		t.Fatalf("expected 1 exercise entry in mock, got %d", len(mock.exerciseEntries))
	}
}

// TestExerciseEntryController_CreateExerciseEntries_StrengthNormalizesMetrics
// mirrors the cardio test: strength sets must not carry stray cardio
// metrics through to persistence.
func TestExerciseEntryController_CreateExerciseEntries_StrengthNormalizesMetrics(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)

	sets := []ExerciseSetInput{
		{Reps: 5, Weight: 100, RestTime: 90, DurationSeconds: 600, DistanceMeters: 2000, AvgHeartRate: 140, CaloriesBurned: 100},
	}

	created, err := ec.CreateExerciseEntries("user-1", "ex-1", models.ExerciseTypeStrength, "", time.Now(), sets)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := created[0]
	if got.DurationSeconds != 0 || got.DistanceMeters != 0 || got.AvgHeartRate != 0 || got.CaloriesBurned != 0 {
		t.Errorf("expected cardio metrics zeroed on strength entry, got %+v", got)
	}
	if got.Reps != 5 || got.Weight != 100 || got.RestTime != 90 {
		t.Errorf("expected strength metrics preserved, got %+v", got)
	}
}

// TestExerciseEntryController_CreateExerciseEntries_CardioValidationRejects
// asserts the whole multi-set submission is rejected (before any row is
// written) when one cardio set is missing its mandatory metrics.
func TestExerciseEntryController_CreateExerciseEntries_CardioValidationRejects(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)

	sets := []ExerciseSetInput{
		{DurationSeconds: 1500, DistanceMeters: 5000}, // valid
		{DurationSeconds: 900},                        // missing distance
	}

	_, err := ec.CreateExerciseEntries("user-1", "ex-2", models.ExerciseTypeCardio, "", time.Now(), sets)
	if !errors.Is(err, ErrDistanceRequired) {
		t.Fatalf("expected ErrDistanceRequired, got %v", err)
	}
	if len(mock.exerciseEntries) != 0 {
		t.Errorf("expected no rows persisted on validation failure, got %d", len(mock.exerciseEntries))
	}
}

// TestExerciseEntryController_UpdateExerciseEntry_Cardio verifies the
// update path validates and normalizes cardio edits too.
func TestExerciseEntryController_UpdateExerciseEntry_Cardio(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseID: "ex-2", Reps: 0, Weight: 0, DurationSeconds: 1500, DistanceMeters: 5000},
	}

	updated, err := ec.UpdateExerciseEntry("entry-1", "user-1", "ex-2", models.ExerciseTypeCardio, "negative split",
		ExerciseSetInput{Reps: 77, Weight: 77, DurationSeconds: 1440, DistanceMeters: 5000}, time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updated.Reps != 0 || updated.Weight != 0 {
		t.Errorf("expected client-sent strength metrics zeroed on cardio update, got %+v", updated)
	}
	if updated.DurationSeconds != 1440 || updated.DistanceMeters != 5000 {
		t.Errorf("expected cardio metrics updated, got %+v", updated)
	}

	// Missing distance must reject.
	_, err = ec.UpdateExerciseEntry("entry-1", "user-1", "ex-2", models.ExerciseTypeCardio, "",
		ExerciseSetInput{DurationSeconds: 1440}, time.Now())
	if !errors.Is(err, ErrDistanceRequired) {
		t.Fatalf("expected ErrDistanceRequired on cardio update, got %v", err)
	}
}

func TestExerciseEntryController_DeleteExerciseEntry(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)
	mock := ec.repo.(*mockRepository)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseName: "Squat", Reps: 5, Weight: 100},
	}

	err := ec.DeleteExerciseEntry("entry-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(mock.exerciseEntries) != 0 {
		t.Fatalf("expected exercise entry to be deleted, got %d exercise entries", len(mock.exerciseEntries))
	}
}

func TestExerciseEntryController_List(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)

	exercises, err := ec.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(exercises) != 2 {
		t.Fatalf("expected 2 exercises, got %d", len(exercises))
	}
}

func TestExerciseEntryController_List_Error(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	mock.errList = errors.New("db error")

	_, err := ec.List()
	if err == nil {
		t.Fatal("expected error from repository")
	}
}

func TestExerciseEntryController_GetExerciseEntriesByExercise(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	now := time.Now()
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 5, Weight: 100, CreatedAt: now.Add(-2 * time.Hour)},
		{ID: "entry-2", UserID: "user-1", ExerciseID: "ex-2", ExerciseName: "Bench Press", Reps: 8, Weight: 80, CreatedAt: now.Add(-1 * time.Hour)},
		{ID: "entry-3", UserID: "user-1", ExerciseID: "ex-1", ExerciseName: "Squat", Reps: 3, Weight: 110, CreatedAt: now},
	}

	page, err := ec.GetExerciseEntriesByExercise("ex-1", "user-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page.ExerciseEntries) != 2 {
		t.Fatalf("expected 2 squat exercise entries, got %d", len(page.ExerciseEntries))
	}
	if page.Stats.MaxWeight != 110 {
		t.Errorf("expected max weight 110, got %v", page.Stats.MaxWeight)
	}
	if page.Stats.LastSet.Weight != 110 {
		t.Errorf("expected last set weight 110, got %v", page.Stats.LastSet.Weight)
	}
}

func TestExerciseEntryController_GetExerciseEntriesByExercise_Pagination(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	now := time.Now()
	var exerciseEntries []models.ExerciseEntry
	for i := 0; i < 30; i++ {
		exerciseEntries = append(exerciseEntries, models.ExerciseEntry{
			ID:         "entry-" + string(rune('a'+i)),
			UserID:     "user-1",
			ExerciseID: "ex-1",
			Reps:       5,
			Weight:     float64(100 + i),
			CreatedAt:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	mock.exerciseEntries = exerciseEntries

	page1, err := ec.GetExerciseEntriesByExercise("ex-1", "user-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page1.ExerciseEntries) != ExerciseHistoryPageSize {
		t.Fatalf("page 1: expected %d exercise entries, got %d", ExerciseHistoryPageSize, len(page1.ExerciseEntries))
	}
	if !page1.HasNext {
		t.Error("page 1 should have a next page")
	}
	if page1.HasPrev {
		t.Error("page 1 should not have a previous page")
	}
	if page1.Page != 1 {
		t.Errorf("expected page 1, got %d", page1.Page)
	}
	// Stats should reflect the full history, not the slice.
	if page1.Stats.MaxWeight != 129 {
		t.Errorf("expected max weight 129 across all 30 exercise entries, got %v", page1.Stats.MaxWeight)
	}

	page2, err := ec.GetExerciseEntriesByExercise("ex-1", "user-1", 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page2.ExerciseEntries) != 5 {
		t.Fatalf("page 2: expected 5 exercise entries (30 - 25), got %d", len(page2.ExerciseEntries))
	}
	if page2.HasNext {
		t.Error("page 2 (last page) should not have a next page")
	}
	if !page2.HasPrev {
		t.Error("page 2 should have a previous page")
	}

	page3, err := ec.GetExerciseEntriesByExercise("ex-1", "user-1", 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(page3.ExerciseEntries) != 0 {
		t.Fatalf("page 3 (beyond data): expected 0 exercise entries, got %d", len(page3.ExerciseEntries))
	}
	if page3.HasNext {
		t.Error("page 3 (beyond data) should not have a next page")
	}
	if !page3.HasPrev {
		t.Error("page 3 (beyond data) should still have a previous page")
	}
}

func TestExerciseEntryController_GetExerciseEntriesByExercise_ClampsInvalidPage(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	mock.exerciseEntries = []models.ExerciseEntry{
		{ID: "entry-1", UserID: "user-1", ExerciseID: "ex-1", Reps: 5, Weight: 100},
	}

	page, err := ec.GetExerciseEntriesByExercise("ex-1", "user-1", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Page != 1 {
		t.Errorf("expected page to be clamped to 1, got %d", page.Page)
	}
}

func TestExerciseEntryController_GetExerciseEntriesByExercise_EmptyStats(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)

	page, err := ec.GetExerciseEntriesByExercise("ex-1", "user-1", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if page.Stats.MaxWeight != 0 {
		t.Errorf("expected max weight 0 for empty history, got %v", page.Stats.MaxWeight)
	}
	if page.Stats.LastSet.ID != "" {
		t.Errorf("expected zero-value last set for empty history, got %+v", page.Stats.LastSet)
	}
	if page.HasNext || page.HasPrev {
		t.Error("empty history should have no pagination state")
	}
}

func TestExerciseEntryController_GetRecentExerciseEntriesForChart(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	now := time.Now()
	var exerciseEntries []models.ExerciseEntry
	// 50 exercise entries across two users / two exercises; the chart should
	// only receive the matching (exercise, user) pair, capped at chart size.
	for i := 0; i < 50; i++ {
		exerciseID := "ex-1"
		userID := "user-1"
		if i%2 == 0 {
			exerciseID = "ex-other"
		}
		if i%3 == 0 {
			userID = "user-other"
		}
		exerciseEntries = append(exerciseEntries, models.ExerciseEntry{
			ID:         "entry-" + string(rune('a'+i%26)) + string(rune('A'+i/26)),
			UserID:     userID,
			ExerciseID: exerciseID,
			Reps:       5,
			Weight:     float64(100 + i),
			CreatedAt:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	mock.exerciseEntries = exerciseEntries

	got, err := ec.GetRecentExerciseEntriesForChart("ex-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) > ExerciseHistoryChartSize {
		t.Errorf("expected at most %d exercise entries, got %d", ExerciseHistoryChartSize, len(got))
	}
	for _, e := range got {
		if e.ExerciseID != "ex-1" || e.UserID != "user-1" {
			t.Errorf("exercise entry from other scope leaked into chart: %+v", e)
		}
	}
}

func TestExerciseEntryController_GetRecentExerciseEntriesForChart_Empty(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)
	got, err := ec.GetRecentExerciseEntriesForChart("ex-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty chart exercise entries, got %d", len(got))
	}
}

// TestExerciseEntryController_GetAllExerciseEntriesForChart verifies the
// dedicated /chart-view controller method returns every exercise entry the
// user has logged for the given exercise — not the 30-entry cap used by the
// small chart on the history page.
func TestExerciseEntryController_GetAllExerciseEntriesForChart(t *testing.T) {
	ec, mock := setupExerciseEntryController(t)
	now := time.Now()
	var exerciseEntries []models.ExerciseEntry
	// 100 exercise entries for the target (exercise, user) pair — well over
	// the 30-entry cap used by GetRecentExerciseEntriesForChart. We also
	// seed rows for other exercises and other users to confirm the
	// underlying paginated repo call scopes correctly.
	for i := 0; i < 100; i++ {
		exerciseID := "ex-1"
		userID := "user-1"
		if i%5 == 0 {
			exerciseID = "ex-other"
		}
		if i%7 == 0 {
			userID = "user-other"
		}
		exerciseEntries = append(exerciseEntries, models.ExerciseEntry{
			ID:         "entry-" + string(rune('a'+i%26)) + string(rune('A'+i/26)),
			UserID:     userID,
			ExerciseID: exerciseID,
			Reps:       5,
			Weight:     float64(100 + i),
			CreatedAt:  now.Add(time.Duration(i) * time.Minute),
		})
	}
	mock.exerciseEntries = exerciseEntries

	got, err := ec.GetAllExerciseEntriesForChart("ex-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected exercise entries to be returned, got 0")
	}
	// The 30-entry cap from GetRecentExerciseEntriesForChart must NOT apply here.
	if len(got) <= ExerciseHistoryChartSize {
		t.Errorf("expected more than %d exercise entries (uncapped), got %d", ExerciseHistoryChartSize, len(got))
	}
	// Every returned exercise entry must be scoped to the requested (exercise, user).
	for _, e := range got {
		if e.ExerciseID != "ex-1" || e.UserID != "user-1" {
			t.Errorf("exercise entry from other scope leaked into chart: %+v", e)
		}
	}
}

// TestExerciseEntryController_GetAllExerciseEntriesForChart_Empty asserts the
// method returns a (possibly nil) empty slice with no error when the user has
// no exercise entries for the exercise.
func TestExerciseEntryController_GetAllExerciseEntriesForChart_Empty(t *testing.T) {
	ec, _ := setupExerciseEntryController(t)
	got, err := ec.GetAllExerciseEntriesForChart("ex-1", "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty chart exercise entries, got %d", len(got))
	}
}
