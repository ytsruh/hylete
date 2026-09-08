-- name: UpsertHealthSnapshot :one
INSERT INTO health_snapshots (
    id, user_id, snapshot_date, tz,
    steps, distance_meters, active_energy_kcal, basal_energy_kcal,
    exercise_minutes, sleep_seconds,
    weight, weight_measured_at,
    bmi, bmi_measured_at,
    body_fat_percentage, body_fat_measured_at,
    lean_body_mass, lean_mass_measured_at,
    heart_rate, heart_rate_measured_at,
    resting_heart_rate, resting_hr_measured_at,
    walking_heart_rate_avg, walking_hr_measured_at,
    hrv_ms, hrv_measured_at,
    cardio_recovery_bpm, cardio_recovery_measured_at,
    vo2_max, vo2_measured_at
) VALUES (
    ?, ?, ?, ?,
    ?, ?, ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?,
    ?, ?
)
ON CONFLICT(user_id, snapshot_date) DO UPDATE SET
    tz = excluded.tz,
    steps = excluded.steps,
    distance_meters = excluded.distance_meters,
    active_energy_kcal = excluded.active_energy_kcal,
    basal_energy_kcal = excluded.basal_energy_kcal,
    exercise_minutes = excluded.exercise_minutes,
    sleep_seconds = excluded.sleep_seconds,
    weight = excluded.weight,
    weight_measured_at = excluded.weight_measured_at,
    bmi = excluded.bmi,
    bmi_measured_at = excluded.bmi_measured_at,
    body_fat_percentage = excluded.body_fat_percentage,
    body_fat_measured_at = excluded.body_fat_measured_at,
    lean_body_mass = excluded.lean_body_mass,
    lean_mass_measured_at = excluded.lean_mass_measured_at,
    heart_rate = excluded.heart_rate,
    heart_rate_measured_at = excluded.heart_rate_measured_at,
    resting_heart_rate = excluded.resting_heart_rate,
    resting_hr_measured_at = excluded.resting_hr_measured_at,
    walking_heart_rate_avg = excluded.walking_heart_rate_avg,
    walking_hr_measured_at = excluded.walking_hr_measured_at,
    hrv_ms = excluded.hrv_ms,
    hrv_measured_at = excluded.hrv_measured_at,
    cardio_recovery_bpm = excluded.cardio_recovery_bpm,
    cardio_recovery_measured_at = excluded.cardio_recovery_measured_at,
    vo2_max = excluded.vo2_max,
    vo2_measured_at = excluded.vo2_measured_at,
    updated_at = CURRENT_TIMESTAMP
RETURNING *;

-- name: GetHealthSnapshotByDate :one
SELECT * FROM health_snapshots
WHERE user_id = ? AND snapshot_date = ?;

-- name: ListHealthSnapshotsRange :many
SELECT * FROM health_snapshots
WHERE user_id = ? AND snapshot_date >= ? AND snapshot_date <= ?
ORDER BY snapshot_date DESC;
