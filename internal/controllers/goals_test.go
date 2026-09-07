package controllers

import (
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"hylete/internal/models"
)

// mockGoalRepository is an in-memory implementation of
// models.GoalRepo used by the controller tests. It tracks per-method
// error injection points and exposes the internal state for tests
// that want to assert on what the controller wrote.
type mockGoalRepository struct {
	mu sync.Mutex

	goals map[string]*models.Goal

	// Inject errors per method. nil means "behave normally".
	errCreate      error
	errGetByID     error
	errList        error
	errUpdate      error
	errMarkComplete error
	errReopen      error
	errDelete      error
}

func newMockGoalRepository() *mockGoalRepository {
	return &mockGoalRepository{
		goals: map[string]*models.Goal{},
	}
}

func (m *mockGoalRepository) Create(g *models.Goal) error {
	if m.errCreate != nil {
		return m.errCreate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g.ID = "goal-" + fmt.Sprint(len(m.goals)+1)
	cp := *g
	m.goals[g.ID] = &cp
	*g = cp
	return nil
}

func (m *mockGoalRepository) GetByID(id, userID string) (*models.Goal, error) {
	if m.errGetByID != nil {
		return nil, m.errGetByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil, nil
	}
	cp := *g
	return &cp, nil
}

func (m *mockGoalRepository) List(userID string) ([]models.Goal, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	var active, completed []models.Goal
	for _, g := range m.goals {
		if g.UserID != userID {
			continue
		}
		cp := *g
		if g.CompletedAt != nil {
			completed = append(completed, cp)
		} else {
			active = append(active, cp)
		}
	}
	out := append([]models.Goal{}, active...)
	out = append(out, completed...)
	return out, nil
}

func (m *mockGoalRepository) Update(g *models.Goal, userID string) error {
	if m.errUpdate != nil {
		return m.errUpdate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	existing, ok := m.goals[g.ID]
	if !ok || existing.UserID != userID {
		return nil
	}
	existing.Title = g.Title
	existing.Description = g.Description
	existing.StartDate = g.StartDate
	existing.TargetDate = g.TargetDate
	existing.EndDate = g.EndDate
	return nil
}

func (m *mockGoalRepository) MarkComplete(id, userID string, completedAt time.Time) error {
	if m.errMarkComplete != nil {
		return m.errMarkComplete
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil
	}
	if g.CompletedAt != nil {
		return nil
	}
	cp := completedAt
	g.CompletedAt = &cp
	return nil
}

func (m *mockGoalRepository) Reopen(id, userID string) error {
	if m.errReopen != nil {
		return m.errReopen
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil
	}
	g.CompletedAt = nil
	return nil
}

func (m *mockGoalRepository) Delete(id, userID string) error {
	if m.errDelete != nil {
		return m.errDelete
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.goals[id]
	if !ok || g.UserID != userID {
		return nil
	}
	delete(m.goals, id)
	return nil
}

// --- Tests ---

func TestGoalsController_CreateGoal_Success(t *testing.T) {
	repo := newMockGoalRepository()
	ctrl := NewGoalsController(repo)

	in := CreateGoalInput{
		Title:       "Run a 5k",
		Description: "Train for the summer",
	}
	g, err := ctrl.CreateGoal("user-1", in)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if g.ID == "" {
		t.Error("expected generated ID, got empty string")
	}
	if g.Title != "Run a 5k" {
		t.Errorf("expected title 'Run a 5k', got %q", g.Title)
	}
}

func TestGoalsController_CreateGoal_RepoError(t *testing.T) {
	repo := newMockGoalRepository()
	repo.errCreate = errors.New("db error")
	ctrl := NewGoalsController(repo)

	_, err := ctrl.CreateGoal("user-1", CreateGoalInput{Title: "x"})
	if err == nil {
		t.Fatal("expected error from repo, got nil")
	}
}

func TestGoalsController_GetGoal_NotFound(t *testing.T) {
	ctrl := NewGoalsController(newMockGoalRepository())
	_, err := ctrl.GetGoal("missing", "user-1")
	if !errors.Is(err, ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

func TestGoalsController_UpdateGoal_NotFound(t *testing.T) {
	ctrl := NewGoalsController(newMockGoalRepository())
	_, err := ctrl.UpdateGoal("missing", "user-1", UpdateGoalInput{Title: "x"})
	if !errors.Is(err, ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

func TestGoalsController_MarkComplete_Success(t *testing.T) {
	repo := newMockGoalRepository()
	ctrl := NewGoalsController(repo)
	created, err := ctrl.CreateGoal("user-1", CreateGoalInput{Title: "x"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}

	updated, err := ctrl.MarkComplete(created.ID, "user-1", time.Now())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !updated.IsComplete() {
		t.Error("expected updated goal to be complete")
	}
}

func TestGoalsController_MarkComplete_NotFound(t *testing.T) {
	ctrl := NewGoalsController(newMockGoalRepository())
	_, err := ctrl.MarkComplete("missing", "user-1", time.Now())
	if !errors.Is(err, ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

func TestGoalsController_Reopen_Success(t *testing.T) {
	repo := newMockGoalRepository()
	ctrl := NewGoalsController(repo)
	created, err := ctrl.CreateGoal("user-1", CreateGoalInput{Title: "x"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if _, err := ctrl.MarkComplete(created.ID, "user-1", time.Now()); err != nil {
		t.Fatalf("setup mark: %v", err)
	}

	reopened, err := ctrl.Reopen(created.ID, "user-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reopened.IsComplete() {
		t.Error("expected reopened goal to be active")
	}
}

func TestGoalsController_Reopen_NotFound(t *testing.T) {
	ctrl := NewGoalsController(newMockGoalRepository())
	_, err := ctrl.Reopen("missing", "user-1")
	if !errors.Is(err, ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

func TestGoalsController_DeleteGoal_NotFound(t *testing.T) {
	ctrl := NewGoalsController(newMockGoalRepository())
	err := ctrl.DeleteGoal("missing", "user-1")
	if !errors.Is(err, ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound, got %v", err)
	}
}

func TestGoalsController_DeleteGoal_Success(t *testing.T) {
	repo := newMockGoalRepository()
	ctrl := NewGoalsController(repo)
	created, err := ctrl.CreateGoal("user-1", CreateGoalInput{Title: "x"})
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	if err := ctrl.DeleteGoal(created.ID, "user-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// A follow-up get should now report not found.
	_, err = ctrl.GetGoal(created.ID, "user-1")
	if !errors.Is(err, ErrGoalNotFound) {
		t.Errorf("expected ErrGoalNotFound after delete, got %v", err)
	}
}
