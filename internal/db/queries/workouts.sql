-- Workouts: dated containers scoped to the user.

-- name: CreateWorkout :one
INSERT INTO workouts (id, user_id, source_workout_id, name, notes, status, scheduled_start, scheduled_end, completed_at, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkout :one
SELECT * FROM workouts
WHERE id = ? AND user_id = ?;

-- name: ListWorkouts :many
-- Planned first (scheduled_start ascending, unscheduled last), then by
-- recency. The CASE emulates NULLS LAST for scheduled_start.
SELECT * FROM workouts
WHERE user_id = ?
ORDER BY CASE WHEN status = 'planned' THEN 0 ELSE 1 END, CASE WHEN scheduled_start IS NULL THEN 1 ELSE 0 END, scheduled_start ASC, created_at DESC;

-- name: ListWorkoutsByRange :many
-- Workouts whose scheduled window overlaps the given range.
-- Unscheduled rows (scheduled_start IS NULL) are excluded -- they
-- surface in ListWorkouts. Cancelled workouts are excluded so
-- cancelling genuinely clears the calendar.
SELECT * FROM workouts
WHERE user_id = ?
  AND status != 'cancelled'
  AND scheduled_start IS NOT NULL
  AND scheduled_start <= ?
  AND (scheduled_end IS NULL OR scheduled_end >= ?)
ORDER BY scheduled_start ASC;

-- name: UpdateWorkout :exec
UPDATE workouts
SET name = ?,
    notes = ?,
    scheduled_start = ?,
    scheduled_end = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: SetWorkoutStatus :exec
-- completed_at is managed by the caller: set on complete, cleared on
-- reopen/cancel so status changes own the timestamp (same pattern as goals).
UPDATE workouts
SET status = ?,
    completed_at = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: DeleteWorkout :exec
DELETE FROM workouts
WHERE id = ? AND user_id = ?;

-- name: CreateWorkoutBlock :one
INSERT INTO workout_blocks (id, workout_id, type, position, rounds, rest_between_rounds_seconds, interval_seconds, time_cap_seconds)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListWorkoutBlocks :many
SELECT * FROM workout_blocks
WHERE workout_id = ?
ORDER BY position ASC;

-- name: DeleteWorkoutBlocksByWorkout :exec
DELETE FROM workout_blocks
WHERE workout_id = ?;

-- name: CreateWorkoutItem :one
INSERT INTO workout_items (id, block_id, exercise_id, position, target_sets, target_reps, target_weight, target_rest_seconds, target_duration_seconds, target_distance_meters, target_avg_heart_rate, target_calories)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListWorkoutItemsByWorkout :many
SELECT wi.* FROM workout_items wi
JOIN workout_blocks wb ON wi.block_id = wb.id
WHERE wb.workout_id = ?
ORDER BY wb.position ASC, wi.position ASC;

-- name: ListWorkoutItemsByWorkoutWithExercises :many
-- Workout items with their exercise catalogue names and types so
-- detail reads do not need a lookup per item.
SELECT wi.*, e.name AS exercise_name, e.type AS exercise_type FROM workout_items wi
JOIN workout_blocks wb ON wi.block_id = wb.id
JOIN exercises e ON wi.exercise_id = e.id
WHERE wb.workout_id = ?
ORDER BY wb.position ASC, wi.position ASC;

-- name: GetWorkoutItemContext :one
-- Parentage of one workout item scoped to the user. Used to validate
-- the exercise-entry logging triple (the item must belong to the
-- workout the entry claims).
SELECT w.id AS workout_id, wb.id AS block_id, wi.id AS workout_item_id FROM workout_items wi
JOIN workout_blocks wb ON wi.block_id = wb.id
JOIN workouts w ON wb.workout_id = w.id
WHERE wi.id = ? AND w.user_id = ?;

-- name: GetWorkoutItem :one
-- Single workout item scoped to the user via its workout parentage.
-- Used to validate the exercise-entry logging triple.
SELECT wi.* FROM workout_items wi
JOIN workout_blocks wb ON wi.block_id = wb.id
JOIN workouts w ON wb.workout_id = w.id
WHERE wi.id = ? AND w.user_id = ?;

-- name: GetWorkoutBlock :one
-- Single workout block scoped to the user via its workout parentage.
SELECT wb.* FROM workout_blocks wb
JOIN workouts w ON wb.workout_id = w.id
WHERE wb.id = ? AND w.user_id = ?;
