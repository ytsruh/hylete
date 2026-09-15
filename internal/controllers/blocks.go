package controllers

import (
	"errors"
	"strings"

	"hylete/internal/models"
)

// Block validation sentinels. Routes map these to 400s; anything
// else is a 500.
var (
	ErrBlockNotFound         = errors.New("block not found")
	ErrBlockNameRequired     = errors.New("block name is required")
	ErrBlockNameTooLong      = errors.New("block name must be 100 characters or less")
	ErrBlockDescriptionLong  = errors.New("block description must be 1000 characters or less")
	ErrBlockTypeInvalid      = errors.New("block type must be standard, circuit, amrap or emom")
	ErrBlockRoundsRequired   = errors.New("rounds must be at least 1 for circuits and EMOMs")
	ErrBlockRoundsTooMany    = errors.New("rounds must be 100 or less")
	ErrBlockRoundsUnused     = errors.New("rounds only applies to circuits and EMOMs")
	ErrBlockRestInvalid      = errors.New("rest must be between 0 and 3600 seconds")
	ErrBlockRestUnused       = errors.New("rest only applies to circuits")
	ErrBlockTimeCapRequired  = errors.New("time cap is required for AMRAPs")
	ErrBlockTimeCapInvalid   = errors.New("time cap must be between 60 and 86400 seconds")
	ErrBlockTimeCapUnused    = errors.New("time cap only applies to AMRAPs")
	ErrBlockIntervalRequired = errors.New("interval is required for EMOMs")
	ErrBlockIntervalInvalid  = errors.New("interval must be between 15 and 3600 seconds")
	ErrBlockIntervalUnused   = errors.New("interval only applies to EMOMs")
	ErrBlockItemsRequired    = errors.New("a block needs at least 1 exercise")
	ErrBlockItemsTooMany     = errors.New("a block can hold at most 20 exercises")
	ErrBlockExerciseRequired = errors.New("block item exercise is required")
	ErrBlockExerciseNotFound = errors.New("exercise not found")
	ErrBlockTargetTooLong    = errors.New("target must be 500 characters or less")
)

// Block limits.
const (
	BlockNameMaxLen        = 100
	BlockDescriptionMaxLen = 1000
	BlockItemsMin          = 1
	BlockItemsMax          = 20
	BlockTargetMaxLen      = 500
	BlockRoundsMax         = 100
	BlockRestMax           = 3600
	BlockTimeCapMin        = 60
	BlockTimeCapMax        = 86400
	BlockIntervalMin       = 15
	BlockIntervalMax       = 3600
	BlockEmomRoundsMax     = 240
)

// BlockExerciseLookup resolves an exercise ID for validation. The
// models ExerciseRepository implements this; tests substitute a fake.
type BlockExerciseLookup interface {
	GetExerciseByID(id string, userID string) (*models.Exercise, error)
}

// BlocksController orchestrates block CRUD. Depends on the BlockRepo
// interface (models/) and a narrow exercise lookup so route tests
// can substitute fakes without a database.
type BlocksController struct {
	repo     models.BlockRepo
	exercise BlockExerciseLookup
}

// NewBlocksController constructs a BlocksController backed by the
// supplied repositories.
func NewBlocksController(repo models.BlockRepo, exercise BlockExerciseLookup) *BlocksController {
	return &BlocksController{repo: repo, exercise: exercise}
}

// BlockItemInput is one planned exercise in a create/update call.
// TargetText is free-text only in V1.
type BlockItemInput struct {
	ExerciseID string
	TargetText string
}

// CreateBlockInput bundles the editable block fields for create.
type CreateBlockInput struct {
	Name            string
	Description     string
	Type            models.BlockType
	Rounds          int
	RestSeconds     int
	TimeCapSeconds  int
	IntervalSeconds int
	Items           []BlockItemInput
}

// UpdateBlockInput is the same shape as CreateBlockInput. Items are
// fully replaced on update (simplest correct V1 semantic).
type UpdateBlockInput struct {
	Name            string
	Description     string
	Type            models.BlockType
	Rounds          int
	RestSeconds     int
	TimeCapSeconds  int
	IntervalSeconds int
	Items           []BlockItemInput
}

// ListBlocks returns every block summary for the user.
func (bc *BlocksController) ListBlocks(userID string) ([]models.BlockSummary, error) {
	return bc.repo.List(userID)
}

// GetBlock fetches a single block with items. Returns
// ErrBlockNotFound when missing or owned by another user.
func (bc *BlocksController) GetBlock(id, userID string) (*models.Block, error) {
	b, err := bc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if b == nil {
		return nil, ErrBlockNotFound
	}
	return b, nil
}

// CreateBlock validates the input, resolves each item's exercise
// (so unknown IDs fail before anything is stored), and persists.
func (bc *BlocksController) CreateBlock(userID string, in CreateBlockInput) (*models.Block, error) {
	items, err := bc.validateAndResolveItems(userID, in.Items)
	if err != nil {
		return nil, err
	}
	b := &models.Block{
		UserID:          userID,
		Name:            strings.TrimSpace(in.Name),
		Description:     strings.TrimSpace(in.Description),
		Type:            in.Type,
		Rounds:          in.Rounds,
		RestSeconds:     in.RestSeconds,
		TimeCapSeconds:  in.TimeCapSeconds,
		IntervalSeconds: in.IntervalSeconds,
		Items:           items,
	}
	if err := validateBlockFields(b); err != nil {
		return nil, err
	}
	if err := bc.repo.Create(b); err != nil {
		return nil, err
	}
	return b, nil
}

// UpdateBlock validates, resolves exercises, and overwrites the
// block plus its items. Returns ErrBlockNotFound when missing.
func (bc *BlocksController) UpdateBlock(id, userID string, in UpdateBlockInput) (*models.Block, error) {
	existing, err := bc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrBlockNotFound
	}
	items, err := bc.validateAndResolveItems(userID, in.Items)
	if err != nil {
		return nil, err
	}
	existing.Name = strings.TrimSpace(in.Name)
	existing.Description = strings.TrimSpace(in.Description)
	existing.Type = in.Type
	existing.Rounds = in.Rounds
	existing.RestSeconds = in.RestSeconds
	existing.TimeCapSeconds = in.TimeCapSeconds
	existing.IntervalSeconds = in.IntervalSeconds
	existing.Items = items
	if err := validateBlockFields(existing); err != nil {
		return nil, err
	}
	if err := bc.repo.Update(existing, userID); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteBlock hard-deletes a block scoped to the user. Returns
// ErrBlockNotFound when missing.
func (bc *BlocksController) DeleteBlock(id, userID string) error {
	existing, err := bc.repo.GetByID(id, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrBlockNotFound
	}
	return bc.repo.Delete(id, userID)
}

// validateAndResolveItems checks item count, requires an exercise
// ID per item, caps target length, and resolves each exercise so a
// typo'd ID surfaces as ErrBlockExerciseNotFound instead of a
// foreign-key error. Position is implicit (slice order).
func (bc *BlocksController) validateAndResolveItems(userID string, ins []BlockItemInput) ([]models.BlockItem, error) {
	if len(ins) < BlockItemsMin {
		return nil, ErrBlockItemsRequired
	}
	if len(ins) > BlockItemsMax {
		return nil, ErrBlockItemsTooMany
	}
	out := make([]models.BlockItem, 0, len(ins))
	for i, in := range ins {
		if strings.TrimSpace(in.ExerciseID) == "" {
			return nil, ErrBlockExerciseRequired
		}
		if len(in.TargetText) > BlockTargetMaxLen {
			return nil, ErrBlockTargetTooLong
		}
		ex, err := bc.exercise.GetExerciseByID(in.ExerciseID, userID)
		if err != nil {
			return nil, err
		}
		if ex == nil {
			return nil, ErrBlockExerciseNotFound
		}
		out = append(out, models.BlockItem{
			ExerciseID:   ex.ID,
			ExerciseName: ex.Name,
			ExerciseType: ex.Type,
			Position:     i,
			TargetText:   strings.TrimSpace(in.TargetText),
		})
	}
	return out, nil
}

// validateBlockFields checks the name/description lengths and the
// kind-specific config. Standard ignores config; each other kind
// requires its own fields and rejects the rest so a miscopied
// payload cannot silently mean something else.
func validateBlockFields(b *models.Block) error {
	if b.Name == "" {
		return ErrBlockNameRequired
	}
	if len(b.Name) > BlockNameMaxLen {
		return ErrBlockNameTooLong
	}
	if len(b.Description) > BlockDescriptionMaxLen {
		return ErrBlockDescriptionLong
	}
	if !b.Type.IsValid() {
		return ErrBlockTypeInvalid
	}
	switch b.Type {
	case models.BlockTypeStandard:
		if b.Rounds != 0 {
			return ErrBlockRoundsUnused
		}
		if b.RestSeconds != 0 {
			return ErrBlockRestUnused
		}
		if b.TimeCapSeconds != 0 {
			return ErrBlockTimeCapUnused
		}
		if b.IntervalSeconds != 0 {
			return ErrBlockIntervalUnused
		}
	case models.BlockTypeCircuit:
		if b.Rounds < 1 {
			return ErrBlockRoundsRequired
		}
		if b.Rounds > BlockRoundsMax {
			return ErrBlockRoundsTooMany
		}
		if b.RestSeconds < 0 || b.RestSeconds > BlockRestMax {
			return ErrBlockRestInvalid
		}
		if b.TimeCapSeconds != 0 {
			return ErrBlockTimeCapUnused
		}
		if b.IntervalSeconds != 0 {
			return ErrBlockIntervalUnused
		}
	case models.BlockTypeAmrap:
		if b.TimeCapSeconds < BlockTimeCapMin || b.TimeCapSeconds > BlockTimeCapMax {
			if b.TimeCapSeconds == 0 {
				return ErrBlockTimeCapRequired
			}
			return ErrBlockTimeCapInvalid
		}
		if b.Rounds != 0 {
			return ErrBlockRoundsUnused
		}
		if b.RestSeconds != 0 {
			return ErrBlockRestUnused
		}
		if b.IntervalSeconds != 0 {
			return ErrBlockIntervalUnused
		}
	case models.BlockTypeEmom:
		if b.IntervalSeconds < BlockIntervalMin || b.IntervalSeconds > BlockIntervalMax {
			if b.IntervalSeconds == 0 {
				return ErrBlockIntervalRequired
			}
			return ErrBlockIntervalInvalid
		}
		if b.Rounds < 1 {
			return ErrBlockRoundsRequired
		}
		if b.Rounds > BlockEmomRoundsMax {
			return ErrBlockRoundsTooMany
		}
		if b.RestSeconds != 0 {
			return ErrBlockRestUnused
		}
		if b.TimeCapSeconds != 0 {
			return ErrBlockTimeCapUnused
		}
	}
	return nil
}
