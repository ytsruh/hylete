package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"
	"hylete/internal/utils"

	"github.com/google/uuid"
)

// WeightPhotoAngle identifies which of the three progress-photo slots
// a weight entry photo belongs to. Front is the default: every photo
// uploaded before the three-angle feature is treated as front-facing.
type WeightPhotoAngle string

const (
	// WeightPhotoFront is the front-facing progress photo. The only
	// angle that existed before the three-photo feature, so legacy
	// rows always land here.
	WeightPhotoFront WeightPhotoAngle = "front"
	// WeightPhotoSide is the side-facing progress photo.
	WeightPhotoSide WeightPhotoAngle = "side"
	// WeightPhotoBack is the back-facing progress photo.
	WeightPhotoBack WeightPhotoAngle = "back"
)

// IsValid reports whether the angle is one of the three known slots.
func (a WeightPhotoAngle) IsValid() bool {
	switch a {
	case WeightPhotoFront, WeightPhotoSide, WeightPhotoBack:
		return true
	default:
		return false
	}
}

// ParseWeightPhotoAngle normalises a raw angle string from the API.
// Empty means "front" so older clients that never send an angle keep
// comparing front photos without a contract break on the query param.
func ParseWeightPhotoAngle(raw string) (WeightPhotoAngle, bool) {
	if raw == "" {
		return WeightPhotoFront, true
	}
	angle := WeightPhotoAngle(raw)
	if !angle.IsValid() {
		return "", false
	}
	return angle, true
}

// SharedWeightPhotoAngles returns the angles both entries have photos
// for, in front/side/back preference order. Used to auto-pick the
// comparison angle and to tell the user what else is comparable when
// their chosen angle is missing on one side.
func SharedWeightPhotoAngles(a, b *WeightEntry) []WeightPhotoAngle {
	if a == nil || b == nil {
		return nil
	}
	var out []WeightPhotoAngle
	for _, angle := range []WeightPhotoAngle{WeightPhotoFront, WeightPhotoSide, WeightPhotoBack} {
		if a.HasPhotoForAngle(angle) && b.HasPhotoForAngle(angle) {
			out = append(out, angle)
		}
	}
	return out
}

// WeightEntry represents a single body weight entry for a user. Each
// entry can hold up to three progress photos (front, side, back);
// any slot may be empty.
type WeightEntry struct {
	ID            string
	UserID        string
	Weight        float64
	Notes         string
	FrontPhotoKey string
	SidePhotoKey  string
	BackPhotoKey  string
	CreatedAt     time.Time
}

// FormattedWeight returns the weight labelled with the given unit.
// No conversion happens — the value is rendered as "%.1f <unit>" so
// the number is displayed using whatever unit the user prefers.
func (w *WeightEntry) FormattedWeight(unit string) string {
	return FormatWeight(w.Weight, unit)
}

// FormattedDate returns the date in UK format (DD/MM/YY).
func (w *WeightEntry) FormattedDate() string {
	return w.CreatedAt.Format("02/01/06")
}

// FormattedDateLong returns the date in a long human-readable form
// ("01 Jan 2026") used for image-comparison labels where space is
// less constrained than in the table column.
func (w *WeightEntry) FormattedDateLong() string {
	return w.CreatedAt.Format("02 Jan 2006")
}

// PhotoKeyForAngle returns the R2 storage key for the given angle
// slot, or "" when that slot is empty.
func (w *WeightEntry) PhotoKeyForAngle(angle WeightPhotoAngle) string {
	switch angle {
	case WeightPhotoSide:
		return w.SidePhotoKey
	case WeightPhotoBack:
		return w.BackPhotoKey
	default:
		return w.FrontPhotoKey
	}
}

// HasPhotoForAngle returns true if the entry has a photo in the given
// angle slot.
func (w *WeightEntry) HasPhotoForAngle(angle WeightPhotoAngle) bool {
	return w.PhotoKeyForAngle(angle) != ""
}

// HasPhoto returns true if the entry has at least one photo in any
// angle slot.
func (w *WeightEntry) HasPhoto() bool {
	return w.FrontPhotoKey != "" || w.SidePhotoKey != "" || w.BackPhotoKey != ""
}

// PhotoURLForAngle returns the public URL for the entry's photo in
// the given angle slot, or "" when that slot is empty.
func (w *WeightEntry) PhotoURLForAngle(angle WeightPhotoAngle) string {
	return utils.PublicURLFor(w.PhotoKeyForAngle(angle))
}

// PhotoURL returns the public URL for the entry's front photo, or ""
// when there is none. Front is the default because it is the only
// angle legacy rows can have.
func (w *WeightEntry) PhotoURL() string {
	return w.PhotoURLForAngle(WeightPhotoFront)
}

// PhotoCount returns how many of the three angle slots hold a photo.
func (w *WeightEntry) PhotoCount() int {
	n := 0
	if w.FrontPhotoKey != "" {
		n++
	}
	if w.SidePhotoKey != "" {
		n++
	}
	if w.BackPhotoKey != "" {
		n++
	}
	return n
}

// PhotoKeys returns the non-empty R2 storage keys across all three
// angle slots, in front/side/back order. Used for best-effort R2
// cleanup on delete.
func (w *WeightEntry) PhotoKeys() []string {
	var out []string
	for _, angle := range []WeightPhotoAngle{WeightPhotoFront, WeightPhotoSide, WeightPhotoBack} {
		if key := w.PhotoKeyForAngle(angle); key != "" {
			out = append(out, key)
		}
	}
	return out
}

// WeightRepository provides CRUD operations for weight entries using sqlc-generated queries.
type WeightRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewWeightRepository creates a new weight repository backed by sqlc.
func NewWeightRepository(dbConn *db.DB) *WeightRepository {
	return &WeightRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Create persists a new weight entry.
func (r *WeightRepository) Create(entry *WeightEntry) error {
	ctx := context.Background()
	entryUUID := uuid.New().String()
	_, err := r.queries.CreateWeightEntry(ctx, db.CreateWeightEntryParams{
		ID:            entryUUID,
		UserID:        entry.UserID,
		Weight:        entry.Weight,
		Notes:         stringToNullString(entry.Notes),
		FrontPhotoKey: optionalString(entry.FrontPhotoKey),
		SidePhotoKey:  optionalString(entry.SidePhotoKey),
		BackPhotoKey:  optionalString(entry.BackPhotoKey),
		CreatedAt:     entry.CreatedAt,
	})
	if err != nil {
		return fmt.Errorf("failed to create weight entry: %w", err)
	}
	entry.ID = entryUUID
	return nil
}

// GetByID retrieves a weight entry by ID scoped to a user.
// Returns nil if not found.
func (r *WeightRepository) GetByID(id string, userID string) (*WeightEntry, error) {
	ctx := context.Background()
	row, err := r.queries.GetWeightEntry(ctx, db.GetWeightEntryParams{
		ID:     id,
		UserID: userID,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get weight entry: %w", err)
	}
	return mapWeightEntryRow(row), nil
}

// List returns all weight entries for a user ordered by created_at descending.
func (r *WeightRepository) List(userID string) ([]WeightEntry, error) {
	ctx := context.Background()
	rows, err := r.queries.ListWeightEntries(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list weight entries: %w", err)
	}
	return mapWeightEntryRows(rows), nil
}

// Update updates an existing weight entry including its created_at date.
// Scopes to the given user ID.
func (r *WeightRepository) Update(entry *WeightEntry, userID string) error {
	ctx := context.Background()
	err := r.queries.UpdateWeightEntry(ctx, db.UpdateWeightEntryParams{
		Weight:        entry.Weight,
		Notes:         stringToNullString(entry.Notes),
		FrontPhotoKey: optionalString(entry.FrontPhotoKey),
		SidePhotoKey:  optionalString(entry.SidePhotoKey),
		BackPhotoKey:  optionalString(entry.BackPhotoKey),
		CreatedAt:     entry.CreatedAt,
		ID:            entry.ID,
		UserID:        userID,
	})
	if err != nil {
		return fmt.Errorf("failed to update weight entry: %w", err)
	}
	return nil
}

// Delete removes a weight entry by ID. Scopes to the given user ID.
func (r *WeightRepository) Delete(id string, userID string) error {
	ctx := context.Background()
	err := r.queries.DeleteWeightEntry(ctx, db.DeleteWeightEntryParams{
		ID:     id,
		UserID: userID,
	})
	if err != nil {
		return fmt.Errorf("failed to delete weight entry: %w", err)
	}
	return nil
}

// GetByIDs retrieves a small set of weight entries by ID, all scoped to
// the given user ID. The query is hard-coded to two IDs; callers needing
// a different count should add a new sqlc query. Entries that don't
// exist or don't belong to the user are simply omitted from the result.
func (r *WeightRepository) GetByIDs(idA, idB, userID string) ([]WeightEntry, error) {
	ctx := context.Background()
	rows, err := r.queries.GetWeightEntriesByIDs(ctx, db.GetWeightEntriesByIDsParams{
		ID:     idA,
		ID_2:   idB,
		UserID: userID,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get weight entries by ids: %w", err)
	}
	return mapWeightEntryRows(rows), nil
}

// --- Mapping helpers ---

func mapWeightEntryRow(row db.WeightEntry) *WeightEntry {
	return &WeightEntry{
		ID:            row.ID,
		UserID:        row.UserID,
		Weight:        row.Weight,
		Notes:         nullStringToString(row.Notes),
		FrontPhotoKey: nullStringToString(row.FrontPhotoKey),
		SidePhotoKey:  nullStringToString(row.SidePhotoKey),
		BackPhotoKey:  nullStringToString(row.BackPhotoKey),
		CreatedAt:     row.CreatedAt,
	}
}

func mapWeightEntryRows(rows []db.WeightEntry) []WeightEntry {
	entries := make([]WeightEntry, len(rows))
	for i, row := range rows {
		entries[i] = WeightEntry{
			ID:            row.ID,
			UserID:        row.UserID,
			Weight:        row.Weight,
			Notes:         nullStringToString(row.Notes),
			FrontPhotoKey: nullStringToString(row.FrontPhotoKey),
			SidePhotoKey:  nullStringToString(row.SidePhotoKey),
			BackPhotoKey:  nullStringToString(row.BackPhotoKey),
			CreatedAt:     row.CreatedAt,
		}
	}
	return entries
}

// optionalString returns a sql.NullString that is NULL when s is empty.
// This is used for fields like photo keys that should be NULLABLE in the DB
// when no value is provided (as opposed to the existing stringToNullString
// helper which always returns Valid: true).
func optionalString(s string) sql.NullString {
	return sql.NullString{String: s, Valid: s != ""}
}
