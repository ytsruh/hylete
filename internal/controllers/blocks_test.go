package controllers

import (
	"strings"
	"sync"
	"testing"

	"hylete/internal/models"
)

// fakeBlockRepo is an in-memory models.BlockRepo for controller
// tests. Scoping mirrors the real repository: reads/writes only
// match rows owned by the supplied userID.
type fakeBlockRepo struct {
	mu     sync.Mutex
	blocks map[string]*models.Block
	seq    int
}

func newFakeBlockRepo() *fakeBlockRepo {
	return &fakeBlockRepo{blocks: map[string]*models.Block{}}
}

func (f *fakeBlockRepo) Create(b *models.Block) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.seq++
	b.ID = "block-test-" + string(rune('0'+f.seq))
	cp := *b
	cp.Items = append([]models.BlockItem{}, b.Items...)
	for i := range cp.Items {
		cp.Items[i].ID = cp.ID + "-item-" + string(rune('0'+i))
		cp.Items[i].BlockID = cp.ID
	}
	f.blocks[cp.ID] = &cp
	*b = cp
	return nil
}

func (f *fakeBlockRepo) GetByID(id, userID string) (*models.Block, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.blocks[id]
	if !ok || b.UserID != userID {
		return nil, nil
	}
	cp := *b
	cp.Items = append([]models.BlockItem{}, b.Items...)
	return &cp, nil
}

func (f *fakeBlockRepo) List(userID string) ([]models.BlockSummary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []models.BlockSummary
	for _, b := range f.blocks {
		if b.UserID != userID {
			continue
		}
		cp := *b
		out = append(out, models.BlockSummary{Block: cp, ItemCount: len(b.Items)})
	}
	if out == nil {
		out = []models.BlockSummary{}
	}
	return out, nil
}

func (f *fakeBlockRepo) Update(b *models.Block, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	existing, ok := f.blocks[b.ID]
	if !ok || existing.UserID != userID {
		return nil
	}
	cp := *b
	cp.Items = append([]models.BlockItem{}, b.Items...)
	f.blocks[b.ID] = &cp
	return nil
}

func (f *fakeBlockRepo) Delete(id, userID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if b, ok := f.blocks[id]; ok && b.UserID == userID {
		delete(f.blocks, id)
	}
	return nil
}

// fakeBlockExercises resolves a fixed catalog of exercise IDs for
// validation tests. Unknown IDs return (nil, nil) like the real
// repository's not-found path.
type fakeBlockExercises struct {
	known map[string]models.Exercise
}

func newFakeBlockExercises(ids ...string) *fakeBlockExercises {
	f := &fakeBlockExercises{known: map[string]models.Exercise{}}
	for _, id := range ids {
		f.known[id] = models.Exercise{ID: id, Name: "Exercise " + id, Type: models.ExerciseTypeStrength}
	}
	return f
}

func (f *fakeBlockExercises) GetExerciseByID(id string, _ string) (*models.Exercise, error) {
	ex, ok := f.known[id]
	if !ok {
		return nil, nil
	}
	cp := ex
	return &cp, nil
}

func testBlocksController() *BlocksController {
	return NewBlocksController(newFakeBlockRepo(), newFakeBlockExercises("ex-1", "ex-2", "ex-3"))
}

func TestCreateBlockStandard(t *testing.T) {
	bc := testBlocksController()
	b, err := bc.CreateBlock("user-1", CreateBlockInput{
		Name:  " Push Day ",
		Type:  models.BlockTypeStandard,
		Items: []BlockItemInput{{ExerciseID: "ex-1", TargetText: "3x5 @ 100kg"}, {ExerciseID: "ex-2"}},
	})
	if err != nil {
		t.Fatalf("CreateBlock returned error: %v", err)
	}
	if b.Name != "Push Day" {
		t.Errorf("expected trimmed name, got %q", b.Name)
	}
	if len(b.Items) != 2 || b.Items[0].Position != 0 || b.Items[1].Position != 1 {
		t.Errorf("expected 2 items with positions 0,1, got %+v", b.Items)
	}
	if b.Items[0].ExerciseName == "" {
		t.Error("expected resolved exercise name on item")
	}
}

func TestCreateBlockValidation(t *testing.T) {
	cases := []struct {
		name string
		in   CreateBlockInput
		want error
	}{
		{"empty name", CreateBlockInput{Type: models.BlockTypeStandard, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockNameRequired},
		{"name too long", CreateBlockInput{Name: strings.Repeat("n", 101), Type: models.BlockTypeStandard, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockNameTooLong},
		{"bad type", CreateBlockInput{Name: "x", Type: "superset", Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockTypeInvalid},
		{"no items", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard}, ErrBlockItemsRequired},
		{"too many items", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard, Items: make([]BlockItemInput, 21)}, ErrBlockItemsTooMany},
		{"missing exercise", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard, Items: []BlockItemInput{{}}}, ErrBlockExerciseRequired},
		{"unknown exercise", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard, Items: []BlockItemInput{{ExerciseID: "nope"}}}, ErrBlockExerciseNotFound},
		{"target too long", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard, Items: []BlockItemInput{{ExerciseID: "ex-1", TargetText: strings.Repeat("t", 501)}}}, ErrBlockTargetTooLong},
		{"circuit needs rounds", CreateBlockInput{Name: "x", Type: models.BlockTypeCircuit, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockRoundsRequired},
		{"standard rejects rounds", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard, Rounds: 3, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockRoundsUnused},
		{"standard rejects rest", CreateBlockInput{Name: "x", Type: models.BlockTypeStandard, RestSeconds: 60, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockRestUnused},
		{"amrap needs cap", CreateBlockInput{Name: "x", Type: models.BlockTypeAmrap, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockTimeCapRequired},
		{"amrap cap too small", CreateBlockInput{Name: "x", Type: models.BlockTypeAmrap, TimeCapSeconds: 30, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockTimeCapInvalid},
		{"emom needs interval", CreateBlockInput{Name: "x", Type: models.BlockTypeEmom, Rounds: 10, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockIntervalRequired},
		{"emom needs rounds", CreateBlockInput{Name: "x", Type: models.BlockTypeEmom, IntervalSeconds: 60, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockRoundsRequired},
		{"circuit rejects cap", CreateBlockInput{Name: "x", Type: models.BlockTypeCircuit, Rounds: 3, TimeCapSeconds: 600, Items: []BlockItemInput{{ExerciseID: "ex-1"}}}, ErrBlockTimeCapUnused},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bc := testBlocksController()
			if _, err := bc.CreateBlock("user-1", tc.in); err != tc.want {
				t.Errorf("expected %v, got %v", tc.want, err)
			}
		})
	}
	// 21 identical items trip the count check before exercise
	// resolution — fill the IDs so the case above stays a pure
	// count test even after resolution runs first.
}

func TestCreateBlockAllKinds(t *testing.T) {
	bc := testBlocksController()
	kinds := []CreateBlockInput{
		{Name: "c", Type: models.BlockTypeCircuit, Rounds: 4, RestSeconds: 90, Items: []BlockItemInput{{ExerciseID: "ex-1"}}},
		{Name: "a", Type: models.BlockTypeAmrap, TimeCapSeconds: 600, Items: []BlockItemInput{{ExerciseID: "ex-1"}}},
		{Name: "e", Type: models.BlockTypeEmom, Rounds: 12, IntervalSeconds: 60, Items: []BlockItemInput{{ExerciseID: "ex-1"}}},
	}
	for _, in := range kinds {
		if _, err := bc.CreateBlock("user-1", in); err != nil {
			t.Errorf("kind %s returned error: %v", in.Type, err)
		}
	}
}

func TestGetUpdateDeleteBlockScoping(t *testing.T) {
	repo := newFakeBlockRepo()
	bc := NewBlocksController(repo, newFakeBlockExercises("ex-1", "ex-2"))
	created, err := bc.CreateBlock("user-1", CreateBlockInput{
		Name: "Legs", Type: models.BlockTypeStandard,
		Items: []BlockItemInput{{ExerciseID: "ex-1", TargetText: "5x5"}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if _, err := bc.GetBlock(created.ID, "user-2"); err != ErrBlockNotFound {
		t.Errorf("cross-user get should 404, got %v", err)
	}
	updated, err := bc.UpdateBlock(created.ID, "user-1", UpdateBlockInput{
		Name: "Legs v2", Description: "heavy", Type: models.BlockTypeCircuit, Rounds: 3, RestSeconds: 60,
		Items: []BlockItemInput{{ExerciseID: "ex-2", TargetText: "3 rounds"}},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "Legs v2" || len(updated.Items) != 1 || updated.Items[0].ExerciseID != "ex-2" {
		t.Errorf("unexpected updated block: %+v", updated)
	}
	if err := bc.DeleteBlock(created.ID, "user-2"); err != ErrBlockNotFound {
		t.Errorf("cross-user delete should 404, got %v", err)
	}
	if err := bc.DeleteBlock(created.ID, "user-1"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := bc.GetBlock(created.ID, "user-1"); err != ErrBlockNotFound {
		t.Errorf("deleted block should 404, got %v", err)
	}
}
