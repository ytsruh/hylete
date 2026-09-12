-- Workouts are user-owned planned sessions composed of blocks
-- (many-to-many via workout_blocks) and scheduled onto calendar
-- days via workout_assignments (one row per day, date-only text).
-- Every query is scoped to user_id (via the workouts row, or the
-- denormalized user_id on assignments) so a request can never
-- read or mutate another user's plan.

-- name: CreateWorkout :one
INSERT INTO workouts (id, user_id, title, description, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkout :one
SELECT * FROM workouts
WHERE id = ? AND user_id = ?;

-- name: ListWorkouts :many
-- Newest first; rowid breaks created_at ties (same convention as
-- blocks and exercise_entries ordering).
SELECT * FROM workouts
WHERE user_id = ?
ORDER BY created_at DESC, rowid DESC;

-- name: ListWorkoutsWithBlockCount :many
-- List view needs per-workout block counts without N+1 queries.
SELECT w.*, COUNT(wb.id) AS block_count FROM workouts w
LEFT JOIN workout_blocks wb ON wb.workout_id = w.id
WHERE w.user_id = ?
GROUP BY w.id
ORDER BY w.created_at DESC, w.rowid DESC;

-- name: UpdateWorkout :exec
-- Overwrites the editable workout fields and bumps updated_at.
-- Blocks are replaced separately via DeleteWorkoutBlocks +
-- CreateWorkoutBlock.
UPDATE workouts
SET title = ?,
    description = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: DeleteWorkout :exec
DELETE FROM workouts
WHERE id = ? AND user_id = ?;

-- name: CreateWorkoutBlock :one
INSERT INTO workout_blocks (id, workout_id, block_id, position, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING *;

-- name: ListWorkoutBlocks :many
SELECT * FROM workout_blocks
WHERE workout_id = ?
ORDER BY position ASC;

-- name: ListWorkoutBlocksWithBlock :many
-- Detail view resolves each linked block's name/type in one
-- query. Ownership is gated by the caller's prior GetWorkout
-- (scoped to user_id); this query only orders the links. Blocks
-- deleted after linking disappear from this join (CASCADE
-- removes the link row) - the workout itself survives.
SELECT wb.id, wb.workout_id, wb.block_id, wb.position, wb.created_at,
       b.name AS block_name, b.block_type AS block_type
FROM workout_blocks wb
JOIN blocks b ON b.id = wb.block_id
WHERE wb.workout_id = ?
ORDER BY wb.position ASC;

-- name: DeleteWorkoutBlocks :exec
-- Full block replacement on update: delete-all then re-insert in
-- a transaction (the repository owns the tx). Also keeps deletes
-- correct on databases ignoring ON DELETE CASCADE.
DELETE FROM workout_blocks
WHERE workout_id = ?;

-- name: CreateWorkoutAssignment :one
-- One row per planned day. The UNIQUE(workout_id,
-- scheduled_date) guard makes repeat-expansion idempotent.
INSERT INTO workout_assignments (id, workout_id, user_id, scheduled_date, created_at)
VALUES (?, ?, ?, ?, ?)
RETURNING id, workout_id, user_id, scheduled_date, created_at;

-- name: ListWorkoutAssignmentsByDateRange :many
-- Calendar range query: every assignment for the user on
-- [start, end] (inclusive, YYYY-MM-DD text compares
-- lexicographically) with the workout title resolved so the
-- day cell can render without N+1 queries.
SELECT a.id, a.workout_id, a.user_id, a.scheduled_date, a.created_at,
       w.title AS workout_title
FROM workout_assignments a
JOIN workouts w ON w.id = a.workout_id
WHERE a.user_id = ? AND a.scheduled_date >= ? AND a.scheduled_date <= ?
ORDER BY a.scheduled_date ASC, a.created_at ASC;

-- name: GetWorkoutAssignment :one
SELECT id, workout_id, user_id, scheduled_date, created_at FROM workout_assignments
WHERE id = ?;

-- name: ListWorkoutAssignmentsForWorkout :many
SELECT id, workout_id, user_id, scheduled_date, created_at FROM workout_assignments
WHERE workout_id = ?
ORDER BY scheduled_date ASC;

-- name: DeleteWorkoutAssignment :exec
-- Removes a single planned day. Scoped to the user so a guessed
-- assignment ID cannot move another user's plan.
DELETE FROM workout_assignments
WHERE id = ? AND user_id = ?;

-- name: DeleteWorkoutAssignmentsForWorkout :exec
DELETE FROM workout_assignments
WHERE workout_id = ?;
