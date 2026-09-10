-- name: CreateWeightEntry :one
INSERT INTO weight_entries (id, user_id, weight, notes, front_photo_key, side_photo_key, back_photo_key, created_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetWeightEntry :one
SELECT * FROM weight_entries
WHERE id = ? AND user_id = ?;

-- name: ListWeightEntries :many
SELECT * FROM weight_entries
WHERE user_id = ?
ORDER BY created_at DESC;

-- name: UpdateWeightEntry :exec
UPDATE weight_entries
SET weight = ?, notes = ?, front_photo_key = ?, side_photo_key = ?, back_photo_key = ?, created_at = ?
WHERE id = ? AND user_id = ?;

-- name: DeleteWeightEntry :exec
DELETE FROM weight_entries
WHERE id = ? AND user_id = ?;

-- name: GetWeightEntriesByIDs :many
SELECT * FROM weight_entries
WHERE id IN (?, ?) AND user_id = ?;
