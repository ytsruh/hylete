import XCTest
@testable import Hylete

/// Tests for `CalendarMath`, the pure date helpers behind
/// `WeekCalendarView`. Every test pins an explicit `Calendar`
/// (UTC, fixed firstWeekday) so results are deterministic no
/// matter the machine's region settings.
final class CalendarMathTests: XCTestCase {

    /// Gregorian calendar, UTC, weeks start Sunday.
    private var sundayFirst: Calendar {
        var cal = Calendar(identifier: .gregorian)
        cal.firstWeekday = 1 // Sunday
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }

    /// Same but Monday-first (en-GB style locale).
    private var mondayFirst: Calendar {
        var cal = sundayFirst
        cal.firstWeekday = 2 // Monday
        return cal
    }

    private func date(
        _ year: Int, _ month: Int, _ day: Int,
        hour: Int = 12, minute: Int = 0,
        in calendar: Calendar
    ) -> Date {
        calendar.date(from: DateComponents(year: year, month: month, day: day, hour: hour, minute: minute))!
    }

    // MARK: - startOfDay

    func testStartOfDayTruncatesTime() {
        let cal = sundayFirst
        let input = date(2026, 8, 19, hour: 15, minute: 42, in: cal)
        let expected = date(2026, 8, 19, hour: 0, minute: 0, in: cal)
        XCTAssertEqual(CalendarMath.startOfDay(input, calendar: cal), expected)
    }

    // MARK: - startOfWeek

    /// Wed 19 Aug 2026 sits in the week starting Sun 16 Aug for a
    /// Sunday-first calendar.
    func testStartOfWeekSundayFirst() {
        let cal = sundayFirst
        let wednesday = date(2026, 8, 19, in: cal)
        XCTAssertEqual(
            CalendarMath.startOfWeek(for: wednesday, calendar: cal),
            date(2026, 8, 16, hour: 0, in: cal)
        )
    }

    /// Same Wednesday lands on Mon 17 Aug when the week starts
    /// on Monday — locale-driven boundaries are the whole point.
    func testStartOfWeekMondayFirst() {
        let cal = mondayFirst
        let wednesday = date(2026, 8, 19, in: cal)
        XCTAssertEqual(
            CalendarMath.startOfWeek(for: wednesday, calendar: cal),
            date(2026, 8, 17, hour: 0, in: cal)
        )
    }

    /// The first day of a week is its own week start.
    func testStartOfWeekIsIdempotentOnBoundary() {
        let cal = sundayFirst
        let sunday = date(2026, 8, 16, hour: 23, in: cal) // late Sunday night
        XCTAssertEqual(
            CalendarMath.startOfWeek(for: sunday, calendar: cal),
            date(2026, 8, 16, hour: 0, in: cal)
        )
    }

    // MARK: - addWeeks

    func testAddWeeksForwardAndBack() {
        let cal = sundayFirst
        let weekStart = date(2026, 8, 16, hour: 0, in: cal)
        XCTAssertEqual(CalendarMath.addWeeks(1, to: weekStart, calendar: cal), date(2026, 8, 23, hour: 0, in: cal))
        XCTAssertEqual(CalendarMath.addWeeks(-1, to: weekStart, calendar: cal), date(2026, 8, 9, hour: 0, in: cal))
        XCTAssertEqual(CalendarMath.addWeeks(-52, to: weekStart, calendar: cal), date(2025, 8, 17, hour: 0, in: cal))
    }

    /// Crossing a DST transition must not shift the wall-clock
    /// time of the week start. Uses London time where 2026's
    /// BST→GMT fallback happens on 25 Oct.
    func testAddWeeksSurvivesDSTFallback() {
        var cal = sundayFirst
        cal.timeZone = TimeZone(identifier: "Europe/London")!
        let before = date(2026, 10, 18, hour: 0, in: cal)
        let after = CalendarMath.addWeeks(2, to: before, calendar: cal)
        let components = cal.dateComponents([.hour, .minute], from: after)
        XCTAssertEqual(components.hour, 0)
        XCTAssertEqual(components.minute, 0)
    }

    // MARK: - days(inWeekOf:)

    func testDaysInWeekAreSevenConsecutiveMidnights() {
        let cal = sundayFirst
        let weekStart = date(2026, 8, 16, hour: 0, in: cal)
        let days = CalendarMath.days(inWeekOf: weekStart, calendar: cal)
        XCTAssertEqual(days.count, 7)
        XCTAssertEqual(days.first, weekStart)
        XCTAssertEqual(days.last, date(2026, 8, 22, hour: 0, in: cal))
        // Consecutive: each day is exactly one day after the last.
        for (prev, next) in zip(days, days.dropFirst()) {
            XCTAssertEqual(next.timeIntervalSince(prev), 86_400, accuracy: 0.001)
        }
    }

    // MARK: - isSameDay

    func testIsSameDayAcrossMidnight() {
        let cal = sundayFirst
        let lateNight = date(2026, 8, 19, hour: 23, minute: 59, in: cal)
        let earlyMorning = date(2026, 8, 19, hour: 0, minute: 1, in: cal)
        let nextDay = date(2026, 8, 20, hour: 0, minute: 1, in: cal)
        XCTAssertTrue(CalendarMath.isSameDay(lateNight, earlyMorning, calendar: cal))
        XCTAssertFalse(CalendarMath.isSameDay(lateNight, nextDay, calendar: cal))
    }

    // MARK: - weekdaySymbols

    func testWeekdaySymbolsRotateWithFirstWeekday() {
        // veryShortWeekdaySymbols are single letters.
        XCTAssertEqual(CalendarMath.weekdaySymbols(calendar: sundayFirst).first, "S")
        XCTAssertEqual(CalendarMath.weekdaySymbols(calendar: mondayFirst).first, "M")
        // Both orderings are rotations of the same seven symbols.
        XCTAssertEqual(CalendarMath.weekdaySymbols(calendar: sundayFirst).count, 7)
        XCTAssertEqual(CalendarMath.weekdaySymbols(calendar: mondayFirst).count, 7)
    }
}
