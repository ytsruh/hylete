package controllers

import (
	"fmt"
	"sync"
	"testing"

	"hylete/internal/models"
)

// fakeWorkoutRepo is an in-memory models.WorkoutRepo for
// controller tests. Scoping mirrors the real repository:
// reads/writes only match rows owned by the supplied userID.
type fakeWorkoutRepo struct {
	mu          sync.Mutex
	workouts    map[string]*models.Workout
	assignments map[string]*models.WorkoutAssignment
	seq         int
	aseq        int
}

func newFakeWorkoutRepo() *fakeWorkoutRepo {
	return &fakeWorkoutRepo{
		workouts:    map[string]*models.Workout{},
		assignments: map[string]*models.WorkoutAssignment{},
	}
}

func (f *fakeWorkoutRepo) Create(w *models.Workout) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	w.ID = fmt.Sprintf("workout-%d", f.seq)
	cp := *w
	cp.Blocks = append([]models.WorkoutBlock{}, w.Blocks...)
	for i := range cp.Blocks {
		cp.Blocks[i].ID = fmt.Sprintf("%s-link-%d", cp.ID, i)
		cp.Blocks[i].WorkoutID = cp.ID
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

func (f *fakeWorkoutRepo) Delete(id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if w, ok := f.workouts[id]; ok && w.UserID == userID {
		delete(f.workouts, id)
		for aid, a := range f.assignments {
			if a.WorkoutID == id {
				delete(f.assignments, aid)
			}
		}
	}
	return nil
}

func (f *fakeWorkoutRepo) Duplicate(id, userID string, copySchedule bool) (*models.Workout, error) {
	f.mu.Lock()
	src, ok := f.workouts[id]
	if !ok || src.UserID != userID {
		f.mu.Unlock()
		return nil, nil
	}
	cp := *src
	cp.Blocks = append([]models.WorkoutBlock{}, src.Blocks...)
	f.mu.Unlock()

	dup := &models.Workout{
		UserID:      userID,
		Title:       cp.Title + " copy",
		Description: cp.Description,
		Blocks:      cp.Blocks,
	}
	if err := f.Create(dup); err != nil {
		return nil, err
	}
	if copySchedule {
		var dates []string
		f.mu.Lock()
		for _, a := range f.assignments {
			if a.WorkoutID == id {
				dates = append(dates, a.ScheduledDate)
			}
		}
		f.mu.Unlock()
		if _, err := f.AddAssignments(dup.ID, userID, dates); err != nil {
			return nil, err
		}
	}
	return dup, nil
}

func (f *fakeWorkoutRepo) ListAssignmentsForWorkout(workoutID string) ([]models.WorkoutAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []models.WorkoutAssignment
	for _, a := range f.assignments {
		if a.WorkoutID == workoutID {
			out = append(out, *a)
		}
	}
	if out == nil {
		out = []models.WorkoutAssignment{}
	}
	return out, nil
}

func (f *fakeWorkoutRepo) ListAssignmentsByDateRange(userID, start, end string) ([]models.WorkoutAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []models.WorkoutAssignment
	for _, w := range f.workouts {
		if w.UserID != userID {
			continue
		}
		for _, a := range f.assignments {
			if a.WorkoutID == w.ID && a.ScheduledDate >= start && a.ScheduledDate <= end {
				cp := *a
				cp.WorkoutTitle = w.Title
				out = append(out, cp)
			}
		}
	}
	if out == nil {
		out = []models.WorkoutAssignment{}
	}
	return out, nil
}

func (f *fakeWorkoutRepo) AddAssignments(workoutID, userID string, dates []string) ([]models.WorkoutAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []models.WorkoutAssignment
	for _, d := range dates {
		dup := false
		for _, a := range f.assignments {
			if a.WorkoutID == workoutID && a.ScheduledDate == d {
				dup = true
				break
			}
		}
		if dup {
			continue
		}
		f.aseq++
		a := &models.WorkoutAssignment{
			ID:            fmt.Sprintf("assign-%d", f.aseq),
			WorkoutID:     workoutID,
			ScheduledDate: d,
		}
		f.assignments[a.ID] = a
		out = append(out, *a)
	}
	return out, nil
}

func (f *fakeWorkoutRepo) GetAssignment(id, userID string) (*models.WorkoutAssignment, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	a, ok := f.assignments[id]
	if !ok {
		return nil, nil
	}
	w, ok := f.workouts[a.WorkoutID]
	if !ok || w.UserID != userID {
		return nil, nil
	}
	cp := *a
	return &cp, nil
}

func (f *fakeWorkoutRepo) DeleteAssignment(id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if a, ok := f.assignments[id]; ok {
		if w, ok := f.workouts[a.WorkoutID]; ok && w.UserID == userID {
			delete(f.assignments, id)
		}
	}
	return nil
}

// fakeWorkoutBlocks resolves a fixed catalog of block IDs for
// validation tests. Unknown IDs return (nil, nil) like the real
// repository's not-found path.
type fakeWorkoutBlocks struct {
	known map[string]models.Block
}

func newFakeWorkoutBlocks(ids ...string) *fakeWorkoutBlocks {
	f := &fakeWorkoutBlocks{known: map[string]models.Block{}}
	for _, id := range ids {
		f.known[id] = models.Block{ID: id, Name: "Block " + id, Type: models.BlockTypeStandard}
	}
	return f
}

func (f *fakeWorkoutBlocks) GetByID(id string, _ string) (*models.Block, error) {
	b, ok := f.known[id]
	if !ok {
		return nil, nil
	}
	cp := b
	return &cp, nil
}

func testWorkoutsController() *WorkoutsController {
	return NewWorkoutsController(newFakeWorkoutRepo(), newFakeWorkoutBlocks("b-1", "b-2", "b-3"))
}

func TestCreateWorkoutSuccess(t *testing.T) {
	wc := testWorkoutsController()
	w, err := wc.CreateWorkout("u1", CreateWorkoutInput{
		Title:       "Push Day",
		Description: "Chest + shoulders",
		BlockIDs:    []string{"b-1", "b-2"},
	})
	if err != nil {
		t.Fatalf("CreateWorkout: %v", err)
	}
	if w.ID == "" {
		t.Fatal("expected generated id")
	}
	if len(w.Blocks) != 2 {
		t.Fatalf("blocks len = %d, want 2", len(w.Blocks))
	}
	if w.Blocks[0].Position != 0 || w.Blocks[1].Position != 1 {
		t.Fatalf("unexpected positions: %+v", w.Blocks)
	}
	if w.Blocks[0].BlockName == "" {
		t.Fatalf("expected resolved block name: %+v", w.Blocks[0])
	}
}

func TestCreateWorkoutValidation(t *testing.T) {
	wc := testWorkoutsController()
	cases := []struct {
		name string
		in   CreateWorkoutInput
		want error
	}{
		{"empty title", CreateWorkoutInput{BlockIDs: []string{"b-1"}}, ErrWorkoutTitleRequired},
		{"no blocks", CreateWorkoutInput{Title: "x"}, ErrWorkoutBlocksRequired},
		{"unknown block", CreateWorkoutInput{Title: "x", BlockIDs: []string{"nope"}}, ErrWorkoutBlockNotFound},
		{"blank block", CreateWorkoutInput{Title: "x", BlockIDs: []string{"  "}}, ErrWorkoutBlockRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := wc.CreateWorkout("u1", tc.in); err != tc.want {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestCreateWorkoutSameBlockRepeats(t *testing.T) {
	wc := testWorkoutsController()
	w, err := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "x", BlockIDs: []string{"b-1", "b-1"}})
	if err != nil {
		t.Fatalf("repeats should be allowed: %v", err)
	}
	if len(w.Blocks) != 2 || w.Blocks[0].BlockID != w.Blocks[1].BlockID {
		t.Fatalf("unexpected blocks: %+v", w.Blocks)
	}
}

func TestUpdateWorkoutReplacesBlocks(t *testing.T) {
	wc := testWorkoutsController()
	w, err := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "v1", BlockIDs: []string{"b-1"}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := wc.UpdateWorkout(w.ID, "u1", UpdateWorkoutInput{Title: "v2", BlockIDs: []string{"b-2", "b-3"}})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "v2" || len(updated.Blocks) != 2 {
		t.Fatalf("unexpected updated workout: %+v", updated)
	}
	if _, err := wc.UpdateWorkout("missing", "u1", UpdateWorkoutInput{Title: "x", BlockIDs: []string{"b-1"}}); err != ErrWorkoutNotFound {
		t.Fatalf("err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestDeleteWorkout(t *testing.T) {
	wc := testWorkoutsController()
	w, _ := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "x", BlockIDs: []string{"b-1"}})
	if err := wc.DeleteWorkout(w.ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if _, err := wc.GetWorkout(w.ID, "u1"); err != ErrWorkoutNotFound {
		t.Fatalf("err = %v, want ErrWorkoutNotFound", err)
	}
	if err := wc.DeleteWorkout("missing", "u1"); err != ErrWorkoutNotFound {
		t.Fatalf("err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestDuplicateWorkout(t *testing.T) {
	wc := testWorkoutsController()
	w, _ := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "Base", BlockIDs: []string{"b-1", "b-2"}})
	if _, err := wc.AddAssignments(w.ID, "u1", []string{"2026-09-14"}); err != nil {
		t.Fatal(err)
	}

	dup, err := wc.DuplicateWorkout(w.ID, "u1", false)
	if err != nil {
		t.Fatal(err)
	}
	if dup.Title != "Base copy" {
		t.Fatalf("title = %q, want %q", dup.Title, "Base copy")
	}
	if len(dup.Blocks) != 2 {
		t.Fatalf("blocks len = %d, want 2", len(dup.Blocks))
	}
	sched, err := wc.GetWorkoutSchedule(dup.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sched) != 0 {
		t.Fatalf("schedule len = %d, want 0 (no copy)", len(sched))
	}

	withSched, err := wc.DuplicateWorkout(w.ID, "u1", true)
	if err != nil {
		t.Fatal(err)
	}
	sched, err = wc.GetWorkoutSchedule(withSched.ID, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(sched) != 1 || sched[0].ScheduledDate != "2026-09-14" {
		t.Fatalf("unexpected copied schedule: %+v", sched)
	}

	if _, err := wc.DuplicateWorkout("missing", "u1", false); err != ErrWorkoutNotFound {
		t.Fatalf("err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestAddAssignmentsValidation(t *testing.T) {
	wc := testWorkoutsController()
	w, _ := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "x", BlockIDs: []string{"b-1"}})

	got, err := wc.AddAssignments(w.ID, "u1", []string{"2026-09-14", "2026-09-15"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	// Duplicate days are skipped idempotently.
	got, err = wc.AddAssignments(w.ID, "u1", []string{"2026-09-14"})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("len = %d, want 0 (skip)", len(got))
	}

	for _, dates := range [][]string{{}, {"not-a-date"}, {"2026-13-40"}} {
		if _, err := wc.AddAssignments(w.ID, "u1", dates); err == nil {
			t.Fatalf("dates %v: expected error", dates)
		}
	}
	if _, err := wc.AddAssignments("missing", "u1", []string{"2026-09-14"}); err != ErrWorkoutNotFound {
		t.Fatalf("err = %v, want ErrWorkoutNotFound", err)
	}
}

func TestDeleteAssignment(t *testing.T) {
	wc := testWorkoutsController()
	w, _ := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "x", BlockIDs: []string{"b-1"}})
	got, _ := wc.AddAssignments(w.ID, "u1", []string{"2026-09-14"})
	if err := wc.DeleteAssignment(got[0].ID, "u1"); err != nil {
		t.Fatal(err)
	}
	if err := wc.DeleteAssignment(got[0].ID, "u1"); err != ErrWorkoutAssignmentNotFound {
		t.Fatalf("err = %v, want ErrWorkoutAssignmentNotFound", err)
	}
	// Another user's assignment is invisible.
	got, _ = wc.AddAssignments(w.ID, "u1", []string{"2026-09-15"})
	if err := wc.DeleteAssignment(got[0].ID, "u2"); err != ErrWorkoutAssignmentNotFound {
		t.Fatalf("err = %v, want ErrWorkoutAssignmentNotFound", err)
	}
}

func TestGetScheduleRange(t *testing.T) {
	wc := testWorkoutsController()
	w, _ := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "x", BlockIDs: []string{"b-1"}})
	if _, err := wc.AddAssignments(w.ID, "u1", []string{"2026-09-14", "2026-09-20"}); err != nil {
		t.Fatal(err)
	}

	got, err := wc.GetSchedule("u1", "2026-09-14", "2026-09-20")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].WorkoutTitle != "x" {
		t.Fatalf("title = %q, want resolved title", got[0].WorkoutTitle)
	}

	narrow, err := wc.GetSchedule("u1", "2026-09-15", "2026-09-19")
	if err != nil {
		t.Fatal(err)
	}
	if len(narrow) != 0 {
		t.Fatalf("len = %d, want 0", len(narrow))
	}

	if _, err := wc.GetSchedule("u1", "2026-09-20", "2026-09-14"); err != ErrWorkoutScheduleRangeOrder {
		t.Fatalf("err = %v, want order error", err)
	}
	if _, err := wc.GetSchedule("u1", "2026-01-01", "2026-12-31"); err != ErrWorkoutScheduleRangeSpan {
		t.Fatalf("err = %v, want span error", err)
	}
	if _, err := wc.GetSchedule("u1", "not-a-date", "2026-09-14"); err != ErrWorkoutDateInvalid {
		t.Fatalf("err = %v, want date error", err)
	}
}

func TestWorkoutScoping(t *testing.T) {
	wc := testWorkoutsController()
	w, _ := wc.CreateWorkout("u1", CreateWorkoutInput{Title: "x", BlockIDs: []string{"b-1"}})
	if _, err := wc.GetWorkout(w.ID, "u2"); err != ErrWorkoutNotFound {
		t.Fatalf("cross-user read err = %v, want ErrWorkoutNotFound", err)
	}
}
