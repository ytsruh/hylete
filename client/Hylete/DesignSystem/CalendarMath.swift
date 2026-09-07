import Foundation

/// Pure, testable date helpers backing `WeekCalendarView`.
///
/// Everything here is a static function taking an explicit
/// `Calendar` (defaulting to `.current`) so unit tests can pin a
/// fixed locale/firstWeekday instead of depending on the device's
/// region settings. All returned dates are start-of-day instants
/// in the given calendar's timezone.
public enum CalendarMath {

    /// Midnight of the day containing `date`.
    public static func startOfDay(_ date: Date, calendar: Calendar = .current) -> Date {
        calendar.startOfDay(for: date)
    }

    /// Start-of-day for the first day of the week containing
    /// `date`. The first weekday follows the calendar's locale
    /// (`firstWeekday`) — Sunday for en-US, Monday for en-GB, etc.
    public static func startOfWeek(for date: Date, calendar: Calendar = .current) -> Date {
        let day = calendar.startOfDay(for: date)
        let weekday = calendar.component(.weekday, from: day)
        let back = (weekday - calendar.firstWeekday + 7) % 7
        return calendar.date(byAdding: .day, value: -back, to: day) ?? day
    }

    /// Start-of-day exactly `weeks` weeks away from a week start.
    /// Negative values move into the past.
    public static func addWeeks(_ weeks: Int, to weekStart: Date, calendar: Calendar = .current) -> Date {
        calendar.date(byAdding: .weekOfYear, value: weeks, to: calendar.startOfDay(for: weekStart))
            ?? calendar.startOfDay(for: weekStart)
    }

    /// The seven start-of-day dates in the week beginning with
    /// `weekStart`, in display order.
    public static func days(inWeekOf weekStart: Date, calendar: Calendar = .current) -> [Date] {
        let start = calendar.startOfDay(for: weekStart)
        return (0..<7).compactMap { calendar.date(byAdding: .day, value: $0, to: start) }
    }

    /// `true` when both instants fall on the same calendar day.
    public static func isSameDay(_ a: Date, _ b: Date, calendar: Calendar = .current) -> Bool {
        calendar.isDate(a, inSameDayAs: b)
    }

    /// Weekday initials in the calendar's display order (e.g.
    /// ["S", "M", "T", ...] for a Sunday-first locale), aligned
    /// index-for-index with `days(inWeekOf:)`.
    public static func weekdaySymbols(calendar: Calendar = .current) -> [String] {
        let all = calendar.veryShortWeekdaySymbols // Sun..Sat
        let first = calendar.firstWeekday - 1      // 0-based offset into `all`
        return Array(all[first...] + all[..<first])
    }
}
