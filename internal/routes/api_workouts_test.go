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

// mockWorkoutRepository is an in-memory models.WorkoutRepo for the
// workouts API tests. Scoping mirrors the real repository.
type mockWorkoutRepository struct {
	mu       sync.Mutex
	workouts map[string]*models.Workout
	seq      int
}

func newMockWorkoutRepository() *mockWorkoutRepository {
	return &mockWorkoutRepository{workouts: map[string]*models.Workout{}}
}

func (m *mockWorkoutRepository) Create(w *models.Workout) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	w.ID = fmt.Sprintf("workout-%d", m.seq)
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	for i := range cp.Blocks {
		cp.Blocks[i].ID = fmt.Sprintf("%s-wb-%d", cp.ID, i)
		cp.Blocks[i].WorkoutID = cp.ID
		cp.Blocks[i].Position = i
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
		done := 0
		for _, b := range w.Blocks {
			if b.Status == models.WorkoutBlockDone {
				done++
			}
		}
		cp := *w
		out = append(out, models.WorkoutSummary{Workout: cp, BlockCount: len(w.Blocks), DoneCount: done})
	}
	return out, nil
}

func (m *mockWorkoutRepository) ListRange(userID, from, to string) ([]models.WorkoutSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.WorkoutSummary{}
	for _, w := range m.workouts {
		if w.UserID != userID {
			continue
		}
		if w.ScheduledDate < from || w.ScheduledDate > to {
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

func (m *mockWorkoutRepository) SetBlockStatus(workoutID, workoutBlockID string, status models.WorkoutBlockStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workouts[workoutID]
	if !ok {
		return nil
	}
	for i := range w.Blocks {
		if w.Blocks[i].ID == workoutBlockID {
			w.Blocks[i].Status = status
		}
	}
	return nil
}

func (m *mockWorkoutRepository) GetWorkoutBlock(workoutID, workoutBlockID string) (*models.WorkoutBlock, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workouts[workoutID]
	if !ok {
		return nil, nil
	}
	for _, b := range w.Blocks {
		if b.ID == workoutBlockID {
			cp := b
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockWorkoutRepository) CountBlockUsage(blockID, userID string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var n int64
	for _, w := range m.workouts {
		if w.UserID != userID {
			continue
		}
		for _, b := range w.Blocks {
			if b.BlockID == blockID {
				n++
				break
			}
		}
	}
	return n, nil
}

func (m *mockWorkoutRepository) Delete(id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workouts[id]; ok && w.UserID == userID {
		delete(m.workouts, id)
	}
	return nil
}

func (m *mockWorkoutRepository) CreateBatch(ws []*models.Workout) error {
	for _, w := range ws {
		if err := m.Create(w); err != nil {
			return err
		}
	}
	return nil
}

func (m *mockWorkoutRepository) MarkInProgressIfPlanned(workoutID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workouts[workoutID]; ok && w.UserID == userID && w.Status == models.WorkoutStatusPlanned {
		w.Status = models.WorkoutStatusInProgress
	}
	return nil
}

func (m *mockWorkoutRepository) SetWorkoutStatus(workoutID, userID string, status models.WorkoutStatus) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if w, ok := m.workouts[workoutID]; ok && w.UserID == userID {
		w.Status = status
	}
	return nil
}

func (m *mockWorkoutRepository) MarkCompletedIfBlocksDone(workoutID, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	w, ok := m.workouts[workoutID]
	if !ok || w.UserID != userID {
		return nil
	}
	for _, b := range w.Blocks {
		if b.Status == models.WorkoutBlockPending {
			return nil
		}
	}
	if len(w.Blocks) > 0 {
		w.Status = models.WorkoutStatusCompleted
	}
	return nil
}

// mockWorkoutBlockLookup resolves block IDs for workout validation.
// blk-1/blk-2 belong to whoever asks (tests set UserID-agnostic
// rows); anything else is unknown. blk-1 carries one planned item so
// ?include=items tests can assert the embed path.
type mockWorkoutBlockLookup struct{}

func (mockWorkoutBlockLookup) GetByID(id string, userID string) (*models.Block, error) {
	switch id {
	case "blk-1":
		return &models.Block{ID: "blk-1", UserID: userID, Name: "Push", Type: models.BlockTypeStandard, Items: []models.BlockItem{
			{ID: "bi-1", BlockID: "blk-1", ExerciseID: "ex-1", ExerciseName: "Squat", ExerciseType: models.ExerciseTypeStrength, Position: 0, TargetText: "3x5"},
		}}, nil
	case "blk-2":
		return &models.Block{ID: "blk-2", UserID: userID, Name: "Pull", Type: models.BlockTypeCircuit}, nil
	default:
		return nil, nil
	}
}

// setupWorkoutsHandler wires workout + block routes onto the shared
// test handler with fresh in-memory repos. The exercise entry
// controller is pointed at the same mock workout repo so linked-set
// tests exercise the real validation + in_progress flip.
func setupWorkoutsHandler(t *testing.T) (*Handler, *mockWorkoutRepository, *mockUserRepository, *mockBlockRepository, *echo.Echo) {
	t.Helper()
	h, mockExercises, mockUser, e := setupHandler(t)
	workoutRepo := newMockWorkoutRepository()
	h.SetWorkoutsController(controllers.NewWorkoutsController(workoutRepo, mockWorkoutBlockLookup{}))
	h.SetExerciseWorkoutsResolver(workoutRepo)
	blockRepo := newMockBlockRepository()
	h.SetBlocksController(controllers.NewBlocksController(blockRepo, mockExercises))
	return h, workoutRepo, mockUser, blockRepo, e
}

type workoutsResponse struct {
	Workouts []WorkoutSummaryDTO `json:"workouts"`
}

func validCreateWorkoutRequest() CreateWorkoutRequest {
	return CreateWorkoutRequest{
		Name:          "Monday Strength",
		Description:   "Heavy day",
		ScheduledDate: "2026-09-14",
		Blocks:        []CreateWorkoutBlockRequest{{BlockID: "blk-1"}, {BlockID: "blk-2"}},
	}
}

func TestAPICreateWorkout_Success(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "w@example.com", "W")

	w := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, validCreateWorkoutRequest()), http.StatusCreated)
	if w.Name != "Monday Strength" {
		t.Errorf("name = %q", w.Name)
	}
	if w.Status != "planned" {
		t.Errorf("status = %q, want planned", w.Status)
	}
	if len(w.Blocks) != 2 || w.Blocks[0].BlockName != "Push" {
		t.Errorf("blocks not resolved: %+v", w.Blocks)
	}
	if w.Blocks[0].Position != 0 || w.Blocks[1].Position != 1 {
		t.Errorf("positions wrong: %+v", w.Blocks)
	}
}

func TestAPICreateWorkout_Validation(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "wv@example.com", "WV")

	cases := []struct {
		name string
		mut  func(*CreateWorkoutRequest)
	}{
		{"empty name", func(in *CreateWorkoutRequest) { in.Name = "" }},
		{"bad date", func(in *CreateWorkoutRequest) { in.ScheduledDate = "yesterday" }},
		{"no blocks", func(in *CreateWorkoutRequest) { in.Blocks = nil }},
		{"unknown block", func(in *CreateWorkoutRequest) { in.Blocks = []CreateWorkoutBlockRequest{{BlockID: "nope"}} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validCreateWorkoutRequest()
			tc.mut(&in)
			rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, in)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestAPIWorkout_CRUD(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "wc@example.com", "WC")

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, validCreateWorkoutRequest()), http.StatusCreated)

	got := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+created.ID, token, nil), http.StatusOK)
	if got.ID != created.ID {
		t.Errorf("id = %q, want %q", got.ID, created.ID)
	}

	upd := validCreateWorkoutRequest()
	upd.Name = "Renamed"
	upd.Status = "in_progress"
	upd.Blocks = []CreateWorkoutBlockRequest{{BlockID: "blk-2"}}
	// UpdateWorkoutRequest has the same shape — reuse via JSON round-trip.
	var updBody UpdateWorkoutRequest
	updBody.Name = upd.Name
	updBody.Description = upd.Description
	updBody.ScheduledDate = upd.ScheduledDate
	updBody.Status = upd.Status
	updBody.Blocks = upd.Blocks
	updated := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/workouts/"+created.ID, token, updBody), http.StatusOK)
	if updated.Name != "Renamed" || updated.Status != "in_progress" || len(updated.Blocks) != 1 {
		t.Errorf("unexpected update result: %+v", updated)
	}

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/workouts/"+created.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, body = %s", rec.Code, rec.Body.String())
	}
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+created.ID, token, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("get-after-delete status = %d, want 404", rec.Code)
	}
}

func TestAPIWorkout_BlockStatusAndDuplicate(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "ws@example.com", "WS")

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, validCreateWorkoutRequest()), http.StatusCreated)

	updated := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPatch, "/api/v1/workouts/"+created.ID+"/blocks/"+created.Blocks[0].ID, token,
		UpdateWorkoutBlockStatusRequest{Status: "done"}), http.StatusOK)
	if updated.Blocks[0].Status != "done" {
		t.Errorf("block status = %q, want done", updated.Blocks[0].Status)
	}

	rec := apiDo(t, e, http.MethodPatch, "/api/v1/workouts/"+created.ID+"/blocks/"+created.Blocks[0].ID, token,
		UpdateWorkoutBlockStatusRequest{Status: "bogus"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad status code = %d, want 400", rec.Code)
	}

	dup := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate", token,
		DuplicateWorkoutRequest{ScheduledDate: "2026-09-21"}), http.StatusCreated)
	if dup.ID == created.ID || dup.ScheduledDate != "2026-09-21" || dup.Status != "planned" {
		t.Errorf("unexpected duplicate: %+v", dup)
	}
	if dup.Name != "Monday Strength" {
		t.Errorf("duplicate name = %q, want %q", dup.Name, "Monday Strength")
	}
	for _, b := range dup.Blocks {
		if b.Status != "pending" {
			t.Errorf("duplicated block status = %q, want pending", b.Status)
		}
	}
}

func TestAPIListWorkouts_Range(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "wr@example.com", "WR")

	mk := func(name, date string) {
		in := validCreateWorkoutRequest()
		in.Name = name
		in.ScheduledDate = date
		decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, in), http.StatusCreated)
	}
	mk("W1", "2026-09-14")
	mk("W2", "2026-09-21")

	all := decodeAPI[workoutsResponse](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts", token, nil), http.StatusOK)
	if len(all.Workouts) != 2 {
		t.Fatalf("all len = %d, want 2", len(all.Workouts))
	}
	ranged := decodeAPI[workoutsResponse](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts?from=2026-09-14&to=2026-09-14", token, nil), http.StatusOK)
	if len(ranged.Workouts) != 1 || ranged.Workouts[0].Name != "W1" {
		t.Errorf("range result = %+v, want [W1]", ranged.Workouts)
	}
	rec := apiDo(t, e, http.MethodGet, "/api/v1/workouts?from=2026-09-21&to=2026-09-14", token, nil)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("reversed range status = %d, want 400", rec.Code)
	}
}

func TestAPIDeleteBlock_BlockedByWorkout(t *testing.T) {
	h, workoutRepo, mockUser, blockRepo, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "wb@example.com", "WB")
	userID := "user-wb@example.com"

	// Create a real block through the API (needs a real exercise:
	// the shared mock seeds ex-1/ex-2).
	created := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
		Name: "Push", Type: "standard",
		Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}},
	}), http.StatusCreated)
	_ = blockRepo

	// Reference it from a workout directly in the workout repo mock.
	w := &models.Workout{
		UserID: userID, Name: "W", ScheduledDate: "2026-09-14",
		Status: models.WorkoutStatusPlanned,
		Blocks: []models.WorkoutBlock{{BlockID: created.ID, Status: models.WorkoutBlockPending}},
	}
	if err := workoutRepo.Create(w); err != nil {
		t.Fatal(err)
	}

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/blocks/"+created.ID, token, nil)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body.String())
	}
	errBody := decodeAPIError(t, rec, http.StatusConflict)
	if errBody.Error == "" {
		t.Error("expected a human-readable 409 message")
	}

	// Unreferenced blocks still delete cleanly.
	other := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
		Name: "Pull", Type: "standard",
		Items: []CreateBlockItemRequest{{ExerciseID: "ex-2"}},
	}), http.StatusCreated)
	rec = apiDo(t, e, http.MethodDelete, "/api/v1/blocks/"+other.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Errorf("unreferenced delete status = %d, want 204, body = %s", rec.Code, rec.Body.String())
	}
}

func TestAPIWorkout_NotFound(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "wn@example.com", "WN")

	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/api/v1/workouts/missing"},
		{http.MethodDelete, "/api/v1/workouts/missing"},
		{http.MethodPost, "/api/v1/workouts/missing/duplicate"},
	} {
		var body any
		if tc.method == http.MethodPost {
			body = DuplicateWorkoutRequest{ScheduledDate: "2026-09-21"}
		}
		rec := apiDo(t, e, tc.method, tc.path, token, body)
		if rec.Code != http.StatusNotFound {
			t.Errorf("%s %s status = %d, want 404", tc.method, tc.path, rec.Code)
		}
	}
}

func TestAPIDuplicateWorkoutBatch(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "wbatch@example.com", "WB")

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, validCreateWorkoutRequest()), http.StatusCreated)

	resp := decodeAPI[workoutsResponse](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate-batch", token,
		DuplicateWorkoutBatchRequest{Dates: []string{"2026-09-21", "2026-09-28"}}), http.StatusCreated)
	if len(resp.Workouts) != 2 {
		t.Fatalf("len = %d, want 2", len(resp.Workouts))
	}
	for _, w := range resp.Workouts {
		if w.Name != created.Name {
			t.Errorf("name = %q, want %q (repeats keep the source name)", w.Name, created.Name)
		}
		if w.Status != "planned" {
			t.Errorf("status = %q, want planned", w.Status)
		}
	}

	// Over the cap: 400 without creating anything.
	many := make([]string, 0, 51)
	for i := 1; i <= 51; i++ {
		many = append(many, fmt.Sprintf("2027-01-%02d", (i%28)+1))
	}
	rec := apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate-batch", token,
		DuplicateWorkoutBatchRequest{Dates: many})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("51 dates status = %d, want 400", rec.Code)
	}

	// Bad date: 400, and the valid sibling is not created either.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/"+created.ID+"/duplicate-batch", token,
		DuplicateWorkoutBatchRequest{Dates: []string{"2026-10-05", "not-a-date"}})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad date status = %d, want 400", rec.Code)
	}
	all := decodeAPI[workoutsResponse](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts", token, nil), http.StatusOK)
	if len(all.Workouts) != 3 {
		t.Errorf("total workouts = %d, want 3 (1 original + 2 batch, no partial write)", len(all.Workouts))
	}

	// Missing source: 404.
	rec = apiDo(t, e, http.MethodPost, "/api/v1/workouts/missing/duplicate-batch", token,
		DuplicateWorkoutBatchRequest{Dates: []string{"2026-10-05"}})
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing source status = %d, want 404", rec.Code)
	}
}

// TestAPIWorkout_IncludeItems is the player single-call fetch: detail
// with ?include=items embeds every block's planned exercises. Without
// the param the shape is unchanged (plain WorkoutDTO).
func TestAPIWorkout_IncludeItems(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "pi@example.com", "PI")

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, validCreateWorkoutRequest()), http.StatusCreated)

	plain := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+created.ID, token, nil), http.StatusOK)
	if len(plain.Blocks) != 2 {
		t.Fatalf("plain blocks = %d, want 2", len(plain.Blocks))
	}

	withItems := decodeAPI[WorkoutWithItemsDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+created.ID+"?include=items", token, nil), http.StatusOK)
	if withItems.ID != created.ID {
		t.Errorf("id = %q, want %q", withItems.ID, created.ID)
	}
	if len(withItems.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2", len(withItems.Blocks))
	}
	// blk-1 carries one planned item in the mock lookup; blk-2 none.
	if len(withItems.Blocks[0].Items) != 1 || withItems.Blocks[0].Items[0].ExerciseName != "Squat" {
		t.Errorf("block 0 items = %+v, want 1 Squat row", withItems.Blocks[0].Items)
	}
	if withItems.Blocks[1].Items == nil || len(withItems.Blocks[1].Items) != 0 {
		t.Errorf("block 1 items = %+v, want empty non-nil slice", withItems.Blocks[1].Items)
	}

	rec := apiDo(t, e, http.MethodGet, "/api/v1/workouts/missing?include=items", token, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d, want 404", rec.Code)
	}
}

// TestAPIWorkout_PlayerLogging covers the player loop over HTTP: log a
// linked set (echoes the link IDs, flips planned to in_progress),
// resume via the per-workout entries endpoint, then resolve every
// block and watch the workout auto-complete.
func TestAPIWorkout_PlayerLogging(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "pl@example.com", "PL")

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, CreateWorkoutRequest{
		Name: "Solo", Description: "", ScheduledDate: "2026-09-14",
		Blocks: []CreateWorkoutBlockRequest{{BlockID: "blk-1"}},
	}), http.StatusCreated)

	wid := created.ID
	wbid := created.Blocks[0].ID
	entries := decodeAPI[[]ExerciseEntryDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets: []CreateSetInput{
			{Reps: 5, Weight: 100, WorkoutID: &wid, BlockID: strPtr("blk-1"), WorkoutBlockID: &wbid},
		},
	}), http.StatusCreated)
	if entries[0].WorkoutID == nil || *entries[0].WorkoutID != wid {
		t.Errorf("workout_id = %+v, want %q", entries[0].WorkoutID, wid)
	}
	if entries[0].BlockID == nil || *entries[0].BlockID != "blk-1" {
		t.Errorf("block_id = %+v, want blk-1", entries[0].BlockID)
	}

	got := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+wid, token, nil), http.StatusOK)
	if got.Status != "in_progress" {
		t.Errorf("status = %q, want in_progress after first linked set", got.Status)
	}

	resume := decodeAPI[[]ExerciseEntryDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+wid+"/exercise-entries", token, nil), http.StatusOK)
	if len(resume) != 1 || resume[0].ID != entries[0].ID {
		t.Errorf("resume = %+v, want the 1 linked set", resume)
	}

	// Unknown link: 400, and the resume list is unchanged.
	badID := "nope"
	rec := apiDo(t, e, http.MethodPost, "/api/v1/exercise-entries", token, CreateExerciseEntriesRequest{
		ExerciseID: "ex-1",
		Sets:       []CreateSetInput{{Reps: 5, Weight: 100, WorkoutID: &badID}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("unknown workout status = %d, want 400", rec.Code)
	}

	// Last block to done: workout auto-completes.
	done := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPatch, "/api/v1/workouts/"+wid+"/blocks/"+wbid, token,
		UpdateWorkoutBlockStatusRequest{Status: "done"}), http.StatusOK)
	if done.Status != "completed" {
		t.Errorf("status = %q, want completed after last block done", done.Status)
	}

	// Cross-user resume: 404, no leak.
	otherToken, _ := loginUser(t, h, mockUser, "other@example.com", "Other")
	rec = apiDo(t, e, http.MethodGet, "/api/v1/workouts/"+wid+"/exercise-entries", otherToken, nil)
	if rec.Code != http.StatusNotFound {
		t.Errorf("cross-user status = %d, want 404", rec.Code)
	}
}

// TestAPIWorkout_StatusPatch is the HTTP-level regression test for the
// "Completed but 0/2 blocks" bug: PATCH status flips the workout while
// done counts survive (1/2 here, not 0/2). Also covers bad status
// (400) and missing workout (404).
func TestAPIWorkout_StatusPatch(t *testing.T) {
	h, _, mockUser, _, e := setupWorkoutsHandler(t)
	token, _ := loginUser(t, h, mockUser, "sp@example.com", "SP")

	created := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/workouts", token, validCreateWorkoutRequest()), http.StatusCreated)

	updated := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPatch, "/api/v1/workouts/"+created.ID+"/blocks/"+created.Blocks[0].ID, token,
		UpdateWorkoutBlockStatusRequest{Status: "done"}), http.StatusOK)
	if updated.Blocks[0].Status != "done" {
		t.Fatalf("block status = %q, want done", updated.Blocks[0].Status)
	}

	completed := decodeAPI[WorkoutDTO](t, apiDo(t, e, http.MethodPatch, "/api/v1/workouts/"+created.ID+"/status", token,
		UpdateWorkoutStatusRequest{Status: "completed"}), http.StatusOK)
	if completed.Status != "completed" {
		t.Errorf("status = %q, want completed", completed.Status)
	}
	if len(completed.Blocks) != 2 || completed.Blocks[0].Status != "done" || completed.Blocks[1].Status != "pending" {
		t.Errorf("blocks = %+v, want done/pending preserved", completed.Blocks)
	}

	summaries := decodeAPI[workoutsResponse](t, apiDo(t, e, http.MethodGet, "/api/v1/workouts", token, nil), http.StatusOK)
	if len(summaries.Workouts) != 1 || summaries.Workouts[0].DoneCount != 1 {
		t.Errorf("summaries = %+v, want 1 workout with done_count 1", summaries.Workouts)
	}

	rec := apiDo(t, e, http.MethodPatch, "/api/v1/workouts/"+created.ID+"/status", token,
		UpdateWorkoutStatusRequest{Status: "someday"})
	if rec.Code != http.StatusBadRequest {
		t.Errorf("bad status code = %d, want 400", rec.Code)
	}
	rec = apiDo(t, e, http.MethodPatch, "/api/v1/workouts/missing/status", token,
		UpdateWorkoutStatusRequest{Status: "completed"})
	if rec.Code != http.StatusNotFound {
		t.Errorf("missing status = %d, want 404", rec.Code)
	}
}

func strPtr(s string) *string { return &s }
