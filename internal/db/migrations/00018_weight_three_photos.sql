-- +goose Up
-- Split the single weight-entry photo into three angle slots
-- (front / side / back). Every existing photo is assumed to be
-- front-facing, so the rename preserves all data with no backfill.
ALTER TABLE weight_entries RENAME COLUMN photo_key TO front_photo_key;
ALTER TABLE weight_entries ADD COLUMN side_photo_key TEXT;
ALTER TABLE weight_entries ADD COLUMN back_photo_key TEXT;

-- +goose Down
ALTER TABLE weight_entries DROP COLUMN back_photo_key;
ALTER TABLE weight_entries DROP COLUMN side_photo_key;
ALTER TABLE weight_entries RENAME COLUMN front_photo_key TO photo_key;
