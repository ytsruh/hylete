package models

import (
	"errors"
	"slices"
	"time"
)

// WorkoutBlockType describes how the workout items inside a workout
// block relate to each other during execution: straight items run in
// order with no linkage, superset/circuit items rotate each round,
// emom items run on a fixed interval, amrap items run as many rounds
// as possible inside a time cap.
type WorkoutBlockType string

const (
	WorkoutBlockTypeStraight WorkoutBlockType = "straight"
	WorkoutBlockTypeSuperset WorkoutBlockType = "superset"
	WorkoutBlockTypeCircuit  WorkoutBlockType = "circuit"
	WorkoutBlockTypeEMOM     WorkoutBlockType = "emom"
	WorkoutBlockTypeAMRAP    WorkoutBlockType = "amrap"
)

// IsValid checks if the workout block type is a known value.
func (t WorkoutBlockType) IsValid() bool {
	return slices.Contains([]WorkoutBlockType{
		WorkoutBlockTypeStraight,
		WorkoutBlockTypeSuperset,
		WorkoutBlockTypeCircuit,
		WorkoutBlockTypeEMOM,
		WorkoutBlockTypeAMRAP,
	}, t)
}

// WorkoutStatus is the lifecycle of a dated workout. There is no
// in-progress state: a planned workout simply accumulates logged
// exercise entries until the user marks it complete or cancelled.
type WorkoutStatus string

const (
	WorkoutStatusPlanned   WorkoutStatus = "planned"
	WorkoutStatusCompleted WorkoutStatus = "completed"
	WorkoutStatusCancelled WorkoutStatus = "cancelled"
)

// IsValid checks if the workout status is a known value.
func (s WorkoutStatus) IsValid() bool {
	return slices.Contains([]WorkoutStatus{
		WorkoutStatusPlanned,
		WorkoutStatusCompleted,
		WorkoutStatusCancelled,
	}, s)
}

// Block parameter validation sentinels. Returned by
// ValidateWorkoutBlockParams so callers (the JSON API handlers) can
// map them to human-readable 400 responses while unit tests can
// assert on identity with errors.Is.
var (
	// ErrBlockIntervalRequired is returned when an emom block has no positive interval.
	ErrBlockIntervalRequired = errors.New("interval is required for emom blocks")
	// ErrBlockTimeCapRequired is returned when an amrap block has no positive time cap.
	ErrBlockTimeCapRequired = errors.New("time cap is required for amrap blocks")
	// ErrScheduleEndBeforeStart is returned when a workout's scheduled end precedes its start.
	ErrScheduleEndBeforeStart = errors.New("scheduled end must not be before scheduled start")
)

// Workout is a dated container users plan ahead and log exercise
// entries into. SourceWorkoutID records which workout the workout was
// copied from (duplicate / plan-ahead copies — provenance only, nil
// for workouts authored from scratch); it is cleared when the source
// is deleted and never affects the copy's own blocks.
type Workout struct {
	ID              string
	UserID          string
	SourceWorkoutID *string
	Name           string
	Notes          string
	Status         WorkoutStatus
	ScheduledStart *time.Time
	ScheduledEnd   *time.Time
	CompletedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// IsComplete reports whether the workout is marked completed.
func (w *Workout) IsComplete() bool {
	return w.Status == WorkoutStatusCompleted
}

// WorkoutBlock groups workout items inside a dated workout.
// Position orders blocks within the workout. Rounds applies to
// superset/circuit/emom/amrap blocks (straight blocks always run a
// single round). RestBetweenRoundsSeconds applies to
// superset/circuit blocks, IntervalSeconds to emom blocks,
// TimeCapSeconds to amrap blocks.
type WorkoutBlock struct {
	ID                       string
	WorkoutID                string
	Type                     WorkoutBlockType
	Position                 int
	Rounds                   int
	RestBetweenRoundsSeconds int
	IntervalSeconds          int
	TimeCapSeconds           int
}

// WorkoutItem is one prescribed exercise inside a workout block,
// pointing at the global exercise catalogue. TargetSets is how many
// sets/sessions are prescribed; the remaining target fields carry both
// metric pairs (strength: TargetReps/TargetWeight/TargetRestSeconds,
// cardio: TargetDurationSeconds/TargetDistanceMeters plus optional
// heart-rate/calories) and which pair applies is decided by the linked
// exercise's type — the server zeroes the pair that does not apply,
// mirroring ExerciseEntry. All targets are optional: an item may be
// just a linked exercise with all-zero targets (an open prescription).
type WorkoutItem struct {
	ID                    string
	BlockID               string
	ExerciseID            string
	ExerciseName          string
	ExerciseType          ExerciseType
	Position              int
	TargetSets            int
	TargetReps            int
	TargetWeight          float64
	TargetRestSeconds     int
	TargetDurationSeconds int
	TargetDistanceMeters  float64
	TargetAvgHeartRate    int
	TargetCalories        float64
}

// WorkoutBlockWithItems bundles a workout block with its prescribed
// items in position order.
type WorkoutBlockWithItems struct {
	Block WorkoutBlock
	Items []WorkoutItem
}

// WorkoutDetail is a workout with its full block/item tree plus the
// exercise entries logged into it. LoggedExerciseEntries holds every
// exercise entry with this workout's ID in logging order; entries that
// reference a workout item are rendered under that item, while
// AdHocExerciseEntries holds the entries logged into the workout
// without an item link.
type WorkoutDetail struct {
	Workout               Workout
	Blocks                []WorkoutBlockWithItems
	LoggedExerciseEntries []ExerciseEntry
	AdHocExerciseEntries  []ExerciseEntry
}

// ValidateWorkoutBlockParams checks the execution parameters for a
// block type: emom blocks need a positive interval, amrap blocks need
// a positive time cap. Unknown types are rejected. Straight blocks
// ignore round extras (rounds are normalised to 1 on write).
func ValidateWorkoutBlockParams(blockType WorkoutBlockType, intervalSeconds, timeCapSeconds int) error {
	switch blockType {
	case WorkoutBlockTypeStraight, WorkoutBlockTypeSuperset, WorkoutBlockTypeCircuit:
		return nil
	case WorkoutBlockTypeEMOM:
		if intervalSeconds <= 0 {
			return ErrBlockIntervalRequired
		}
		return nil
	case WorkoutBlockTypeAMRAP:
		if timeCapSeconds <= 0 {
			return ErrBlockTimeCapRequired
		}
		return nil
	default:
		return errors.New("unknown workout block type")
	}
}

// NormalizeWorkoutBlockRounds forces straight blocks to a single
// round so readers never have to special-case rounds on unlinked
// collections.
func NormalizeWorkoutBlockRounds(blockType WorkoutBlockType, rounds int) int {
	if blockType == WorkoutBlockTypeStraight {
		return 1
	}
	if rounds < 1 {
		return 1
	}
	return rounds
}

// WorkoutItemTargets carries the prescribed targets of a workout item
// for normalisation. Every field is optional: an item
// may be just a linked exercise with no numbers (an open
// prescription), in which case all targets are zero. Zero is
// unambiguous — a real prescription always carries reps > 0 or a
// positive duration/distance.
type WorkoutItemTargets struct {
	TargetSets            int
	TargetReps            int
	TargetWeight          float64
	TargetRestSeconds     int
	TargetDurationSeconds int
	TargetDistanceMeters  float64
	TargetAvgHeartRate    int
	TargetCalories        float64
}

// NormalizeWorkoutItemTargets zeroes the target pair that does not
// apply to the given exercise type so exactly one pair is ever
// non-zero on disk, mirroring ExerciseSetInput normalisation.
func NormalizeWorkoutItemTargets(exerciseType ExerciseType, targets WorkoutItemTargets) WorkoutItemTargets {
	normalised := ExerciseSetInput{
		Reps:            targets.TargetReps,
		Weight:          targets.TargetWeight,
		RestTime:        targets.TargetRestSeconds,
		DurationSeconds: targets.TargetDurationSeconds,
		DistanceMeters:  targets.TargetDistanceMeters,
		AvgHeartRate:    targets.TargetAvgHeartRate,
		CaloriesBurned:  targets.TargetCalories,
	}.NormalizeForExerciseType(exerciseType)
	targets.TargetReps = normalised.Reps
	targets.TargetWeight = normalised.Weight
	targets.TargetRestSeconds = normalised.RestTime
	targets.TargetDurationSeconds = normalised.DurationSeconds
	targets.TargetDistanceMeters = normalised.DistanceMeters
	targets.TargetAvgHeartRate = normalised.AvgHeartRate
	targets.TargetCalories = normalised.CaloriesBurned
	return targets
}

// WorkoutLinkage carries the optional workout links for an exercise
// entry submission. All three empty/zero means the exercise entries
// are standalone. WorkoutItemID requires WorkoutID (an item link is
// always inside a workout); a positive RoundNumber requires WorkoutID
// (rounds only exist inside a workout).
type WorkoutLinkage struct {
	WorkoutID     string
	WorkoutItemID string
	RoundNumber   int
}

// IsStandalone reports whether the linkage carries no workout links.
func (l WorkoutLinkage) IsStandalone() bool {
	return l.WorkoutID == "" && l.WorkoutItemID == "" && l.RoundNumber == 0
}

// ValidateWorkoutSchedule checks that a workout's scheduled end does
// not precede its start. Either bound may be nil (unscheduled draft);
// only the both-present case is constrained.
func ValidateWorkoutSchedule(start, end *time.Time) error {
	if start != nil && end != nil && end.Before(*start) {
		return ErrScheduleEndBeforeStart
	}
	return nil
}
