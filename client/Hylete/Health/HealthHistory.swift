import Foundation

/// One device-local calendar day of HealthKit data, ready to
/// become a server snapshot. Values are canonical units
/// (matching `HealthMetric.formatted` expectations); measuredAt
/// holds the actual HealthKit observation date per latest-type
/// metric, or nothing when the value was carried forward from
/// an earlier sample or never measured.
public struct DailyHealthSnapshot: Equatable {
    /// Device-local calendar date ("YYYY-MM-DD").
    public let date: String
    /// Device timezone identifier (e.g. "Europe/London").
    public let timeZone: String
    public var values: [HealthMetric: Double]
    public var measuredAt: [HealthMetric: Date]

    public init(
        date: String,
        timeZone: String,
        values: [HealthMetric: Double] = [:],
        measuredAt: [HealthMetric: Date] = [:]
    ) {
        self.date = date
        self.timeZone = timeZone
        self.values = values
        self.measuredAt = measuredAt
    }

    /// True when the day holds nothing worth uploading: every
    /// metric is zero/absent and no observation date exists. A
    /// genuinely tracked day always leaves some trace (basal
    /// energy, a heart sample), so all-empty reliably means
    /// "nothing recorded" — including watch-off days, which are
    /// equally correctly skipped. A measured zero (value 0 with
    /// a `measuredAt`) counts as data, not emptiness.
    public var isEmpty: Bool {
        values.values.allSatisfy { $0 == 0 } && measuredAt.isEmpty
    }
}

/// Interface for fetching multi-day HealthKit history (backfill
/// + catch-up sync). Separate from `HealthDataProvider` (which
/// serves the today/latest display) so each consumer depends
/// only on what it uses. `days` are device-local midnights,
/// oldest first; the result matches them 1:1 in order.
public protocol HealthHistoryProvider {
    /// False on devices without HealthKit (iPad, restricted).
    /// Both live and mock stores already implement this for
    /// `HealthDataProvider`; declaring it here lets the sync
    /// store gate on availability through this protocol alone.
    func isAvailable() -> Bool
    func fetchHistory(days: [Date]) async -> [DailyHealthSnapshot]
}

/// Device-local "YYYY-MM-DD" string for a midnight — the day
/// key used for snapshot dates and confirmed-date bookkeeping
/// (single helper so both sides always agree). Respects the
/// given calendar's timezone, so tests pinning UTC stay
/// deterministic.
public func snapshotDateString(_ midnight: Date, calendar: Calendar = .current) -> String {
    let comps = calendar.dateComponents([.year, .month, .day], from: midnight)
    return String(format: "%04d-%02d-%02d", comps.year ?? 0, comps.month ?? 0, comps.day ?? 0)
}

/// Device-local midnights for the past `count` days. By
/// default today is excluded (still accumulating, so never
/// snapshotted for backfill); `includingToday` counts today as
/// day one for the rolling re-verify window. Oldest first.
public func syncDays(count: Int, includingToday: Bool = false, calendar: Calendar = .current, now: Date = Date()) -> [Date] {
    guard count > 0 else { return [] }
    let today = calendar.startOfDay(for: now)
    let startBack = includingToday ? 0 : 1
    return (0..<count).compactMap { i in
        calendar.date(byAdding: .day, value: -(startBack + i), to: today)
    }.reversed()
}

/// Buckets raw samples into per-day latest values with
/// carry-forward: each day takes the most recent sample ending
/// before the next midnight, so slow metrics (weight, VO2max)
/// form step functions instead of gaps. `measuredAt` is the
/// sample's own date — analysis treats a value as an
/// observation only when it falls inside its day.
///
/// Pure function of its inputs (no HealthKit) so backfill
/// semantics are unit-testable.
public func bucketLatest(
    samples: [(date: Date, value: Double)],
    days: [Date],
    calendar: Calendar = .current
) -> [Date: (value: Double, measuredAt: Date)] {
    let sorted = samples.sorted { $0.date < $1.date }
    var out: [Date: (value: Double, measuredAt: Date)] = [:]
    for day in days {
        guard let next = calendar.date(byAdding: .day, value: 1, to: day) else { continue }
        // Last sample ending before the next midnight; days are
        // ascending lookups over ascending samples, so a linear
        // scan from the end is fine for backfill scale.
        if let latest = sorted.last(where: { $0.date < next }) {
            out[day] = (latest.value, latest.date)
        }
    }
    return out
}

/// Buckets sleep segments into asleep-seconds per device-local
/// day, splitting segments that straddle midnight by overlap.
/// Pure function (no HealthKit) for the same testability
/// reason as `bucketLatest`.
public func bucketSleep(
    segments: [(start: Date, end: Date)],
    days: [Date],
    calendar: Calendar = .current
) -> [Date: Double] {
    var out: [Date: Double] = [:]
    for day in days {
        guard let next = calendar.date(byAdding: .day, value: 1, to: day) else { continue }
        var seconds = 0.0
        for seg in segments {
            let overlapStart = max(seg.start, day)
            let overlapEnd = min(seg.end, next)
            if overlapEnd > overlapStart {
                seconds += overlapEnd.timeIntervalSince(overlapStart)
            }
        }
        if seconds > 0 {
            out[day] = seconds / 3_600
        }
    }
    return out
}
