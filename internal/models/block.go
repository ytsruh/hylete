package models

import (
	"context"
	"database/sql"
	"fmt"
	"slices"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// BlockType is the kind of planned exercise group. It decides which
// config columns carry meaning (never client guesswork):
//   - standard: a simple ordered list of exercises; config is ignored.
//   - circuit: rotate through the items for Rounds with RestSeconds
//     between rounds.
//   - amrap: as many rounds as possible in TimeCapSeconds.
//   - emom: every IntervalSeconds start the next item, for Rounds minutes.
type BlockType string

const (
	BlockTypeStandard BlockType = "standard"
	BlockTypeCircuit  BlockType = "circuit"
	BlockTypeAmrap    BlockType = "amrap"
	BlockTypeEmom     BlockType = "emom"
)

// IsValid reports whether the block type is a known value.
func (t BlockType) IsValid() bool {
	return slices.Contains([]BlockType{BlockTypeStandard, BlockTypeCircuit, BlockTypeAmrap, BlockTypeEmom}, t)
}

// Block is a user-owned planned group of exercises. The Items slice
// holds planned references to the exercise catalog (see BlockItem) —
// not logged exercise entries. Logging happens later by copying a
// block's items into new ExerciseEntry rows.
type Block struct {
	ID              string
	UserID          string
	Name            string
	Description     string
	Type            BlockType
	Rounds          int
	RestSeconds     int
	TimeCapSeconds  int
	IntervalSeconds int
	Items           []BlockItem
	CreatedAt       time.Time
	UpdatedAt       time.Time
}

// BlockItem is a single planned exercise inside a Block. TargetText is
// free-text only in V1 (e.g. "3x5 @ 100kg", "heavy, RPE 8") — no
// structured prescription. ExerciseName/Type are resolved at read
// time for display; they are never written by the client.
type BlockItem struct {
	ID           string
	BlockID      string
	ExerciseID   string
	ExerciseName string
	ExerciseType ExerciseType
	Position     int
	TargetText   string
	CreatedAt    time.Time
}

// BlockSummary is the list-view shape: the block plus its item count
// so the UI can render "3 exercises · 4 rounds" without N+1 queries.
type BlockSummary struct {
	Block
	ItemCount int
}

// TypeDisplayName returns the user-facing label for the block kind.
func (b *Block) TypeDisplayName() string {
	switch b.Type {
	case BlockTypeCircuit:
		return "Circuit"
	case BlockTypeAmrap:
		return "AMRAP"
	case BlockTypeEmom:
		return "EMOM"
	default:
		return "Standard"
	}
}

// BlockRepository provides CRUD for blocks using sqlc queries. All
// block reads/writes scope to the owning user; item reads are gated
// by a prior scoped GetBlock so a guessed block ID cannot leak
// another user's plan.
type BlockRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewBlockRepository creates a block repository backed by sqlc.
func NewBlockRepository(dbConn *db.DB) *BlockRepository {
	return &BlockRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Create persists a new block with its items. Assigns generated IDs
// back onto the supplied value. Items are stored in slice order
// (position = index). Owns created_at/updated_at via time.Now().
func (r *BlockRepository) Create(b *Block) error {
	ctx := context.Background()
	id := uuid.New().String()
	now := time.Now()
	row, err := r.queries.CreateBlock(ctx, db.CreateBlockParams{
		ID:              id,
		UserID:          b.UserID,
		Name:            b.Name,
		Description:     b.Description,
		BlockType:       string(b.Type),
		Rounds:          int64(b.Rounds),
		RestSeconds:     int64(b.RestSeconds),
		TimeCapSeconds:  int64(b.TimeCapSeconds),
		IntervalSeconds: int64(b.IntervalSeconds),
		CreatedAt:       now,
		UpdatedAt:       now,
	})
	if err != nil {
		return fmt.Errorf("failed to create block: %w", err)
	}
	b.ID = row.ID
	b.CreatedAt = row.CreatedAt
	b.UpdatedAt = row.UpdatedAt
	if len(b.Items) == 0 {
		return nil
	}
	return r.replaceItems(ctx, b.ID, b.Items, &b.Items)
}

// GetByID returns a block with its items (ordered by position) and
// each item's exercise name/type resolved. Returns nil when not
// found. Scoped to the user.
func (r *BlockRepository) GetByID(id, userID string) (*Block, error) {
	ctx := context.Background()
	row, err := r.queries.GetBlock(ctx, db.GetBlockParams{ID: id, UserID: userID})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get block: %w", err)
	}
	b := mapBlockRow(row)
	items, err := r.queries.ListBlockItemsWithExercise(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to list block items: %w", err)
	}
	b.Items = make([]BlockItem, 0, len(items))
	for _, it := range items {
		b.Items = append(b.Items, BlockItem{
			ID:           it.ID,
			BlockID:      it.BlockID,
			ExerciseID:   it.ExerciseID,
			ExerciseName: it.ExerciseName,
			ExerciseType: ExerciseType(it.ExerciseType),
			Position:     int(it.Position),
			TargetText:   it.TargetText,
			CreatedAt:    it.CreatedAt,
		})
	}
	return &b, nil
}

// List returns every block for the user (newest first) with item
// counts. Items themselves are not loaded — use GetByID for detail.
func (r *BlockRepository) List(userID string) ([]BlockSummary, error) {
	ctx := context.Background()
	rows, err := r.queries.ListBlocksWithItemCount(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("failed to list blocks: %w", err)
	}
	out := make([]BlockSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, BlockSummary{
			Block: Block{
				ID:              row.ID,
				UserID:          row.UserID,
				Name:            row.Name,
				Description:     row.Description,
				Type:            BlockType(row.BlockType),
				Rounds:          int(row.Rounds),
				RestSeconds:     int(row.RestSeconds),
				TimeCapSeconds:  int(row.TimeCapSeconds),
				IntervalSeconds: int(row.IntervalSeconds),
				CreatedAt:       row.CreatedAt,
				UpdatedAt:       row.UpdatedAt,
			},
			ItemCount: int(row.ItemCount),
		})
	}
	return out, nil
}

// Update overwrites a block's fields and fully replaces its items
// (delete-all + re-insert in one transaction). Assigns fresh item
// IDs back onto b.Items. Scoped to the user.
func (r *BlockRepository) Update(b *Block, userID string) error {
	ctx := context.Background()
	if err := r.queries.UpdateBlock(ctx, db.UpdateBlockParams{
		Name:            b.Name,
		Description:     b.Description,
		BlockType:       string(b.Type),
		Rounds:          int64(b.Rounds),
		RestSeconds:     int64(b.RestSeconds),
		TimeCapSeconds:  int64(b.TimeCapSeconds),
		IntervalSeconds: int64(b.IntervalSeconds),
		ID:              b.ID,
		UserID:          userID,
	}); err != nil {
		return fmt.Errorf("failed to update block: %w", err)
	}
	return r.replaceItems(ctx, b.ID, b.Items, &b.Items)
}

// Delete removes a block and its items (explicit item delete first
// so databases ignoring ON DELETE CASCADE stay correct). Scoped to
// the user.
func (r *BlockRepository) Delete(id, userID string) error {
	ctx := context.Background()
	if err := r.queries.DeleteBlockItems(ctx, id); err != nil {
		return fmt.Errorf("failed to delete block items: %w", err)
	}
	return r.queries.DeleteBlock(ctx, db.DeleteBlockParams{ID: id, UserID: userID})
}

// replaceItems deletes every item on the block then inserts the
// supplied items in slice order. Item IDs are regenerated.
func (r *BlockRepository) replaceItems(ctx context.Context, blockID string, items []BlockItem, out *[]BlockItem) error {
	return r.db.Transaction(func(tx *sql.Tx) error {
		q := r.queries.WithTx(tx)
		if err := q.DeleteBlockItems(ctx, blockID); err != nil {
			return err
		}
		now := time.Now()
		fresh := make([]BlockItem, 0, len(items))
		for i, it := range items {
			row, err := q.CreateBlockItem(ctx, db.CreateBlockItemParams{
				ID:         uuid.New().String(),
				BlockID:    blockID,
				ExerciseID: it.ExerciseID,
				Position:   int64(i),
				TargetText: it.TargetText,
				CreatedAt:  now,
			})
			if err != nil {
				return err
			}
			fresh = append(fresh, BlockItem{
				ID:           row.ID,
				BlockID:      row.BlockID,
				ExerciseID:   row.ExerciseID,
				ExerciseName: it.ExerciseName,
				ExerciseType: it.ExerciseType,
				Position:     int(row.Position),
				TargetText:   row.TargetText,
				CreatedAt:    row.CreatedAt,
			})
		}
		*out = fresh
		return nil
	})
}

// mapBlockRow converts a sqlc Block row into a domain Block without
// items (callers load items separately).
func mapBlockRow(row db.Block) Block {
	return Block{
		ID:              row.ID,
		UserID:          row.UserID,
		Name:            row.Name,
		Description:     row.Description,
		Type:            BlockType(row.BlockType),
		Rounds:          int(row.Rounds),
		RestSeconds:     int(row.RestSeconds),
		TimeCapSeconds:  int(row.TimeCapSeconds),
		IntervalSeconds: int(row.IntervalSeconds),
		CreatedAt:       row.CreatedAt,
		UpdatedAt:       row.UpdatedAt,
	}
}
