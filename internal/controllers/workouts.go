package controllers

import (
	"errors"
	"strings"
	"time"

	"hylete/internal/models"
)

// Workout validation sentinels. Routes map these to 400s; anything
// else is a 500.
var (
	ErrWorkoutNotFound           = errors.New("workout not found")
	ErrWorkoutNameRequired       = errors.New("workout name is required")
	ErrWorkoutNameTooLong        = errors.New("workout name must be 100 characters or less")
	ErrWorkoutDescriptionLong    = errors.New("workout description must be 1000 characters or less")
	ErrWorkoutDateRequired       = errors.New("scheduled date is required")
	ErrWorkoutDateInvalid        = errors.New("scheduled date must be YYYY-MM-DD")
	ErrWorkoutStatusInvalid      = errors.New("workout status must be planned, in_progress, completed or skipped")
	ErrWorkoutBlocksRequired     = errors.New("a workout needs at least 1 block")
	ErrWorkoutBlocksTooMany      = errors.New("a workout can hold at most 20 blocks")
	ErrWorkoutBlockRequired      = errors.New("workout block is required")
	ErrWorkoutBlockNotFound      = errors.New("block not found")
	ErrWorkoutBlockStatusInvalid = errors.New("block status must be pending, done or skipped")
	ErrWorkoutRangeInvalid       = errors.New("from date must not be after to date")
	ErrWorkoutBulkRequired       = errors.New("at least 1 date is required")
	ErrWorkoutBulkTooMany        = errors.New("at most 50 workouts can be created per batch")
	ErrWorkoutBulkDuplicateDate  = errors.New("duplicate dates are not allowed in a batch")
)

// Workout limits.
const (
	WorkoutNameMaxLen        = 100
	WorkoutDescriptionMaxLen = 1000
	WorkoutBlocksMin         = 1
	WorkoutBlocksMax         = 20
	// WorkoutBulkMax caps recurring-duplicate batches. Fifty covers
	// a year of weekly repeats or twelve weeks at four sessions a
	// week; larger plans split into multiple batches.
	WorkoutBulkMax = 50
)

// WorkoutBlockLookup resolves a block ID for validation. The models
// BlockRepository implements GetByID; tests substitute a fake.
type WorkoutBlockLookup interface {
	GetByID(id string, userID string) (*models.Block, error)
}

// WorkoutsController orchestrates workout CRUD. Depends on the
// WorkoutRepo interface (models/) and a narrow block lookup so route
// tests can substitute fakes without a database.
type WorkoutsController struct {
	repo   models.WorkoutRepo
	blocks WorkoutBlockLookup
}

// NewWorkoutsController constructs a WorkoutsController backed by the
// supplied repositories.
func NewWorkoutsController(repo models.WorkoutRepo, blocks WorkoutBlockLookup) *WorkoutsController {
	return &WorkoutsController{repo: repo, blocks: blocks}
}

// WorkoutBlockInput is one planned block in a create/update call.
// Position is implicit (slice order); status always starts pending
// on create.
type WorkoutBlockInput struct {
	BlockID string
}

// CreateWorkoutInput bundles the editable workout fields for create.
type CreateWorkoutInput struct {
	Name          string
	Description   string
	ScheduledDate string
	Status        models.WorkoutStatus
	Blocks        []WorkoutBlockInput
}

// UpdateWorkoutInput is the same shape as CreateWorkoutInput. Blocks
// are fully replaced on update (simplest correct V1 semantic) with
// statuses reset to pending.
type UpdateWorkoutInput struct {
	Name          string
	Description   string
	ScheduledDate string
	Status        models.WorkoutStatus
	Blocks        []WorkoutBlockInput
}

// ListWorkouts returns every workout summary for the user, or the
// inclusive date range when from/to are both set. Empty from/to
// means no range filter.
func (wc *WorkoutsController) ListWorkouts(userID, from, to string) ([]models.WorkoutSummary, error) {
	if from == "" && to == "" {
		return wc.repo.List(userID)
	}
	if from == "" || to == "" {
		return nil, ErrWorkoutDateInvalid
	}
	if _, err := time.Parse("2006-01-02", from); err != nil {
		return nil, ErrWorkoutDateInvalid
	}
	if _, err := time.Parse("2006-01-02", to); err != nil {
		return nil, ErrWorkoutDateInvalid
	}
	if from > to {
		return nil, ErrWorkoutRangeInvalid
	}
	return wc.repo.ListRange(userID, from, to)
}

// GetWorkout fetches a single workout with blocks. Returns
// ErrWorkoutNotFound when missing or owned by another user.
func (wc *WorkoutsController) GetWorkout(id, userID string) (*models.Workout, error) {
	w, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if w == nil {
		return nil, ErrWorkoutNotFound
	}
	return w, nil
}

// CreateWorkout validates the input, resolves each block (so unknown
// IDs fail before anything is stored), and persists.
func (wc *WorkoutsController) CreateWorkout(userID string, in CreateWorkoutInput) (*models.Workout, error) {
	blocks, err := wc.validateAndResolveBlocks(userID, in.Blocks)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = models.WorkoutStatusPlanned
	}
	w := &models.Workout{
		UserID:        userID,
		Name:          strings.TrimSpace(in.Name),
		Description:   strings.TrimSpace(in.Description),
		ScheduledDate: strings.TrimSpace(in.ScheduledDate),
		Status:        status,
		Blocks:        blocks,
	}
	if err := validateWorkoutFields(w); err != nil {
		return nil, err
	}
	if err := wc.repo.Create(w); err != nil {
		return nil, err
	}
	return w, nil
}

// UpdateWorkout validates, resolves blocks, and overwrites the
// workout plus its blocks (statuses reset to pending). Returns
// ErrWorkoutNotFound when missing.
func (wc *WorkoutsController) UpdateWorkout(id, userID string, in UpdateWorkoutInput) (*models.Workout, error) {
	existing, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrWorkoutNotFound
	}
	blocks, err := wc.validateAndResolveBlocks(userID, in.Blocks)
	if err != nil {
		return nil, err
	}
	status := in.Status
	if status == "" {
		status = models.WorkoutStatusPlanned
	}
	existing.Name = strings.TrimSpace(in.Name)
	existing.Description = strings.TrimSpace(in.Description)
	existing.ScheduledDate = strings.TrimSpace(in.ScheduledDate)
	existing.Status = status
	existing.Blocks = blocks
	if err := validateWorkoutFields(existing); err != nil {
		return nil, err
	}
	if err := wc.repo.Update(existing, userID); err != nil {
		return nil, err
	}
	return existing, nil
}

// DeleteWorkout hard-deletes a workout scoped to the user. Returns
// ErrWorkoutNotFound when missing.
func (wc *WorkoutsController) DeleteWorkout(id, userID string) error {
	existing, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return err
	}
	if existing == nil {
		return ErrWorkoutNotFound
	}
	return wc.repo.Delete(id, userID)
}

// SetWorkoutBlockStatus marks one block in a workout done/skipped/
// pending. Returns ErrWorkoutNotFound when the workout or join row
// is missing (or owned by another user).
func (wc *WorkoutsController) SetWorkoutBlockStatus(workoutID, workoutBlockID, userID string, status models.WorkoutBlockStatus) (*models.Workout, error) {
	if !status.IsValid() {
		return nil, ErrWorkoutBlockStatusInvalid
	}
	existing, err := wc.repo.GetByID(workoutID, userID)
	if err != nil {
		return nil, err
	}
	if existing == nil {
		return nil, ErrWorkoutNotFound
	}
	row, err := wc.repo.GetWorkoutBlock(workoutID, workoutBlockID)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, ErrWorkoutNotFound
	}
	if err := wc.repo.SetBlockStatus(workoutID, workoutBlockID, status); err != nil {
		return nil, err
	}
	return wc.repo.GetByID(workoutID, userID)
}

// DuplicateWorkout copies a workout (exact name, description,
// block order) onto a new scheduled date. Block statuses reset to
// pending and the workout status resets to planned. Returns
// ErrWorkoutNotFound when missing.
func (wc *WorkoutsController) DuplicateWorkout(id, userID, scheduledDate string) (*models.Workout, error) {
	scheduledDate = strings.TrimSpace(scheduledDate)
	if scheduledDate == "" {
		return nil, ErrWorkoutDateRequired
	}
	if _, err := time.Parse("2006-01-02", scheduledDate); err != nil {
		return nil, ErrWorkoutDateInvalid
	}
	src, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, ErrWorkoutNotFound
	}
	blocks := make([]models.WorkoutBlock, 0, len(src.Blocks))
	for _, b := range src.Blocks {
		blocks = append(blocks, models.WorkoutBlock{
			BlockID:          b.BlockID,
			BlockName:        b.BlockName,
			BlockDescription: b.BlockDescription,
			BlockType:        b.BlockType,
			Status:           models.WorkoutBlockPending,
			ItemCount:        b.ItemCount,
		})
	}
	cp := &models.Workout{
		UserID:        userID,
		Name:          src.Name,
		Description:   src.Description,
		ScheduledDate: scheduledDate,
		Status:        models.WorkoutStatusPlanned,
		Blocks:        blocks,
	}
	if err := validateWorkoutFields(cp); err != nil {
		return nil, err
	}
	if err := wc.repo.Create(cp); err != nil {
		return nil, err
	}
	return cp, nil
}

// CountBlockUsage returns how many of the user's workouts reference
// the given block. Backs the 409 guard on block deletion.
func (wc *WorkoutsController) CountBlockUsage(blockID, userID string) (int64, error) {
	return wc.repo.CountBlockUsage(blockID, userID)
}

// DuplicateWorkoutBatch copies a workout onto several scheduled
// dates at once (recurring schedule: weekly, every N days, … — the
// client expands the pattern into an explicit date list). Copies keep
// the source's exact name (repeats are distinguished by date),
// description, and block order; block statuses reset to pending and
// workout statuses to planned. Overlaps with existing workouts are
// allowed. All-or-nothing: every date is validated before anything is
// stored, and persistence runs in a single transaction, so a failure
// leaves zero new rows. Returns ErrWorkoutNotFound when missing.
func (wc *WorkoutsController) DuplicateWorkoutBatch(id, userID string, dates []string) ([]*models.Workout, error) {
	if len(dates) == 0 {
		return nil, ErrWorkoutBulkRequired
	}
	if len(dates) > WorkoutBulkMax {
		return nil, ErrWorkoutBulkTooMany
	}
	clean := make([]string, 0, len(dates))
	seen := map[string]bool{}
	for _, d := range dates {
		d = strings.TrimSpace(d)
		if d == "" {
			return nil, ErrWorkoutDateRequired
		}
		if _, err := time.Parse("2006-01-02", d); err != nil {
			return nil, ErrWorkoutDateInvalid
		}
		if seen[d] {
			return nil, ErrWorkoutBulkDuplicateDate
		}
		seen[d] = true
		clean = append(clean, d)
	}
	src, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return nil, err
	}
	if src == nil {
		return nil, ErrWorkoutNotFound
	}
	out := make([]*models.Workout, 0, len(clean))
	for _, date := range clean {
		blocks := make([]models.WorkoutBlock, 0, len(src.Blocks))
		for _, b := range src.Blocks {
			blocks = append(blocks, models.WorkoutBlock{
				BlockID:          b.BlockID,
				BlockName:        b.BlockName,
				BlockDescription: b.BlockDescription,
				BlockType:        b.BlockType,
				Status:           models.WorkoutBlockPending,
				ItemCount:        b.ItemCount,
			})
		}
		cp := &models.Workout{
			UserID:        userID,
			Name:          src.Name,
			Description:   src.Description,
			ScheduledDate: date,
			Status:        models.WorkoutStatusPlanned,
			Blocks:        blocks,
		}
		if err := validateWorkoutFields(cp); err != nil {
			return nil, err
		}
		out = append(out, cp)
	}
	if err := wc.repo.CreateBatch(out); err != nil {
		return nil, err
	}
	return out, nil
}

// validateAndResolveBlocks checks block count, requires a block ID
// per row, and resolves each block so a typo'd ID surfaces as
// ErrWorkoutBlockNotFound instead of a foreign-key error. Position
// is implicit (slice order); statuses start pending.
func (wc *WorkoutsController) validateAndResolveBlocks(userID string, ins []WorkoutBlockInput) ([]models.WorkoutBlock, error) {
	if len(ins) < WorkoutBlocksMin {
		return nil, ErrWorkoutBlocksRequired
	}
	if len(ins) > WorkoutBlocksMax {
		return nil, ErrWorkoutBlocksTooMany
	}
	out := make([]models.WorkoutBlock, 0, len(ins))
	for i, in := range ins {
		if strings.TrimSpace(in.BlockID) == "" {
			return nil, ErrWorkoutBlockRequired
		}
		b, err := wc.blocks.GetByID(in.BlockID, userID)
		if err != nil {
			return nil, err
		}
		if b == nil {
			return nil, ErrWorkoutBlockNotFound
		}
		out = append(out, models.WorkoutBlock{
			BlockID:          b.ID,
			BlockName:        b.Name,
			BlockDescription: b.Description,
			BlockType:        b.Type,
			Position:         i,
			Status:           models.WorkoutBlockPending,
			ItemCount:        len(b.Items),
		})
	}
	return out, nil
}

// validateWorkoutFields checks the name/description lengths, the
// YYYY-MM-DD date, and the status value.
func validateWorkoutFields(w *models.Workout) error {
	if w.Name == "" {
		return ErrWorkoutNameRequired
	}
	if len(w.Name) > WorkoutNameMaxLen {
		return ErrWorkoutNameTooLong
	}
	if len(w.Description) > WorkoutDescriptionMaxLen {
		return ErrWorkoutDescriptionLong
	}
	if w.ScheduledDate == "" {
		return ErrWorkoutDateRequired
	}
	if _, err := time.Parse("2006-01-02", w.ScheduledDate); err != nil {
		return ErrWorkoutDateInvalid
	}
	if !w.Status.IsValid() {
		return ErrWorkoutStatusInvalid
	}
	return nil
}
