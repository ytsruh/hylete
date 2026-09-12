-- +goose Up
-- Logging spine for workouts: optional links from an exercise entry
-- into the workout tree. workout_id groups the exercise entry into a
-- dated workout; workout_item_id is the most precise link (which
-- planned item the exercise entry logged against); round_number records
-- which round of a multi-round block the exercise entry belongs to
-- (0 = unrounded / ad-hoc set inside the workout).
--
-- All three are nullable/zeroable so standalone logging is unchanged:
-- an exercise entry with no workout_id is a standalone set/session.
-- ON DELETE SET NULL so deleting a workout, block, or item never
-- deletes logged exercise entries -- they survive as standalone rows.
ALTER TABLE exercise_entries ADD COLUMN workout_id TEXT REFERENCES workouts(id) ON DELETE SET NULL;
ALTER TABLE exercise_entries ADD COLUMN workout_item_id TEXT REFERENCES workout_items(id) ON DELETE SET NULL;
ALTER TABLE exercise_entries ADD COLUMN round_number INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_entries_workout ON exercise_entries(workout_id);
CREATE INDEX IF NOT EXISTS idx_entries_workout_item ON exercise_entries(workout_item_id);

-- +goose Down
DROP INDEX IF EXISTS idx_entries_workout_item;
DROP INDEX IF EXISTS idx_entries_workout;
ALTER TABLE exercise_entries DROP COLUMN round_number;
ALTER TABLE exercise_entries DROP COLUMN workout_item_id;
ALTER TABLE exercise_entries DROP COLUMN workout_id;
