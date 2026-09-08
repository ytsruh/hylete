package controllers

import (
	"errors"
	"time"

	"hylete/internal/models"
)

// Health snapshot validation sentinels. These are client
// mistakes (bad dates, oversized batches) so routes map them to
// 400 rather than 500. Metric range bounds live on the DTO's
// validator tags instead — see UpsertHealthSnapshotsRequest.
var (
	// ErrHealthSnapshotDateInvalid is returned when a snapshot
	// date is not a YYYY-MM-DD calendar date.
	ErrHealthSnapshotDateInvalid = errors.New("snapshot_date must be a YYYY-MM-DD date")

	// ErrHealthSnapshotDateFuture is returned when a snapshot
	// date is more than two days ahead of the server clock. The
	// tolerance covers device-local dates east of the server
	// (up to UTC+14) plus a day of clock skew — anything beyond
	// that is almost certainly a client bug.
	ErrHealthSnapshotDateFuture = errors.New("snapshot_date must not be in the future")

	// ErrHealthSnapshotBatchTooLarge is returned when a single
	// upsert batch exceeds maxHealthSnapshotBatch. The 90-day
	// backfill plus headroom fits comfortably; larger batches
	// are chunked client-side.
	ErrHealthSnapshotBatchTooLarge = errors.New("too many snapshots in one request")
)

// maxHealthSnapshotBatch caps the snapshots array per upsert
// request. Bounds the transaction so a hostile or buggy client
// can't pile unbounded work onto one request.
const maxHealthSnapshotBatch = 120

// HealthSnapshotController handles Apple Health snapshot
// business logic: idempotent daily upserts and range reads.
type HealthSnapshotController struct {
	repo models.HealthSnapshotRepo
}

// NewHealthSnapshotController creates a new HealthSnapshotController.
// The repo is accepted as an interface so tests can substitute
// an in-memory fake; production wiring is done in main.go.
func NewHealthSnapshotController(repo models.HealthSnapshotRepo) *HealthSnapshotController {
	return &HealthSnapshotController{repo: repo}
}

// HealthSnapshotInput is one snapshot in an upsert batch.
// Metric units are canonical (metres, kcal, bpm, ms, ml/kg/min,
// unitless mass); measured_at timestamps are optional and nil
// when the value was carried forward or never measured.
type HealthSnapshotInput struct {
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
}

// UpsertSnapshots validates and persists a batch of snapshots
// for the given user. All dates are validated before any write
// so a bad batch fails atomically; valid rows then upsert one
// by one (re-uploads overwrite the same date). Returns the
// persisted rows in request order.
func (hc *HealthSnapshotController) UpsertSnapshots(userID string, inputs []HealthSnapshotInput) ([]models.HealthSnapshot, error) {
	if len(inputs) > maxHealthSnapshotBatch {
		return nil, ErrHealthSnapshotBatchTooLarge
	}
	for _, in := range inputs {
		if err := validateSnapshotDate(in.SnapshotDate); err != nil {
			return nil, err
		}
	}
	out := make([]models.HealthSnapshot, 0, len(inputs))
	for _, in := range inputs {
		entry := &models.HealthSnapshot{
			UserID:              userID,
			SnapshotDate:        in.SnapshotDate,
			Tz:                  in.Tz,
			Steps:               in.Steps,
			DistanceMeters:      in.DistanceMeters,
			ActiveEnergyKcal:    in.ActiveEnergyKcal,
			BasalEnergyKcal:     in.BasalEnergyKcal,
			ExerciseMinutes:     in.ExerciseMinutes,
			SleepSeconds:        in.SleepSeconds,
			Weight:              in.Weight,
			WeightMeasuredAt:    normalizeMeasuredAt(in.WeightMeasuredAt),
			BMI:                 in.BMI,
			BMIMeasuredAt:       normalizeMeasuredAt(in.BMIMeasuredAt),
			BodyFatPercentage:   in.BodyFatPercentage,
			BodyFatMeasuredAt:   normalizeMeasuredAt(in.BodyFatMeasuredAt),
			LeanBodyMass:        in.LeanBodyMass,
			LeanMassMeasuredAt:  normalizeMeasuredAt(in.LeanMassMeasuredAt),
			HeartRate:           in.HeartRate,
			HeartRateMeasuredAt: normalizeMeasuredAt(in.HeartRateMeasuredAt),
			RestingHeartRate:    in.RestingHeartRate,
			RestingHRMeasuredAt: normalizeMeasuredAt(in.RestingHRMeasuredAt),
			WalkingHeartRateAvg: in.WalkingHeartRateAvg,
			WalkingHRMeasuredAt: normalizeMeasuredAt(in.WalkingHRMeasuredAt),
			HRV:                 in.HRV,
			HRVMeasuredAt:       normalizeMeasuredAt(in.HRVMeasuredAt),
			CardioRecoveryBPM:   in.CardioRecoveryBPM,
			CardioRecoveryAt:    normalizeMeasuredAt(in.CardioRecoveryAt),
			VO2Max:              in.VO2Max,
			VO2MeasuredAt:       normalizeMeasuredAt(in.VO2MeasuredAt),
		}
		if err := hc.repo.Upsert(entry); err != nil {
			return nil, err
		}
		out = append(out, *entry)
	}
	return out, nil
}

// ListSnapshots returns a user's snapshots within an inclusive
// device-local date range, newest first.
func (hc *HealthSnapshotController) ListSnapshots(userID, startDate, endDate string) ([]models.HealthSnapshot, error) {
	return hc.repo.ListRange(userID, startDate, endDate)
}

// validateSnapshotDate checks the YYYY-MM-DD shape and rejects
// dates more than two days past the server clock (device-local
// dates east of UTC plus skew, per ErrHealthSnapshotDateFuture).
func validateSnapshotDate(date string) error {
	parsed, err := time.Parse("2006-01-02", date)
	if err != nil {
		return ErrHealthSnapshotDateInvalid
	}
	if parsed.After(time.Now().Add(48 * time.Hour)) {
		return ErrHealthSnapshotDateFuture
	}
	return nil
}

// normalizeMeasuredAt maps a zero timestamp to nil so an
// explicitly-zero time never lands in a measured_at column as a
// misleading 0001-01-01 "observation".
func normalizeMeasuredAt(t *time.Time) *time.Time {
	if t == nil || t.IsZero() {
		return nil
	}
	return t
}
