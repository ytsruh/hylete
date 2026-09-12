-- +goose Up
-- Workouts v1: dated workout containers users plan ahead and log
-- exercise entries into.
--
-- workouts (+ workout_blocks + workout_items) are dated containers.
-- workouts.source_workout_id records provenance when a workout was
-- copied from another workout (duplicate / plan-ahead copies, SET
-- NULL on source delete) and is NULL for workouts authored from
-- scratch. Copies are snapshots: editing or deleting the source
-- never touches existing copies or logged exercise entries.
--
-- Block "type" (straight | superset | circuit | emom | amrap) is a
-- free-form TEXT column validated at the API layer, matching how
-- exercises.type is handled. Target columns on workout_items carry
-- both metric pairs (strength: target_sets/reps/weight/rest;
-- cardio: target_duration/distance (+ HR/calories)); which pair
-- applies is decided by the linked exercise's type and the server
-- zeroes the pair that does not apply. All targets are optional: an
-- item may be just a linked exercise with all-zero targets (an open
-- prescription).
CREATE TABLE workouts (
    id                TEXT PRIMARY KEY,
    user_id           TEXT NOT NULL REFERENCES users(id),
    source_workout_id TEXT REFERENCES workouts(id) ON DELETE SET NULL,
    name              TEXT NOT NULL,
    notes             TEXT,
    status            TEXT NOT NULL DEFAULT 'planned',
    scheduled_start   DATETIME,
    scheduled_end     DATETIME,
    completed_at      DATETIME,
    created_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_workouts_user ON workouts(user_id);
CREATE INDEX idx_workouts_user_status ON workouts(user_id, status);
CREATE INDEX idx_workouts_user_scheduled ON workouts(user_id, scheduled_start);

CREATE TABLE workout_blocks (
    id                           TEXT PRIMARY KEY,
    workout_id                   TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    type                         TEXT NOT NULL DEFAULT 'straight',
    position                     INTEGER NOT NULL DEFAULT 0,
    rounds                       INTEGER NOT NULL DEFAULT 1,
    rest_between_rounds_seconds  INTEGER NOT NULL DEFAULT 0,
    interval_seconds             INTEGER NOT NULL DEFAULT 0,
    time_cap_seconds             INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX idx_workout_blocks_workout ON workout_blocks(workout_id, position);

CREATE TABLE workout_items (
    id                      TEXT PRIMARY KEY,
    block_id                TEXT NOT NULL REFERENCES workout_blocks(id) ON DELETE CASCADE,
    exercise_id             TEXT NOT NULL REFERENCES exercises(id),
    position                INTEGER NOT NULL DEFAULT 0,
    target_sets             INTEGER NOT NULL DEFAULT 0,
    target_reps             INTEGER NOT NULL DEFAULT 0,
    target_weight           REAL NOT NULL DEFAULT 0,
    target_rest_seconds     INTEGER NOT NULL DEFAULT 0,
    target_duration_seconds INTEGER NOT NULL DEFAULT 0,
    target_distance_meters  REAL NOT NULL DEFAULT 0,
    target_avg_heart_rate   INTEGER NOT NULL DEFAULT 0,
    target_calories         REAL NOT NULL DEFAULT 0
);
CREATE INDEX idx_workout_items_block ON workout_items(block_id, position);
CREATE INDEX idx_workout_items_exercise ON workout_items(exercise_id);

-- +goose Down
DROP INDEX IF EXISTS idx_workout_items_exercise;
DROP INDEX IF EXISTS idx_workout_items_block;
DROP TABLE IF EXISTS workout_items;
DROP INDEX IF EXISTS idx_workout_blocks_workout;
DROP TABLE IF EXISTS workout_blocks;
DROP INDEX IF EXISTS idx_workouts_user_scheduled;
DROP INDEX IF EXISTS idx_workouts_user_status;
DROP INDEX IF EXISTS idx_workouts_user;
DROP TABLE IF EXISTS workouts;
