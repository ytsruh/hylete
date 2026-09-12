package models

import (
	"errors"
	"testing"
	"time"
)

func TestWorkoutBlockType_IsValid(t *testing.T) {
	for _, typ := range []WorkoutBlockType{
		WorkoutBlockTypeStraight,
		WorkoutBlockTypeSuperset,
		WorkoutBlockTypeCircuit,
		WorkoutBlockTypeEMOM,
		WorkoutBlockTypeAMRAP,
	} {
		if !typ.IsValid() {
			t.Errorf("type %q should be valid", typ)
		}
	}
	if WorkoutBlockType("tabata").IsValid() {
		t.Error("unknown type should be invalid")
	}
	if WorkoutBlockType("").IsValid() {
		t.Error("empty type should be invalid")
	}
}

func TestWorkoutStatus_IsValid(t *testing.T) {
	for _, s := range []WorkoutStatus{
		WorkoutStatusPlanned,
		WorkoutStatusCompleted,
		WorkoutStatusCancelled,
	} {
		if !s.IsValid() {
			t.Errorf("status %q should be valid", s)
		}
	}
	if WorkoutStatus("active").IsValid() {
		t.Error("in-progress status must not exist in v1")
	}
}

func TestValidateWorkoutBlockParams(t *testing.T) {
	tests := []struct {
		name     string
		block    WorkoutBlockType
		interval int
		timeCap  int
		wantErr  error
	}{
		{"straight ignores extras", WorkoutBlockTypeStraight, 0, 0, nil},
		{"superset needs nothing", WorkoutBlockTypeSuperset, 0, 0, nil},
		{"circuit needs nothing", WorkoutBlockTypeCircuit, 0, 0, nil},
		{"emom with interval", WorkoutBlockTypeEMOM, 60, 0, nil},
		{"emom without interval", WorkoutBlockTypeEMOM, 0, 0, ErrBlockIntervalRequired},
		{"amrap with cap", WorkoutBlockTypeAMRAP, 0, 600, nil},
		{"amrap without cap", WorkoutBlockTypeAMRAP, 0, 0, ErrBlockTimeCapRequired},
		{"unknown type", WorkoutBlockType("tabata"), 0, 0, errors.New("unknown workout block type")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateWorkoutBlockParams(tt.block, tt.interval, tt.timeCap)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || err.Error() != tt.wantErr.Error() {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNormalizeWorkoutBlockRounds(t *testing.T) {
	if got := NormalizeWorkoutBlockRounds(WorkoutBlockTypeStraight, 5); got != 1 {
		t.Errorf("straight rounds = %d, want 1", got)
	}
	if got := NormalizeWorkoutBlockRounds(WorkoutBlockTypeSuperset, 3); got != 3 {
		t.Errorf("superset rounds = %d, want 3", got)
	}
	if got := NormalizeWorkoutBlockRounds(WorkoutBlockTypeCircuit, 0); got != 1 {
		t.Errorf("zero rounds = %d, want floor of 1", got)
	}
}

func TestNormalizeWorkoutItemTargets_AllowsOpenPrescription(t *testing.T) {
	// Targets are fully optional: an all-zero item (just a linked
	// exercise) normalises to itself for both types.
	for _, typ := range []ExerciseType{ExerciseTypeStrength, ExerciseTypeCardio} {
		got := NormalizeWorkoutItemTargets(typ, WorkoutItemTargets{})
		if got != (WorkoutItemTargets{}) {
			t.Errorf("type %q: open targets changed to %+v", typ, got)
		}
	}
}

func TestNormalizeWorkoutItemTargets(t *testing.T) {
	// Strength items drop cardio targets.
	got := NormalizeWorkoutItemTargets(ExerciseTypeStrength, WorkoutItemTargets{
		TargetReps: 8, TargetWeight: 60,
		TargetDurationSeconds: 1500, TargetDistanceMeters: 5000,
		TargetAvgHeartRate: 150, TargetCalories: 300,
	})
	if got.TargetDurationSeconds != 0 || got.TargetDistanceMeters != 0 || got.TargetAvgHeartRate != 0 || got.TargetCalories != 0 {
		t.Errorf("strength kept cardio targets: %+v", got)
	}
	if got.TargetReps != 8 || got.TargetWeight != 60 {
		t.Errorf("strength lost its own targets: %+v", got)
	}
	// Cardio items drop strength targets (including rest).
	got = NormalizeWorkoutItemTargets(ExerciseTypeCardio, WorkoutItemTargets{
		TargetReps: 8, TargetWeight: 60, TargetRestSeconds: 90,
		TargetDurationSeconds: 1500, TargetDistanceMeters: 5000,
	})
	if got.TargetReps != 0 || got.TargetWeight != 0 || got.TargetRestSeconds != 0 {
		t.Errorf("cardio kept strength targets: %+v", got)
	}
	if got.TargetDurationSeconds != 1500 || got.TargetDistanceMeters != 5000 {
		t.Errorf("cardio lost its own targets: %+v", got)
	}
}

func TestValidateWorkoutSchedule(t *testing.T) {
	start := time.Now()
	end := start.Add(time.Hour)
	if err := ValidateWorkoutSchedule(&start, &end); err != nil {
		t.Fatalf("valid range rejected: %v", err)
	}
	if err := ValidateWorkoutSchedule(nil, nil); err != nil {
		t.Fatalf("unscheduled rejected: %v", err)
	}
	if err := ValidateWorkoutSchedule(&start, nil); err != nil {
		t.Fatalf("open-ended rejected: %v", err)
	}
	if err := ValidateWorkoutSchedule(&end, &start); !errors.Is(err, ErrScheduleEndBeforeStart) {
		t.Fatalf("inverted range: error = %v, want ErrScheduleEndBeforeStart", err)
	}
}

func TestWorkoutLinkage_IsStandalone(t *testing.T) {
	if !(WorkoutLinkage{}).IsStandalone() {
		t.Error("empty linkage should be standalone")
	}
	if (WorkoutLinkage{WorkoutID: "w-1"}).IsStandalone() {
		t.Error("workout link should not be standalone")
	}
	if (WorkoutLinkage{RoundNumber: 2}).IsStandalone() {
		t.Error("round number should not be standalone")
	}
}
