package controllers

import (
	"errors"
	"strings"

	"hylete/internal/models"
)

const (
	MinTitleLength   = 5
	MaxTitleLength   = 100
	MinMessageLength = 10
	MaxMessageLength = 1000
)

var (
	ErrTitleTooShort   = errors.New("title must be at least 5 characters")
	ErrTitleTooLong    = errors.New("title must be at most 100 characters")
	ErrMessageTooShort = errors.New("message must be at least 10 characters")
	ErrMessageTooLong  = errors.New("message must be at most 1000 characters")
)

type FeedbackController struct {
	repo models.FeedbackRepoInterface
}

func NewFeedbackController(repo models.FeedbackRepoInterface) *FeedbackController {
	return &FeedbackController{repo: repo}
}

func (fc *FeedbackController) Submit(title, message string, userID string) error {
	title = strings.TrimSpace(title)
	message = strings.TrimSpace(message)

	if len(title) < MinTitleLength {
		return ErrTitleTooShort
	}
	if len(title) > MaxTitleLength {
		return ErrTitleTooLong
	}
	if len(message) < MinMessageLength {
		return ErrMessageTooShort
	}
	if len(message) > MaxMessageLength {
		return ErrMessageTooLong
	}

	feedback := &models.Feedback{
		UserID:  userID,
		Title:   title,
		Message: message,
	}

	return fc.repo.Create(feedback)
}

func (fc *FeedbackController) AdminList(filter string) ([]*models.Feedback, error) {
	return fc.repo.GetAll(filter)
}

func (fc *FeedbackController) AdminDetail(id string) (*models.Feedback, error) {
	feedback, err := fc.repo.GetByID(id)
	if err != nil {
		return nil, err
	}
	if feedback == nil {
		return nil, ErrNotFound
	}
	return feedback, nil
}

func (fc *FeedbackController) Close(id string) error {
	feedback, err := fc.repo.GetByID(id)
	if err != nil {
		return err
	}
	if feedback == nil {
		return ErrNotFound
	}

	return fc.repo.UpdateStatus(id, !feedback.IsClosed)
}
