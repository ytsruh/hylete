package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// AI report types stored in the ai_reports.type column. "Coach"
// is the user-facing name; the codebase always says ai_report so
// it is never confused with a workout or goal.
const (
	// AIReportTypeWeekly is the Monday Coach review. The only
	// type generated today.
	AIReportTypeWeekly = "weekly"
	// AIReportTypeInsight is a future daily store/dismiss card.
	// No generator exists yet; the type is reserved here so
	// callers validate against one list.
	AIReportTypeInsight = "insight"
	// AIReportTypeMonthly is a future month-in-review story.
	// Reserved, no generator yet.
	AIReportTypeMonthly = "monthly"
)

// IsValidAIReportType reports whether t is a known ai_reports type.
func IsValidAIReportType(t string) bool {
	switch t {
	case AIReportTypeWeekly, AIReportTypeInsight, AIReportTypeMonthly:
		return true
	}
	return false
}

// AIReport is one stored Coach report: the validated JSON payload
// produced from the versioned prompt plus generation metadata.
// PayloadJSON is returned verbatim by the API; the server never
// re-interprets it, so prompt evolution cannot break old rows.
type AIReport struct {
	ID            string
	UserID        string
	Type          string
	PeriodStart   time.Time
	PeriodEnd     time.Time
	PromptVersion string
	Model         string
	PayloadJSON   string
	TokensIn      int
	TokensOut     int
	ReadAt        *time.Time
	DismissedAt   *time.Time
	CreatedAt     time.Time
}

// IsRead reports whether the user has marked the report read.
func (r *AIReport) IsRead() bool { return r != nil && r.ReadAt != nil }

// IsDismissed reports whether the user has dismissed the report.
func (r *AIReport) IsDismissed() bool { return r != nil && r.DismissedAt != nil }

// AIReportRepo defines the interface for ai_reports data access.
// The Coach service depends on this so tests can substitute an
// in-memory fake without touching sqlc.
type AIReportRepo interface {
	// Create persists a new report and assigns the generated ID.
	Create(r *AIReport) error
	// Get returns the report for (user, type, periodStart) or
	// nil when none exists. Used for cron idempotency.
	Get(userID, reportType string, periodStart time.Time) (*AIReport, error)
	// GetByID returns a single report scoped to the user, or
	// nil when not found.
	GetByID(id, userID string) (*AIReport, error)
	// Latest returns the newest report of a type for the user,
	// or nil when the user has none yet.
	Latest(userID, reportType string) (*AIReport, error)
	// List returns up to limit reports of a type for the user,
	// newest first.
	List(userID, reportType string, limit int) ([]AIReport, error)
	// MarkRead stamps read_at. Idempotent.
	MarkRead(id, userID string) error
	// MarkDismissed stamps dismissed_at. Idempotent.
	MarkDismissed(id, userID string) error
}

// AIReportRepository persists ai_reports using sqlc-generated queries.
type AIReportRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewAIReportRepository creates a repository backed by sqlc.
func NewAIReportRepository(dbConn *db.DB) *AIReportRepository {
	return &AIReportRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Compile-time check to ensure AIReportRepository implements AIReportRepo.
var _ AIReportRepo = (*AIReportRepository)(nil)

// Create persists a new report, assigning a generated UUID.
func (r *AIReportRepository) Create(report *AIReport) error {
	ctx := context.Background()
	row, err := r.queries.CreateAIReport(ctx, db.CreateAIReportParams{
		ID:            uuid.New().String(),
		UserID:        report.UserID,
		Type:          report.Type,
		PeriodStart:   report.PeriodStart,
		PeriodEnd:     report.PeriodEnd,
		PromptVersion: report.PromptVersion,
		Model:         report.Model,
		PayloadJson:   report.PayloadJSON,
		TokensIn:      int64(report.TokensIn),
		TokensOut:     int64(report.TokensOut),
	})
	if err != nil {
		return fmt.Errorf("failed to create AI report: %w", err)
	}
	*report = *mapAIReportRow(row)
	return nil
}

// Get returns the report for (user, type, periodStart) or nil.
func (r *AIReportRepository) Get(userID, reportType string, periodStart time.Time) (*AIReport, error) {
	ctx := context.Background()
	row, err := r.queries.GetAIReport(ctx, db.GetAIReportParams{
		UserID:      userID,
		Type:        reportType,
		PeriodStart: periodStart,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get AI report: %w", err)
	}
	return mapAIReportRow(row), nil
}

// GetByID returns a single report scoped to the user, or nil.
func (r *AIReportRepository) GetByID(id, userID string) (*AIReport, error) {
	ctx := context.Background()
	row, err := r.queries.GetAIReportByID(ctx, db.GetAIReportByIDParams{
		ID:     id,
		UserID: userID,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get AI report: %w", err)
	}
	return mapAIReportRow(row), nil
}

// Latest returns the newest report of a type for the user, or nil.
func (r *AIReportRepository) Latest(userID, reportType string) (*AIReport, error) {
	ctx := context.Background()
	row, err := r.queries.GetLatestAIReport(ctx, db.GetLatestAIReportParams{
		UserID: userID,
		Type:   reportType,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get latest AI report: %w", err)
	}
	return mapAIReportRow(row), nil
}

// List returns up to limit reports of a type, newest first.
func (r *AIReportRepository) List(userID, reportType string, limit int) ([]AIReport, error) {
	if limit <= 0 || limit > 52 {
		limit = 12
	}
	ctx := context.Background()
	rows, err := r.queries.ListAIReports(ctx, db.ListAIReportsParams{
		UserID: userID,
		Type:   reportType,
		Limit:  int64(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list AI reports: %w", err)
	}
	out := make([]AIReport, len(rows))
	for i, row := range rows {
		out[i] = *mapAIReportRow(row)
	}
	return out, nil
}

// MarkRead stamps read_at. Idempotent.
func (r *AIReportRepository) MarkRead(id, userID string) error {
	ctx := context.Background()
	if err := r.queries.MarkAIReportRead(ctx, db.MarkAIReportReadParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("failed to mark AI report read: %w", err)
	}
	return nil
}

// MarkDismissed stamps dismissed_at. Idempotent.
func (r *AIReportRepository) MarkDismissed(id, userID string) error {
	ctx := context.Background()
	if err := r.queries.MarkAIReportDismissed(ctx, db.MarkAIReportDismissedParams{
		ID:     id,
		UserID: userID,
	}); err != nil {
		return fmt.Errorf("failed to dismiss AI report: %w", err)
	}
	return nil
}

// mapAIReportRow converts a sqlc AiReport row into a domain AIReport.
func mapAIReportRow(row db.AiReport) *AIReport {
	return &AIReport{
		ID:            row.ID,
		UserID:        row.UserID,
		Type:          row.Type,
		PeriodStart:   row.PeriodStart,
		PeriodEnd:     row.PeriodEnd,
		PromptVersion: row.PromptVersion,
		Model:         row.Model,
		PayloadJSON:   row.PayloadJson,
		TokensIn:      int(row.TokensIn),
		TokensOut:     int(row.TokensOut),
		ReadAt:        nullTimeToTimePtr(row.ReadAt),
		DismissedAt:   nullTimeToTimePtr(row.DismissedAt),
		CreatedAt:     row.CreatedAt,
	}
}
