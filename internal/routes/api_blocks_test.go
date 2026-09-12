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

// mockBlockRepository is an in-memory models.BlockRepo for the
// blocks API tests. Scoping mirrors the real repository.
type mockBlockRepository struct {
	mu     sync.Mutex
	blocks map[string]*models.Block
	seq    int
}

func newMockBlockRepository() *mockBlockRepository {
	return &mockBlockRepository{blocks: map[string]*models.Block{}}
}

func (m *mockBlockRepository) Create(b *models.Block) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.seq++
	b.ID = fmt.Sprintf("block-%d", m.seq)
	cp := *b
	cp.Items = append([]models.BlockItem{}, b.Items...)
	for i := range cp.Items {
		cp.Items[i].ID = fmt.Sprintf("%s-item-%d", cp.ID, i)
		cp.Items[i].BlockID = cp.ID
	}
	m.blocks[cp.ID] = &cp
	*b = cp
	return nil
}

func (m *mockBlockRepository) GetByID(id, userID string) (*models.Block, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	b, ok := m.blocks[id]
	if !ok || b.UserID != userID {
		return nil, nil
	}
	cp := *b
	cp.Items = append([]models.BlockItem{}, b.Items...)
	return &cp, nil
}

func (m *mockBlockRepository) List(userID string) ([]models.BlockSummary, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := []models.BlockSummary{}
	for _, b := range m.blocks {
		if b.UserID != userID {
			continue
		}
		cp := *b
		out = append(out, models.BlockSummary{Block: cp, ItemCount: len(b.Items)})
	}
	return out, nil
}

func (m *mockBlockRepository) Update(b *models.Block, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.blocks[b.ID]
	if !ok || existing.UserID != userID {
		return nil
	}
	cp := *b
	cp.Items = append([]models.BlockItem{}, b.Items...)
	m.blocks[b.ID] = &cp
	return nil
}

func (m *mockBlockRepository) Delete(id, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if b, ok := m.blocks[id]; ok && b.UserID == userID {
		delete(m.blocks, id)
	}
	return nil
}

// setupBlocksHandler wires block routes onto the shared test
// handler: the exercise mock from setupHandler (seeded with ex-1,
// ex-2) backs exercise resolution, and a fresh block repo backs
// storage.
func setupBlocksHandler(t *testing.T) (*Handler, *mockBlockRepository, *mockUserRepository, *echo.Echo) {
	t.Helper()
	h, mockExercises, mockUser, e := setupHandler(t)
	repo := newMockBlockRepository()
	h.SetBlocksController(controllers.NewBlocksController(repo, mockExercises))
	return h, repo, mockUser, e
}

type blocksResponse struct {
	Blocks []BlockSummaryDTO `json:"blocks"`
}

func TestAPIListBlocks_Empty(t *testing.T) {
	h, _, mockUser, e := setupBlocksHandler(t)
	token, _ := loginUser(t, h, mockUser, "bl@example.com", "BL")

	rec := apiDo(t, e, http.MethodGet, "/api/v1/blocks", token, nil)
	resp := decodeAPI[blocksResponse](t, rec, http.StatusOK)
	if len(resp.Blocks) != 0 {
		t.Fatalf("blocks len = %d, want 0", len(resp.Blocks))
	}
}

func TestAPICreateBlock_Success(t *testing.T) {
	h, _, mockUser, e := setupBlocksHandler(t)
	token, _ := loginUser(t, h, mockUser, "bc@example.com", "BC")

	rec := apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
		Name:        "Push Day",
		Description: "Chest + shoulders",
		Type:        "circuit",
		Rounds:      4,
		RestSeconds: 90,
		Items: []CreateBlockItemRequest{
			{ExerciseID: "ex-1", TargetText: "3x5 @ 100kg"},
			{ExerciseID: "ex-2"},
		},
	})
	b := decodeAPI[BlockDTO](t, rec, http.StatusCreated)
	if b.ID == "" {
		t.Fatal("expected generated id")
	}
	if b.Type != "circuit" || b.Rounds != 4 || b.RestSeconds != 90 {
		t.Fatalf("unexpected config: %+v", b)
	}
	if len(b.Items) != 2 {
		t.Fatalf("items len = %d, want 2", len(b.Items))
	}
	if b.Items[0].ExerciseName == "" || b.Items[0].TargetText != "3x5 @ 100kg" {
		t.Fatalf("unexpected first item: %+v", b.Items[0])
	}
	if b.Items[0].Position != 0 || b.Items[1].Position != 1 {
		t.Fatalf("unexpected positions: %+v", b.Items)
	}
}

func TestAPICreateBlock_Validation(t *testing.T) {
	h, _, mockUser, e := setupBlocksHandler(t)
	token, _ := loginUser(t, h, mockUser, "bv@example.com", "BV")

	cases := []struct {
		name string
		body CreateBlockRequest
	}{
		{"empty name", CreateBlockRequest{Type: "standard", Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}}}},
		{"bad type", CreateBlockRequest{Name: "x", Type: "superset", Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}}}},
		{"no items", CreateBlockRequest{Name: "x", Type: "standard"}},
		{"unknown exercise", CreateBlockRequest{Name: "x", Type: "standard", Items: []CreateBlockItemRequest{{ExerciseID: "nope"}}}},
		{"circuit needs rounds", CreateBlockRequest{Name: "x", Type: "circuit", Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}}}},
		{"straight rejects rounds", CreateBlockRequest{Name: "x", Type: "standard", Rounds: 3, Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			rec := apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, tc.body)
			decodeAPIError(t, rec, http.StatusBadRequest)
		})
	}
}

func TestAPIGetBlock_SuccessAndNotFound(t *testing.T) {
	h, _, mockUser, e := setupBlocksHandler(t)
	token, _ := loginUser(t, h, mockUser, "bg@example.com", "BG")

	created := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
		Name:  "Legs",
		Type:  "standard",
		Items: []CreateBlockItemRequest{{ExerciseID: "ex-1", TargetText: "5x5"}},
	}), http.StatusCreated)

	got := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodGet, "/api/v1/blocks/"+created.ID, token, nil), http.StatusOK)
	if got.ID != created.ID || got.Name != "Legs" {
		t.Fatalf("unexpected block: %+v", got)
	}

	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/blocks/missing", token, nil), http.StatusNotFound)
}

func TestAPIUpdateBlock_Success(t *testing.T) {
	h, _, mockUser, e := setupBlocksHandler(t)
	token, _ := loginUser(t, h, mockUser, "bu@example.com", "BU")

	created := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
		Name:  "Full Body",
		Type:  "standard",
		Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}},
	}), http.StatusCreated)

	updated := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPut, "/api/v1/blocks/"+created.ID, token, UpdateBlockRequest{
		Name:            "Full Body v2",
		Description:     "heavier",
		Type:            "emom",
		Rounds:          12,
		IntervalSeconds: 60,
		Items: []CreateBlockItemRequest{
			{ExerciseID: "ex-1", TargetText: "minute 1"},
			{ExerciseID: "ex-2", TargetText: "minute 2"},
		},
	}), http.StatusOK)
	if updated.Name != "Full Body v2" || updated.Type != "emom" || len(updated.Items) != 2 {
		t.Fatalf("unexpected updated block: %+v", updated)
	}

	decodeAPIError(t, apiDo(t, e, http.MethodPut, "/api/v1/blocks/missing", token, UpdateBlockRequest{
		Name:  "x",
		Type:  "standard",
		Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}},
	}), http.StatusNotFound)
}

func TestAPIDeleteBlock_SuccessAndNotFound(t *testing.T) {
	h, _, mockUser, e := setupBlocksHandler(t)
	token, _ := loginUser(t, h, mockUser, "bd@example.com", "BD")

	created := decodeAPI[BlockDTO](t, apiDo(t, e, http.MethodPost, "/api/v1/blocks", token, CreateBlockRequest{
		Name:  "Temp",
		Type:  "standard",
		Items: []CreateBlockItemRequest{{ExerciseID: "ex-1"}},
	}), http.StatusCreated)

	rec := apiDo(t, e, http.MethodDelete, "/api/v1/blocks/"+created.ID, token, nil)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204", rec.Code)
	}
	decodeAPIError(t, apiDo(t, e, http.MethodGet, "/api/v1/blocks/"+created.ID, token, nil), http.StatusNotFound)
	decodeAPIError(t, apiDo(t, e, http.MethodDelete, "/api/v1/blocks/missing", token, nil), http.StatusNotFound)
}

func TestAPIBlocks_RequiresAuth(t *testing.T) {
	_, _, _, e := setupBlocksHandler(t)
	for _, tc := range []struct {
		method, path string
	}{
		{http.MethodGet, "/api/v1/blocks"},
		{http.MethodPost, "/api/v1/blocks"},
		{http.MethodGet, "/api/v1/blocks/x"},
		{http.MethodPut, "/api/v1/blocks/x"},
		{http.MethodDelete, "/api/v1/blocks/x"},
	} {
		rec := apiDo(t, e, tc.method, tc.path, "", nil)
		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("%s %s status = %d, want 401", tc.method, tc.path, rec.Code)
		}
	}
}
