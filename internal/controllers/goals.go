package controllers

import (
	"errors"
	"time"

	"hylete/internal/models"
)

// ErrGoalNotFound is returned when the supplied goal ID does not
// resolve to a row owned by the caller. Routes surface this as a 404
// in the browser.
var ErrGoalNotFound = errors.New("goal not found")

// GoalsController orchestrates goal CRUD. The repository dependency
// is the GoalRepo interface from models/, so route tests can
// substitute a fake without touching the real sqlc implementation.
type GoalsController struct {
	repo models.GoalRepo
}

// NewGoalsController constructs a GoalsController backed by the
// supplied repository.
func NewGoalsController(repo models.GoalRepo) *GoalsController {
	return &GoalsController{repo: repo}
}

// ListGoals returns every goal for the user (active first, then
// completed).
func (gc *GoalsController) ListGoals(userID string) ([]models.Goal, error) {
	return gc.repo.List(userID)
}

// CreateGoalInput bundles the editable fields on a goal. The route
// parses the form, validates, and calls CreateGoal with this struct
// so the controller signature stays stable as we add fields.
type CreateGoalInput struct {
	Title       string
	Description string
	StartDate   *time.Time
	TargetDate  *time.Time
	EndDate     *time.Time
}

// CreateGoal persists a new goal with the supplied fields and
// returns the freshly-stored row (including its generated ID).
// Created_at and updated_at are set to time.Now() by the repository.
func (gc *GoalsController) CreateGoal(userID string, in CreateGoalInput) (*models.Goal, error) {
	g := &models.Goal{
		UserID:      userID,
		Title:       in.Title,
		Description: in.Description,
		StartDate:   in.StartDate,
		TargetDate:  in.TargetDate,
		EndDate:     in.EndDate,
	}
	if err := gc.repo.Create(g); err != nil {
		return nil, err
	}
	return g, nil
}

// UpdateGoalInput is the same shape as CreateGoalInput but carries
// the goal ID so Update can find the right row. Kept as its own
// type (rather than reusing CreateGoalInput) so adding a future
// "update-only" field doesn't accidentally appear on create.
type UpdateGoalInput struct {
	Title       string
	Description string
	StartDate   *time.Time
	TargetDate  *time.Time
	EndDate     *time.Time
}

// UpdateGoal overwrites the editable fields of a goal. Returns
// ErrGoalNotFound when the goal does not exist or is owned by
// another user.
func (gc *GoalsController) UpdateGoal(id, userID string, in UpdateGoalInput) (*models.Goal, error) {
	existing, err := gc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrGoalNotFound
	}
	existing.Title = in.Title
	existing.Description = in.Description
	existing.StartDate = in.StartDate
	existing.TargetDate = in.TargetDate
	existing.EndDate = in.EndDate
	if err := gc.repo.Update(existing, userID); err != nil {
		return nil, err
	}
	return existing, nil
}

// GetGoal fetches a single goal. Returns ErrGoalNotFound when the
// goal is not found or not owned by the caller.
func (gc *GoalsController) GetGoal(id, userID string) (*models.Goal, error) {
	g, err := gc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if g == nil {
		return nil, ErrGoalNotFound
	}
	return g, nil
}

// MarkComplete sets completed_at to the supplied time (typically
// time.Now()). No-op if the goal is already complete. Returns the
// updated row, or ErrGoalNotFound if the goal doesn't exist.
func (gc *GoalsController) MarkComplete(id, userID string, completedAt time.Time) (*models.Goal, error) {
	existing, err := gc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrGoalNotFound
	}
	if err := gc.repo.MarkComplete(id, userID, completedAt); err != nil {
		return nil, err
	}
	return gc.repo.GetByID(id, userID)
}

// Reopen clears completed_at. No-op if already active. Returns
// ErrGoalNotFound when the goal is missing.
func (gc *GoalsController) Reopen(id, userID string) (*models.Goal, error) {
	existing, err := gc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrGoalNotFound
	}
	if err := gc.repo.Reopen(id, userID); err != nil {
		return nil, err
	}
	return gc.repo.GetByID(id, userID)
}

// DeleteGoal hard-deletes a goal scoped to the user. Returns
// ErrGoalNotFound when the goal is missing.
func (gc *GoalsController) DeleteGoal(id, userID string) error {
	existing, err := gc.repo.GetByID(id, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrGoalNotFound
	}
	return gc.repo.Delete(id, userID)
}
