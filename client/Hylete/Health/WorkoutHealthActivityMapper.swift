import HealthKit

/// Maps a planned Hylete workout onto a single Apple
/// `HKWorkoutActivityType` for live Health recording.
///
/// Hylete blocks carry structure (`standard/circuit/amrap/emom`),
/// not physiology — so inference counts planned *items* by
/// `exerciseType` (`strength/cardio/other`, see
/// `BlockItemDTO.isCardio`) instead of block types. One type per
/// workout (no multi-activity segments in v1); the user can
/// always override the inferred default in the player picker.
///
/// HealthKit-free core (`infer(strength:cardio:other:)`) keeps
/// the math unit-testable on Simulator without a Health store;
/// the `BlockItemDTO` convenience just counts.
public enum WorkoutHealthActivityMapper {
    /// Short picker list for the player's type override. Ordered
    /// most-likely first for a lifting app; outdoor route types
    /// (running/cycling/walking) gate GPS via
    /// `isOutdoorRouteEligible`.
    public static let selectableTypes: [HKWorkoutActivityType] = [
        .traditionalStrengthTraining,
        .highIntensityIntervalTraining,
        .running,
        .cycling,
        .walking,
        .rowing,
        .other,
    ]

    /// Server key for one activity type (the case name, e.g.
    /// "traditionalStrengthTraining"). The server stores the key
    /// opaquely; this is the contract between client and server.
    /// Unknown types collapse to "other" rather than persisting a
    /// value no client understands.
    public static func key(for type: HKWorkoutActivityType) -> String {
        switch type {
        case .traditionalStrengthTraining: return "traditionalStrengthTraining"
        case .highIntensityIntervalTraining: return "highIntensityIntervalTraining"
        case .running: return "running"
        case .cycling: return "cycling"
        case .walking: return "walking"
        case .rowing: return "rowing"
        default: return "other"
        }
    }

    /// Resolves a server key to an activity type. Nil, blank, and
    /// unknown keys yield nil so callers fall back to inference —
    /// the safety net for pre-feature servers and corrupt values.
    public static func activityType(forKey key: String?) -> HKWorkoutActivityType? {
        switch key?.trimmingCharacters(in: .whitespacesAndNewlines) {
        case "traditionalStrengthTraining": return .traditionalStrengthTraining
        case "highIntensityIntervalTraining": return .highIntensityIntervalTraining
        case "running": return .running
        case "cycling": return .cycling
        case "walking": return .walking
        case "rowing": return .rowing
        case "other": return .other
        default: return nil
        }
    }

    /// User-facing label for the picker and Live page. HealthKit
    /// provides no display strings, so these are Hylete-owned.
    public static func displayName(for type: HKWorkoutActivityType) -> String {
        switch type {
        case .traditionalStrengthTraining: return "Strength"
        case .highIntensityIntervalTraining: return "HIIT"
        case .running: return "Running"
        case .cycling: return "Cycling"
        case .walking: return "Walking"
        case .rowing: return "Rowing"
        case .functionalStrengthTraining: return "Functional"
        default: return "Other"
        }
    }

    /// Infers one activity type from item counts. Rules:
    /// - no items at all (or other-only) → `.other`
    /// - strength-only → `.traditionalStrengthTraining`
    /// - cardio-only or mixed with cardio majority/tie → `.highIntensityIntervalTraining`
    ///   (generic cardio default; the user picks Running/Cycling
    ///   for outdoor GPS)
    /// - strength majority → `.traditionalStrengthTraining`
    public static func infer(strength: Int, cardio: Int, other: Int) -> HKWorkoutActivityType {
        let strength = max(0, strength)
        let cardio = max(0, cardio)
        guard strength + cardio + other > 0 else { return .other }
        guard cardio > 0 else { return strength > 0 ? .traditionalStrengthTraining : .other }
        guard strength > 0 else { return .highIntensityIntervalTraining }
        return cardio >= strength ? .highIntensityIntervalTraining : .traditionalStrengthTraining
    }

    /// Convenience over planned items (counts via
    /// `exercise_type`, case-insensitive; unknown → other).
    public static func infer(items: [BlockItemDTO]) -> HKWorkoutActivityType {
        var strength = 0
        var cardio = 0
        var other = 0
        for item in items {
            switch item.exerciseType.lowercased() {
            case "cardio": cardio += 1
            case "strength": strength += 1
            default: other += 1
            }
        }
        return infer(strength: strength, cardio: cardio, other: other)
    }

    /// `true` only for the outdoor distance types that may start
    /// a GPS route (outdoor-only decision). Everything else —
    /// including treadmill-style cardio and rowing — records
    /// time/HR/kcal with no location prompt.
    public static func isOutdoorRouteEligible(_ type: HKWorkoutActivityType) -> Bool {
        type == .running || type == .cycling || type == .walking
    }
}
