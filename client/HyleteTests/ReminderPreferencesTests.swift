import XCTest
@testable import Hylete

/// Tests for `ReminderPreferencesDTO`, the iOS mirror of the
/// server's reminder schedule (`GET`/`PUT /api/v1/me/reminders`).
/// Covers the display helpers the Profile row and editor rely on
/// (summary text, day-of-week visibility, hour parsing) plus the
/// `omitempty` decoding contract: the server omits `day_of_week`
/// for off/daily, which must decode to nil rather than fail.
final class ReminderPreferencesTests: XCTestCase {

    // MARK: - summary

    func testSummaryOffWhenDisabled() {
        let prefs = ReminderPreferencesDTO(enabled: false, frequency: "daily", dayOfWeek: nil, time: "09:00")
        XCTAssertEqual(prefs.summary, "Off")
    }

    func testSummaryOffWhenFrequencyOff() {
        let prefs = ReminderPreferencesDTO(enabled: true, frequency: "off", dayOfWeek: nil, time: "09:00")
        XCTAssertEqual(prefs.summary, "Off")
    }

    func testSummaryDaily() {
        let prefs = ReminderPreferencesDTO(enabled: true, frequency: "daily", dayOfWeek: nil, time: "07:00")
        XCTAssertEqual(prefs.summary, "Daily 07:00 UTC")
    }

    func testSummaryWeeklyIncludesDay() {
        let prefs = ReminderPreferencesDTO(enabled: true, frequency: "weekly", dayOfWeek: 0, time: "09:00")
        XCTAssertEqual(prefs.summary, "Weekly Sun 09:00 UTC")
    }

    func testSummaryBiweeklyIncludesDay() {
        let prefs = ReminderPreferencesDTO(enabled: true, frequency: "biweekly", dayOfWeek: 5, time: "18:00")
        XCTAssertEqual(prefs.summary, "Biweekly Fri 18:00 UTC")
    }

    // MARK: - needsDayOfWeek

    func testNeedsDayOfWeekOnlyForWeeklyAndBiweekly() {
        for frequency in ["off", "daily"] {
            let prefs = ReminderPreferencesDTO(enabled: true, frequency: frequency, dayOfWeek: nil, time: "09:00")
            XCTAssertFalse(prefs.needsDayOfWeek, frequency)
        }
        for frequency in ["weekly", "biweekly"] {
            let prefs = ReminderPreferencesDTO(enabled: true, frequency: frequency, dayOfWeek: 1, time: "09:00")
            XCTAssertTrue(prefs.needsDayOfWeek, frequency)
        }
    }

    // MARK: - hour

    func testHourParsesValidTime() {
        let prefs = ReminderPreferencesDTO(enabled: true, frequency: "daily", dayOfWeek: nil, time: "07:00")
        XCTAssertEqual(prefs.hour, 7)
    }

    func testHourFallsBackToNineWhenMalformed() {
        for bad in ["", "09:30", "25:00", "nine", "9:00"] {
            let prefs = ReminderPreferencesDTO(enabled: true, frequency: "daily", dayOfWeek: nil, time: bad)
            XCTAssertEqual(prefs.hour, 9, bad)
        }
    }

    // MARK: - decoding

    /// The server omits `day_of_week` for off/daily (`omitempty`);
    /// that must decode to nil rather than fail the whole payload.
    func testDecodesMissingDayOfWeekAsNil() throws {
        let json = """
        {"enabled":true,"frequency":"daily","time":"07:00"}
        """.data(using: .utf8)!
        let prefs = try JSONDecoder().decode(ReminderPreferencesDTO.self, from: json)
        XCTAssertEqual(prefs.frequency, "daily")
        XCTAssertNil(prefs.dayOfWeek)
    }

    func testRoundTripsUpdateRequestKeys() throws {
        let request = UpdateReminderPreferencesRequest(
            enabled: true, frequency: "weekly", dayOfWeek: 1, time: "09:00"
        )
        let data = try JSONEncoder().encode(request)
        let keys = try JSONSerialization.jsonObject(with: data) as! [String: Any]
        XCTAssertEqual(keys["day_of_week"] as? Int, 1)
    }
}
