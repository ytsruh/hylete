package controllers

import (
	"database/sql"
	"errors"
	"sync"
	"testing"
	"time"

	"hylete/internal/models"
)

// fakeWorkoutRepo is an in-memory models.WorkoutRepo.
type fakeWorkoutRepo struct {
	mu        sync.Mutex
	workouts  map[string]*models.Workout
	trees     map[string][]models.WorkoutBlockWithItems
	seq       int
	bulkCalls int
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
		ID:     "workout-" + string(rune('0'+f.seq)),
		UserID: userID, SourceWorkoutID: sourceWorkoutID,
		Name: name, Notes: notes, Status: status,
		ScheduledStart: start, ScheduledEnd: end,
	}
	f.workouts[w.ID] = w
	for i, b := range blocks {
		wb := models.WorkoutBlockWithItems{
			Block: models.WorkoutBlock{
				ID:        "wb-" + w.ID + string(rune('0'+i)),
				WorkoutID: w.ID, Type: b.Type, Position: i, Rounds: b.Rounds,
				RestBetweenRoundsSeconds: b.RestBetweenRoundsSeconds,
				IntervalSeconds:          b.IntervalSeconds, TimeCapSeconds: b.TimeCapSeconds,
			},
		}
		for j, item := range b.Items {
			wb.Items = append(wb.Items, models.WorkoutItem{
				ID:      "wi-" + wb.Block.ID + string(rune('0'+j)),
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
		if !w.ScheduledStart.After(end) && (w.ScheduledEnd == nil || !w.ScheduledEnd.Before(start)) {
			out = append(out, *w)
		}
	}
	return out, nil
}

func (f *fakeWorkoutRepo) UpdateWorkoutHeader(workoutID, userID, name, notes string, start, end *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[workoutID]
	if !ok || w.UserID != userID {
		return nil
	}
	w.Name = name
	w.Notes = notes
	w.ScheduledStart = start
	w.ScheduledEnd = end
	return nil
}

func (f *fakeWorkoutRepo) SetWorkoutStatus(workoutID, userID string, status models.WorkoutStatus, completedAt *time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	w, ok := f.workouts[workoutID]
	if !ok || w.UserID != userID {
		return nil
	}
	w.Status = status
	w.CompletedAt = completedAt
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
	f.mu.Lock()
	f.bulkCalls++
	f.mu.Unlock()
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

// seedStrengthExercise adds a strength exercise to the shared exercise
// mock so block-tree validation can resolve it.
func seedStrengthExercise(m *mockRepository, id, name string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.exercises = append(m.exercises, models.Exercise{ID: id, Name: name, Type: models.ExerciseTypeStrength})
}

func strengthBlock(exerciseID string) models.WorkoutBlockInput {
	return models.WorkoutBlockInput{
		Type:   models.WorkoutBlockTypeStraight,
		Rounds: 1,
		Items: []models.WorkoutItemInput{
			{
				ExerciseID: exerciseID,
				TargetSets: 3,
				Targets:    models.WorkoutItemTargets{TargetReps: 8, TargetWeight: 60},
			},
		},
	}
}

// seedSourceWorkout creates a workout with one strength block to
// copy from in duplicate/bulk tests.
func seedSourceWorkout(t *testing.T, wc *WorkoutController, userID, name string) *models.WorkoutDetail {
	t.Helper()
	detail, err := wc.CreateWorkout(userID, CreateWorkoutInput{
		Name: name, Blocks: []models.WorkoutBlockInput{strengthBlock("ex-1")},
	})
	if err != nil {
		t.Fatalf("seed source workout: %v", err)
	}
	return detail
}

func TestWorkoutController_CreateValidation(t *testing.T) {
	exercises := newMockRepository()
	seedStrengthExercise(exercises, "ex-1", "Squat")
	workouts := newFakeWorkoutRepo()
	wc := NewWorkoutController(workouts, exercises, exercises)

	if _, err := wc.CreateWorkout("user-1", CreateWorkoutInput{Name: "  "}); !errors.Is(err, ErrWorkoutNameRequired) {
		t.Errorf("blank name: error = %v", err)
	}
	if _, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "x", Blocks: []models.WorkoutBlockInput{{Type: "tabata"}},
	}); !errors.Is(err, ErrUnknownWorkoutBlockType) {
		t.Errorf("bad block type: error = %v", err)
	}
	if _, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "x", Blocks: []models.WorkoutBlockInput{strengthBlock("nope")},
	}); !errors.Is(err, ErrWorkoutExerciseNotFound) {
		t.Errorf("unknown exercise: error = %v", err)
	}
	if _, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "x",
		Blocks: []models.WorkoutBlockInput{{
			Type: models.WorkoutBlockTypeEMOM,
			Items: []models.WorkoutItemInput{{ExerciseID: "ex-1",
				Targets: models.WorkoutItemTargets{TargetReps: 5}}},
		}},
	}); !errors.Is(err, models.ErrBlockIntervalRequired) {
		t.Errorf("emom without interval: error = %v", err)
	}
	// Inverted schedule is rejected.
	start := time.Now().Add(2 * time.Hour)
	end := time.Now()
	if _, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "x", ScheduledStart: &start, ScheduledEnd: &end,
		Blocks: []models.WorkoutBlockInput{strengthBlock("ex-1")},
	}); !errors.Is(err, models.ErrScheduleEndBeforeStart) {
		t.Errorf("inverted schedule: error = %v", err)
	}
	// Items without targets are open prescriptions and save fine.
	created, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "x",
		Blocks: []models.WorkoutBlockInput{{
			Type:  models.WorkoutBlockTypeStraight,
			Items: []models.WorkoutItemInput{{ExerciseID: "ex-1"}},
		}},
	})
	if err != nil {
		t.Fatalf("open item rejected: %v", err)
	}
	if len(created.Blocks) != 1 || len(created.Blocks[0].Items) != 1 {
		t.Fatalf("tree not persisted: %+v", created.Blocks)
	}
	// Workouts authored from scratch carry no provenance.
	if created.Workout.SourceWorkoutID != nil {
		t.Errorf("scratch workout has source provenance: %v", *created.Workout.SourceWorkoutID)
	}
	if created.Workout.Status != models.WorkoutStatusPlanned {
		t.Errorf("status = %q, want planned", created.Workout.Status)
	}
}

func TestWorkoutController_Duplicate(t *testing.T) {
	exercises := newMockRepository()
	seedStrengthExercise(exercises, "ex-1", "Squat")
	workouts := newFakeWorkoutRepo()
	wc := NewWorkoutController(workouts, exercises, exercises)

	src := seedSourceWorkout(t, wc, "user-1", "Lower A")

	dup, err := wc.DuplicateWorkout(src.Workout.ID, "user-1", DuplicateWorkoutInput{})
	if err != nil {
		t.Fatalf("duplicate: %v", err)
	}
	if dup.Workout.Name != "Copy of Lower A" {
		t.Errorf("name = %q", dup.Workout.Name)
	}
	if dup.Workout.ID == src.Workout.ID {
		t.Error("duplicate shares the source ID")
	}
	if dup.Workout.SourceWorkoutID == nil || *dup.Workout.SourceWorkoutID != src.Workout.ID {
		t.Errorf("provenance lost: %+v", dup.Workout)
	}
	// The copy holds its own block rows (different IDs): snapshots,
	// never live references.
	if len(dup.Blocks) != 1 || len(dup.Blocks[0].Items) != 1 {
		t.Fatalf("tree not copied: %+v", dup.Blocks)
	}
	if dup.Blocks[0].Block.ID == src.Blocks[0].Block.ID {
		t.Error("duplicate shares block IDs with the source (not a snapshot)")
	}
	// Explicit name and schedule override the inherited values.
	start := time.Now().Add(24 * time.Hour)
	end := start.Add(time.Hour)
	custom, err := wc.DuplicateWorkout(src.Workout.ID, "user-1", DuplicateWorkoutInput{
		Name: "Friday legs", ScheduledStart: &start, ScheduledEnd: &end,
	})
	if err != nil {
		t.Fatalf("duplicate with overrides: %v", err)
	}
	if custom.Workout.Name != "Friday legs" {
		t.Errorf("name = %q", custom.Workout.Name)
	}
	if !custom.Workout.ScheduledStart.Equal(start) || !custom.Workout.ScheduledEnd.Equal(end) {
		t.Errorf("schedule not overridden: %+v", custom.Workout)
	}
	if _, err := wc.DuplicateWorkout("missing", "user-1", DuplicateWorkoutInput{}); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("missing source: error = %v", err)
	}
}

func TestWorkoutController_BulkCreate(t *testing.T) {
	exercises := newMockRepository()
	seedStrengthExercise(exercises, "ex-1", "Squat")
	workouts := newFakeWorkoutRepo()
	wc := NewWorkoutController(workouts, exercises, exercises)

	src := seedSourceWorkout(t, wc, "user-1", "Lower A")
	base := time.Now()
	instances := []BulkWorkoutInstance{
		{ScheduledStart: ptrTime(base.Add(24 * time.Hour)), ScheduledEnd: ptrTime(base.Add(25 * time.Hour))},
		{Name: "Week 2", ScheduledStart: ptrTime(base.Add(8 * 24 * time.Hour)), ScheduledEnd: ptrTime(base.Add(8*24*time.Hour + time.Hour))},
	}
	created, err := wc.BulkCreateWorkouts("user-1", src.Workout.ID, instances)
	if err != nil {
		t.Fatalf("bulk: %v", err)
	}
	if len(created) != 2 {
		t.Fatalf("len = %d, want 2", len(created))
	}
	// Blank instance names fall back to the source name.
	if created[0].Name != "Lower A" || created[1].Name != "Week 2" {
		t.Errorf("names = %q, %q", created[0].Name, created[1].Name)
	}
	for _, w := range created {
		if w.SourceWorkoutID == nil || *w.SourceWorkoutID != src.Workout.ID {
			t.Errorf("provenance lost on %s", w.ID)
		}
	}
	// Empty instances return an empty (non-nil) slice without touching the repo.
	empty, err := wc.BulkCreateWorkouts("user-1", src.Workout.ID, nil)
	if err != nil || empty == nil || len(empty) != 0 {
		t.Fatalf("empty bulk = %v, %v", empty, err)
	}
	// A bad instance fails before any write (fail-fast validation).
	badStart := base.Add(3 * time.Hour)
	badEnd := base
	if _, err := wc.BulkCreateWorkouts("user-1", src.Workout.ID, []BulkWorkoutInstance{
		{ScheduledStart: &badStart, ScheduledEnd: &badEnd},
	}); !errors.Is(err, models.ErrScheduleEndBeforeStart) {
		t.Errorf("bad instance: error = %v", err)
	}
	if _, err := wc.BulkCreateWorkouts("user-1", "missing", instances); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("missing source: error = %v", err)
	}
}

func ptrTime(t time.Time) *time.Time { return &t }

func TestWorkoutController_StatusTransitions(t *testing.T) {
	exercises := newMockRepository()
	workouts := newFakeWorkoutRepo()
	wc := NewWorkoutController(workouts, exercises, exercises)

	detail, err := wc.CreateWorkout("user-1", CreateWorkoutInput{Name: "legs"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := detail.Workout.ID

	completed, err := wc.Complete(id, "user-1", time.Now())
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if completed.Workout.Status != models.WorkoutStatusCompleted || completed.Workout.CompletedAt == nil {
		t.Errorf("not completed: %+v", completed.Workout)
	}
	reopened, err := wc.Reopen(id, "user-1")
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	if reopened.Workout.Status != models.WorkoutStatusPlanned || reopened.Workout.CompletedAt != nil {
		t.Errorf("not reopened: %+v", reopened.Workout)
	}
	cancelled, err := wc.Cancel(id, "user-1")
	if err != nil {
		t.Fatalf("cancel: %v", err)
	}
	if cancelled.Workout.Status != models.WorkoutStatusCancelled {
		t.Errorf("not cancelled: %+v", cancelled.Workout)
	}
	if err := wc.DeleteWorkout(id, "user-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := wc.GetWorkoutDetail(id, "user-1"); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("deleted workout: error = %v", err)
	}
	if _, err := wc.Complete("missing", "user-1", time.Now()); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("missing complete: error = %v", err)
	}
}

func TestWorkoutController_GetDetailSplitsEntries(t *testing.T) {
	exercises := newMockRepository()
	seedStrengthExercise(exercises, "ex-1", "Squat")
	workouts := newFakeWorkoutRepo()
	entries := newMockRepository()
	entries.exercises = exercises.exercises
	wc := NewWorkoutController(workouts, exercises, entries)

	detail, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "legs", Blocks: []models.WorkoutBlockInput{strengthBlock("ex-1")},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	itemID := detail.Blocks[0].Items[0].ID
	entries.exerciseEntries = []models.ExerciseEntry{
		{ID: "e-1", UserID: "user-1", ExerciseID: "ex-1", WorkoutID: detail.Workout.ID, WorkoutItemID: itemID},
		{ID: "e-2", UserID: "user-1", ExerciseID: "ex-1", WorkoutID: detail.Workout.ID},
		{ID: "e-3", UserID: "user-1", ExerciseID: "ex-1"},
	}
	got, err := wc.GetWorkoutDetail(detail.Workout.ID, "user-1")
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if len(got.LoggedExerciseEntries) != 1 || got.LoggedExerciseEntries[0].ID != "e-1" {
		t.Errorf("linked = %+v", got.LoggedExerciseEntries)
	}
	if len(got.AdHocExerciseEntries) != 1 || got.AdHocExerciseEntries[0].ID != "e-2" {
		t.Errorf("ad-hoc = %+v", got.AdHocExerciseEntries)
	}
}

func TestWorkoutController_ValidateExerciseEntryLinkage(t *testing.T) {
	exercises := newMockRepository()
	seedStrengthExercise(exercises, "ex-1", "Squat")
	workouts := newFakeWorkoutRepo()
	wc := NewWorkoutController(workouts, exercises, exercises)

	// Standalone always passes.
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{}); err != nil {
		t.Fatalf("standalone: %v", err)
	}
	// Item without workout.
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{WorkoutItemID: "wi-x"}); !errors.Is(err, ErrWorkoutItemNeedsWorkout) {
		t.Errorf("item w/o workout: error = %v", err)
	}
	// Round without workout.
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{RoundNumber: 1}); !errors.Is(err, ErrWorkoutRoundNeedsWorkout) {
		t.Errorf("round w/o workout: error = %v", err)
	}
	// Negative round.
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{WorkoutID: "w", RoundNumber: -1}); !errors.Is(err, ErrWorkoutRoundInvalid) {
		t.Errorf("negative round: error = %v", err)
	}
	// Unknown workout.
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{WorkoutID: "missing"}); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("unknown workout: error = %v", err)
	}

	// Happy path: item belongs to the workout.
	detail, err := wc.CreateWorkout("user-1", CreateWorkoutInput{
		Name: "legs", Blocks: []models.WorkoutBlockInput{strengthBlock("ex-1")},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	itemID := detail.Blocks[0].Items[0].ID
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{
		WorkoutID: detail.Workout.ID, WorkoutItemID: itemID, RoundNumber: 2,
	}); err != nil {
		t.Fatalf("valid link: %v", err)
	}
	// Workout-only (ad-hoc) passes.
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{WorkoutID: detail.Workout.ID}); err != nil {
		t.Fatalf("ad-hoc link: %v", err)
	}
	// Item from another workout mismatches.
	other, err := wc.CreateWorkout("user-1", CreateWorkoutInput{Name: "other"})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}
	if err := wc.ValidateExerciseEntryLinkage("user-1", models.WorkoutLinkage{
		WorkoutID: other.Workout.ID, WorkoutItemID: itemID,
	}); !errors.Is(err, ErrWorkoutItemMismatch) {
		t.Errorf("cross-workout item: error = %v", err)
	}
	// Other user's workout is invisible.
	if err := wc.ValidateExerciseEntryLinkage("user-2", models.WorkoutLinkage{WorkoutID: detail.Workout.ID}); !errors.Is(err, ErrWorkoutNotFound) {
		t.Errorf("other user: error = %v", err)
	}
}
