-- +goose Up
-- Squashed initial schema (previously migrations 00001-00021).
-- One file declares the full final state: every CREATE is
-- IF NOT EXISTS so a re-run is a harmless no-op. Tables are
-- declared in dependency order (referenced tables first) for
-- readability; SQLite does not require this.
--
-- Deliberately omitted legacy artifacts:
--   - push_subscriptions + idx_push_subs_user (web-push stack removed)
--   - users.reminder_push_enabled (never read by any code path)
--   - ai_reports.read_at (dropped while the table was young)
--   - users.age (replaced by date_of_birth; a stored age goes stale)
--   - weight_entries.photo_key (renamed to front_photo_key)
CREATE TABLE IF NOT EXISTS exercises (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    aliases TEXT NOT NULL DEFAULT '',
    description TEXT,
    video_url TEXT,
    img_url TEXT,
    img_url_original TEXT,
    type TEXT NOT NULL DEFAULT 'other'
);

CREATE TABLE IF NOT EXISTS users (
    id TEXT PRIMARY KEY,
    name TEXT NOT NULL,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    is_admin INTEGER NOT NULL DEFAULT 0,
    target_weight REAL,
    weight_unit TEXT NOT NULL DEFAULT 'kg',
    distance_unit TEXT NOT NULL DEFAULT 'km',
    reminder_enabled        INTEGER NOT NULL DEFAULT 0,
    reminder_frequency      TEXT    NOT NULL DEFAULT 'weekly',
    reminder_day_of_week    INTEGER,
    reminder_time           TEXT    NOT NULL DEFAULT '09:00',
    reminder_email_enabled  INTEGER NOT NULL DEFAULT 1,
    reminder_next_fire_at   DATETIME,
    reminder_last_fired_at  DATETIME,
    ai_opt_in INTEGER NOT NULL DEFAULT 0,
    ai_goal_text TEXT NOT NULL DEFAULT '',
    height_cm REAL,
    gender TEXT NOT NULL DEFAULT '',
    date_of_birth TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
CREATE INDEX IF NOT EXISTS idx_users_reminder_due ON users(reminder_next_fire_at);

-- Blocks are user-owned planned exercise groups (Beta). block_items
-- rows are planned references to the exercise catalog with a
-- free-text target — not logged exercise entries (see below):
-- logged sets/sessions live in exercise_entries and feed history,
-- charts, and exports, while a plan has no metrics yet.
CREATE TABLE IF NOT EXISTS blocks (
    id               TEXT PRIMARY KEY,
    user_id          TEXT NOT NULL REFERENCES users(id),
    name             TEXT NOT NULL,
    description      TEXT NOT NULL DEFAULT '',
    block_type       TEXT NOT NULL DEFAULT 'standard',
    rounds           INTEGER NOT NULL DEFAULT 0,
    rest_seconds     INTEGER NOT NULL DEFAULT 0,
    time_cap_seconds INTEGER NOT NULL DEFAULT 0,
    interval_seconds INTEGER NOT NULL DEFAULT 0,
    created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_blocks_user ON blocks(user_id);

CREATE TABLE IF NOT EXISTS block_items (
    id          TEXT PRIMARY KEY,
    block_id    TEXT NOT NULL REFERENCES blocks(id) ON DELETE CASCADE,
    exercise_id TEXT NOT NULL REFERENCES exercises(id),
    position    INTEGER NOT NULL,
    target_text TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_block_items_block ON block_items(block_id, position);

-- Workouts are user-owned scheduled collections of blocks.
-- workout_blocks rows are ordered live references to blocks with
-- their own per-block status.
CREATE TABLE IF NOT EXISTS workouts (
    id             TEXT PRIMARY KEY,
    user_id        TEXT NOT NULL REFERENCES users(id),
    name           TEXT NOT NULL,
    description    TEXT NOT NULL DEFAULT '',
    scheduled_date TEXT NOT NULL,
    status         TEXT NOT NULL DEFAULT 'planned',
    created_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at     DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_workouts_user_date ON workouts(user_id, scheduled_date);

CREATE TABLE IF NOT EXISTS workout_blocks (
    id         TEXT PRIMARY KEY,
    workout_id TEXT NOT NULL REFERENCES workouts(id) ON DELETE CASCADE,
    block_id   TEXT NOT NULL REFERENCES blocks(id),
    position   INTEGER NOT NULL,
    status     TEXT NOT NULL DEFAULT 'pending',
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(workout_id, position)
);
CREATE INDEX IF NOT EXISTS idx_workout_blocks_workout ON workout_blocks(workout_id, position);
CREATE INDEX IF NOT EXISTS idx_workout_blocks_block ON workout_blocks(block_id);

CREATE TABLE IF NOT EXISTS exercise_entries (
    id TEXT PRIMARY KEY,
    exercise_id TEXT NOT NULL,
    user_id TEXT REFERENCES users(id),
    reps INTEGER NOT NULL,
    weight REAL NOT NULL,
    notes TEXT,
    rest_time INTEGER NOT NULL DEFAULT 0,
    -- Cardio metrics (0 = not recorded). Which metric pair applies is decided
    -- by the linked exercise's type: strength entries use reps/weight/rest_time,
    -- cardio entries use duration_seconds/distance_meters (+ optional HR/kcal).
    duration_seconds INTEGER NOT NULL DEFAULT 0,
    distance_meters REAL NOT NULL DEFAULT 0,
    avg_heart_rate INTEGER NOT NULL DEFAULT 0,
    calories_burned REAL NOT NULL DEFAULT 0,
    -- Workout Player attribution (all nullable; NULL = logged outside a
    -- workout). workout_id + block_id are stable across workout edits;
    -- workout_block_id is the precise join row but is nulled when
    -- UpdateWorkout regenerates join IDs. All ON DELETE SET NULL so
    -- deleting a workout/block never destroys logged history. Columns
    -- are plain TEXT (nullable by default); do not add an explicit
    -- NULL keyword - libsql misparses it as NOT NULL.
    workout_id TEXT REFERENCES workouts(id) ON DELETE SET NULL,
    block_id TEXT REFERENCES blocks(id) ON DELETE SET NULL,
    workout_block_id TEXT REFERENCES workout_blocks(id) ON DELETE SET NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (exercise_id) REFERENCES exercises(id)
);

CREATE INDEX IF NOT EXISTS idx_entries_exercise ON exercise_entries(exercise_id);
CREATE INDEX IF NOT EXISTS idx_entries_user ON exercise_entries(user_id);
CREATE INDEX IF NOT EXISTS idx_entries_created ON exercise_entries(created_at);
CREATE INDEX IF NOT EXISTS idx_entries_workout ON exercise_entries(user_id, workout_id);
CREATE INDEX IF NOT EXISTS idx_entries_block ON exercise_entries(block_id);

CREATE TABLE IF NOT EXISTS feedback (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    is_closed INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_feedback_user ON feedback(user_id);
CREATE INDEX IF NOT EXISTS idx_feedback_created ON feedback(created_at);
CREATE INDEX IF NOT EXISTS idx_feedback_closed ON feedback(is_closed);

-- Weight entries carry three angle slots (front / side / back);
-- every legacy single photo is front-facing by convention.
CREATE TABLE IF NOT EXISTS weight_entries (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    weight REAL NOT NULL,
    notes TEXT,
    front_photo_key TEXT,
    side_photo_key TEXT,
    back_photo_key TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_weight_entries_user ON weight_entries(user_id);
CREATE INDEX IF NOT EXISTS idx_weight_entries_created ON weight_entries(created_at);

-- Daily Apple Health snapshots (one row per user per calendar
-- day, upserted by the iOS client). Units are canonical,
-- mirroring the exercise_entries convention: distances in
-- metres, energy in kilocalories, heart rates in bpm, HRV in
-- milliseconds, VO2max in ml/kg/min, weight/lean mass as
-- unitless numbers labelled at render time. All metric columns
-- are NOT NULL DEFAULT 0 ("0 means not recorded").
--
-- The ten *_measured_at columns are the deliberate exception:
-- NULL means "no observation that day" (value carried forward
-- from an earlier sample or never measured). snapshot_date is
-- the device-local calendar date (YYYY-MM-DD) plus the device
-- timezone, because HealthKit day boundaries are local.
CREATE TABLE IF NOT EXISTS health_snapshots (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    snapshot_date TEXT NOT NULL,
    tz         TEXT NOT NULL DEFAULT '',
    steps      INTEGER NOT NULL DEFAULT 0,
    distance_meters REAL NOT NULL DEFAULT 0,
    active_energy_kcal REAL NOT NULL DEFAULT 0,
    basal_energy_kcal REAL NOT NULL DEFAULT 0,
    exercise_minutes REAL NOT NULL DEFAULT 0,
    sleep_seconds REAL NOT NULL DEFAULT 0,
    weight REAL NOT NULL DEFAULT 0,
    weight_measured_at DATETIME,
    bmi REAL NOT NULL DEFAULT 0,
    bmi_measured_at DATETIME,
    body_fat_percentage REAL NOT NULL DEFAULT 0,
    body_fat_measured_at DATETIME,
    lean_body_mass REAL NOT NULL DEFAULT 0,
    lean_mass_measured_at DATETIME,
    heart_rate REAL NOT NULL DEFAULT 0,
    heart_rate_measured_at DATETIME,
    resting_heart_rate REAL NOT NULL DEFAULT 0,
    resting_hr_measured_at DATETIME,
    walking_heart_rate_avg REAL NOT NULL DEFAULT 0,
    walking_hr_measured_at DATETIME,
    hrv_ms REAL NOT NULL DEFAULT 0,
    hrv_measured_at DATETIME,
    cardio_recovery_bpm REAL NOT NULL DEFAULT 0,
    cardio_recovery_measured_at DATETIME,
    vo2_max REAL NOT NULL DEFAULT 0,
    vo2_measured_at DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, snapshot_date)
);
CREATE INDEX IF NOT EXISTS idx_health_snapshots_user ON health_snapshots(user_id);
CREATE INDEX IF NOT EXISTS idx_health_snapshots_date ON health_snapshots(snapshot_date);

-- Auth tokens: short-lived, single-use credentials used to verify
-- ownership of an email address. The application stores only the
-- sha256 hash; the raw token is only ever embedded in the email
-- link. Consumption is a single UPDATE guarded on
-- (purpose, token_hash, unused, not expired).
CREATE TABLE IF NOT EXISTS auth_tokens (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    purpose    TEXT NOT NULL,
    token_hash TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at    DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS idx_auth_tokens_lookup ON auth_tokens(token_hash, purpose);
CREATE INDEX IF NOT EXISTS idx_auth_tokens_user ON auth_tokens(user_id);

-- Goals are todo-style records with optional start, target, and end dates.
-- completed_at is nullable; when set, the goal is considered complete.
CREATE TABLE IF NOT EXISTS goals (
    id           TEXT PRIMARY KEY,
    user_id      TEXT NOT NULL REFERENCES users(id),
    title        TEXT NOT NULL,
    description  TEXT,
    start_date   DATETIME,
    target_date  DATETIME,
    end_date     DATETIME,
    completed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX IF NOT EXISTS idx_goals_user ON goals(user_id);
-- The list page filters active vs completed in two separate queries;
-- this index supports both.
CREATE INDEX IF NOT EXISTS idx_goals_completed ON goals(user_id, completed_at);

-- Coach (user-facing name) weekly reports and future insight kinds.
-- kind is stored in the "type" column: 'weekly' now; 'insight' and
-- 'monthly' reuse this table later with no new migrations for
-- storage. payload_json is the validated report JSON produced from
-- the versioned prompt in internal/ai/prompts/weekly_review.md.
CREATE TABLE IF NOT EXISTS ai_reports (
    id         TEXT PRIMARY KEY,
    user_id    TEXT NOT NULL REFERENCES users(id),
    type       TEXT NOT NULL DEFAULT 'weekly' CHECK(type IN ('weekly', 'insight', 'monthly')),
    period_start DATE NOT NULL,
    period_end   DATE NOT NULL,
    prompt_version TEXT NOT NULL DEFAULT '',
    model        TEXT NOT NULL DEFAULT '',
    payload_json TEXT NOT NULL DEFAULT '{}',
    tokens_in    INTEGER NOT NULL DEFAULT 0,
    tokens_out   INTEGER NOT NULL DEFAULT 0,
    dismissed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_ai_reports_user_type_period ON ai_reports(user_id, type, period_start);
CREATE INDEX IF NOT EXISTS idx_ai_reports_user ON ai_reports(user_id, type, period_start DESC);

-- +goose Down
DROP INDEX IF EXISTS idx_ai_reports_user;
DROP INDEX IF EXISTS idx_ai_reports_user_type_period;
DROP TABLE IF EXISTS ai_reports;
DROP INDEX IF EXISTS idx_goals_completed;
DROP INDEX IF EXISTS idx_goals_user;
DROP TABLE IF EXISTS goals;
DROP INDEX IF EXISTS idx_auth_tokens_user;
DROP INDEX IF EXISTS idx_auth_tokens_lookup;
DROP TABLE IF EXISTS auth_tokens;
DROP INDEX IF EXISTS idx_health_snapshots_date;
DROP INDEX IF EXISTS idx_health_snapshots_user;
DROP TABLE IF EXISTS health_snapshots;
DROP INDEX IF EXISTS idx_weight_entries_created;
DROP INDEX IF EXISTS idx_weight_entries_user;
DROP TABLE IF EXISTS weight_entries;
DROP INDEX IF EXISTS idx_feedback_closed;
DROP INDEX IF EXISTS idx_feedback_created;
DROP INDEX IF EXISTS idx_feedback_user;
DROP TABLE IF EXISTS feedback;
DROP INDEX IF EXISTS idx_entries_block;
DROP INDEX IF EXISTS idx_entries_workout;
DROP INDEX IF EXISTS idx_entries_created;
DROP INDEX IF EXISTS idx_entries_user;
DROP INDEX IF EXISTS idx_entries_exercise;
DROP TABLE IF EXISTS exercise_entries;
DROP INDEX IF EXISTS idx_workout_blocks_block;
DROP INDEX IF EXISTS idx_workout_blocks_workout;
DROP TABLE IF EXISTS workout_blocks;
DROP INDEX IF EXISTS idx_workouts_user_date;
DROP TABLE IF EXISTS workouts;
DROP INDEX IF EXISTS idx_block_items_block;
DROP TABLE IF EXISTS block_items;
DROP INDEX IF EXISTS idx_blocks_user;
DROP TABLE IF EXISTS blocks;
DROP INDEX IF EXISTS idx_users_reminder_due;
DROP INDEX IF EXISTS idx_users_email;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS exercises;
