-- name: Create :one
INSERT INTO exercises (id, name, aliases, description, video_url, img_url, img_url_original, type)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)
RETURNING id;

-- name: GetByName :one
SELECT id, name, aliases, description, video_url, img_url, img_url_original, type
FROM exercises
WHERE name = ?;

-- name: List :many
SELECT id, name, aliases, description, video_url, img_url, img_url_original, type
FROM exercises
ORDER BY name;

-- name: GetByID :one
SELECT id, name, aliases, description, video_url, img_url, img_url_original, type
FROM exercises
WHERE id = ?;

-- name: Update :one
UPDATE exercises
SET name = ?, aliases = ?, description = ?, video_url = ?, img_url = ?, img_url_original = ?, type = ?
WHERE id = ?
RETURNING id, name, aliases, description, video_url, img_url, img_url_original, type;
