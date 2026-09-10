import XCTest
@testable import Hylete

/// Tests for the Coach DTOs, the iOS mirror of
/// `internal/routes/api_coach.go`. Covers the golden decode of a
/// weekly report (every payload section present), the thin-data
/// template (empty arrays, single recommendation), and the
/// preferences contract (snake_case keys, 1000-char cap).
final class CoachTests: XCTestCase {

    // MARK: - Golden decode

    private var fullReportJSON: String {
        """
        {"id":"r1","type":"weekly","period_start":"2026-09-07T00:00:00Z",
         "period_end":"2026-09-14T00:00:00Z","prompt_version":"v1","model":"test-model",
         "payload":{"summary":"Solid week.",
          "progress_per_goal":[{"goal":"Bench 100kg","status":"On track"}],
          "prs":["Bench e1RM 102.5"],
          "stalling":["Squat stalled 4 weeks"],
          "trends":{"volume":"up 12%","frequency":"steady","bodyweight":"stable"},
          "adherence":"100% of recent average (4 sessions)",
          "recovery_signals":["Sleep down 1.2h/night vs 4-wk median"],
          "recommendations":["Hold squat volume, add 2.5kg.","Bench 4x5 at 90.","Row twice."]},
         "created_at":"2026-09-14T06:00:01Z"}
        """
    }

    func testDecodesFullReport() throws {
        let report = try APIClient.jsonDecoder.decode(
            CoachReportDTO.self, from: Data(fullReportJSON.utf8))
        XCTAssertEqual(report.id, "r1")
        XCTAssertEqual(report.type, "weekly")
        XCTAssertEqual(report.promptVersion, "v1")
        XCTAssertEqual(report.payload.summary, "Solid week.")
        XCTAssertEqual(report.payload.progressPerGoal.count, 1)
        XCTAssertEqual(report.payload.progressPerGoal[0].goal, "Bench 100kg")
        XCTAssertEqual(report.payload.prs, ["Bench e1RM 102.5"])
        XCTAssertEqual(report.payload.stalling, ["Squat stalled 4 weeks"])
        XCTAssertEqual(report.payload.trends.volume, "up 12%")
        XCTAssertEqual(report.payload.recommendations.count, 3)
        XCTAssertFalse(report.isDismissed)
    }

    func testDecodesDismissedStamp() throws {
        let json = """
        {"id":"r1","type":"weekly","period_start":"2026-09-07T00:00:00Z",
         "period_end":"2026-09-14T00:00:00Z","prompt_version":"v1","model":"m",
         "payload":{"summary":"S","recommendations":["A"]},
         "dismissed_at":"2026-09-16T08:00:00Z",
         "created_at":"2026-09-14T06:00:01Z"}
        """
        let report = try APIClient.jsonDecoder.decode(
            CoachReportDTO.self, from: Data(json.utf8))
        XCTAssertTrue(report.isDismissed)
    }

    /// The thin-data template omits the optional arrays; they
    /// must decode to empty rather than fail.
    func testDecodesThinDataPayload() throws {
        let json = """
        {"id":"r2","type":"weekly","period_start":"2026-09-07T00:00:00Z",
         "period_end":"2026-09-14T00:00:00Z","prompt_version":"v1","model":"none",
         "payload":{"summary":"Not enough training data this week.",
          "adherence":"50% of recent average (2 sessions)",
          "recommendations":["Log at least 3 sessions next week."]},
         "created_at":"2026-09-14T06:00:01Z"}
        """
        let report = try APIClient.jsonDecoder.decode(
            CoachReportDTO.self, from: Data(json.utf8))
        XCTAssertTrue(report.payload.prs.isEmpty)
        XCTAssertTrue(report.payload.stalling.isEmpty)
        XCTAssertEqual(report.payload.trends.volume, "n/a")
        XCTAssertEqual(report.payload.recommendations.count, 1)
    }

    func testReportsResponseEnvelope() throws {
        let json = "{\"reports\":[\(fullReportJSON)]}"
        let response = try APIClient.jsonDecoder.decode(
            CoachReportsResponse.self, from: Data(json.utf8))
        XCTAssertEqual(response.reports.count, 1)
        XCTAssertEqual(response.reports[0].id, "r1")
    }

    // MARK: - Preferences

    func testPreferencesDecodeSnakeCase() throws {
        let json = """
        {"opt_in":true,"goal_text":"Bench 100kg"}
        """
        let prefs = try JSONDecoder().decode(CoachPreferencesDTO.self, from: Data(json.utf8))
        XCTAssertTrue(prefs.optIn)
        XCTAssertEqual(prefs.goalText, "Bench 100kg")
    }

    func testPreferencesRequestEncodesSnakeCase() throws {
        let request = UpdateCoachPreferencesRequest(optIn: true, goalText: "Aim")
        let raw = try APIClient.jsonEncoder.encode(request)
        let body = String(data: raw, encoding: .utf8) ?? ""
        XCTAssertTrue(body.contains("\"opt_in\":true"), body)
        XCTAssertTrue(body.contains("\"goal_text\":\"Aim\""), body)
    }

    func testMaxGoalTextLengthMatchesServer() {
        // The server rejects >1000 chars with a 400
        // (models.AIGoalTextMaxLength); the editor disables Save
        // at the same boundary.
        XCTAssertEqual(CoachPreferencesDTO.maxGoalTextLength, 1000)
    }
}
