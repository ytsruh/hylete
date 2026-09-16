package cron

import (
	"context"
	"errors"
	"testing"
	"time"

	aicoach "hylete/internal/ai"
)

// fakeCoachService is an in-memory CoachWeeklyService: it records
// the time the job passed and returns a canned TickResult.
type fakeCoachService struct {
	gotNow time.Time
	res    aicoach.TickResult
	calls  int
}

func (f *fakeCoachService) RunWeekly(_ context.Context, now time.Time) aicoach.TickResult {
	f.gotNow = now
	f.calls++
	return f.res
}

func TestNewCoachWeeklyNilSvc(t *testing.T) {
	if _, err := NewCoachWeekly(nil, nil); err == nil {
		t.Fatal("expected error for nil svc, got nil")
	}
}

func TestCoachWeeklyPassesUTCTime(t *testing.T) {
	// 05:30 BST on a Monday is 04:30 UTC: the job must pass the
	// UTC projection to the service so week bounds are stable
	// regardless of where the server runs.
	now := time.Date(2026, 9, 14, 5, 30, 0, 0, time.FixedZone("BST", 3600))
	svc := &fakeCoachService{res: aicoach.TickResult{UsersSeen: 1, Generated: 1}}
	j, err := NewCoachWeekly(svc, fixedClock{t: now})
	if err != nil {
		t.Fatalf("NewCoachWeekly failed: %v", err)
	}
	res := j.Run(context.Background())
	if svc.calls != 1 {
		t.Fatalf("expected 1 service call, got %d", svc.calls)
	}
	if !svc.gotNow.Equal(now.UTC()) {
		t.Fatalf("expected UTC time %v, got %v", now.UTC(), svc.gotNow)
	}
	if res.UsersSeen != 1 || res.Generated != 1 {
		t.Fatalf("expected result passthrough, got %+v", res)
	}
}

func TestCoachWeeklyPassesThroughFailures(t *testing.T) {
	svc := &fakeCoachService{res: aicoach.TickResult{
		UsersSeen: 1,
		Failures:  1,
		FailuresDetail: []aicoach.TickFailure{
			{UserID: "u1", Err: "ai: endpoint returned 401: ..."},
		},
	}}
	j, err := NewCoachWeekly(svc, fixedClock{t: time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewCoachWeekly failed: %v", err)
	}
	res := j.Run(context.Background())
	if res.Failures != 1 || len(res.FailuresDetail) != 1 {
		t.Fatalf("expected 1 failure detail, got %+v", res)
	}
	if res.FailuresDetail[0].UserID != "u1" {
		t.Fatalf("expected user u1, got %+v", res.FailuresDetail[0])
	}
}

func TestCoachWeeklyListError(t *testing.T) {
	svc := &fakeCoachService{res: aicoach.TickResult{
		Failures:  1,
		ListError: errors.New("db down").Error(),
		FailuresDetail: []aicoach.TickFailure{
			{Err: "db down"},
		},
	}}
	j, err := NewCoachWeekly(svc, fixedClock{t: time.Date(2026, 9, 14, 4, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatalf("NewCoachWeekly failed: %v", err)
	}
	res := j.Run(context.Background())
	if res.ListError == "" {
		t.Fatal("expected ListError passthrough")
	}
}
