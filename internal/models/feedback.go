package models

import (
	"context"
	"database/sql"
	"time"

	"github.com/google/uuid"
	"hylete/internal/db"
)

type Feedback struct {
	ID        string
	UserID    string
	UserName  string
	Title     string
	Message   string
	IsClosed  bool
	CreatedAt time.Time
	UpdatedAt time.Time
}

type FeedbackRepository interface {
	Create(feedback *Feedback) error
	GetAll(filter string) ([]*Feedback, error)
	GetByID(id string) (*Feedback, error)
	UpdateStatus(id string, isClosed bool) error
}

type FeedbackRepo struct {
	db *db.Queries
}

func NewFeedbackRepository(database *db.DB) *FeedbackRepo {
	return &FeedbackRepo{db: db.New(database.Conn())}
}

var _ FeedbackRepository = (*FeedbackRepo)(nil)

func (r *FeedbackRepo) Create(feedback *Feedback) error {
	ctx := context.Background()
	feedback.ID = uuid.New().String()
	f, err := r.db.CreateFeedback(ctx, db.CreateFeedbackParams{
		ID:      feedback.ID,
		UserID:  feedback.UserID,
		Title:   feedback.Title,
		Message: feedback.Message,
	})
	if err != nil {
		return err
	}
	feedback.ID = f.ID
	feedback.IsClosed = f.IsClosed == 1
	if f.CreatedAt.Valid {
		feedback.CreatedAt = f.CreatedAt.Time
	}
	if f.UpdatedAt.Valid {
		feedback.UpdatedAt = f.UpdatedAt.Time
	}
	return nil
}

func (r *FeedbackRepo) GetAll(filter string) ([]*Feedback, error) {
	ctx := context.Background()

	switch filter {
	case "open":
		rows, err := r.db.GetAllOpen(ctx)
		if err != nil {
			return nil, err
		}
		return mapFeedbackRows(rows), nil
	case "closed":
		rows, err := r.db.GetAllClosed(ctx)
		if err != nil {
			return nil, err
		}
		return mapFeedbackClosedRows(rows), nil
	default:
		rows, err := r.db.GetAll(ctx)
		if err != nil {
			return nil, err
		}
		return mapFeedbackAllRows(rows), nil
	}
}

func mapFeedbackAllRows(rows []db.GetAllRow) []*Feedback {
	result := make([]*Feedback, len(rows))
	for i, f := range rows {
		result[i] = &Feedback{
			ID:       f.ID,
			UserID:   f.UserID,
			Title:    f.Title,
			Message:  f.Message,
			IsClosed: f.IsClosed == 1,
		}
		if f.CreatedAt.Valid {
			result[i].CreatedAt = f.CreatedAt.Time
		}
		if f.UpdatedAt.Valid {
			result[i].UpdatedAt = f.UpdatedAt.Time
		}
		if f.UserName.Valid {
			result[i].UserName = f.UserName.String
		}
	}
	return result
}

func mapFeedbackClosedRows(rows []db.GetAllClosedRow) []*Feedback {
	result := make([]*Feedback, len(rows))
	for i, f := range rows {
		result[i] = &Feedback{
			ID:       f.ID,
			UserID:   f.UserID,
			Title:    f.Title,
			Message:  f.Message,
			IsClosed: f.IsClosed == 1,
		}
		if f.CreatedAt.Valid {
			result[i].CreatedAt = f.CreatedAt.Time
		}
		if f.UpdatedAt.Valid {
			result[i].UpdatedAt = f.UpdatedAt.Time
		}
		if f.UserName.Valid {
			result[i].UserName = f.UserName.String
		}
	}
	return result
}

func mapFeedbackRows(rows []db.GetAllOpenRow) []*Feedback {
	result := make([]*Feedback, len(rows))
	for i, f := range rows {
		result[i] = &Feedback{
			ID:       f.ID,
			UserID:   f.UserID,
			Title:    f.Title,
			Message:  f.Message,
			IsClosed: f.IsClosed == 1,
		}
		if f.CreatedAt.Valid {
			result[i].CreatedAt = f.CreatedAt.Time
		}
		if f.UpdatedAt.Valid {
			result[i].UpdatedAt = f.UpdatedAt.Time
		}
		if f.UserName.Valid {
			result[i].UserName = f.UserName.String
		}
	}
	return result
}

func (r *FeedbackRepo) GetByID(id string) (*Feedback, error) {
	ctx := context.Background()
	f, err := r.db.GetFeedbackByID(ctx, id)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := &Feedback{
		ID:       f.ID,
		UserID:   f.UserID,
		Title:    f.Title,
		Message:  f.Message,
		IsClosed: f.IsClosed == 1,
	}
	if f.CreatedAt.Valid {
		result.CreatedAt = f.CreatedAt.Time
	}
	if f.UpdatedAt.Valid {
		result.UpdatedAt = f.UpdatedAt.Time
	}
	if f.UserName.Valid {
		result.UserName = f.UserName.String
	}
	return result, nil
}

func (r *FeedbackRepo) UpdateStatus(id string, isClosed bool) error {
	ctx := context.Background()
	var closedVal int64 = 0
	if isClosed {
		closedVal = 1
	}
	_, err := r.db.UpdateStatus(ctx, db.UpdateStatusParams{
		IsClosed: closedVal,
		ID:       id,
	})
	return err
}
