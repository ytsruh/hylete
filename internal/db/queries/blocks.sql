-- Blocks are user-owned planned exercise groups. Every query is
-- scoped to user_id (via the blocks row) so a request can never read
-- or mutate another user's plan.

-- name: CreateBlock :one
INSERT INTO blocks (id, user_id, name, description, block_type, rounds, rest_seconds, time_cap_seconds, interval_seconds, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetBlock :one
SELECT * FROM blocks
WHERE id = ? AND user_id = ?;

-- name: ListBlocks :many
-- Newest first; rowid breaks created_at ties (same convention as
-- exercise_entries ordering).
SELECT * FROM blocks
WHERE user_id = ?
ORDER BY created_at DESC, rowid DESC;

-- name: ListBlocksWithItemCount :many
-- List view needs per-block item counts without N+1 queries.
SELECT b.*, COUNT(i.id) AS item_count FROM blocks b
LEFT JOIN block_items i ON i.block_id = b.id
WHERE b.user_id = ?
GROUP BY b.id
ORDER BY b.created_at DESC, b.rowid DESC;

-- name: UpdateBlock :exec
-- Overwrites the editable block fields and bumps updated_at.
-- Items are replaced separately via DeleteBlockItems + CreateBlockItem.
UPDATE blocks
SET name = ?,
    description = ?,
    block_type = ?,
    rounds = ?,
    rest_seconds = ?,
    time_cap_seconds = ?,
    interval_seconds = ?,
    updated_at = CURRENT_TIMESTAMP
WHERE id = ? AND user_id = ?;

-- name: DeleteBlock :exec
DELETE FROM blocks
WHERE id = ? AND user_id = ?;

-- name: CreateBlockItem :one
INSERT INTO block_items (id, block_id, exercise_id, position, target_text, created_at)
VALUES (?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: ListBlockItems :many
SELECT * FROM block_items
WHERE block_id = ?
ORDER BY position ASC;

-- name: ListBlockItemsWithExercise :many
-- Detail view resolves each planned item to its exercise name/type
-- in one query. Ownership is gated by the caller's prior GetBlock
-- (scoped to user_id); this query only orders the items.
SELECT i.id, i.block_id, i.exercise_id, i.position, i.target_text, i.created_at,
       e.name AS exercise_name, e.type AS exercise_type
FROM block_items i
JOIN exercises e ON e.id = i.exercise_id
WHERE i.block_id = ?
ORDER BY i.position ASC;

-- name: DeleteBlockItems :exec
-- Full item replacement on update: delete-all then re-insert in a
-- transaction (the repository owns the tx). Also keeps deletes
-- correct on databases ignoring ON DELETE CASCADE.
DELETE FROM block_items
WHERE block_id = ?;
