import Foundation

/// One Apple Health vital Hylete can display (read-only).
///
/// The enum is deliberately HealthKit-free: it carries the
/// display metadata (title, SF Symbol, daily-total vs latest
/// semantics) and the canonical-unit formatting, so unit tests
/// and SwiftUI previews never need `HKHealthStore`. The live
/// HealthKit mapping (quantity identifiers, `HKUnit`s) lives
/// in `LiveHealthStore`; both sides share the canonical units
/// documented on `Kind`.
///
/// Canonical provider units:
/// - mass (weight, lean mass): kilograms
/// - distance: metres
/// - energy: kilocalories
/// - heart rates / recovery: beats per minute
/// - HRV: milliseconds
/// - cardio fitness (VO2 max): ml/kg/min
/// - body fat: percent 0–100
/// - sleep: hours, exercise: minutes, steps: count
public enum HealthMetric: String, CaseIterable, Identifiable, Hashable {
    case steps
    case distance
    case activeEnergy
    case basalEnergy
    case exerciseTime
    case heartRate
    case restingHeartRate
    case walkingHeartRateAverage
    case heartRateVariability
    case cardioRecovery
    case cardioFitness
    case weight
    case bmi
    case bodyFatPercentage
    case leanBodyMass
    case sleep

    public var id: String { rawValue }

    /// Whether the card shows today's cumulative total or the
    /// most-recent sample. Drives which HealthKit query the live
    /// store runs (`HKStatisticsQuery` vs most-recent sample).
    public enum Kind {
        case dailyTotal
        case latest
    }

    public var kind: Kind {
        switch self {
        case .steps, .distance, .activeEnergy, .basalEnergy,
             .exerciseTime, .sleep:
            return .dailyTotal
        case .heartRate, .restingHeartRate, .walkingHeartRateAverage,
             .heartRateVariability, .cardioRecovery, .cardioFitness,
             .weight, .bmi, .bodyFatPercentage, .leanBodyMass:
            return .latest
        }
    }

    /// User-facing card title.
    public var title: String {
        switch self {
        case .steps: return "Steps"
        case .distance: return "Distance"
        case .activeEnergy: return "Active Energy"
        case .basalEnergy: return "Resting Energy"
        case .exerciseTime: return "Exercise Time"
        case .heartRate: return "Heart Rate"
        case .restingHeartRate: return "Resting HR"
        case .walkingHeartRateAverage: return "Walking Avg HR"
        case .heartRateVariability: return "HRV"
        case .cardioRecovery: return "Cardio Recovery"
        case .cardioFitness: return "Cardio Fitness"
        case .weight: return "Weight"
        case .bmi: return "BMI"
        case .bodyFatPercentage: return "Body Fat"
        case .leanBodyMass: return "Lean Mass"
        case .sleep: return "Sleep"
        }
    }

    /// SF Symbol for the `StatCard` icon disk.
    public var systemImage: String {
        switch self {
        case .steps: return "figure.walk"
        case .distance: return "map"
        case .activeEnergy: return "flame.fill"
        case .basalEnergy: return "bed.double.fill"
        case .exerciseTime: return "timer"
        case .heartRate: return "heart.fill"
        case .restingHeartRate: return "heart"
        case .walkingHeartRateAverage: return "figure.walk.motion"
        case .heartRateVariability: return "waveform.path.ecg"
        case .cardioRecovery: return "heart.circle"
        case .cardioFitness: return "lungs.fill"
        case .weight: return "scalemass.fill"
        case .bmi: return "person.fill"
        case .bodyFatPercentage: return "percent"
        case .leanBodyMass: return "figure.arms.open"
        case .sleep: return "moon.fill"
        }
    }

    /// Formats a canonical-unit value for display. Mass follows
    /// the user's Hylete `weightUnit` ("kg"/"lb"), distance
    /// follows `distanceUnit` ("km"/"mi"); everything else has a
    /// fixed unit. Returns an em dash when `value` is nil so
    /// cards render a stable empty state.
    public func formatted(
        value: Double?,
        weightUnit: String = "kg",
        distanceUnit: String = "km"
    ) -> String {
        guard let value else { return "—" }
        switch self {
        case .steps:
            return "\(Int(value).formatted(.number.grouping(.automatic))) steps"
        case .weight, .leanBodyMass:
            let converted = weightUnit.lowercased() == "lb" ? value * 2.20462 : value
            return String(format: "%.1f %@", converted, weightUnit)
        case .bmi:
            return String(format: "%.1f", value)
        case .bodyFatPercentage:
            return String(format: "%.1f %%", value)
        case .heartRate, .restingHeartRate, .walkingHeartRateAverage, .cardioRecovery:
            return String(format: "%.0f bpm", value)
        case .heartRateVariability:
            return String(format: "%.0f ms", value)
        case .cardioFitness:
            return String(format: "%.1f VO₂", value)
        case .activeEnergy, .basalEnergy:
            return String(format: "%.0f kcal", value)
        case .sleep:
            return String(format: "%.1f h", value)
        case .exerciseTime:
            return String(format: "%.0f min", value)
        case .distance:
            if distanceUnit.lowercased() == "mi" {
                return String(format: "%.2f mi", value / 1_609.344)
            }
            return String(format: "%.2f km", value / 1_000.0)
        }
    }
}
