-- +goose Up
-- Workout Player logging links. Each exercise entry logged from a
-- workout player carries stable attribution back to the workout and
-- block it came from:
--   - workout_id: the scheduled workout (stable, survives edits).
--   - block_id: the catalogue block (stable across workout edits,
--     which delete-all + re-insert workout_blocks join rows).
--   - workout_block_id: the precise workout_blocks join row
--     (fragile - nulled when UpdateWorkout regenerates join IDs;
--     block_id remains as the fallback).
-- All nullable so historic rows stay valid; all ON DELETE SET NULL
-- so deleting a workout/block never destroys logged history.
ALTER TABLE exercise_entries ADD COLUMN workout_id TEXT REFERENCES workouts(id) ON DELETE SET NULL;
ALTER TABLE exercise_entries ADD COLUMN block_id TEXT REFERENCES blocks(id) ON DELETE SET NULL;
ALTER TABLE exercise_entries ADD COLUMN workout_block_id TEXT REFERENCES workout_blocks(id) ON DELETE SET NULL;
CREATE INDEX idx_entries_workout ON exercise_entries(user_id, workout_id);
CREATE INDEX idx_entries_block ON exercise_entries(block_id);

-- +goose Down
DROP INDEX IF EXISTS idx_entries_block;
DROP INDEX IF EXISTS idx_entries_workout;
ALTER TABLE exercise_entries DROP COLUMN workout_block_id;
ALTER TABLE exercise_entries DROP COLUMN block_id;
ALTER TABLE exercise_entries DROP COLUMN workout_id;
