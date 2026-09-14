package cron

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeSweepRepo is an in-memory WorkoutSweepRepo: it records the
// today value the sweeper passed and returns canned counts.
type fakeSweepRepo struct {
	gotToday  string
	workouts  int64
	blocks    int64
	sweepErr  error
	callCount int
}

func (f *fakeSweepRepo) SweepStalePlannedWorkouts(today string) (int64, int64, error) {
	f.gotToday = today
	f.callCount++
	if f.sweepErr != nil {
		return 0, 0, f.sweepErr
	}
	return f.workouts, f.blocks, nil
}

func TestNewWorkoutSweeperNilRepo(t *testing.T) {
	if _, err := NewWorkoutSweeper(nil, nil); err == nil {
		t.Fatal("expected error for nil repo, got nil")
	}
}

func TestWorkoutSweeperPassesTodayUTC(t *testing.T) {
	// 00:30 BST on 2026-09-15 is 23:30 UTC on 2026-09-14: the
	// sweep must use the UTC date, not the local one.
	now := time.Date(2026, 9, 15, 0, 30, 0, 0, time.FixedZone("BST", 3600))
	repo := &fakeSweepRepo{workouts: 2, blocks: 3}
	s, err := NewWorkoutSweeper(repo, fixedClock{t: now})
	if err != nil {
		t.Fatalf("NewWorkoutSweeper failed: %v", err)
	}
	res := s.Run(context.Background())
	if !res.Attempted {
		t.Fatal("expected Attempted=true")
	}
	if repo.gotToday != "2026-09-14" {
		t.Fatalf("expected today=2026-09-14, got %q", repo.gotToday)
	}
	if res.WorkoutsSkipped != 2 || res.BlocksSkipped != 3 {
		t.Fatalf("expected 2 workouts/3 blocks, got %d/%d", res.WorkoutsSkipped, res.BlocksSkipped)
	}
	if res.ListError != "" {
		t.Fatalf("expected no ListError, got %q", res.ListError)
	}
}

func TestWorkoutSweeperRepoError(t *testing.T) {
	repo := &fakeSweepRepo{sweepErr: errors.New("db down")}
	s, err := NewWorkoutSweeper(repo, fixedClock{t: time.Date(2026, 9, 15, 1, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewWorkoutSweeper failed: %v", err)
	}
	res := s.Run(context.Background())
	if res.Attempted {
		t.Fatal("expected Attempted=false on repo error")
	}
	if res.ListError == "" {
		t.Fatal("expected ListError to be set on repo error")
	}
	if res.WorkoutsSkipped != 0 || res.BlocksSkipped != 0 {
		t.Fatalf("expected zero counts on error, got %d/%d", res.WorkoutsSkipped, res.BlocksSkipped)
	}
}
