-- +goose Up
-- Comma-separated alternate names for an exercise (e.g. "seated row,cable row").
-- Never rendered visibly; only used for client-side filtering on the web
-- exercise lists and the iOS picker (matches the same way the name does:
-- case-insensitive substring). Edited in the admin exercise form.
ALTER TABLE exercises ADD COLUMN aliases TEXT NOT NULL DEFAULT '';

-- +goose Down
ALTER TABLE exercises DROP COLUMN aliases;
