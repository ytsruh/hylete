package controllers

import (
	"errors"
	"sync"
	"testing"

	"hylete/internal/models"
)

type mockAdminRepository struct {
	mu        sync.Mutex
	exercises []models.Exercise

	errGetByID    error
	errGetByName  error
	errUpdate     error
	errCreateNoTx error
	errList       error
}

func newMockAdminRepository() *mockAdminRepository {
	return &mockAdminRepository{}
}

func (m *mockAdminRepository) GetByID(id string) (*models.Exercise, error) {
	if m.errGetByID != nil {
		return nil, m.errGetByID
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.ID == id {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockAdminRepository) GetByName(name string) (*models.Exercise, error) {
	if m.errGetByName != nil {
		return nil, m.errGetByName
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == name {
			cp := e
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockAdminRepository) Update(id string, params models.UpdateExerciseParams) (*models.Exercise, error) {
	if m.errUpdate != nil {
		return nil, m.errUpdate
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, e := range m.exercises {
		if e.ID == id {
			m.exercises[i].Name = params.Name
			m.exercises[i].Description = params.Description
			m.exercises[i].VideoURL = params.VideoURL
			m.exercises[i].ImgURL = params.ImgURL
			m.exercises[i].ImgURLOriginal = params.ImgURLOriginal
			m.exercises[i].Type = params.Type
			cp := m.exercises[i]
			return &cp, nil
		}
	}
	return nil, nil
}

func (m *mockAdminRepository) CreateNoTx(params models.CreateExerciseParams) (string, error) {
	if m.errCreateNoTx != nil {
		return "", m.errCreateNoTx
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, e := range m.exercises {
		if e.Name == params.Name {
			return e.ID, nil
		}
	}
	id := "mock-id-" + params.Name
	m.exercises = append(m.exercises, models.Exercise{ID: id, Name: params.Name, Description: params.Description, VideoURL: params.VideoURL, ImgURL: params.ImgURL, ImgURLOriginal: params.ImgURLOriginal, Type: params.Type})
	return id, nil
}

func (m *mockAdminRepository) List() ([]models.Exercise, error) {
	if m.errList != nil {
		return nil, m.errList
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]models.Exercise, len(m.exercises))
	copy(result, m.exercises)
	return result, nil
}

func TestAdminController_List(t *testing.T) {
	mock := newMockAdminRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
		{ID: "ex-2", Name: "Bench Press"},
	}

	ctrl := NewAdminController(mock)
	exercises, err := ctrl.List()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(exercises) != 2 {
		t.Fatalf("expected 2 exercises, got %d", len(exercises))
	}
}

func TestAdminController_List_Error(t *testing.T) {
	mock := newMockAdminRepository()
	mock.errList = errors.New("database error")

	ctrl := NewAdminController(mock)
	_, err := ctrl.List()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAdminController_Get(t *testing.T) {
	mock := newMockAdminRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
		{ID: "ex-2", Name: "Bench Press"},
	}

	ctrl := NewAdminController(mock)

	t.Run("found", func(t *testing.T) {
		ex, err := ctrl.Get("ex-1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.Name != "Squat" {
			t.Errorf("expected 'Squat', got %q", ex.Name)
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := ctrl.Get("non-existent")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("expected ErrNotFound, got %v", err)
		}
	})

	t.Run("error", func(t *testing.T) {
		mock.errGetByID = errors.New("database error")
		_, err := ctrl.Get("ex-1")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		mock.errGetByID = nil
	})
}

func TestAdminController_Create(t *testing.T) {
	mock := newMockAdminRepository()
	ctrl := NewAdminController(mock)

	t.Run("success", func(t *testing.T) {
		ex, err := ctrl.Create(models.CreateExerciseParams{Name: "Deadlift"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.Name != "Deadlift" {
			t.Errorf("expected 'Deadlift', got %q", ex.Name)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := ctrl.Create(models.CreateExerciseParams{Name: "   "})
		if err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
	})

	t.Run("repository error", func(t *testing.T) {
		mock.errCreateNoTx = errors.New("database error")
		_, err := ctrl.Create(models.CreateExerciseParams{Name: "Squat"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		mock.errCreateNoTx = nil
	})
}

func TestAdminController_Create_DuplicateName(t *testing.T) {
	mock := newMockAdminRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Bench Press"},
	}
	ctrl := NewAdminController(mock)

	_, err := ctrl.Create(models.CreateExerciseParams{Name: "Bench Press"})
	if !errors.Is(err, ErrExerciseNameExists) {
		t.Fatalf("expected ErrExerciseNameExists, got %v", err)
	}

	_, err = ctrl.Create(models.CreateExerciseParams{Name: "  Bench Press  "})
	if !errors.Is(err, ErrExerciseNameExists) {
		t.Fatalf("expected ErrExerciseNameExists for whitespace-padded duplicate, got %v", err)
	}
}

func TestAdminController_Create_GetByNameError(t *testing.T) {
	mock := newMockAdminRepository()
	mock.errGetByName = errors.New("database error")
	ctrl := NewAdminController(mock)

	_, err := ctrl.Create(models.CreateExerciseParams{Name: "New Exercise"})
	if err == nil {
		t.Fatal("expected error when GetByName fails, got nil")
	}
	if errors.Is(err, ErrExerciseNameExists) {
		t.Errorf("GetByName error should not be reported as ErrExerciseNameExists, got %v", err)
	}
}

func TestAdminController_Update(t *testing.T) {
	mock := newMockAdminRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
	}

	ctrl := NewAdminController(mock)

	t.Run("success", func(t *testing.T) {
		ex, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "Front Squat"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.Name != "Front Squat" {
			t.Errorf("expected 'Front Squat', got %q", ex.Name)
		}
	})

	t.Run("empty name", func(t *testing.T) {
		_, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "   "})
		if err == nil {
			t.Fatal("expected error for empty name, got nil")
		}
	})

	t.Run("repository error", func(t *testing.T) {
		mock.errUpdate = errors.New("database error")
		_, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "Squat"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		mock.errUpdate = nil
	})
}

func TestAdminController_Update_DuplicateName(t *testing.T) {
	mock := newMockAdminRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
		{ID: "ex-2", Name: "Bench Press"},
	}
	ctrl := NewAdminController(mock)

	t.Run("renaming to another exercise's name fails", func(t *testing.T) {
		_, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "Bench Press"})
		if !errors.Is(err, ErrExerciseNameExists) {
			t.Fatalf("expected ErrExerciseNameExists, got %v", err)
		}
	})

	t.Run("renaming to its own current name succeeds", func(t *testing.T) {
		_, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "Squat", Type: models.ExerciseTypeStrength})
		if err != nil {
			t.Fatalf("unexpected error renaming to own name: %v", err)
		}
	})

	t.Run("renaming to its own current name with whitespace succeeds", func(t *testing.T) {
		_, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "  Squat  "})
		if err != nil {
			t.Fatalf("unexpected error renaming to own name with whitespace: %v", err)
		}
	})
}

func TestAdminController_Create_Validation(t *testing.T) {
	mock := newMockAdminRepository()
	ctrl := NewAdminController(mock)

	t.Run("invalid type defaults to other", func(t *testing.T) {
		ex, err := ctrl.Create(models.CreateExerciseParams{Name: "Test", Type: "invalid"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.Type != models.ExerciseTypeOther {
			t.Errorf("expected type 'other', got %q", ex.Type)
		}
	})

	t.Run("invalid video URL returns error", func(t *testing.T) {
		_, err := ctrl.Create(models.CreateExerciseParams{
			Name:     "Test",
			VideoURL: "not-a-valid-url",
		})
		if err == nil {
			t.Fatal("expected error for invalid video URL, got nil")
		}
		if err.Error() != "video URL must be a valid URL" {
			t.Errorf("unexpected error message: %v", err)
		}
	})

	t.Run("all fields are set on returned exercise", func(t *testing.T) {
		params := models.CreateExerciseParams{
			Name:           "Full Exercise",
			Description:    "A description",
			VideoURL:       "https://example.com/video.mp4",
			ImgURL:         "exercises/abc.jpg",
			ImgURLOriginal: "exercises/abc_original.jpg",
			Type:           models.ExerciseTypeStrength,
		}
		ex, err := ctrl.Create(params)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.Name != params.Name {
			t.Errorf("Name = %q, want %q", ex.Name, params.Name)
		}
		if ex.Description != params.Description {
			t.Errorf("Description = %q, want %q", ex.Description, params.Description)
		}
		if ex.VideoURL != params.VideoURL {
			t.Errorf("VideoURL = %q, want %q", ex.VideoURL, params.VideoURL)
		}
		if ex.ImgURL != params.ImgURL {
			t.Errorf("ImgURL = %q, want %q", ex.ImgURL, params.ImgURL)
		}
		if ex.ImgURLOriginal != params.ImgURLOriginal {
			t.Errorf("ImgURLOriginal = %q, want %q", ex.ImgURLOriginal, params.ImgURLOriginal)
		}
		if ex.Type != params.Type {
			t.Errorf("Type = %q, want %q", ex.Type, params.Type)
		}
	})
}

func TestAdminController_Update_Validation(t *testing.T) {
	mock := newMockAdminRepository()
	mock.exercises = []models.Exercise{
		{ID: "ex-1", Name: "Squat"},
	}
	ctrl := NewAdminController(mock)

	t.Run("invalid type defaults to other", func(t *testing.T) {
		ex, err := ctrl.Update("ex-1", models.UpdateExerciseParams{Name: "Squat", Type: "invalid"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ex.Type != models.ExerciseTypeOther {
			t.Errorf("expected type 'other', got %q", ex.Type)
		}
	})

	t.Run("invalid video URL returns error", func(t *testing.T) {
		_, err := ctrl.Update("ex-1", models.UpdateExerciseParams{
			Name:     "Squat",
			VideoURL: "missing-scheme.com",
		})
		if err == nil {
			t.Fatal("expected error for invalid video URL, got nil")
		}
	})
}
