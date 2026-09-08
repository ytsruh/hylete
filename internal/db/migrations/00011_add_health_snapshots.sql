-- +goose Up
-- Daily Apple Health snapshots uploaded read-only from the iOS
-- client (one row per user per calendar day). The phone owns
-- the data — HealthKit lives on-device — so the server never
-- pulls; the client POSTs snapshots and this table keeps one
-- current row per (user_id, snapshot_date) via upsert.
--
-- Units are canonical, mirroring the exercise_entries
-- convention: distances in metres, energy in kilocalories,
-- heart rates in bpm, HRV in milliseconds, VO2max in
-- ml/kg/min, weight/lean mass as unitless numbers labelled at
-- render time. All metric columns are NOT NULL DEFAULT 0
-- ("0 means not recorded").
--
-- The ten *_measured_at columns are the deliberate exception:
-- NULL means "no observation that day" (value carried forward
-- from an earlier sample or never measured). A magic-zero date
-- would corrupt exactly the analysis this table exists to
-- serve, so absence stays NULL. snapshot_date is the
-- device-local calendar date (YYYY-MM-DD) plus the device
-- timezone, because HealthKit day boundaries are local.
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

-- +goose Down
DROP INDEX IF EXISTS idx_health_snapshots_date;
DROP INDEX IF EXISTS idx_health_snapshots_user;
DROP TABLE IF EXISTS health_snapshots;
