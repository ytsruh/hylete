import XCTest
@testable import Hylete

/// Tests for `WorkoutDates`, the pure `YYYY-MM-DD` helpers behind
/// the workout schedule views. Round-trip and malformed-input cases
/// only — display strings are locale-dependent by design, so tests
/// assert structure (non-empty, passthrough) rather than exact text.
final class WorkoutDatesTests: XCTestCase {

    func testDayStringRoundTrip() {
        // Midday avoids any DST-midnight edge in the device zone.
        let date = Calendar.current.date(from: DateComponents(year: 2026, month: 9, day: 14, hour: 12))!
        let dayString = WorkoutDates.dayString(from: date)
        XCTAssertEqual(dayString, "2026-09-14")
        XCTAssertNotNil(WorkoutDates.date(from: dayString))
    }

    func testDateFromMalformedReturnsNil() {
        XCTAssertNil(WorkoutDates.date(from: "not-a-date"))
        XCTAssertNil(WorkoutDates.date(from: "14/09/2026"))
        XCTAssertNil(WorkoutDates.date(from: ""))
    }

    func testDisplayFallsBackToRawString() {
        XCTAssertEqual(WorkoutDates.display("bogus"), "bogus")
        XCTAssertFalse(WorkoutDates.display("2026-09-14").isEmpty)
    }

    func testRelativeDisplayToday() {
        let today = WorkoutDates.dayString(from: Date())
        XCTAssertEqual(WorkoutDates.relativeDisplay(today), "Today")
    }

    // MARK: - Recurrence expansion

    /// Single weekday × N weeks from the start day itself. Expected
    /// dates are built with independent calendar math so the test
    /// holds in any calendar (no hardcoded year numbers).
    func testDatesForWeeklySingleDay() {
        let calendar = Calendar.current
        let start = calendar.date(from: DateComponents(year: 2026, month: 9, day: 14, hour: 12))!
        let weekday = calendar.component(.weekday, from: start)
        let got = WorkoutDates.datesForWeekly(starting: start, weekdays: [weekday], weeks: 6)
        XCTAssertEqual(got.count, 6)
        let expected = (0 ..< 6).map { i in
            WorkoutDates.dayString(from: calendar.date(byAdding: .day, value: i * 7, to: start)!)
        }
        XCTAssertEqual(got, expected)
        XCTAssertEqual(got.first, WorkoutDates.dayString(from: start))
    }

    /// Two weekdays × 2 weeks: every date matches a chosen weekday
    /// and the window never exceeds two calendar weeks.
    func testDatesForWeeklyTwoDays() {
        let calendar = Calendar.current
        let start = calendar.date(from: DateComponents(year: 2026, month: 9, day: 14, hour: 12))!
        let next = calendar.date(byAdding: .day, value: 1, to: start)!
        let days: Set<Int> = [
            calendar.component(.weekday, from: start),
            calendar.component(.weekday, from: next),
        ]
        let got = WorkoutDates.datesForWeekly(starting: start, weekdays: days, weeks: 2)
        XCTAssertEqual(got.count, 4)
        XCTAssertEqual(got, got.sorted())
        for dayString in got {
            guard let day = WorkoutDates.date(from: dayString) else {
                XCTFail("unparseable date \(dayString)")
                continue
            }
            XCTAssertTrue(days.contains(calendar.component(.weekday, from: day)))
        }
        let last = WorkoutDates.date(from: got.last!)!
        XCTAssertLessThanOrEqual(last.timeIntervalSince(start), 14 * 86400)
    }

    func testDatesForWeeklyEmpty() {
        let start = Date()
        XCTAssertTrue(WorkoutDates.datesForWeekly(starting: start, weekdays: [], weeks: 6).isEmpty)
        XCTAssertTrue(WorkoutDates.datesForWeekly(starting: start, weekdays: [2], weeks: 0).isEmpty)
    }

    func testDatesForInterval() {
        let calendar = Calendar.current
        let start = calendar.date(from: DateComponents(year: 2026, month: 9, day: 14, hour: 12))!
        let got = WorkoutDates.datesForInterval(starting: start, everyDays: 2, occurrences: 4)
        let expected = (0 ..< 4).map { i in
            WorkoutDates.dayString(from: calendar.date(byAdding: .day, value: i * 2, to: start)!)
        }
        XCTAssertEqual(got, expected)
    }

    func testDatesForIntervalEmpty() {
        let start = Date()
        XCTAssertTrue(WorkoutDates.datesForInterval(starting: start, everyDays: 0, occurrences: 4).isEmpty)
        XCTAssertTrue(WorkoutDates.datesForInterval(starting: start, everyDays: 2, occurrences: 0).isEmpty)
    }

    func testOrderedWeekdaysCoversAllDays() {
        let rows = WorkoutDates.orderedWeekdays()
        XCTAssertEqual(rows.count, 7)
        XCTAssertEqual(Set(rows.map(\.number)), Set(1 ... 7))
        XCTAssertTrue(rows.allSatisfy { !$0.symbol.isEmpty })
    }
}
