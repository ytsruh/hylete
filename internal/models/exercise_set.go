package models

import (
	"errors"
)

// Sentinel errors returned by ValidateExerciseSetInput so callers (the JSON
// API handlers) can map them to human-readable 400 responses while unit
// tests can assert on identity with errors.Is.
var (
	// ErrRepsRequired is returned when a strength exercise entry is submitted without repetitions.
	ErrRepsRequired = errors.New("reps must be at least 1")
	// ErrDurationRequired is returned when a cardio exercise entry is submitted without a duration.
	ErrDurationRequired = errors.New("duration is required for cardio exercises")
	// ErrDistanceRequired is returned when a cardio exercise entry is submitted without a distance.
	ErrDistanceRequired = errors.New("distance is required for cardio exercises")
)

// ExerciseSetInput describes a single set to be persisted as part of a multi-set
// exercise entry submission. All sets within a submission share an exercise,
// user, notes and timestamp; only per-set values differ. Strength sets carry
// Reps/Weight/RestTime, cardio sets carry DurationSeconds/DistanceMeters plus
// optional AvgHeartRate/CaloriesBurned — which pair applies is decided by the
// exercise's type via ValidateExerciseSetInput and NormalizeForExerciseType.
type ExerciseSetInput struct {
	Reps            int
	Weight          float64
	RestTime        int
	DurationSeconds int
	DistanceMeters  float64
	AvgHeartRate    int
	CaloriesBurned  float64
}

// ValidateExerciseSetInput checks one set against the requirements for its
// exercise's type: strength entries need at least one rep, cardio entries need
// both a positive duration and a positive distance. Numeric range limits
// (max weight, max duration…) are enforced earlier by the request validators;
// this covers the type-conditional rules they cannot express.
func ValidateExerciseSetInput(exerciseType ExerciseType, in ExerciseSetInput) error {
	switch exerciseType {
	case ExerciseTypeCardio:
		if in.DurationSeconds <= 0 {
			return ErrDurationRequired
		}
		if in.DistanceMeters <= 0 {
			return ErrDistanceRequired
		}
	default:
		if in.Reps < 1 {
			return ErrRepsRequired
		}
	}
	return nil
}

// NormalizeForExerciseType zeroes the metric pair that does not apply to the
// given exercise type so exactly one pair is ever non-zero on disk: strength
// entries never keep cardio metrics and vice versa. Rest time is also dropped
// from cardio entries because there is no per-set rest to rest between.
func (in ExerciseSetInput) NormalizeForExerciseType(exerciseType ExerciseType) ExerciseSetInput {
	if exerciseType == ExerciseTypeCardio {
		in.Reps = 0
		in.Weight = 0
		in.RestTime = 0
		return in
	}
	in.DurationSeconds = 0
	in.DistanceMeters = 0
	in.AvgHeartRate = 0
	in.CaloriesBurned = 0
	return in
}
