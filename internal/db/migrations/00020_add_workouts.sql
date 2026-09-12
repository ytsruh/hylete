-- +goose Up
-- Workouts are user-owned planned sessions (Beta). A workout has a
-- title, optional description, and 1-20 blocks in workout_blocks
-- (many-to-many over blocks: one block can appear in many
-- workouts, and the same block may repeat inside one workout so
-- no UNIQUE(workout_id, block_id) is enforced - ordering is by
-- position only).
--
-- Scheduling is a separate concern: workout_assignments holds one
-- row per planned calendar day (date-only YYYY-MM-DD text, not an
-- instant - day boundaries are computed client-side like the
-- exercise-entries range endpoint). Repeats are expanded into
-- individual rows at creation time so each day stays independently
-- movable/deletable with no recurrence engine. user_id is
-- denormalized onto assignments so calendar range queries stay
-- user-scoped without joining workouts.
CREATE TABLE workouts (
    id          TEXT PRIMARY KEY,
    user_id     TEXT NOT NULL REFERENCES users(id),
    title       TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workouts_user ON workouts(user_id);

CREATE TABLE workout_blocks (
    id         TEXT PRIMARY KEY,
    workout_id TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    block_id   TEXT NOT NULL REFERENCES blocks(id) ON DELETE CASCADE,
    position   INTEGER NOT NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workout_blocks_workout ON workout_blocks(workout_id, position);

CREATE TABLE workout_assignments (
    id             TEXT PRIMARY KEY,
    workout_id     TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    user_id        TEXT NOT NULL REFERENCES users(id),
    scheduled_date TEXT NOT NULL,
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workout_id, scheduled_date)
);
CREATE INDEX idx_workout_assignments_user_date ON workout_assignments(user_id, scheduled_date);

-- +goose Down
DROP INDEX IF EXISTS idx_workout_assignments_user_date;
DROP TABLE IF EXISTS workout_assignments;
DROP INDEX IF EXISTS idx_workout_blocks_workout;
DROP TABLE IF EXISTS workout_blocks;
DROP INDEX IF EXISTS idx_workouts_user;
DROP TABLE IF EXISTS workouts;
