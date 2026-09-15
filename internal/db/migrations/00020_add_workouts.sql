-- +goose Up
-- Workouts are user-owned scheduled collections of blocks. A workout
-- has a name, optional description, a device-local scheduled date
-- (YYYY-MM-DD, same convention as health_snapshots.snapshot_date),
-- and an overall status. The workout_blocks join holds ordered live
-- references to blocks (not snapshots): editing a block propagates
-- to every workout referencing it.
--
-- workout_blocks rows carry their own per-block completion status
-- (pending|done|skipped); the workout-level status is set explicitly
-- by the client and is never derived server-side in V1.
CREATE TABLE workouts (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id),
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    scheduled_date TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'planned',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workouts_user_date ON workouts(user_id, scheduled_date);

CREATE TABLE workout_blocks (
    id         TEXT PRIMARY KEY,
    workout_id TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    block_id   TEXT NOT NULL REFERENCES blocks(id),
    position   INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workout_id, position)
);
CREATE INDEX idx_workout_blocks_workout ON workout_blocks(workout_id, position);
CREATE INDEX idx_workout_blocks_block ON workout_blocks(block_id);

-- +goose Down
DROP INDEX IF EXISTS idx_workout_blocks_block;
DROP INDEX IF EXISTS idx_workout_blocks_workout;
DROP TABLE IF EXISTS workout_blocks;
DROP INDEX IF EXISTS idx_workouts_user_date;
DROP TABLE IF EXISTS workouts;
