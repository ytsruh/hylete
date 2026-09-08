import Foundation

/// Upload contract for POST /api/v1/health-snapshots. One
/// payload per device-local day, canonical units (the provider
/// already returns kg / metres / kcal / bpm / ms / ml/kg/min /
/// unitless mass), snake_case keys mirroring
/// `UpsertHealthSnapshotsRequest`. Absent metrics encode as 0
// ("not recorded"); absent measured_at dates are omitted via
/// optionals so the server stores NULL (no observation) rather
/// than a magic date.
public struct HealthSnapshotPayload: Encodable, Equatable {
    public let snapshotDate: String
    public let tz: String
    public let steps: Int
    public let distanceMeters: Double
    public let activeEnergyKcal: Double
    public let basalEnergyKcal: Double
    public let exerciseMinutes: Double
    public let sleepSeconds: Double
    public let weight: Double
    public let weightMeasuredAt: Date?
    public let bmi: Double
    public let bmiMeasuredAt: Date?
    public let bodyFatPercentage: Double
    public let bodyFatMeasuredAt: Date?
    public let leanBodyMass: Double
    public let leanMassMeasuredAt: Date?
    public let heartRate: Double
    public let heartRateMeasuredAt: Date?
    public let restingHeartRate: Double
    public let restingHRMeasuredAt: Date?
    public let walkingHeartRateAvg: Double
    public let walkingHRMeasuredAt: Date?
    public let hrvMs: Double
    public let hrvMeasuredAt: Date?
    public let cardioRecoveryBpm: Double
    public let cardioRecoveryMeasuredAt: Date?
    public let vo2Max: Double
    public let vo2MeasuredAt: Date?

    enum CodingKeys: String, CodingKey {
        case snapshotDate = "snapshot_date"
        case tz
        case steps
        case distanceMeters = "distance_meters"
        case activeEnergyKcal = "active_energy_kcal"
        case basalEnergyKcal = "basal_energy_kcal"
        case exerciseMinutes = "exercise_minutes"
        case sleepSeconds = "sleep_seconds"
        case weight
        case weightMeasuredAt = "weight_measured_at"
        case bmi
        case bmiMeasuredAt = "bmi_measured_at"
        case bodyFatPercentage = "body_fat_percentage"
        case bodyFatMeasuredAt = "body_fat_measured_at"
        case leanBodyMass = "lean_body_mass"
        case leanMassMeasuredAt = "lean_mass_measured_at"
        case heartRate = "heart_rate"
        case heartRateMeasuredAt = "heart_rate_measured_at"
        case restingHeartRate = "resting_heart_rate"
        case restingHRMeasuredAt = "resting_hr_measured_at"
        case walkingHeartRateAvg = "walking_heart_rate_avg"
        case walkingHRMeasuredAt = "walking_hr_measured_at"
        case hrvMs = "hrv_ms"
        case hrvMeasuredAt = "hrv_measured_at"
        case cardioRecoveryBpm = "cardio_recovery_bpm"
        case cardioRecoveryMeasuredAt = "cardio_recovery_measured_at"
        case vo2Max = "vo2_max"
        case vo2MeasuredAt = "vo2_measured_at"
    }

    /// Builds an upload payload from one history day. Sleep is
    /// stored in seconds (the history layer buckets hours);
    /// every other value passes through untouched.
    public init(day: DailyHealthSnapshot) {
        snapshotDate = day.date
        tz = day.timeZone
        let v = day.values
        let m = day.measuredAt
        steps = Int(v[.steps] ?? 0)
        distanceMeters = v[.distance] ?? 0
        activeEnergyKcal = v[.activeEnergy] ?? 0
        basalEnergyKcal = v[.basalEnergy] ?? 0
        exerciseMinutes = v[.exerciseTime] ?? 0
        sleepSeconds = (v[.sleep] ?? 0) * 3_600
        weight = v[.weight] ?? 0
        weightMeasuredAt = m[.weight]
        bmi = v[.bmi] ?? 0
        bmiMeasuredAt = m[.bmi]
        bodyFatPercentage = v[.bodyFatPercentage] ?? 0
        bodyFatMeasuredAt = m[.bodyFatPercentage]
        leanBodyMass = v[.leanBodyMass] ?? 0
        leanMassMeasuredAt = m[.leanBodyMass]
        heartRate = v[.heartRate] ?? 0
        heartRateMeasuredAt = m[.heartRate]
        restingHeartRate = v[.restingHeartRate] ?? 0
        restingHRMeasuredAt = m[.restingHeartRate]
        walkingHeartRateAvg = v[.walkingHeartRateAverage] ?? 0
        walkingHRMeasuredAt = m[.walkingHeartRateAverage]
        hrvMs = v[.heartRateVariability] ?? 0
        hrvMeasuredAt = m[.heartRateVariability]
        cardioRecoveryBpm = v[.cardioRecovery] ?? 0
        cardioRecoveryMeasuredAt = m[.cardioRecovery]
        vo2Max = v[.cardioFitness] ?? 0
        vo2MeasuredAt = m[.cardioFitness]
    }
}

/// Interface the sync store uploads through (consumer-defined
/// interface, per codebase convention). `APIClient` conforms
/// via extension; tests inject a fake.
public protocol HealthSnapshotAPI {
    func upsertHealthSnapshots(_ snapshots: [HealthSnapshotPayload]) async throws -> [HealthSnapshotDTO]
}

/// In-memory fake for previews and unit tests. Records every
/// uploaded chunk; optional `error` makes the next call throw.
public final class MockSnapshotAPI: HealthSnapshotAPI {
    public private(set) var uploaded: [[HealthSnapshotPayload]] = []
    public var error: Error?

    public init() {}

    public func upsertHealthSnapshots(_ snapshots: [HealthSnapshotPayload]) async throws -> [HealthSnapshotDTO] {
        if let error { throw error }
        uploaded.append(snapshots)
        return []
    }
}
