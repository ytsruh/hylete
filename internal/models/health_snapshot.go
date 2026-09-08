package models

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"hylete/internal/db"

	"github.com/google/uuid"
)

// HealthSnapshot is one day of Apple Health vitals for a user,
// uploaded read-only from the iOS client (the server never pulls
// HealthKit — that data lives on-device). One row per
// (user_id, snapshot_date); re-uploads overwrite via upsert so
// retries, backfills, and late Watch corrections are safe.
//
// Units are canonical: distances in metres, energy in
// kilocalories, heart rates in bpm, HRV in milliseconds, VO2max
// in ml/kg/min, weight/lean mass as unitless numbers labelled
// at render time. Zero means "not recorded".
//
// The ten MeasuredAt fields are timestamps of the actual
// HealthKit observation behind a latest-type value (nil when
// the value was carried forward from an earlier sample or never
// measured). Analysis must treat a value as an observation only
// when its MeasuredAt falls inside the snapshot date. Totals
// (steps, distance, energies, exercise, sleep) need no
// timestamp — they are inherently measured that day.
type HealthSnapshot struct {
	ID                   string
	UserID               string
	SnapshotDate         string
	Tz                   string
	Steps                int64
	DistanceMeters       float64
	ActiveEnergyKcal     float64
	BasalEnergyKcal      float64
	ExerciseMinutes      float64
	SleepSeconds         float64
	Weight               float64
	WeightMeasuredAt     *time.Time
	BMI                  float64
	BMIMeasuredAt        *time.Time
	BodyFatPercentage    float64
	BodyFatMeasuredAt    *time.Time
	LeanBodyMass         float64
	LeanMassMeasuredAt   *time.Time
	HeartRate            float64
	HeartRateMeasuredAt  *time.Time
	RestingHeartRate     float64
	RestingHRMeasuredAt  *time.Time
	WalkingHeartRateAvg  float64
	WalkingHRMeasuredAt  *time.Time
	HRV                  float64
	HRVMeasuredAt        *time.Time
	CardioRecoveryBPM    float64
	CardioRecoveryAt     *time.Time
	VO2Max               float64
	VO2MeasuredAt        *time.Time
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// HealthSnapshotRepository persists health snapshots using
// sqlc-generated queries.
type HealthSnapshotRepository struct {
	db      *db.DB
	queries *db.Queries
}

// NewHealthSnapshotRepository creates a new health snapshot
// repository backed by sqlc.
func NewHealthSnapshotRepository(dbConn *db.DB) *HealthSnapshotRepository {
	return &HealthSnapshotRepository{
		db:      dbConn,
		queries: db.New(dbConn.Conn()),
	}
}

// Upsert inserts a snapshot or replaces the row for the same
// (user_id, snapshot_date) when one already exists. The ID is
// freshly generated per call but ignored on conflict, so
// callers can blindly re-upload backfills and corrections.
func (r *HealthSnapshotRepository) Upsert(entry *HealthSnapshot) error {
	ctx := context.Background()
	row, err := r.queries.UpsertHealthSnapshot(ctx, db.UpsertHealthSnapshotParams{
		ID:                       uuid.New().String(),
		UserID:                   entry.UserID,
		SnapshotDate:             entry.SnapshotDate,
		Tz:                       entry.Tz,
		Steps:                    entry.Steps,
		DistanceMeters:           entry.DistanceMeters,
		ActiveEnergyKcal:         entry.ActiveEnergyKcal,
		BasalEnergyKcal:          entry.BasalEnergyKcal,
		ExerciseMinutes:          entry.ExerciseMinutes,
		SleepSeconds:             entry.SleepSeconds,
		Weight:                   entry.Weight,
		WeightMeasuredAt:         timePtrToNullTime(entry.WeightMeasuredAt),
		Bmi:                      entry.BMI,
		BmiMeasuredAt:            timePtrToNullTime(entry.BMIMeasuredAt),
		BodyFatPercentage:        entry.BodyFatPercentage,
		BodyFatMeasuredAt:        timePtrToNullTime(entry.BodyFatMeasuredAt),
		LeanBodyMass:             entry.LeanBodyMass,
		LeanMassMeasuredAt:       timePtrToNullTime(entry.LeanMassMeasuredAt),
		HeartRate:                entry.HeartRate,
		HeartRateMeasuredAt:      timePtrToNullTime(entry.HeartRateMeasuredAt),
		RestingHeartRate:         entry.RestingHeartRate,
		RestingHrMeasuredAt:      timePtrToNullTime(entry.RestingHRMeasuredAt),
		WalkingHeartRateAvg:      entry.WalkingHeartRateAvg,
		WalkingHrMeasuredAt:      timePtrToNullTime(entry.WalkingHRMeasuredAt),
		HrvMs:                    entry.HRV,
		HrvMeasuredAt:            timePtrToNullTime(entry.HRVMeasuredAt),
		CardioRecoveryBpm:        entry.CardioRecoveryBPM,
		CardioRecoveryMeasuredAt: timePtrToNullTime(entry.CardioRecoveryAt),
		Vo2Max:                   entry.VO2Max,
		Vo2MeasuredAt:            timePtrToNullTime(entry.VO2MeasuredAt),
	})
	if err != nil {
		return fmt.Errorf("failed to upsert health snapshot: %w", err)
	}
	mapped := mapHealthSnapshotRow(row)
	*entry = *mapped
	return nil
}

// GetByDate retrieves a snapshot for a user on a device-local
// calendar date (YYYY-MM-DD). Returns nil when none exists.
func (r *HealthSnapshotRepository) GetByDate(userID, snapshotDate string) (*HealthSnapshot, error) {
	ctx := context.Background()
	row, err := r.queries.GetHealthSnapshotByDate(ctx, db.GetHealthSnapshotByDateParams{
		UserID:       userID,
		SnapshotDate: snapshotDate,
	})
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to get health snapshot: %w", err)
	}
	return mapHealthSnapshotRow(row), nil
}

// ListRange returns a user's snapshots within an inclusive
// device-local date range, newest first.
func (r *HealthSnapshotRepository) ListRange(userID, startDate, endDate string) ([]HealthSnapshot, error) {
	ctx := context.Background()
	rows, err := r.queries.ListHealthSnapshotsRange(ctx, db.ListHealthSnapshotsRangeParams{
		UserID:       userID,
		SnapshotDate: startDate,
		SnapshotDate_2: endDate,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list health snapshots: %w", err)
	}
	out := make([]HealthSnapshot, len(rows))
	for i, row := range rows {
		out[i] = *mapHealthSnapshotRow(row)
	}
	return out, nil
}

// --- Mapping helpers ---

func mapHealthSnapshotRow(row db.HealthSnapshot) *HealthSnapshot {
	return &HealthSnapshot{
		ID:                  row.ID,
		UserID:              row.UserID,
		SnapshotDate:        row.SnapshotDate,
		Tz:                  row.Tz,
		Steps:               row.Steps,
		DistanceMeters:      row.DistanceMeters,
		ActiveEnergyKcal:    row.ActiveEnergyKcal,
		BasalEnergyKcal:     row.BasalEnergyKcal,
		ExerciseMinutes:     row.ExerciseMinutes,
		SleepSeconds:        row.SleepSeconds,
		Weight:              row.Weight,
		WeightMeasuredAt:    nullTimeToTimePtr(row.WeightMeasuredAt),
		BMI:                 row.Bmi,
		BMIMeasuredAt:       nullTimeToTimePtr(row.BmiMeasuredAt),
		BodyFatPercentage:   row.BodyFatPercentage,
		BodyFatMeasuredAt:   nullTimeToTimePtr(row.BodyFatMeasuredAt),
		LeanBodyMass:        row.LeanBodyMass,
		LeanMassMeasuredAt:  nullTimeToTimePtr(row.LeanMassMeasuredAt),
		HeartRate:           row.HeartRate,
		HeartRateMeasuredAt: nullTimeToTimePtr(row.HeartRateMeasuredAt),
		RestingHeartRate:    row.RestingHeartRate,
		RestingHRMeasuredAt: nullTimeToTimePtr(row.RestingHrMeasuredAt),
		WalkingHeartRateAvg: row.WalkingHeartRateAvg,
		WalkingHRMeasuredAt: nullTimeToTimePtr(row.WalkingHrMeasuredAt),
		HRV:                 row.HrvMs,
		HRVMeasuredAt:       nullTimeToTimePtr(row.HrvMeasuredAt),
		CardioRecoveryBPM:   row.CardioRecoveryBpm,
		CardioRecoveryAt:    nullTimeToTimePtr(row.CardioRecoveryMeasuredAt),
		VO2Max:              row.Vo2Max,
		VO2MeasuredAt:       nullTimeToTimePtr(row.Vo2MeasuredAt),
		CreatedAt:           row.CreatedAt,
		UpdatedAt:           row.UpdatedAt,
	}
}
