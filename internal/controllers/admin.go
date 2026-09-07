package controllers

import (
	"errors"
	"fmt"
	"strings"

	"hylete/internal/models"
)

// ErrNotFound is returned when an exercise is not found.
var ErrNotFound = errors.New("exercise not found")

// AdminController handles admin-only exercise operations.
type AdminController struct {
	repo models.AdminRepository
}

// NewAdminController creates a new AdminController instance.
func NewAdminController(repo models.AdminRepository) *AdminController {
	return &AdminController{repo: repo}
}

// List returns all exercises ordered by name.
func (ac *AdminController) List() ([]models.Exercise, error) {
	return ac.repo.List()
}

// Get retrieves a single exercise by ID.
func (ac *AdminController) Get(id string) (*models.Exercise, error) {
	exercise, err := ac.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if exercise == nil {
		return nil, ErrNotFound
	}
	return exercise, nil
}

// Create creates a new exercise. Returns the created exercise.
// Returns ErrExerciseNameExists if an exercise with the same name already exists.
func (ac *AdminController) Create(params models.CreateExerciseParams) (*models.Exercise, error) {
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		return nil, errors.New("exercise name cannot be empty")
	}

	if !params.Type.IsValid() {
		params.Type = models.ExerciseTypeOther
	}

	if !models.ValidateURL(params.VideoURL) {
		return nil, errors.New("video URL must be a valid URL")
	}

	existing, err := ac.repo.GetByName(params.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate exercise: %w", err)
	}
	if existing != nil {
		return nil, ErrExerciseNameExists
	}

	id, err := ac.repo.CreateNoTx(params)
	if err != nil {
		return nil, err
	}

	return &models.Exercise{
		ID:             id,
		Name:           params.Name,
		Description:    params.Description,
		VideoURL:       params.VideoURL,
		ImgURL:         params.ImgURL,
		ImgURLOriginal: params.ImgURLOriginal,
		Type:           params.Type,
	}, nil
}

// Update updates an existing exercise's metadata.
// Returns ErrNotFound if the exercise doesn't exist, or ErrExerciseNameExists
// if the new name is already used by a different exercise.
func (ac *AdminController) Update(id string, params models.UpdateExerciseParams) (*models.Exercise, error) {
	params.Name = strings.TrimSpace(params.Name)
	if params.Name == "" {
		return nil, errors.New("exercise name cannot be empty")
	}

	if !params.Type.IsValid() {
		params.Type = models.ExerciseTypeOther
	}

	if !models.ValidateURL(params.VideoURL) {
		return nil, errors.New("video URL must be a valid URL")
	}

	existing, err := ac.repo.GetByName(params.Name)
	if err != nil {
		return nil, fmt.Errorf("failed to check for duplicate exercise: %w", err)
	}
	if existing != nil && existing.ID != id {
		return nil, ErrExerciseNameExists
	}

	exercise, err := ac.repo.Update(id, params)
	if err != nil {
		return nil, err
	}

	return exercise, nil
}
