import Foundation
import XCTest
@testable import Hylete

/// Tests for `PlayerTimerNotifier`: the bridge between
/// `PlayerTimerStore` fire dates and fallback local
/// notifications. A mock center records schedule/cancel
/// calls; short sleeps let the notifier's unstructured
/// `Task` run — no test ever waits on a real timer.
@MainActor
final class PlayerTimerNotifierTests: XCTestCase {

    private final class MockCenter: TimerNotificationCenter, @unchecked Sendable {
        var status: TimerNotificationAuthStatus = .notDetermined
        var grantOnRequest = true
        var requestCount = 0
        var scheduled: [(id: String, title: String, body: String)] = []
        var cancelled: [String] = []

        func authorizationStatus() async -> TimerNotificationAuthStatus { status }

        func requestAuthorization() async throws -> Bool {
            requestCount += 1
            if grantOnRequest { status = .granted }
            return grantOnRequest
        }

        func schedule(id: String, at date: Date, title: String, body: String) async throws {
            scheduled.append((id, title, body))
        }

        func cancel(id: String) {
            cancelled.append(id)
        }
    }

    private var center: MockCenter!
    private var notifier: PlayerTimerNotifier!
    private var store: PlayerTimerStore!
    private let now = Date(timeIntervalSince1970: 2_000_000)

    override func setUp() {
        super.setUp()
        center = MockCenter()
        notifier = PlayerTimerNotifier(center: center)
        store = PlayerTimerStore()
    }

    /// Lets the notifier's unstructured schedule Task run.
    private func settle() async throws {
        try await Task.sleep(for: .milliseconds(100))
    }

    // MARK: - Permission

    func testFirstStartPromptsThenSchedules() async throws {
        store.startRest(seconds: 90, blockID: "wb-1", blockName: "Push", now: now)
        notifier.timerFireDateChanged(Date().addingTimeInterval(90), store: store)
        try await settle()

        XCTAssertEqual(center.requestCount, 1)
        XCTAssertEqual(center.scheduled.count, 1)
        XCTAssertEqual(center.scheduled.first?.title, "Rest over")
        XCTAssertEqual(notifier.status, .granted)
    }

    func testDeniedSchedulesNothingAndNeverPromptsAgain() async throws {
        center.status = .denied
        store.startRest(seconds: 90, blockID: "wb-1", blockName: "Push", now: now)
        notifier.timerFireDateChanged(Date().addingTimeInterval(90), store: store)
        try await settle()

        XCTAssertEqual(center.requestCount, 0)
        XCTAssertTrue(center.scheduled.isEmpty)
        XCTAssertEqual(notifier.status, .denied)
    }

    func testGrantedSkipsPrompt() async throws {
        center.status = .granted
        store.startRest(seconds: 90, blockID: "wb-1", blockName: "Push", now: now)
        notifier.timerFireDateChanged(Date().addingTimeInterval(90), store: store)
        try await settle()

        XCTAssertEqual(center.requestCount, 0)
        XCTAssertEqual(center.scheduled.count, 1)
    }

    // MARK: - Lifecycle

    func testPauseCancelsPending() async throws {
        center.status = .granted
        store.startRest(seconds: 90, blockID: "wb-1", blockName: "Push", now: now)
        notifier.timerFireDateChanged(Date().addingTimeInterval(90), store: store)
        try await settle()
        let id = try XCTUnwrap(center.scheduled.first?.id)

        store.pause(now: now)
        notifier.timerFireDateChanged(nil, store: store)

        XCTAssertEqual(center.cancelled, [id])
    }

    func testReplaceCancelsOldAndSchedulesNew() async throws {
        center.status = .granted
        store.startRest(seconds: 60, blockID: "wb-1", blockName: "Push", now: now)
        notifier.timerFireDateChanged(Date().addingTimeInterval(60), store: store)
        try await settle()
        let firstID = try XCTUnwrap(center.scheduled.first?.id)

        store.startRest(seconds: 30, blockID: "wb-2", blockName: "Pull", now: now)
        notifier.timerFireDateChanged(Date().addingTimeInterval(30), store: store)
        try await settle()

        XCTAssertEqual(center.cancelled, [firstID])
        XCTAssertEqual(center.scheduled.count, 2)
    }

    func testPastDateSchedulesNothing() async throws {
        center.status = .granted
        notifier.timerFireDateChanged(Date.distantPast, store: store)
        try await settle()

        XCTAssertTrue(center.scheduled.isEmpty)
    }

    // MARK: - Copy

    func testContentPerKind() {
        store.startRest(seconds: 90, blockID: "wb-1", blockName: "Push", now: now)
        XCTAssertEqual(PlayerTimerNotifier.content(for: store).title, "Rest over")

        store.startAMRAP(capSeconds: 600, blockID: "wb-3", blockName: "WOD", now: now)
        XCTAssertEqual(PlayerTimerNotifier.content(for: store).title, "Time!")

        store.startEMOM(rounds: 3, intervalSeconds: 60, blockID: "wb-9", blockName: "Engine", now: now)
        XCTAssertEqual(PlayerTimerNotifier.content(for: store).title, "Round 1 done")

        store.advanceRound(now: now.addingTimeInterval(60))
        store.advanceRound(now: now.addingTimeInterval(120))
        XCTAssertEqual(PlayerTimerNotifier.content(for: store).title, "EMOM complete")
    }
}
