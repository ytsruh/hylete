-- Workouts are user-owned scheduled collections of blocks. Every
-- query is scoped to user_id (via the workouts row) so a request can
-- never read or mutate another user's training schedule.

-- name: CreateWorkout :one
INSERT INTO workouts (id, user_id, name, description, scheduled_date, status, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWorkout :one
SELECT * FROM workouts
WHERE id = ? AND user_id = ?;

-- name: ListWorkouts :many
-- Newest scheduled date first; rowid breaks ties (same convention
-- as exercise_entries ordering).
SELECT * FROM workouts
WHERE user_id = ?
ORDER BY scheduled_date DESC, created_at DESC, rowid DESC;

-- name: ListWorkoutsInRange :many
-- Schedule views (e.g. the dashboard calendar week) fetch one
-- inclusive date range. scheduled_date is YYYY-MM-DD so lexical
-- comparison is chronological.
SELECT * FROM workouts
WHERE user_id = ? AND scheduled_date >= ? AND scheduled_date <= ?
ORDER BY scheduled_date ASC, created_at ASC, rowid ASC;

-- name: ListWorkoutsWithBlockCounts :many
-- List view needs per-workout block + done counts without N+1.
SELECT w.*,
       COUNT(wb.id) AS block_count,
       COUNT(CASE WHEN wb.status = 'done' THEN 1 END) AS done_count
FROM workouts w
LEFT JOIN workout_blocks wb ON wb.workout_id = w.id
WHERE w.user_id = ?
GROUP BY w.id
ORDER BY w.scheduled_date DESC, w.created_at DESC, w.rowid DESC;

-- name: ListWorkoutsInRangeWithBlockCounts :many
-- Range variant of the list view (calendar weeks).
SELECT w.*,
       COUNT(wb.id) AS block_count,
       COUNT(CASE WHEN wb.status = 'done' THEN 1 END) AS done_count
FROM workouts w
LEFT JOIN workout_blocks wb ON wb.workout_id = w.id
WHERE w.user_id = ? AND w.scheduled_date >= ? AND w.scheduled_date <= ?
GROUP BY w.id
ORDER BY w.scheduled_date ASC, w.created_at ASC, w.rowid ASC;

-- name: UpdateWorkout :exec
-- Overwrites the editable workout fields and bumps updated_at.
-- Blocks are replaced separately via DeleteWorkoutBlocks +
-- CreateWorkoutBlock.
UPDATE workouts
SET name = ?,
    description = ?,
    scheduled_date = ?,
    status = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: DeleteWorkout :exec
DELETE FROM workouts
WHERE id = ? AND user_id = ?;

-- name: CreateWorkoutBlock :one
INSERT INTO workout_blocks (id, workout_id, block_id, position, status, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListWorkoutBlocks :many
SELECT * FROM workout_blocks
WHERE workout_id = ?
ORDER BY position ASC;

-- name: ListWorkoutBlocksWithBlock :many
-- Detail view resolves each planned block to its name/type plus
-- item count in one query. Ownership is gated by the caller's prior
-- GetWorkout (scoped to user_id); this query only orders the rows.
SELECT wb.id, wb.workout_id, wb.block_id, wb.position, wb.status, wb.created_at,
       b.name AS block_name, b.description AS block_description, b.block_type AS block_type,
       COUNT(i.id) AS item_count
FROM workout_blocks wb
JOIN blocks b ON b.id = wb.block_id
LEFT JOIN block_items i ON i.block_id = b.id
WHERE wb.workout_id = ?
GROUP BY wb.id
ORDER BY wb.position ASC;

-- name: GetWorkoutBlock :one
SELECT * FROM workout_blocks
WHERE id = ? AND workout_id = ?;

-- name: UpdateWorkoutBlockStatus :exec
UPDATE workout_blocks
SET status = ?
WHERE id = ? AND workout_id = ?;

-- name: UpdateWorkoutStatus :exec
-- Status-only update for the player lifecycle: first linked set flips
-- planned to in_progress, and the last block flip to done/skipped
-- auto-completes the workout. Bumps updated_at. Scoping to the
-- workout ID alone is intentional; callers gate ownership via a
-- prior scoped GetByID.
UPDATE workouts SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?;

-- name: UpdateWorkoutStatusScoped :exec
-- Scoped status-only update for explicit status changes (player
-- Finish button, detail status picker). Touches status alone so
-- block check-offs survive. Scoped to the user directly.
UPDATE workouts SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ? AND user_id = ?;

-- name: DeleteWorkoutBlocks :exec
-- Full block replacement on update: delete-all then re-insert in a
-- transaction (the repository owns the tx). Also keeps deletes
-- correct on databases ignoring ON DELETE CASCADE.
DELETE FROM workout_blocks
WHERE workout_id = ?;

-- name: CountWorkoutsUsingBlock :one
-- Guards block deletion: a block referenced by any of the user's
-- workouts is rejected with a 409 instead of being silently
-- unlinked. Scoped to the user via the workouts join.
SELECT COUNT(*) AS use_count
FROM workout_blocks wb
JOIN workouts w ON w.id = wb.workout_id
WHERE wb.block_id = ? AND w.user_id = ?;

-- name: ListStalePlannedWorkoutsWithoutEntries :many
-- Auto-skip sweep (1am UTC cron): every still-planned workout with
-- scheduled_date before today (YYYY-MM-DD, lexical compare is
-- chronological) that has zero linked exercise entries. Planned-only:
-- in_progress means the user started it, so it is left alone even
-- when nothing is linked yet. Block-less workouts are included (no
-- blocks plus no entries means nothing was done).
SELECT w.id
FROM workouts w
LEFT JOIN exercise_entries e ON e.workout_id = w.id
WHERE w.status = 'planned'
  AND w.scheduled_date < ?
  AND e.id IS NULL;

-- name: CountPendingWorkoutBlocks :one
-- Pending-block count for one workout, read inside the sweep
-- transaction before flipping so the tick can log how many blocks
-- moved alongside the workout.
SELECT COUNT(*) AS pending_count
FROM workout_blocks
WHERE workout_id = ? AND status = 'pending';

-- name: MarkPendingWorkoutBlocksSkipped :exec
-- Sweep body: flip every still-pending block on one workout to
-- skipped. done/skipped rows are untouched.
UPDATE workout_blocks SET status = 'skipped' WHERE workout_id = ? AND status = 'pending';
