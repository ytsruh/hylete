// Package cron contains the scheduled jobs that run in the
// background while the server is up. The package owns the only
// place in the codebase that imports a third-party scheduler
// (github.com/robfig/cron/v3); everything else in the package
// talks to a narrow Scheduler interface so the dep is easy to
// replace with a different runner (a real queue, a system cron,
// an HTTP trigger, etc.) without touching the job code.
//
// The job code itself is decoupled from the cron wrapper: the
// WeightReminder struct exposes a plain Run(ctx) method that the
// cron job calls. Tests can call Run directly without standing
// up a scheduler. Conversely, the Scheduler is testable with a
// fake job function in the rare case we want to exercise the
// cron lifecycle in isolation.
package cron

import (
	"context"
	"fmt"
	"log"
	"time"

	// Aliased because the package itself is now named cron: an
	// unaliased import would read as cron.New inside package cron.
	robfigcron "github.com/robfig/cron/v3"
)

// Job is the function the Scheduler invokes when a scheduled
// tick fires. It receives the application's root context so
// a long-running job can observe shutdown. The Scheduler
// itself does not block on the Job — the job is expected to
// launch any heavy work in its own goroutine (see
// WeightReminder.Run).
type Job func(ctx context.Context)

// Scheduler is the narrow interface the rest of the package
// depends on. Two methods cover the full lifecycle:
//
//   - Start: begin firing jobs on their schedule. Safe to
//     call once per Scheduler; subsequent calls are a no-op.
//   - Stop:  stop firing and wait for any in-flight job to
//     return. Safe to call multiple times; subsequent calls
//     are a no-op.
//
// The Stop semantics — wait for in-flight work to finish —
// match robfig/cron/v3's contract. A future implementation
// behind this interface should preserve that contract so
// callers can rely on Stop() returning only when it is safe
// to exit the process.
type Scheduler interface {
	Start()
	Stop()
}

// NewCronScheduler returns a Scheduler backed by
// robfig/cron/v3. spec is a standard 5-field cron expression
// interpreted in loc. job is the function called each tick.
//
// Cron has a built-in Recover wrapper (per the v3 docs) that
// turns panics in the job into log lines rather than crashing
// the process — we keep that behaviour. Any panic in the
// job function is the job's own responsibility to fix.
//
// Returns an error if the spec is unparseable so the caller
// can fail fast at startup rather than discovering the bad
// spec at the first tick.
func NewCronScheduler(spec string, loc *time.Location, job Job) (Scheduler, error) {
	if loc == nil {
		loc = time.UTC
	}
	if job == nil {
		return nil, fmt.Errorf("cron: job is nil")
	}

	c := robfigcron.New(robfigcron.WithLocation(loc))

	// AddFunc returns an error for an unparseable spec. We
	// surface it before the caller wires the scheduler into
	// main(), so a typo in "0 9 * * 0" fails startup instead
	// of silently never firing.
	if _, err := c.AddFunc(spec, func() {
		// robfig already wraps jobs in Recover, so a panic
		// here becomes a log line rather than a crash. The
		// background context means a long-running job is
		// not bound to the caller's lifetime — the Scheduler
		// has no per-tick context to thread through.
		job(context.Background())
	}); err != nil {
		return nil, fmt.Errorf("cron: parse cron spec %q: %w", spec, err)
	}

	return &cronScheduler{c: c}, nil
}

// cronScheduler is the production Scheduler implementation
// behind the Scheduler interface. Held by value in a small
// struct so the interface can be passed around without
// exposing the underlying *robfigcron.Cron.
type cronScheduler struct {
	c *robfigcron.Cron
}

// Start begins firing jobs on their schedule. cron.Start is
// non-blocking; the underlying goroutine lives for the
// lifetime of the process or until Stop is called.
func (s *cronScheduler) Start() {
	log.Printf("cron: scheduler starting")
	s.c.Start()
}

// Stop stops the scheduler and waits for any in-flight job
// to return. The robfig contract is that calling Stop on a
// stopped cron is safe (it's a no-op), so this method
// matches the Scheduler interface's "safe to call multiple
// times" guarantee.
func (s *cronScheduler) Stop() {
	<-s.c.Stop().Done()
	log.Printf("cron: scheduler stopped")
}

// Compile-time check: the production implementation must
// satisfy the interface the rest of the package depends on.
// If the underlying cron API drifts and breaks the
// interface, this line fails to compile.
var _ Scheduler = (*cronScheduler)(nil)
