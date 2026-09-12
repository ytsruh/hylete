package routes

import (
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// --- Workout route test fakes ---

// fakeWorkoutRepo is an in-memory models.WorkoutRepo for the workout
// route tests. IDs are sequential ("w-1" ...) and reads are scoped to
// the owning user, mirroring the real repository's contract.
type fakeWorkoutRepo struct {
	mu       sync.Mutex
	workouts map[string]*models.Workout
	trees    map[string][]models.WorkoutBlockWithItems
	seq      int
}

func newFakeWorkoutRepo() *fakeWorkoutRepo {
	return &fakeWorkoutRepo{
		workouts: map[string]*models.Workout{},
		trees:    map[string][]models.WorkoutBlockWithItems{},
	}
}

func (f *fakeWorkoutRepo) CreateWorkoutWithTree(userID string, sourceWorkoutID *string, name, notes string, status models.WorkoutStatus, start, end *time.Time, blocks []models.WorkoutBlockInput) (*models.Workout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	w := &models.Workout{
		ID: "w-" + fmt.Sprint(f.seq), UserID: userID, SourceWorkoutID: sourceWorkoutID,
		Name: name, Notes: notes, Status: status,
		ScheduledStart: start, ScheduledEnd: end,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	f.workouts[w.ID] = w
	for i, b := range blocks {
		wb := models.WorkoutBlockWithItems{
			Block: models.WorkoutBlock{
				ID:        "wb-" + w.ID + "-" + fmt.Sprint(i),
				WorkoutID: w.ID, Type: b.Type, Position: i, Rounds: b.Rounds,
				RestBetweenRoundsSeconds: b.RestBetweenRoundsSeconds,
				IntervalSeconds:          b.IntervalSeconds, TimeCapSeconds: b.TimeCapSeconds,
			},
		}
		for j, item := range b.Items {
			wb.Items = append(wb.Items, models.WorkoutItem{
				ID:      "wi-" + wb.Block.ID + "-" + fmt.Sprint(j),
				BlockID: wb.Block.ID, ExerciseID: item.ExerciseID, Position: j,
				TargetSets: item.TargetSets,
				TargetReps: item.Targets.TargetReps, TargetWeight: item.Targets.TargetWeight,
				TargetDurationSeconds: item.Targets.TargetDurationSeconds,
				TargetDistanceMeters:  item.Targets.TargetDistanceMeters,
			})
		}
		f.trees[w.ID] = append(f.trees[w.ID], wb)
	}
	cp := *w
	return &cp, nil
}

func (f *fakeWorkoutRepo) GetWorkout(workoutID, userID string) (*models.Workout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[workoutID]
	if !ok || w.UserID != userID {
		return nil, nil
	}
	cp := *w
	return &cp, nil
}

func (f *fakeWorkoutRepo) GetWorkoutTree(workoutID, userID string) (*models.Workout, []models.WorkoutBlockWithItems, error) {
	w, err := f.GetWorkout(workoutID, userID)
	if err != nil || w == nil {
		return nil, nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return w, f.trees[workoutID], nil
}

func (f *fakeWorkoutRepo) ListWorkouts(userID string) ([]models.Workout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []models.Workout{}
	for _, w := range f.workouts {
		if w.UserID == userID {
			out = append(out, *w)
		}
	}
	return out, nil
}

func (f *fakeWorkoutRepo) ListWorkoutsByRange(userID string, start, end time.Time) ([]models.Workout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []models.Workout{}
	for _, w := range f.workouts {
		if w.UserID != userID || w.ScheduledStart == nil {
			continue
		}
		if w.Status == models.WorkoutStatusCancelled {
			continue
		}
		if !w.ScheduledStart.After(end) && (w.ScheduledEnd == nil || !w.ScheduledEnd.Before(start)) {
			out = append(out, *w)
		}
	}
	return out, nil
}

func (f *fakeWorkoutRepo) UpdateWorkoutHeader(workoutID, userID, name, notes string, start, end *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[workoutID]; ok && w.UserID == userID {
		w.Name = name
		w.Notes = notes
		w.ScheduledStart = start
		w.ScheduledEnd = end
	}
	return nil
}

func (f *fakeWorkoutRepo) SetWorkoutStatus(workoutID, userID string, status models.WorkoutStatus, completedAt *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[workoutID]; ok && w.UserID == userID {
		w.Status = status
		w.CompletedAt = completedAt
	}
	return nil
}

func (f *fakeWorkoutRepo) DeleteWorkout(workoutID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[workoutID]; ok && w.UserID == userID {
		delete(f.workouts, workoutID)
		delete(f.trees, workoutID)
	}
	return nil
}

func (f *fakeWorkoutRepo) BulkCreate(userID string, builds []models.WorkoutBuild) ([]models.Workout, error) {
	out := make([]models.Workout, 0, len(builds))
	for _, b := range builds {
		created, err := f.CreateWorkoutWithTree(userID, b.SourceWorkoutID, b.Name, b.Notes, b.Status, b.Start, b.End, b.Blocks)
		if err != nil {
			return nil, err
		}
		out = append(out, *created)
	}
	return out, nil
}

func (f *fakeWorkoutRepo) GetWorkoutItemContext(workoutItemID, userID string) (string, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for workoutID, blocks := range f.trees {
		w, ok := f.workouts[workoutID]
		if !ok || w.UserID != userID {
			continue
		}
		for _, b := range blocks {
			for _, item := range b.Items {
				if item.ID == workoutItemID {
					return workoutID, b.Block.ID, nil
				}
			}
		}
	}
	return "", "", sql.ErrNoRows
}

// setupWorkoutHandler extends the shared setupHandler with wired
// workout controllers backed by the in-memory fakes above, so the
// workout routes run their real controller logic end to end. The
// shared exercise mock doubles as the exercise catalogue (seeded
// with ex-1/ex-2 by setupHandler).
func setupWorkoutHandler(t *testing.T) (*Handler, *mockRepository, *mockUserRepository, *fakeWorkoutRepo, *echo.Echo) {
	t.Helper()
	h, mock, mockUser, e := setupHandler(t)
	workouts := newFakeWorkoutRepo()
	h.SetWorkoutServices(
		controllers.NewWorkoutController(workouts, mock, mock),
	)
	return h, mock, mockUser, workouts, e
}

// strengthWorkoutBlock is one straight block prescribing 3x8 squats
// on ex-1. The shared catalogue seeds ex-1/ex-2 with an empty type,
// which validates under the default (strength) branch.
func strengthWorkoutBlock() WorkoutBlockInputDTO {
	return WorkoutBlockInputDTO{
		Type:   "straight",
		Rounds: 1,
		Items: []WorkoutItemInputDTO{
			{ExerciseID: "ex-1", TargetSets: 3, TargetReps: 8, TargetWeight: 60},
		},
	}
}

// --- /workouts ---

func TestAPIWorkouts_CreateAndDetail(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, _ := loginUser(t, h, mockUser, "w@example.com", "W")

	// Create with a tree and a schedule.
	start := time.Now().Add(24 * time.Hour).UTC()
	end := start.Add(time.Hour)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "Monday legs", Notes: "squats",
		ScheduledStart: &start, ScheduledEnd: &end,
		Blocks: []WorkoutBlockInputDTO{strengthWorkoutBlock()},
	})
	detail := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)
	if detail.Status != "planned" {
		t.Fatalf("status = %q", detail.Status)
	}
	if len(detail.Blocks) != 1 || len(detail.Blocks[0].Items) != 1 {
		t.Fatalf("tree not saved: %+v", detail.Blocks)
	}
	if item := detail.Blocks[0].Items[0]; item.TargetReps != 8 || item.TargetWeight != 60 {
		t.Fatalf("targets = %+v", item)
	}

	// Detail fetch echoes the tree with empty entry lists (non-nil).
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+detail.ID, token, nil)
	fetched := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusOK)
	if fetched.LoggedEntries == nil || fetched.AdHocEntries == nil {
		t.Fatalf("entry lists must be non-nil: %+v", fetched)
	}

	// Missing workout is a 404.
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts/missing", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPICreateWorkout_ValidationError(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, _ := loginUser(t, h, mockUser, "wv@example.com", "WV")

	// Unknown block type is rejected by the validator tags.
	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name:   "Bad",
		Blocks: []WorkoutBlockInputDTO{{Type: "tabata", Items: []WorkoutItemInputDTO{{ExerciseID: "ex-1"}}}},
	})
	decodeAPIError(t, rec, http.StatusBadRequest)

	// Unknown exercise is a controller-level 400.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name:   "Bad exercise",
		Blocks: []WorkoutBlockInputDTO{{Type: "straight", Items: []WorkoutItemInputDTO{{ExerciseID: "nope", TargetReps: 5}}}},
	})
	apiErr := decodeAPIError(t, rec, http.StatusBadRequest)
	if apiErr.Error == "" {
		t.Fatal("expected error message")
	}
}

func TestAPIDuplicateWorkout(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, _ := loginUser(t, h, mockUser, "dup@example.com", "Dup")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "Lower A", Blocks: []WorkoutBlockInputDTO{strengthWorkoutBlock()},
	})
	created := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)

	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate", token, DuplicateWorkoutRequest{})
	dup := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)
	if dup.Name != "Copy of Lower A" {
		t.Fatalf("name = %q", dup.Name)
	}
	if dup.ID == created.ID {
		t.Error("duplicate shares the source ID")
	}
	if len(dup.Blocks) != 1 || len(dup.Blocks[0].Items) != 1 {
		t.Fatalf("tree not copied: %+v", dup.Blocks)
	}

	// Missing source is a 404.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/missing/duplicate", token, DuplicateWorkoutRequest{})
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIWorkouts_Bulk(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, _ := loginUser(t, h, mockUser, "bulk@example.com", "Bulk")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "Lower A", Blocks: []WorkoutBlockInputDTO{strengthWorkoutBlock()},
	})
	source := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)

	base := time.Now().Add(24 * time.Hour).UTC()
	mkRange := func(days int) (time.Time, time.Time) {
		s := base.Add(time.Duration(days*24) * time.Hour)
		return s, s.Add(time.Hour)
	}
	s1, e1 := mkRange(0)
	s2, e2 := mkRange(7)
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/bulk", token, BulkCreateWorkoutsRequest{
		WorkoutID: source.ID,
		Instances: []BulkWorkoutInstanceDTO{
			{ScheduledStart: &s1, ScheduledEnd: &e1},
			{Name: "Week 2", ScheduledStart: &s2, ScheduledEnd: &e2},
		},
	})
	created := decodeAPI[[]WorkoutDTO](t, rec, http.StatusCreated)
	if len(created) != 2 {
		t.Fatalf("len = %d, want 2", len(created))
	}
	if created[0].Name != "Lower A" || created[1].Name != "Week 2" {
		t.Fatalf("names = %q, %q", created[0].Name, created[1].Name)
	}

	// Missing source is a 404.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/bulk", token, BulkCreateWorkoutsRequest{
		WorkoutID: "missing",
		Instances: []BulkWorkoutInstanceDTO{{Name: "x"}},
	})
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIWorkouts_StatusFlow(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, _ := loginUser(t, h, mockUser, "st@example.com", "ST")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{Name: "legs"})
	detail := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)

	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+detail.ID+"/complete", token, nil)
	completed := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusOK)
	if completed.Status != "completed" || completed.CompletedAt == nil {
		t.Fatalf("completed = %+v", completed)
	}

	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+detail.ID+"/reopen", token, nil)
	reopened := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusOK)
	if reopened.Status != "planned" || reopened.CompletedAt != nil {
		t.Fatalf("reopened = %+v", reopened)
	}

	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+detail.ID+"/cancel", token, nil)
	cancelled := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusOK)
	if cancelled.Status != "cancelled" {
		t.Fatalf("cancelled = %+v", cancelled)
	}

	rec = apiDo(t, e, http.MethodDelete, "/api/v1/workouts/"+detail.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+detail.ID, token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)

	// Missing workout status change is a 404.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/missing/complete", token, nil)
	decodeAPIError(t, rec, http.StatusNotFound)
}

func TestAPIWorkouts_ListAndRange(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, _ := loginUser(t, h, mockUser, "lr@example.com", "LR")

	base := time.Now().Add(24 * time.Hour).UTC()
	s1, e1 := base, base.Add(time.Hour)
	s2, e2 := base.Add(30*24*time.Hour), base.Add(30*24*time.Hour+time.Hour)
	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "Soon", ScheduledStart: &s1, ScheduledEnd: &e1,
	})
	soon := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "Later", ScheduledStart: &s2, ScheduledEnd: &e2,
	})
	decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)

	// Unfiltered list returns both.
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts", token, nil)
	list := decodeAPI[[]WorkoutDTO](t, rec, http.StatusOK)
	if len(list) != 2 {
		t.Fatalf("len = %d, want 2", len(list))
	}

	// Range covering only the first week returns one.
	from := base.Add(-time.Hour).Format(time.RFC3339)
	to := base.Add(7 * 24 * time.Hour).Format(time.RFC3339)
	rec = apiDo(t, e, "GET", "/api/v1/workouts?from="+from+"&to="+to, token, nil)
	ranged := decodeAPI[[]WorkoutDTO](t, rec, http.StatusOK)
	if len(ranged) != 1 || ranged[0].ID != soon.ID {
		t.Fatalf("ranged = %+v", ranged)
	}

	// Half-specified range is a 400.
	rec = apiDo(t, e, "GET", "/api/v1/workouts?from="+from, token, nil)
	decodeAPIError(t, rec, http.StatusBadRequest)

	// Cancelling the workout clears it from the range (the calendar)
	// while the unfiltered list still returns it.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+soon.ID+"/cancel", token, nil)
	decodeAPI[WorkoutDetailDTO](t, rec, http.StatusOK)
	rec = apiDo(t, e, "GET", "/api/v1/workouts?from="+from+"&to="+to, token, nil)
	ranged = decodeAPI[[]WorkoutDTO](t, rec, http.StatusOK)
	if len(ranged) != 0 {
		t.Fatalf("cancelled workout still ranged: %+v", ranged)
	}
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts", token, nil)
	list = decodeAPI[[]WorkoutDTO](t, rec, http.StatusOK)
	if len(list) != 2 {
		t.Fatalf("unfiltered len = %d, want 2", len(list))
	}
}

// --- Exercise-entry workout linkage ---

func TestAPICreateExerciseEntries_WithWorkoutLinkage(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutHandler(t)
	token, user := loginUser(t, h, mockUser, "link@example.com", "Link")
	_ = user

	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "legs", Blocks: []WorkoutBlockInputDTO{strengthWorkoutBlock()},
	})
	workout := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)
	itemID := workout.Blocks[0].Items[0].ID

	// Linked logging echoes the linkage.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets:       []CreateSetInput{{Reps: 5, Weight: 100}},
		WorkoutID:  workout.ID, WorkoutItemID: itemID, RoundNumber: 2,
	})
	entries := decodeAPI[[]ExerciseEntryDTO](t, rec, http.StatusCreated)
	if len(entries) != 1 {
		t.Fatalf("len = %d", len(entries))
	}
	if entries[0].WorkoutID != workout.ID || entries[0].WorkoutItemID != itemID || entries[0].RoundNumber != 2 {
		t.Fatalf("linkage = %+v", entries[0])
	}

	// The detail view surfaces the entry under logged (not ad-hoc).
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+workout.ID, token, nil)
	fetched := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusOK)
	if len(fetched.LoggedEntries) != 1 || len(fetched.AdHocEntries) != 0 {
		t.Fatalf("logged=%d adHoc=%d", len(fetched.LoggedEntries), len(fetched.AdHocEntries))
	}

	// Unknown workout is a 400.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets:       []CreateSetInput{{Reps: 5, Weight: 100}},
		WorkoutID:  "missing",
	})
	decodeAPIError(t, rec, http.StatusBadRequest)

	// Item from a different workout is a 400.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{Name: "other"})
	other := decodeAPI[WorkoutDetailDTO](t, rec, http.StatusCreated)
	rec = apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets:       []CreateSetInput{{Reps: 5, Weight: 100}},
		WorkoutID:  other.ID, WorkoutItemID: itemID,
	})
	decodeAPIError(t, rec, http.StatusBadRequest)

	// Item without a workout is a 400.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID:    "ex-1",
		Sets:          []CreateSetInput{{Reps: 5, Weight: 100}},
		WorkoutItemID: itemID,
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
}

func TestAPIWorkouts_Unauthorized(t *testing.T) {
	_, _, _, _, e := setupWorkoutHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/workouts", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}
