package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	aicoach "hylete/internal/ai"
)

// CoachWeeklyService is the narrow contract the weekly Coach job
// depends on. Defined as an interface (rather than the concrete
// *aicoach.Service) so the job is unit-testable with an in-memory
// fake and the dep stays swappable without touching job code.
type CoachWeeklyService interface {
	// RunWeekly builds the weekly report for every opted-in user.
	// Per-user failures are recorded in TickResult (never returned
	// as an error) so one bad row cannot abort the whole tick.
	RunWeekly(ctx context.Context, now time.Time) aicoach.TickResult
}

// CoachWeekly orchestrates the Monday Coach review tick. Like
// UserReminder and WorkoutSweeper, the job is a plain Run(ctx)
// method decoupled from the cron wrapper: constructed once in main
// and invoked by the Scheduler on every tick. Run owns the tick
// logging (summary + per-user failure lines) so main.go stays
// wiring-only. Tests call Run directly with a fake service.
type CoachWeekly struct {
	svc   CoachWeeklyService
	clock Clock
}

// NewCoachWeekly returns a CoachWeekly bound to the given service.
// svc is required (nil returns an error so a missing dependency
// fails at startup, not at the first tick). clock is optional; nil
// falls back to RealClock.
func NewCoachWeekly(svc CoachWeeklyService, clock Clock) (*CoachWeekly, error) {
	if svc == nil {
		return nil, fmt.Errorf("cron: NewCoachWeekly: svc is nil")
	}
	if clock == nil {
		clock = RealClock{}
	}
	return &CoachWeekly{svc: svc, clock: clock}, nil
}

// Run is the function the Scheduler invokes on every tick. It
// delegates the batch to the Coach service, then logs the summary
// counters plus one line per failed user (user ID + error) so a
// failures=1 tick is diagnosable from the server log alone. The
// result is returned for tests; the cron path discards it (the log
// is the canonical record).
func (j *CoachWeekly) Run(ctx context.Context) aicoach.TickResult {
	start := j.clock.Now().UTC()
	log.Printf("coach: weekly tick starting at %s", start.Format(time.RFC3339))

	res := j.svc.RunWeekly(ctx, start)

	log.Printf("coach: weekly tick users=%d generated=%d reused=%d failures=%d tokens_in=%d tokens_out=%d",
		res.UsersSeen, res.Generated, res.Reused, res.Failures, res.TokensIn, res.TokensOut)
	if res.ListError != "" {
		log.Printf("coach: weekly tick: list opted-in users failed: %s", res.ListError)
	}
	for _, f := range res.FailuresDetail {
		if f.UserID == "" {
			continue
		}
		log.Printf("coach: weekly user %s failed: %s", f.UserID, f.Err)
	}
	return res
}

// Compile-time check: the production service must satisfy the
// interface the job depends on. If RunWeekly's signature ever
// drifts, this line fails to compile and points the maintainer at
// the break.
var _ CoachWeeklyService = (*aicoach.Service)(nil)
