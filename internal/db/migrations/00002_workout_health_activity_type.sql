-- +goose Up
-- Apple Health activity type for a workout (HKWorkoutActivityType
-- case-name key, e.g. 'traditionalStrengthTraining'). Blanket
-- default: this is a new feature and every existing workout
-- records as Strength until re-picked in the editor (accepted
-- risk — see plan). ADD COLUMN with a NOT NULL DEFAULT backfills
-- existing rows automatically; the iOS client falls back to
-- block-mix inference for empty/unknown values as a safety net.
ALTER TABLE workouts ADD COLUMN health_activity_type TEXT NOT NULL DEFAULT 'traditionalStrengthTraining';

-- +goose Down
-- SQLite cannot drop a column without a table rebuild; the Down
-- path recreates workouts minus the column. Data in other columns
-- is preserved.
CREATE TABLE workouts_without_health_type (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id),
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    scheduled_date TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'planned',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
INSERT INTO workouts_without_health_type
    (id, user_id, name, description, scheduled_date, status, created_at, updated_at)
    SELECT id, user_id, name, description, scheduled_date, status, created_at, updated_at
    FROM workouts;
DROP TABLE workouts;
ALTER TABLE workouts_without_health_type RENAME TO workouts;
CREATE INDEX IF NOT EXISTS idx_workouts_user_date ON workouts(user_id, scheduled_date);
