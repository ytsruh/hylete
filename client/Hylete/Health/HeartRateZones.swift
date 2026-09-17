import Foundation

/// Estimated heart-rate training zones (pure math, no HealthKit).
///
/// Apple never shares the user's configured zones or max HR, so
/// the Live page estimates: max HR via Tanaka (208 − 0.7 × age)
/// from the profile date of birth, standard 5-zone cutoffs as
/// fractions of max. An estimate — but the same class of
/// estimate every running watch ships. The honesty mechanism is
/// gating, not disclaimers: `maxHeartRate(dateOfBirth:now:)`
/// returns nil for a missing/unparseable DOB and the zone tile
/// hides, never guessing an age.
public enum HeartRateZones {
    /// Tanaka max-HR estimate for an age in whole years.
    public static func maxHeartRate(ageYears: Int) -> Double {
        208 - 0.7 * Double(max(0, ageYears))
    }

    /// Whole-years age on `now` for a "YYYY-MM-DD" DOB. Nil when
    /// the string is missing or unparseable.
    public static func ageYears(dateOfBirth: String?, now: Date = Date()) -> Int? {
        guard let dateOfBirth, !dateOfBirth.isEmpty else { return nil }
        let formatter = DateFormatter()
        formatter.dateFormat = "yyyy-MM-dd"
        formatter.locale = Locale(identifier: "en_US_POSIX")
        guard let birth = formatter.date(from: dateOfBirth) else { return nil }
        let years = Calendar.current.dateComponents([.year], from: birth, to: now).year
        guard let years, years >= 0, years < 130 else { return nil }
        return years
    }

    /// Tanaka max-HR estimate from a profile DOB. Nil propagates
    /// (missing/bad DOB → no zone tile).
    public static func maxHeartRate(dateOfBirth: String?, now: Date = Date()) -> Double? {
        guard let age = ageYears(dateOfBirth: dateOfBirth, now: now) else { return nil }
        return maxHeartRate(ageYears: age)
    }

    /// Zone index 1–5 for a beats-per-minute reading, or nil
    /// when there is no reading (or no usable max).
    public static func zone(bpm: Double?, maxHeartRate: Double?) -> Int? {
        guard let bpm, bpm > 0, let maxHeartRate, maxHeartRate > 0 else { return nil }
        let fraction = bpm / maxHeartRate
        switch fraction {
        case ..<0.6: return 1
        case ..<0.7: return 2
        case ..<0.8: return 3
        case ..<0.9: return 4
        default: return 5
        }
    }

    /// Short zone name for the tile caption.
    public static func name(for zone: Int) -> String {
        switch zone {
        case 1: return "Recovery"
        case 2: return "Fat Burn"
        case 3: return "Aerobic"
        case 4: return "Threshold"
        default: return "Maximum"
        }
    }
}
