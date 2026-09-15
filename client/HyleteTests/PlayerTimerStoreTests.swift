import Foundation
import XCTest
@testable import Hylete

/// Tests for `PlayerTimerStore`: the player-scoped single
/// shared timer. All time travel goes through the explicit
/// `now:` parameters — no test ever sleeps.
@MainActor
final class PlayerTimerStoreTests: XCTestCase {

    private var store: PlayerTimerStore!
    private let now = Date(timeIntervalSince1970: 1_000_000)

    override func setUp() {
        super.setUp()
        store = PlayerTimerStore()
    }

    // MARK: - Idle

    func testStartsIdle() {
        XCTAssertFalse(store.isActive)
        XCTAssertFalse(store.isRunning)
        XCTAssertFalse(store.isComplete)
        XCTAssertNil(store.endDate)
    }

    // MARK: - Rest

    func testStartRestArmsCountdown() {
        store.startRest(seconds: 90, blockID: "wb-1", blockName: "Push", now: now)

        XCTAssertTrue(store.isActive)
        XCTAssertTrue(store.isRunning)
        XCTAssertFalse(store.isComplete)
        XCTAssertEqual(store.kind, .rest)
        XCTAssertEqual(store.blockID, "wb-1")
        XCTAssertEqual(store.remainingSeconds(at: now), 90)
        XCTAssertEqual(store.displayString(at: now), "1:30")
        XCTAssertEqual(store.subtitle, "Rest · Push")
    }

    func testStartRestIgnoresNonPositiveDurations() {
        store.startRest(seconds: 0, blockID: "wb-1", blockName: "Push", now: now)
        XCTAssertFalse(store.isActive)
        store.startRest(seconds: -5, blockID: "wb-1", blockName: "Push", now: now)
        XCTAssertFalse(store.isActive)
    }

    func testRemainingCountsDownAndClampsAtZero() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)

        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(10)), 50)
        // Past the fire date the display clamps rather than
        // going negative (the pill's sleeper completes it).
        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(500)), 0)
    }

    func testPauseFreezesAndResumeRearmsFromRemainder() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)

        store.pause(now: now.addingTimeInterval(25))
        XCTAssertFalse(store.isRunning)
        XCTAssertNil(store.endDate)
        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(999)), 35)

        store.resume(now: now.addingTimeInterval(100))
        XCTAssertTrue(store.isRunning)
        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(100)), 35)
        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(135)), 0)
    }

    func testCompleteNowTransitionsOnce() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)

        XCTAssertTrue(store.completeNow())
        XCTAssertTrue(store.isComplete)
        XCTAssertFalse(store.isRunning)
        XCTAssertNil(store.endDate)
        // Second call is a no-op so callers can't double-fire
        // the completion haptic.
        XCTAssertFalse(store.completeNow())
    }

    func testResetReturnsToIdle() {
        store.startRest(seconds: 60, blockID: "wb-1", blockName: "Push", now: now)
        store.reset()

        XCTAssertFalse(store.isActive)
        XCTAssertFalse(store.isRunning)
        XCTAssertFalse(store.isComplete)
        XCTAssertNil(store.endDate)
        XCTAssertNil(store.blockID)
    }

    func testNewStartReplacesRunningTimer() {
        store.startRest(seconds: 60, blockID: "wb-1", blockName: "Push", now: now)
        store.startRest(seconds: 30, blockID: "wb-2", blockName: "Pull", now: now)

        XCTAssertTrue(store.isActive)
        XCTAssertFalse(store.isComplete)
        XCTAssertEqual(store.blockID, "wb-2")
        XCTAssertEqual(store.remainingSeconds(at: now), 30)
        XCTAssertEqual(store.subtitle, "Rest · Pull")
    }

    // MARK: - EMOM

    func testStartEMOMHonoursCustomInterval() {
        // Blocks may use non-60s intervals — the player must
        // not inherit the standalone view's hardcoded 60s.
        store.startEMOM(rounds: 4, intervalSeconds: 90, blockID: "wb-9", blockName: "Engine", now: now)

        XCTAssertEqual(store.kind, .emom)
        XCTAssertEqual(store.totalRounds, 4)
        XCTAssertEqual(store.currentRound, 1)
        XCTAssertEqual(store.intervalSeconds, 90)
        XCTAssertEqual(store.remainingSeconds(at: now), 90)
        XCTAssertEqual(store.subtitle, "Round 1/4 · Engine")
    }

    func testStartEMOMIgnoresInvalidConfig() {
        store.startEMOM(rounds: 0, intervalSeconds: 60, blockID: nil, blockName: "E", now: now)
        XCTAssertFalse(store.isActive)
        store.startEMOM(rounds: 5, intervalSeconds: 0, blockID: nil, blockName: "E", now: now)
        XCTAssertFalse(store.isActive)
    }

    func testAdvanceRoundStepsThroughRoundsThenCompletes() {
        store.startEMOM(rounds: 3, intervalSeconds: 60, blockID: nil, blockName: "Engine", now: now)

        XCTAssertTrue(store.advanceRound(now: now.addingTimeInterval(60)))
        XCTAssertEqual(store.currentRound, 2)
        XCTAssertTrue(store.isRunning)
        XCTAssertFalse(store.isComplete)
        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(60)), 60)
        XCTAssertEqual(store.subtitle, "Round 2/3 · Engine")

        XCTAssertTrue(store.advanceRound(now: now.addingTimeInterval(120)))
        XCTAssertEqual(store.currentRound, 3)

        XCTAssertTrue(store.advanceRound(now: now.addingTimeInterval(180)))
        XCTAssertTrue(store.isComplete)
        XCTAssertFalse(store.isRunning)

        // Nothing left to advance — callers must not fire
        // another round-boundary haptic.
        XCTAssertFalse(store.advanceRound(now: now.addingTimeInterval(240)))
    }

    func testAdvanceRoundIsNoOpOffEMOM() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)
        XCTAssertFalse(store.advanceRound(now: now))
        store.reset()
        XCTAssertFalse(store.advanceRound(now: now))
    }

    func testEMOMPauseKeepsRemainderNotFullRound() {
        store.startEMOM(rounds: 3, intervalSeconds: 60, blockID: nil, blockName: "Engine", now: now)
        store.pause(now: now.addingTimeInterval(45))
        store.resume(now: now.addingTimeInterval(45))
        // 15s of the round was left — resume continues it
        // rather than restarting a full 60s round.
        XCTAssertEqual(store.remainingSeconds(at: now.addingTimeInterval(45)), 15)
    }

    // MARK: - AMRAP

    func testStartAMRAPCountsDownFromCap() {
        store.startAMRAP(capSeconds: 600, blockID: "wb-3", blockName: "WOD", now: now)

        XCTAssertEqual(store.kind, .amrap)
        XCTAssertTrue(store.isRunning)
        XCTAssertEqual(store.remainingSeconds(at: now), 600)
        XCTAssertEqual(store.displayString(at: now), "10:00")
        XCTAssertEqual(store.subtitle, "AMRAP · WOD")
    }

    func testStartAMRAPIgnoresNonPositiveCap() {
        store.startAMRAP(capSeconds: 0, blockID: nil, blockName: "WOD", now: now)
        XCTAssertFalse(store.isActive)
    }

    // MARK: - Events

    func testStartsPublishNoEvent() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)
        XCTAssertNil(store.lastEvent)
        store.pause(now: now.addingTimeInterval(10))
        XCTAssertNil(store.lastEvent)
        store.resume(now: now.addingTimeInterval(10))
        XCTAssertNil(store.lastEvent)
    }

    func testCompleteNowPublishesCompletionOnce() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)
        XCTAssertTrue(store.completeNow())
        guard case .sessionComplete = store.lastEvent else {
            return XCTFail("Expected a sessionComplete event")
        }
        let first = store.lastEvent
        XCTAssertFalse(store.completeNow())
        XCTAssertEqual(store.lastEvent, first)
    }

    func testAdvanceRoundPublishesBoundaryThenCompletion() {
        store.startEMOM(rounds: 2, intervalSeconds: 60, blockID: nil, blockName: "Engine", now: now)

        XCTAssertTrue(store.advanceRound(now: now.addingTimeInterval(60)))
        guard case .roundBoundary = store.lastEvent else {
            return XCTFail("Expected a roundBoundary event")
        }
        let boundary = store.lastEvent

        XCTAssertTrue(store.advanceRound(now: now.addingTimeInterval(120)))
        guard case .sessionComplete = store.lastEvent else {
            return XCTFail("Expected a sessionComplete event")
        }
        XCTAssertNotEqual(store.lastEvent, boundary)
    }
    func testResetAndNewStartClearEvent() {
        store.startRest(seconds: 60, blockID: nil, blockName: "Push", now: now)
        store.completeNow()
        XCTAssertNotNil(store.lastEvent)
        store.reset()
        XCTAssertNil(store.lastEvent)

        store.startRest(seconds: 30, blockID: nil, blockName: "Push", now: now)
        store.completeNow()
        XCTAssertNotNil(store.lastEvent)
        store.startRest(seconds: 30, blockID: nil, blockName: "Push", now: now)
        XCTAssertNil(store.lastEvent)
    }

    // MARK: - Boundary callout

    func testBoundaryCallout() {
        XCTAssertEqual(PlayerTimerStore.boundaryCallout(round: 1), "Round 1 — go!")
        XCTAssertEqual(PlayerTimerStore.boundaryCallout(round: 12), "Round 12 — go!")
    }
}
