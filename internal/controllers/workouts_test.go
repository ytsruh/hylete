package controllers

import (
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"hylete/internal/models"
)

// fakeWorkoutRepo is an in-memory models.WorkoutRepo for controller
// tests. Scoping mirrors the real repository.
type fakeWorkoutRepo struct {
	mu       sync.Mutex
	workouts map[string]*models.Workout
	seq      int
}

func newFakeWorkoutRepo() *fakeWorkoutRepo {
	return &fakeWorkoutRepo{workouts: map[string]*models.Workout{}}
}

func (f *fakeWorkoutRepo) Create(w *models.Workout) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	w.ID = fmt.Sprintf("workout-test-%d", f.seq)
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	for i := range cp.Blocks {
		cp.Blocks[i].ID = fmt.Sprintf("%s-block-%d", cp.ID, i)
		cp.Blocks[i].WorkoutID = cp.ID
		cp.Blocks[i].Position = i
	}
	f.workouts[cp.ID] = &cp
	*w = cp
	return nil
}

func (f *fakeWorkoutRepo) GetByID(id, userID string) (*models.Workout, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[id]
	if !ok || w.UserID != userID {
		return nil, nil
	}
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	return &cp, nil
}

func (f *fakeWorkoutRepo) List(userID string) ([]models.WorkoutSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []models.WorkoutSummary{}
	for _, w := range f.workouts {
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

func (f *fakeWorkoutRepo) ListRange(userID, from, to string) ([]models.WorkoutSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []models.WorkoutSummary{}
	for _, w := range f.workouts {
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

func (f *fakeWorkoutRepo) Update(w *models.Workout, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.workouts[w.ID]
	if !ok || existing.UserID != userID {
		return nil
	}
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	f.workouts[w.ID] = &cp
	return nil
}

func (f *fakeWorkoutRepo) SetBlockStatus(workoutID, workoutBlockID string, status models.WorkoutBlockStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[workoutID]
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

func (f *fakeWorkoutRepo) GetWorkoutBlock(workoutID, workoutBlockID string) (*models.WorkoutBlock, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[workoutID]
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

func (f *fakeWorkoutRepo) CountBlockUsage(blockID, userID string) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var n int64
	for _, w := range f.workouts {
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

func (f *fakeWorkoutRepo) Delete(id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[id]; ok && w.UserID == userID {
		delete(f.workouts, id)
	}
	return nil
}

func (f *fakeWorkoutRepo) MarkInProgressIfPlanned(workoutID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[workoutID]; ok && w.UserID == userID && w.Status == models.WorkoutStatusPlanned {
		w.Status = models.WorkoutStatusInProgress
	}
	return nil
}

func (f *fakeWorkoutRepo) SetWorkoutStatus(workoutID, userID string, status models.WorkoutStatus) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[workoutID]; ok && w.UserID == userID {
		w.Status = status
	}
	return nil
}

func (f *fakeWorkoutRepo) SetHealthActivityType(workoutID, userID, activityType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[workoutID]; ok && w.UserID == userID {
		w.HealthActivityType = models.NormalizeHealthActivityType(activityType)
	}
	return nil
}

func (f *fakeWorkoutRepo) MarkCompletedIfBlocksDone(workoutID, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[workoutID]
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

func (f *fakeWorkoutRepo) CreateBatch(ws []*models.Workout) error {
	for _, w := range ws {
		if err := f.Create(w); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeWorkoutRepo) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.workouts)
}

// fakeWorkoutBlockLookup resolves blocks for workout validation.
type fakeWorkoutBlockLookup struct {
	blocks map[string]*models.Block
}

func (f *fakeWorkoutBlockLookup) GetByID(id string, userID string) (*models.Block, error) {
	b, ok := f.blocks[id]
	if !ok || b.UserID != userID {
		return nil, nil
	}
	return b, nil
}

func setupWorkoutsController() (*WorkoutsController, *fakeWorkoutRepo) {
	repo := newFakeWorkoutRepo()
	lookup := &fakeWorkoutBlockLookup{blocks: map[string]*models.Block{
		"blk-1": {ID: "blk-1", UserID: "u1", Name: "Push", Type: models.BlockTypeStandard, Items: []models.BlockItem{{ID: "i1"}}},
		"blk-2": {ID: "blk-2", UserID: "u1", Name: "Pull", Type: models.BlockTypeCircuit, Items: []models.BlockItem{{ID: "i1"}, {ID: "i2"}}},
	}}
	return NewWorkoutsController(repo, lookup), repo
}

func validWorkoutInput() CreateWorkoutInput {
	return CreateWorkoutInput{
		Name:          "Monday Strength",
		Description:   "Heavy day",
		ScheduledDate: "2026-09-14",
		Blocks:        []WorkoutBlockInput{{BlockID: "blk-1"}},
	}
}

func TestCreateWorkout_Success(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	w, err := ctrl.CreateWorkout("u1", validWorkoutInput())
	if err != nil {
		t.Fatalf("CreateWorkout failed: %v", err)
	}
	if w.ID == "" {
		t.Error("expected generated ID")
	}
	if w.Status != models.WorkoutStatusPlanned {
		t.Errorf("status = %q, want planned", w.Status)
	}
	if len(w.Blocks) != 1 || w.Blocks[0].BlockName != "Push" {
		t.Errorf("blocks not resolved: %+v", w.Blocks)
	}
	if w.Blocks[0].Status != models.WorkoutBlockPending {
		t.Errorf("block status = %q, want pending", w.Blocks[0].Status)
	}
}

func TestCreateWorkout_Validation(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	cases := []struct {
		name string
		mut  func(*CreateWorkoutInput)
		want error
	}{
		{"empty name", func(in *CreateWorkoutInput) { in.Name = "  " }, ErrWorkoutNameRequired},
		{"bad date", func(in *CreateWorkoutInput) { in.ScheduledDate = "14/09/2026" }, ErrWorkoutDateInvalid},
		{"missing date", func(in *CreateWorkoutInput) { in.ScheduledDate = "" }, ErrWorkoutDateRequired},
		{"no blocks", func(in *CreateWorkoutInput) { in.Blocks = nil }, ErrWorkoutBlocksRequired},
		{"unknown block", func(in *CreateWorkoutInput) { in.Blocks = []WorkoutBlockInput{{BlockID: "nope"}} }, ErrWorkoutBlockNotFound},
		{"blank block", func(in *CreateWorkoutInput) { in.Blocks = []WorkoutBlockInput{{BlockID: " "}} }, ErrWorkoutBlockRequired},
		{"bad status", func(in *CreateWorkoutInput) { in.Status = "someday" }, ErrWorkoutStatusInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := validWorkoutInput()
			tc.mut(&in)
			if _, err := ctrl.CreateWorkout("u1", in); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateWorkout_TooManyBlocks(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	in := validWorkoutInput()
	for i := 0; i < WorkoutBlocksMax; i++ {
		in.Blocks = append(in.Blocks, WorkoutBlockInput{BlockID: "blk-1"})
	}
	if _, err := ctrl.CreateWorkout("u1", in); !errors.Is(err, ErrWorkoutBlocksTooMany) {
		t.Errorf("err = %v, want ErrWorkoutBlocksTooMany", err)
	}
}

func TestGetWorkout_NotFound(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	if _, err := ctrl.GetWorkout("missing", "u1"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestGetWorkout_OtherUser(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	w, err := ctrl.CreateWorkout("u1", validWorkoutInput())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.GetWorkout(w.ID, "u2"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestSetWorkoutBlockStatus(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	w, err := ctrl.CreateWorkout("u1", validWorkoutInput())
	if err != nil {
		t.Fatal(err)
	}
	updated, err := ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[0].ID, "u1", models.WorkoutBlockDone)
	if err != nil {
		t.Fatalf("SetWorkoutBlockStatus failed: %v", err)
	}
	if updated.Blocks[0].Status != models.WorkoutBlockDone {
		t.Errorf("status = %q, want done", updated.Blocks[0].Status)
	}
	if _, err := ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[0].ID, "u1", "bogus"); !errors.Is(err, ErrWorkoutBlockStatusInvalid) {
		t.Errorf("err = %v, want ErrWorkoutBlockStatusInvalid", err)
	}
	if _, err := ctrl.SetWorkoutBlockStatus(w.ID, "missing", "u1", models.WorkoutBlockDone); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("err = %v, want ErrWorkoutNotFound", err)
	}
}

// TestSetWorkoutBlockStatus_AutoCompletes is the second half of the
// "both" status rule: flipping the last pending block to done/skipped
// auto-completes the workout. A flip that leaves a pending block must
// not complete it.
func TestSetWorkoutBlockStatus_AutoCompletes(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	in := validWorkoutInput()
	in.Blocks = []WorkoutBlockInput{{BlockID: "blk-1"}, {BlockID: "blk-2"}}
	w, err := ctrl.CreateWorkout("u1", in)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[0].ID, "u1", models.WorkoutBlockDone)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status == models.WorkoutStatusCompleted {
		t.Error("workout completed with a block still pending")
	}
	updated, err = ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[1].ID, "u1", models.WorkoutBlockSkipped)
	if err != nil {
		t.Fatal(err)
	}
	if updated.Status != models.WorkoutStatusCompleted {
		t.Errorf("status = %q, want completed after last block resolved", updated.Status)
	}
}

// TestGetWorkoutWithItems resolves every block's planned exercises in
// position order for the player single-call fetch. A block whose
// catalogue row is gone keeps its row with no items so the player can
// still show its name and offer Skip.
func TestGetWorkoutWithItems(t *testing.T) {
	repo := newFakeWorkoutRepo()
	lookup := &fakeWorkoutBlockLookup{blocks: map[string]*models.Block{
		"blk-1": {ID: "blk-1", UserID: "u1", Name: "Push", Type: models.BlockTypeStandard, Items: []models.BlockItem{
			{ID: "i1", BlockID: "blk-1", ExerciseID: "ex-1", ExerciseName: "Squat", Position: 0},
			{ID: "i2", BlockID: "blk-1", ExerciseID: "ex-2", ExerciseName: "Bench", Position: 1},
		}},
	}}
	ctrl := NewWorkoutsController(repo, lookup)
	w, err := ctrl.CreateWorkout("u1", validWorkoutInput())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ctrl.GetWorkoutWithItems(w.ID, "u1")
	if err != nil {
		t.Fatalf("GetWorkoutWithItems failed: %v", err)
	}
	if len(got.Blocks) != 1 || len(got.Blocks[0].Items) != 2 {
		t.Fatalf("blocks = %+v, want 1 block with 2 items", got.Blocks)
	}
	if got.Blocks[0].Items[0].ExerciseName != "Squat" {
		t.Errorf("first item = %q, want Squat", got.Blocks[0].Items[0].ExerciseName)
	}
	if _, err := ctrl.GetWorkoutWithItems(w.ID, "u2"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("wrong-user err = %v, want ErrWorkoutNotFound", err)
	}

	// Deleted catalogue block: row kept, items empty.
	delete(lookup.blocks, "blk-1")
	got, err = ctrl.GetWorkoutWithItems(w.ID, "u1")
	if err != nil {
		t.Fatalf("GetWorkoutWithItems after block delete failed: %v", err)
	}
	if len(got.Blocks) != 1 || len(got.Blocks[0].Items) != 0 {
		t.Errorf("blocks = %+v, want 1 row with 0 items", got.Blocks)
	}
}

// TestGetWorkoutWithItems_TimingConfig carries each block type's time
// config into the player fetch (circuit rounds/rest, AMRAP cap, EMOM
// interval/rounds). Before this, only Items crossed over from the
// catalogue block — EMOM, circuit and AMRAP blocks rendered with no
// time element at all. A deleted catalogue block keeps zeros.
func TestGetWorkoutWithItems_TimingConfig(t *testing.T) {
	repo := newFakeWorkoutRepo()
	lookup := &fakeWorkoutBlockLookup{blocks: map[string]*models.Block{
		"blk-c": {ID: "blk-c", UserID: "u1", Name: "Circ", Type: models.BlockTypeCircuit, Rounds: 4, RestSeconds: 90, Items: []models.BlockItem{{ID: "i1"}}},
		"blk-a": {ID: "blk-a", UserID: "u1", Name: "Am", Type: models.BlockTypeAmrap, TimeCapSeconds: 600, Items: []models.BlockItem{{ID: "i1"}}},
		"blk-e": {ID: "blk-e", UserID: "u1", Name: "Em", Type: models.BlockTypeEmom, Rounds: 12, IntervalSeconds: 60, Items: []models.BlockItem{{ID: "i1"}}},
	}}
	ctrl := NewWorkoutsController(repo, lookup)
	w, err := ctrl.CreateWorkout("u1", CreateWorkoutInput{
		Name:          "Mixed",
		ScheduledDate: "2026-09-14",
		Blocks:        []WorkoutBlockInput{{BlockID: "blk-c"}, {BlockID: "blk-a"}, {BlockID: "blk-e"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := ctrl.GetWorkoutWithItems(w.ID, "u1")
	if err != nil {
		t.Fatalf("GetWorkoutWithItems failed: %v", err)
	}
	if len(got.Blocks) != 3 {
		t.Fatalf("blocks = %d, want 3", len(got.Blocks))
	}
	c, a, e := got.Blocks[0], got.Blocks[1], got.Blocks[2]
	if c.Rounds != 4 || c.RestSeconds != 90 {
		t.Errorf("circuit = %d rounds/%ds rest, want 4/90", c.Rounds, c.RestSeconds)
	}
	if a.TimeCapSeconds != 600 {
		t.Errorf("amrap cap = %d, want 600", a.TimeCapSeconds)
	}
	if e.Rounds != 12 || e.IntervalSeconds != 60 {
		t.Errorf("emom = %d rounds/%ds interval, want 12/60", e.Rounds, e.IntervalSeconds)
	}

	// Deleted catalogue block: timing stays zero, not stale.
	delete(lookup.blocks, "blk-a")
	got, err = ctrl.GetWorkoutWithItems(w.ID, "u1")
	if err != nil {
		t.Fatalf("GetWorkoutWithItems after block delete failed: %v", err)
	}
	if got.Blocks[1].TimeCapSeconds != 0 {
		t.Errorf("deleted-block cap = %d, want 0", got.Blocks[1].TimeCapSeconds)
	}
}

// TestSetWorkoutStatus_PreservesBlocks is the regression test for the
// "Completed but 0/2 blocks" bug: a status-only change must leave the
// plan (blocks and their check-offs) untouched, unlike the full
// replacement UpdateWorkout which resets every block to pending.
func TestSetWorkoutStatus_PreservesBlocks(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	in := validWorkoutInput()
	in.Blocks = []WorkoutBlockInput{{BlockID: "blk-1"}, {BlockID: "blk-2"}}
	w, err := ctrl.CreateWorkout("u1", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[0].ID, "u1", models.WorkoutBlockDone); err != nil {
		t.Fatal(err)
	}
	updated, err := ctrl.SetWorkoutStatus(w.ID, "u1", models.WorkoutStatusCompleted)
	if err != nil {
		t.Fatalf("SetWorkoutStatus failed: %v", err)
	}
	if updated.Status != models.WorkoutStatusCompleted {
		t.Errorf("status = %q, want completed", updated.Status)
	}
	if len(updated.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (plan must survive)", len(updated.Blocks))
	}
	if updated.Blocks[0].Status != models.WorkoutBlockDone || updated.Blocks[1].Status != models.WorkoutBlockPending {
		t.Errorf("block statuses = %q/%q, want done/pending", updated.Blocks[0].Status, updated.Blocks[1].Status)
	}
	if _, err := ctrl.SetWorkoutStatus(w.ID, "u1", "someday"); !errors.Is(err, ErrWorkoutStatusInvalid) {
		t.Errorf("err = %v, want ErrWorkoutStatusInvalid", err)
	}
	if _, err := ctrl.SetWorkoutStatus("missing", "u1", models.WorkoutStatusCompleted); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("err = %v, want ErrWorkoutNotFound", err)
	}
	if _, err := ctrl.SetWorkoutStatus(w.ID, "u2", models.WorkoutStatusCompleted); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("wrong-user err = %v, want ErrWorkoutNotFound", err)
	}
}

// TestSetWorkoutHealthActivityType_PreservesBlocks mirrors the
// status-only regression test: a type-only change must leave the
// plan (blocks and their check-offs) untouched. Also covers empty
// normalizing to the blanket default, oversized keys, and
// missing/cross-user 404s.
func TestSetWorkoutHealthActivityType_PreservesBlocks(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	in := validWorkoutInput()
	in.Blocks = []WorkoutBlockInput{{BlockID: "blk-1"}, {BlockID: "blk-2"}}
	w, err := ctrl.CreateWorkout("u1", in)
	if err != nil {
		t.Fatal(err)
	}
	if w.HealthActivityType != models.DefaultHealthActivityType {
		t.Errorf("type = %q, want blanket default %q", w.HealthActivityType, models.DefaultHealthActivityType)
	}
	if _, err := ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[0].ID, "u1", models.WorkoutBlockDone); err != nil {
		t.Fatal(err)
	}
	updated, err := ctrl.SetWorkoutHealthActivityType(w.ID, "u1", "running")
	if err != nil {
		t.Fatalf("SetWorkoutHealthActivityType failed: %v", err)
	}
	if updated.HealthActivityType != "running" {
		t.Errorf("type = %q, want running", updated.HealthActivityType)
	}
	if len(updated.Blocks) != 2 {
		t.Fatalf("blocks = %d, want 2 (plan must survive)", len(updated.Blocks))
	}
	if updated.Blocks[0].Status != models.WorkoutBlockDone || updated.Blocks[1].Status != models.WorkoutBlockPending {
		t.Errorf("block statuses = %q/%q, want done/pending", updated.Blocks[0].Status, updated.Blocks[1].Status)
	}
	emptied, err := ctrl.SetWorkoutHealthActivityType(w.ID, "u1", "  ")
	if err != nil {
		t.Fatalf("empty type failed: %v", err)
	}
	if emptied.HealthActivityType != models.DefaultHealthActivityType {
		t.Errorf("empty type = %q, want default %q", emptied.HealthActivityType, models.DefaultHealthActivityType)
	}
	if _, err := ctrl.SetWorkoutHealthActivityType(w.ID, "u1", strings.Repeat("x", 65)); !errors.Is(err, ErrWorkoutHealthActivityTypeLong) {
		t.Errorf("err = %v, want ErrWorkoutHealthActivityTypeLong", err)
	}
	if _, err := ctrl.SetWorkoutHealthActivityType("missing", "u1", "running"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("err = %v, want ErrWorkoutNotFound", err)
	}
	if _, err := ctrl.SetWorkoutHealthActivityType(w.ID, "u2", "running"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("wrong-user err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestDuplicateWorkout(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	in := validWorkoutInput()
	in.Status = models.WorkoutStatusCompleted
	w, err := ctrl.CreateWorkout("u1", in)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ctrl.SetWorkoutBlockStatus(w.ID, w.Blocks[0].ID, "u1", models.WorkoutBlockDone); err != nil {
		t.Fatal(err)
	}
	cp, err := ctrl.DuplicateWorkout(w.ID, "u1", "2026-09-21")
	if err != nil {
		t.Fatalf("DuplicateWorkout failed: %v", err)
	}
	if cp.ID == w.ID {
		t.Error("duplicate must have a new ID")
	}
	if cp.Name != "Monday Strength" {
		t.Errorf("name = %q, want %q", cp.Name, "Monday Strength")
	}
	if cp.ScheduledDate != "2026-09-21" {
		t.Errorf("date = %q, want 2026-09-21", cp.ScheduledDate)
	}
	if cp.Status != models.WorkoutStatusPlanned {
		t.Errorf("status = %q, want planned", cp.Status)
	}
	if cp.HealthActivityType != w.HealthActivityType {
		t.Errorf("type = %q, want source %q carried over", cp.HealthActivityType, w.HealthActivityType)
	}
	if cp.Blocks[0].Status != models.WorkoutBlockPending {
		t.Errorf("block status = %q, want pending", cp.Blocks[0].Status)
	}
	if _, err := ctrl.DuplicateWorkout(w.ID, "u1", "not-a-date"); !errors.Is(err, ErrWorkoutDateInvalid) {
		t.Errorf("err = %v, want ErrWorkoutDateInvalid", err)
	}
}

func TestListWorkouts_RangeValidation(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	if _, err := ctrl.ListWorkouts("u1", "2026-09-21", "2026-09-14"); !errors.Is(err, ErrWorkoutRangeInvalid) {
		t.Errorf("err = %v, want ErrWorkoutRangeInvalid", err)
	}
	if _, err := ctrl.ListWorkouts("u1", "2026-09-14", ""); !errors.Is(err, ErrWorkoutDateInvalid) {
		t.Errorf("err = %v, want ErrWorkoutDateInvalid", err)
	}
}

func TestDuplicateWorkoutBatch_Success(t *testing.T) {
	ctrl, _ := setupWorkoutsController()
	w, err := ctrl.CreateWorkout("u1", validWorkoutInput())
	if err != nil {
		t.Fatal(err)
	}
	got, err := ctrl.DuplicateWorkoutBatch(w.ID, "u1", []string{"2026-09-21", "2026-09-28", "2026-10-05"})
	if err != nil {
		t.Fatalf("DuplicateWorkoutBatch failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	seen := map[string]bool{}
	for _, cp := range got {
		if cp.ID == "" || cp.ID == w.ID {
			t.Errorf("bad duplicate ID %q", cp.ID)
		}
		// Repeats keep the source name verbatim — the date
		// distinguishes them.
		if cp.Name != w.Name {
			t.Errorf("name = %q, want %q", cp.Name, w.Name)
		}
		if cp.Status != models.WorkoutStatusPlanned {
			t.Errorf("status = %q, want planned", cp.Status)
		}
		if len(cp.Blocks) != 1 || cp.Blocks[0].Status != models.WorkoutBlockPending {
			t.Errorf("blocks not reset: %+v", cp.Blocks)
		}
		seen[cp.ScheduledDate] = true
	}
	for _, d := range []string{"2026-09-21", "2026-09-28", "2026-10-05"} {
		if !seen[d] {
			t.Errorf("missing copy for %s", d)
		}
	}
}

func TestDuplicateWorkoutBatch_Validation(t *testing.T) {
	ctrl, repo := setupWorkoutsController()
	w, err := ctrl.CreateWorkout("u1", validWorkoutInput())
	if err != nil {
		t.Fatal(err)
	}
	base := repo.count()
	many := make([]string, 0, WorkoutBulkMax+1)
	for i := 0; i <= WorkoutBulkMax; i++ {
		many = append(many, fmt.Sprintf("2026-10-%02d", (i%28)+1))
	}
	cases := []struct {
		name  string
		id    string
		dates []string
		want  error
	}{
		{"empty", w.ID, nil, ErrWorkoutBulkRequired},
		{"too many", w.ID, many, ErrWorkoutBulkTooMany},
		{"bad date", w.ID, []string{"2026-09-21", "someday"}, ErrWorkoutDateInvalid},
		{"blank date", w.ID, []string{" "}, ErrWorkoutDateRequired},
		{"dup date", w.ID, []string{"2026-09-21", "2026-09-21"}, ErrWorkoutBulkDuplicateDate},
		{"missing source", "nope", []string{"2026-09-21"}, ErrWorkoutNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ctrl.DuplicateWorkoutBatch(tc.id, "u1", tc.dates); !errors.Is(err, tc.want) {
				t.Errorf("err = %v, want %v", err, tc.want)
			}
		})
	}
	// All-or-nothing: failed batches store zero rows.
	if n := repo.count(); n != base {
		t.Errorf("workout count = %d, want %d (no partial writes)", n, base)
	}
}
