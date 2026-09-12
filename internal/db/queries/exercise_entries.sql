-- name: CreateExerciseEntry :one
INSERT INTO exercise_entries (id, exercise_id, user_id, reps, weight, notes, rest_time, duration_seconds, distance_meters, avg_heart_rate, calories_burned, workout_id, workout_item_id, round_number, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetExerciseEntry :one
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.id = ? AND e.user_id = ?;

-- name: UpdateExerciseEntry :exec
UPDATE exercise_entries
SET exercise_id = ?, reps = ?, weight = ?, notes = ?, rest_time = ?, duration_seconds = ?, distance_meters = ?, avg_heart_rate = ?, calories_burned = ?, workout_id = ?, workout_item_id = ?, round_number = ?
WHERE id = ? AND user_id = ?;

-- name: UpdateExerciseEntryWithDate :exec
UPDATE exercise_entries
SET exercise_id = ?, reps = ?, weight = ?, notes = ?, rest_time = ?, duration_seconds = ?, distance_meters = ?, avg_heart_rate = ?, calories_burned = ?, workout_id = ?, workout_item_id = ?, round_number = ?, created_at = ?
WHERE id = ? AND user_id = ?;

-- name: DeleteExerciseEntry :exec
DELETE FROM exercise_entries WHERE id = ? AND user_id = ?;

-- name: ListExerciseEntries :many
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.user_id = ?
ORDER BY e.created_at DESC, e.rowid DESC;

-- name: ListExerciseEntriesWithLimit :many
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.user_id = ?
ORDER BY e.created_at DESC, e.rowid DESC
LIMIT ?;

-- name: ListExerciseEntriesLast7Days :many
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.created_at >= datetime('now', '-7 days') AND e.user_id = ?
ORDER BY e.created_at DESC, e.rowid DESC;

-- name: GetExerciseEntriesByExercisePaginated :many
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.exercise_id = ? AND e.user_id = ?
ORDER BY e.created_at DESC, e.rowid DESC
LIMIT ? OFFSET ?;

-- name: GetMaxWeightByExercise :one
-- Heaviest weight logged for a strength exercise. Returns 0 when no exercise entries exist.
SELECT CAST(COALESCE(MAX(weight), 0) AS REAL) FROM exercise_entries
WHERE exercise_id = ? AND user_id = ?;

-- name: GetMaxSetVolumeByExercise :one
-- Best single-set volume (reps * weight) logged for a strength exercise.
-- Returns 0 when no exercise entries exist.
SELECT CAST(COALESCE(MAX(reps * weight), 0) AS REAL) FROM exercise_entries
WHERE exercise_id = ? AND user_id = ?;

-- name: GetBestPaceByExercise :one
-- Fastest pace in seconds per kilometre across a user's cardio exercise entries
-- (duration divided by distance). Entries without distance are excluded;
-- returns 0 when no qualifying exercise entries exist.
SELECT CAST(COALESCE(MIN(duration_seconds * 1000.0 / distance_meters), 0) AS REAL) FROM exercise_entries
WHERE exercise_id = ? AND user_id = ? AND distance_meters > 0 AND duration_seconds > 0;

-- name: GetLongestDistanceByExercise :one
-- Longest distance in metres logged for an exercise. Returns 0 when no exercise entries exist.
SELECT CAST(COALESCE(MAX(distance_meters), 0) AS REAL) FROM exercise_entries
WHERE exercise_id = ? AND user_id = ?;

-- name: GetLastSetByExercise :one
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.exercise_id = ? AND e.user_id = ?
ORDER BY e.created_at DESC, e.rowid DESC
LIMIT 1;

-- name: GetExerciseEntriesByDateRange :many
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.created_at BETWEEN ? AND ? AND e.user_id = ?
ORDER BY e.created_at DESC, e.rowid DESC;

-- name: GetExerciseEntriesByWorkout :many
-- Every exercise entry logged into a workout, oldest first so rounds
-- read in logging order. Scopes to the given user ID.
SELECT e.id, e.exercise_id, t.name as exercise_name, t.type as exercise_type, e.user_id, e.reps, e.weight, e.notes, e.rest_time, e.duration_seconds, e.distance_meters, e.avg_heart_rate, e.calories_burned, e.workout_id, e.workout_item_id, e.round_number, e.created_at
FROM exercise_entries e
JOIN exercises t ON e.exercise_id = t.id
WHERE e.workout_id = ? AND e.user_id = ?
ORDER BY e.created_at ASC, e.rowid ASC;
