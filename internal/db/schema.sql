CREATE TABLE exercises (
    id TEXT PRIMARY KEY,
    name TEXT UNIQUE NOT NULL,
    aliases TEXT NOT NULL DEFAULT '',
    description TEXT,
    video_url TEXT,
    img_url TEXT,
    img_url_original TEXT,
    type TEXT NOT NULL DEFAULT 'other'
);

CREATE TABLE users (
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
    reminder_push_enabled   INTEGER NOT NULL DEFAULT 1,
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

CREATE TABLE exercise_entries (
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
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (exercise_id) REFERENCES exercises(id)
);

CREATE INDEX idx_entries_exercise ON exercise_entries(exercise_id);
CREATE INDEX idx_entries_user ON exercise_entries(user_id);
CREATE INDEX idx_entries_created ON exercise_entries(created_at);
CREATE INDEX idx_users_email ON users(email);

CREATE TABLE feedback (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    title TEXT NOT NULL,
    message TEXT NOT NULL,
    is_closed INTEGER NOT NULL DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_feedback_user ON feedback(user_id);
CREATE INDEX idx_feedback_created ON feedback(created_at);
CREATE INDEX idx_feedback_closed ON feedback(is_closed);

CREATE TABLE weight_entries (
    id TEXT PRIMARY KEY,
    user_id TEXT NOT NULL REFERENCES users(id),
    weight REAL NOT NULL,
    notes TEXT,
    front_photo_key TEXT,
    side_photo_key TEXT,
    back_photo_key TEXT,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_weight_entries_user    ON weight_entries(user_id);
CREATE INDEX idx_weight_entries_created ON weight_entries(created_at);

-- Daily Apple Health snapshots (one row per user per calendar
-- day, upserted by the iOS client). See migration
-- 00011_add_health_snapshots.sql for the units contract and
-- the measured_at NULL semantics.
CREATE TABLE health_snapshots (
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

CREATE INDEX idx_health_snapshots_user ON health_snapshots(user_id);
CREATE INDEX idx_health_snapshots_date ON health_snapshots(snapshot_date);

CREATE TABLE push_subscriptions (
    id           TEXT     PRIMARY KEY,
    user_id      TEXT     NOT NULL REFERENCES users(id),
    endpoint     TEXT     UNIQUE NOT NULL,
    p256dh       TEXT     NOT NULL,
    auth         TEXT     NOT NULL,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_seen_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_push_subs_user ON push_subscriptions(user_id);

CREATE TABLE auth_tokens (
    id         TEXT     PRIMARY KEY,
    user_id    TEXT     NOT NULL REFERENCES users(id),
    purpose    TEXT     NOT NULL,
    token_hash TEXT     NOT NULL,
    expires_at DATETIME NOT NULL,
    used_at    DATETIME,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_auth_tokens_lookup ON auth_tokens(token_hash, purpose);
CREATE INDEX idx_auth_tokens_user ON auth_tokens(user_id);

CREATE TABLE goals (
    id           TEXT     PRIMARY KEY,
    user_id      TEXT     NOT NULL REFERENCES users(id),
    title        TEXT     NOT NULL,
    description  TEXT,
    start_date   DATETIME,
    target_date  DATETIME,
    end_date     DATETIME,
    completed_at DATETIME,
    created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_goals_user      ON goals(user_id);
CREATE INDEX idx_goals_completed ON goals(user_id, completed_at);

CREATE TABLE ai_reports (
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
CREATE UNIQUE INDEX idx_ai_reports_user_type_period ON ai_reports(user_id, type, period_start);
CREATE INDEX idx_ai_reports_user ON ai_reports(user_id, type, period_start DESC);

-- Blocks are user-owned planned exercise groups (Beta). block_items
-- rows are planned references to the exercise catalog with a
-- free-text target — not logged exercise entries (see migration
-- 00019_add_blocks.sql for the rationale).
CREATE TABLE blocks (
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
CREATE INDEX idx_blocks_user ON blocks(user_id);

CREATE TABLE block_items (
    id          TEXT PRIMARY KEY,
    block_id    TEXT NOT NULL REFERENCES blocks(id) ON DELETE CASCADE,
    exercise_id TEXT NOT NULL REFERENCES exercises(id),
    position    INTEGER NOT NULL,
    target_text TEXT NOT NULL DEFAULT '',
    created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_block_items_block ON block_items(block_id, position);