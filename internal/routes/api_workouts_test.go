package routes

import (
	"fmt"
	"net/http"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"

	"hylete/internal/controllers"
	"hylete/internal/models"
)

// mockWorkoutRepository is an in-memory models.WorkoutRepo for
// the workouts API tests. Scoping mirrors the real repository.
type mockWorkoutRepository struct {
	mu          sync.Mutex
	workouts    map[string]*models.Workout
	assignments map[string]*models.WorkoutAssignment
	seq         int
	aseq        int
}

func newMockWorkoutRepository() *mockWorkoutRepository {
	return &mockWorkoutRepository{
		workouts:    map[string]*models.Workout{},
		assignments: map[string]*models.WorkoutAssignment{},
	}
}

func (m *mockWorkoutRepository) Create(w *models.Workout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	w.ID = fmt.Sprintf("workout-%d", m.seq)
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	for i := range cp.Blocks {
		cp.Blocks[i].ID = fmt.Sprintf("%s-link-%d", cp.ID, i)
		cp.Blocks[i].WorkoutID = cp.ID
	}
	m.workouts[cp.ID] = &cp
	*w = cp
	return nil
}

func (m *mockWorkoutRepository) GetByID(id, userID string) (*models.Workout, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workouts[id]
	if !ok || w.UserID != userID {
		return nil, nil
	}
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	return &cp, nil
}

func (m *mockWorkoutRepository) List(userID string) ([]models.WorkoutSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.WorkoutSummary{}
	for _, w := range m.workouts {
		if w.UserID != userID {
			continue
		}
		cp := *w
		out = append(out, models.WorkoutSummary{Workout: cp, BlockCount: len(w.Blocks)})
	}
	return out, nil
}

func (m *mockWorkoutRepository) Update(w *models.Workout, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.workouts[w.ID]
	if !ok || existing.UserID != userID {
		return nil
	}
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	m.workouts[w.ID] = &cp
	return nil
}

func (m *mockWorkoutRepository) Delete(id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workouts[id]; ok && w.UserID == userID {
		delete(m.workouts, id)
		for aid, a := range m.assignments {
			if a.WorkoutID == id {
				delete(m.assignments, aid)
			}
		}
	}
	return nil
}

func (m *mockWorkoutRepository) Duplicate(id, userID string, copySchedule bool) (*models.Workout, error) {
	m.mu.Lock()
	src, ok := m.workouts[id]
	if !ok || src.UserID != userID {
		m.mu.Unlock()
		return nil, nil
	}
	cp := *src
	cp.Blocks = append([]models.WorkoutBlock{}, src.Blocks...)
	var dates []string
	if copySchedule {
		for _, a := range m.assignments {
			if a.WorkoutID == id {
				dates = append(dates, a.ScheduledDate)
			}
		}
	}
	m.mu.Unlock()

	dup := &models.Workout{
		UserID:      userID,
		Title:       cp.Title + " copy",
		Description: cp.Description,
		Blocks:      cp.Blocks,
	}
	if err := m.Create(dup); err != nil {
		return nil, err
	}
	if len(dates) > 0 {
		if _, err := m.AddAssignments(dup.ID, userID, dates); err != nil {
			return nil, err
		}
	}
	return dup, nil
}

func (m *mockWorkoutRepository) ListAssignmentsForWorkout(workoutID string) ([]models.WorkoutAssignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.WorkoutAssignment{}
	for _, a := range m.assignments {
		if a.WorkoutID == workoutID {
			out = append(out, *a)
		}
	}
	return out, nil
}

func (m *mockWorkoutRepository) ListAssignmentsByDateRange(userID, start, end string) ([]models.WorkoutAssignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.WorkoutAssignment{}
	for _, w := range m.workouts {
		if w.UserID != userID {
			continue
		}
		for _, a := range m.assignments {
			if a.WorkoutID == w.ID && a.ScheduledDate >= start && a.ScheduledDate <= end {
				cp := *a
				cp.WorkoutTitle = w.Title
				out = append(out, cp)
			}
		}
	}
	return out, nil
}

func (m *mockWorkoutRepository) AddAssignments(workoutID, userID string, dates []string) ([]models.WorkoutAssignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []models.WorkoutAssignment
	for _, d := range dates {
		dup := false
		for _, a := range m.assignments {
			if a.WorkoutID == workoutID && a.ScheduledDate == d {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		m.aseq++
		a := &models.WorkoutAssignment{
			ID:            fmt.Sprintf("assign-%d", m.aseq),
			WorkoutID:     workoutID,
			ScheduledDate: d,
		}
		m.assignments[a.ID] = a
		out = append(out, *a)
	}
	return out, nil
}

func (m *mockWorkoutRepository) GetAssignment(id, userID string) (*models.WorkoutAssignment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a, ok := m.assignments[id]
	if !ok {
		return nil, nil
	}
	w, ok := m.workouts[a.WorkoutID]
	if !ok || w.UserID != userID {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (m *mockWorkoutRepository) DeleteAssignment(id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a, ok := m.assignments[id]; ok {
		if w, ok := m.workouts[a.WorkoutID]; ok && w.UserID == userID {
			delete(m.assignments, id)
		}
	}
	return nil
}

// setupWorkoutsHandler wires workout routes onto the shared test
// handler. Blocks are created through the real blocks API (backed
// by the in-memory block repo) so workout tests use genuine block
// IDs; the same block repo backs block resolution.
func setupWorkoutsHandler(t *testing.T) (*Handler, *mockWorkoutRepository, *mockUserRepository, *echo.Echo, string, []string) {
	t.Helper()
	h, blockRepo, mockUser, e := setupBlocksHandler(t)
	workoutRepo := newMockWorkoutRepository()
	h.SetWorkoutsController(controllers.NewWorkoutsController(workoutRepo, blockRepo))

	token, _ := loginUser(t, h, mockUser, "wo@example.com", "WO")
	blockIDs := make([]string, 0, 2)
	for _, name := range []string{"Push", "Pull"} {
		b := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
			Name:  name,
			Type:  "standard",
			Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}},
		}), http.StatusCreated)
		blockIDs = append(blockIDs, b.ID)
	}
	return h, workoutRepo, mockUser, e, token, blockIDs
}

type workoutsResponse struct {
	Workouts []WorkoutSummaryDTO `json:"workouts"`
}

type assignmentsResponse struct {
	Assignments []WorkoutAssignmentDTO `json:"assignments"`
}

func TestAPIListWorkouts_Empty(t *testing.T) {
	_, _, _, e, token, _ := setupWorkoutsHandler(t)

	rec := apiDo(t, e, http.MethodGet, "/api/v1/workouts", token, nil)
	resp := decodeAPI[workoutsResponse](t, rec, http.StatusOK)
	if len(resp.Workouts) != 0 {
		t.Fatalf("workouts len = %d, want 0", len(resp.Workouts))
	}
}

func TestAPICreateWorkout_Success(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:       "Push Day",
		Description: "Chest + shoulders",
		BlockIDs:    blockIDs,
	})
	w := decodeAPI[WorkoutDTO](t, rec, http.StatusCreated)
	if w.ID == "" {
		t.Fatal("expected generated id")
	}
	if w.Title != "Push Day" || len(w.Blocks) != 2 {
		t.Fatalf("unexpected workout: %+v", w)
	}
	if w.Blocks[0].Position != 0 || w.Blocks[0].BlockName == "" {
		t.Fatalf("unexpected first block link: %+v", w.Blocks[0])
	}
	if w.Assignments == nil || len(w.Assignments) != 0 {
		t.Fatalf("new workout should carry an empty assignments array: %+v", w.Assignments)
	}
}

func TestAPICreateWorkout_Validation(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	cases := []struct {
		name string
		body CreateWorkoutRequest
	}{
		{"empty title", CreateWorkoutRequest{BlockIDs: blockIDs}},
		{"no blocks", CreateWorkoutRequest{Title: "x"}},
		{"unknown block", CreateWorkoutRequest{Title: "x", BlockIDs: []string{"nope"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, tc.body)
			decodeAPIError(t, rec, http.StatusBadRequest)
		})
	}
}

func TestAPIGetWorkout_SuccessAndNotFound(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:    "Legs",
		BlockIDs: blockIDs[:1],
	}), http.StatusCreated)

	got := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+created.ID, token, nil), http.StatusOK)
	if got.ID != created.ID || got.Title != "Legs" {
		t.Fatalf("unexpected workout: %+v", got)
	}

	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/missing", token, nil), http.StatusNotFound)
	// The static /schedule route must not be captured as an ID.
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/schedule", token, nil), http.StatusBadRequest)
}

func TestAPIUpdateWorkout_Success(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:    "Full Body",
		BlockIDs: blockIDs[:1],
	}), http.StatusCreated)

	updated := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/workouts/"+created.ID, token, UpdateWorkoutRequest{
		Title:       "Full Body v2",
		Description: "heavier",
		BlockIDs:    blockIDs,
	}), http.StatusOK)
	if updated.Title != "Full Body v2" || len(updated.Blocks) != 2 {
		t.Fatalf("unexpected updated workout: %+v", updated)
	}

	decodeAPIError(t, apiDo(t, e, http.MethodPut, "/api/v1/workouts/missing", token, UpdateWorkoutRequest{
		Title:    "x",
		BlockIDs: blockIDs[:1],
	}), http.StatusNotFound)
}

func TestAPIDeleteWorkout_SuccessAndNotFound(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:    "Temp",
		BlockIDs: blockIDs[:1],
	}), http.StatusCreated)

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/workouts/"+created.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+created.ID, token, nil), http.StatusNotFound)
	decodeAPIError(t, apiDo(t, e, http.MethodDelete, "/api/v1/workouts/missing", token, nil), http.StatusNotFound)
}

func TestAPIDuplicateWorkout(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:    "Base",
		BlockIDs: blockIDs,
	}), http.StatusCreated)
	decodeAPI[assignmentsResponse](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/assignments", token, AddWorkoutAssignmentsRequest{
		Dates: []string{"2026-09-14"},
	}), http.StatusCreated)

	// Default: composition copied, schedule not.
	dup := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate", token, nil), http.StatusCreated)
	if dup.Title != "Base copy" || len(dup.Blocks) != 2 {
		t.Fatalf("unexpected duplicate: %+v", dup)
	}
	if len(dup.Assignments) != 0 {
		t.Fatalf("duplicate should start unscheduled: %+v", dup.Assignments)
	}

	withSched := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate", token, DuplicateWorkoutRequest{
		CopySchedule: true,
	}), http.StatusCreated)
	if len(withSched.Assignments) != 1 || withSched.Assignments[0].ScheduledDate != "2026-09-14" {
		t.Fatalf("unexpected copied schedule: %+v", withSched.Assignments)
	}

	decodeAPIError(t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/missing/duplicate", token, nil), http.StatusNotFound)
}

func TestAPIWorkoutAssignments(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:    "Sched",
		BlockIDs: blockIDs[:1],
	}), http.StatusCreated)

	added := decodeAPI[assignmentsResponse](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/assignments", token, AddWorkoutAssignmentsRequest{
		Dates: []string{"2026-09-14", "2026-09-21"},
	}), http.StatusCreated)
	if len(added.Assignments) != 2 {
		t.Fatalf("len = %d, want 2", len(added.Assignments))
	}
	if added.Assignments[0].WorkoutTitle != "Sched" {
		t.Fatalf("expected resolved workout title: %+v", added.Assignments[0])
	}

	// Repeat submit of the same day is skipped, not an error.
	resend := decodeAPI[assignmentsResponse](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/assignments", token, AddWorkoutAssignmentsRequest{
		Dates: []string{"2026-09-14"},
	}), http.StatusCreated)
	if len(resend.Assignments) != 0 {
		t.Fatalf("len = %d, want 0 (idempotent skip)", len(resend.Assignments))
	}

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/workout-assignments/"+added.Assignments[0].ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	decodeAPIError(t, apiDo(t, e, http.MethodDelete, "/api/v1/workout-assignments/"+added.Assignments[0].ID, token, nil), http.StatusNotFound)

	// Bad date payloads are 400s.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/assignments", token, AddWorkoutAssignmentsRequest{
		Dates: []string{"14-09-2026"},
	})
	decodeAPIError(t, rec, http.StatusBadRequest)
	decodeAPIError(t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/missing/assignments", token, AddWorkoutAssignmentsRequest{
		Dates: []string{"2026-09-14"},
	}), http.StatusNotFound)
}

func TestAPIWorkoutScheduleRange(t *testing.T) {
	_, _, _, e, token, blockIDs := setupWorkoutsHandler(t)

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Title:    "Ranged",
		BlockIDs: blockIDs[:1],
	}), http.StatusCreated)
	decodeAPI[assignmentsResponse](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/assignments", token, AddWorkoutAssignmentsRequest{
		Dates: []string{"2026-09-14", "2026-09-21"},
	}), http.StatusCreated)

	got := decodeAPI[assignmentsResponse](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/schedule?from=2026-09-14&to=2026-09-21", token, nil), http.StatusOK)
	if len(got.Assignments) != 2 {
		t.Fatalf("len = %d, want 2", len(got.Assignments))
	}

	narrow := decodeAPI[assignmentsResponse](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/schedule?from=2026-09-15&to=2026-09-20", token, nil), http.StatusOK)
	if len(narrow.Assignments) != 0 {
		t.Fatalf("len = %d, want 0", len(narrow.Assignments))
	}

	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/schedule?from=2026-09-14", token, nil), http.StatusBadRequest)
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/schedule?from=2026-09-21&to=2026-09-14", token, nil), http.StatusBadRequest)
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/schedule?from=2026-01-01&to=2026-12-31", token, nil), http.StatusBadRequest)
}

func TestAPIWorkouts_RequiresAuth(t *testing.T) {
	_, _, _, e, _, _ := setupWorkoutsHandler(t)
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/workouts"},
		{http.MethodPost, "/api/v1/workouts"},
		{http.MethodGet, "/api/v1/workouts/x"},
		{http.MethodPut, "/api/v1/workouts/x"},
		{http.MethodDelete, "/api/v1/workouts/x"},
		{http.MethodPost, "/api/v1/workouts/x/duplicate"},
		{http.MethodPost, "/api/v1/workouts/x/assignments"},
		{http.MethodDelete, "/api/v1/workout-assignments/x"},
		{http.MethodGet, "/api/v1/workouts/schedule?from=2026-09-14&to=2026-09-21"},
	} {
		rec := apiDo(t, e, tc.method, tc.path, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
