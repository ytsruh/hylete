// WorkoutSweep is the nightly auto-skip job for stale planned
// workouts. At 1am UTC the cron tick calls Run, which flips every
// still-planned workout scheduled before today (YYYY-MM-DD) with
// zero linked exercise entries to skipped, along with its
// still-pending blocks. in_progress workouts are left alone (the
// user started them), and done/skipped blocks are untouched. Like
// UserReminder, the job is a plain Run(ctx) method decoupled from
// the cron wrapper so tests call it directly without a scheduler.
package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	"hylete/internal/models"
)

// WorkoutSweepRepo is the data-access surface the workout sweeper
// needs. Defined as an interface (rather than the concrete
// *models.WorkoutRepository) so the job is unit-testable with an
// in-memory fake. today is a YYYY-MM-DD date; lexical comparison
// against scheduled_date is chronological.
type WorkoutSweepRepo interface {
	// SweepStalePlannedWorkouts flips every still-planned workout
	// scheduled before today with zero linked exercise entries to
	// skipped (plus its pending blocks). Returns the number of
	// workouts and blocks flipped.
	SweepStalePlannedWorkouts(today string) (workoutsSkipped int64, blocksSkipped int64, err error)
}

// SweepResult is the aggregate of one workout-sweep Run call. The
// cron path logs it; it exists so tests can assert on the tick
// without parsing log output.
type SweepResult struct {
	WorkoutsSkipped int64
	BlocksSkipped   int64
	Duration        time.Duration
	Attempted       bool
	Now             time.Time
	// ListError is non-empty when the sweep query failed. When set,
	// the counts are zero (the tick surfaces this as a log line).
	ListError string
}

// WorkoutSweeper orchestrates the nightly stale-workout sweep. It
// is stateless and safe for concurrent use: each Run call derives
// today from the clock and delegates to the repo. Constructed once
// in main and invoked by the Scheduler on every tick.
type WorkoutSweeper struct {
	repo  WorkoutSweepRepo
	clock Clock
}

// NewWorkoutSweeper returns a WorkoutSweeper bound to the given
// repo. repo is required (nil returns an error so a missing
// dependency fails at startup, not at the first tick). clock is
// optional; nil falls back to RealClock.
func NewWorkoutSweeper(repo WorkoutSweepRepo, clock Clock) (*WorkoutSweeper, error) {
	if repo == nil {
		return nil, fmt.Errorf("cron: NewWorkoutSweeper: repo is nil")
	}
	if clock == nil {
		clock = RealClock{}
	}
	return &WorkoutSweeper{repo: repo, clock: clock}, nil
}

// Run is the function the Scheduler invokes on every tick. It
// derives today in UTC (YYYY-MM-DD) and sweeps stale planned
// workouts with no linked exercise entries. A repo failure is
// logged and returned in SweepResult — there is nothing useful to
// do without the sweep.
func (s *WorkoutSweeper) Run(ctx context.Context) SweepResult {
	start := s.clock.Now().UTC()
	today := start.Format("2006-01-02")
	log.Printf("cron: workout sweep starting for scheduled_date < %s", today)

	workouts, blocks, err := s.repo.SweepStalePlannedWorkouts(today)
	if err != nil {
		log.Printf("cron: workout sweep failed: %v", err)
		return SweepResult{
			Duration:  time.Since(start),
			Now:       start,
			ListError: err.Error(),
		}
	}

	duration := time.Since(start)
	log.Printf(
		"cron: workout sweep complete: workouts=%d blocks=%d duration=%s",
		workouts,
		blocks,
		duration,
	)

	return SweepResult{
		WorkoutsSkipped: workouts,
		BlocksSkipped:   blocks,
		Duration:        duration,
		Attempted:       true,
		Now:             start,
	}
}

// Compile-time check: the production type must satisfy the
// interface the sweeper depends on. If the repository signature
// ever drifts, this line fails to compile and points the
// maintainer at the break.
var _ WorkoutSweepRepo = (*models.WorkoutRepository)(nil)
