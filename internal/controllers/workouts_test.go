package controllers

import (
	"errors"
	"fmt"
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
