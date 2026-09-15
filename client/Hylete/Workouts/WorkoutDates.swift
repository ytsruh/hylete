import Foundation

/// Date helpers for workouts. The server stores `scheduledDate` as a
/// device-local calendar day (`YYYY-MM-DD`, same convention as the
/// health snapshot date) — never an instant — so formatting and
/// bucketing stay in the device locale with no timezone math.
enum WorkoutDates {
    /// Parses a `YYYY-MM-DD` scheduled date into a local `Date`
    /// (midnight). Nil when the string is malformed.
    static func date(from scheduledDate: String) -> Date? {
        dayFormatter.date(from: scheduledDate)
    }

    /// Encodes a `Date` as `YYYY-MM-DD` in the device locale (for
    /// create/update/duplicate payloads and range queries).
    static func dayString(from date: Date) -> String {
        dayFormatter.string(from: date)
    }

    /// "Mon, Sep 14"-style label for list rows and headers. Falls
    /// back to the raw string when parsing fails so a server-side
    /// oddity never blanks the UI.
    static func display(_ scheduledDate: String) -> String {
        guard let date = dayFormatter.date(from: scheduledDate) else { return scheduledDate }
        return displayFormatter.string(from: date)
    }

    /// "Today"/"Tomorrow" when the date matches, otherwise
    /// `display(_:)`. Used by the dashboard day section.
    static func relativeDisplay(_ scheduledDate: String) -> String {
        guard let date = dayFormatter.date(from: scheduledDate) else { return scheduledDate }
        let calendar = Calendar.current
        if calendar.isDateInToday(date) { return "Today" }
        if calendar.isDateInTomorrow(date) { return "Tomorrow" }
        if calendar.isDateInYesterday(date) { return "Yesterday" }
        return displayFormatter.string(from: date)
    }

    /// Expands a weekly repeat into explicit `YYYY-MM-DD` days:
    /// every day in the `weeks * 7`-day window starting at `start`
    /// (inclusive) whose weekday (1 = Sunday … 7 = Saturday, per
    /// `Calendar.current`) is in `weekdays`. E.g. start Monday,
    /// {Monday}, 6 weeks → 6 Mondays. Results are ascending and
    /// deduplicated. Pure and deterministic — unit-tested.
    static func datesForWeekly(starting start: Date, weekdays: Set<Int>, weeks: Int) -> [String] {
        guard weeks >= 1, !weekdays.isEmpty else { return [] }
        let calendar = Calendar.current
        let startDay = calendar.startOfDay(for: start)
        var out: [String] = []
        for offset in 0 ..< weeks * 7 {
            guard let day = calendar.date(byAdding: .day, value: offset, to: startDay) else { continue }
            if weekdays.contains(calendar.component(.weekday, from: day)) {
                out.append(dayString(from: day))
            }
        }
        return Array(NSOrderedSet(array: out)) as? [String] ?? out
    }

    /// Expands an every-N-days repeat into explicit `YYYY-MM-DD`
    /// days: `start`, `start + everyDays`, … for `occurrences`
    /// dates. Pure and deterministic — unit-tested.
    static func datesForInterval(starting start: Date, everyDays: Int, occurrences: Int) -> [String] {
        guard everyDays >= 1, occurrences >= 1 else { return [] }
        let calendar = Calendar.current
        let startDay = calendar.startOfDay(for: start)
        var out: [String] = []
        for i in 0 ..< occurrences {
            guard let day = calendar.date(byAdding: .day, value: i * everyDays, to: startDay) else { continue }
            out.append(dayString(from: day))
        }
        return out
    }

    /// Weekday picker rows in locale order (Monday-first in most
    /// of Europe, Sunday-first in the US): each row's `Calendar`
    /// weekday number (1 = Sunday … 7 = Saturday) plus its short
    /// symbol. The picker stores the selected numbers in a
    /// `Set<Int>` for `datesForWeekly`.
    static func orderedWeekdays() -> [(number: Int, symbol: String)] {
        let calendar = Calendar.current
        let symbols = calendar.shortWeekdaySymbols
        let first = calendar.firstWeekday
        return (0 ..< 7).map { i in
            let number = ((first - 1 + i) % 7) + 1
            return (number: number, symbol: symbols[number - 1])
        }
    }

    private static let dayFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        f.locale = Locale(identifier: "en_US_POSIX")
        return f
    }()

    private static let displayFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "E, MMM d"
        f.locale = Locale.current
        return f
    }()
}
