import Foundation
import XCTest
@testable import Hylete

/// Tests for the Workout Player: the `?include=items` wire shape,
/// link-ID back-compat on exercise entries, resume bucketing onto
/// planned items, hybrid ready-to-complete derivation, and
/// on-device draft persistence.
///
/// Network paths run through the `StubURLProtocol` helper defined
/// in `WorkoutStoreTests.swift` (same test target) with byte-exact
/// server bodies, so a wire-format mismatch fails here instead of
/// as an error state in the app.
@MainActor
final class WorkoutPlayerTests: XCTestCase {

    /// Captured `GET /api/v1/workouts/:id?include=items`.
    private let withItemsBody = """
        {"id":"wo-1","name":"Day 1","description":"","scheduled_date":"2026-09-14","status":"planned","blocks":[{"id":"wb-1","block_id":"blk-1","block_name":"Push","block_description":"","block_type":"standard","position":0,"status":"pending","item_count":2,"items":[{"id":"bi-1","exercise_id":"ex-1","exercise_name":"Squat","exercise_type":"strength","position":0,"target_text":"3x5"},{"id":"bi-2","exercise_id":"ex-2","exercise_name":"Bench","exercise_type":"strength","position":1,"target_text":""}]}],"created_at":"2026-09-14T11:09:48.050784+01:00","updated_at":"2026-09-14T11:09:48.050784+01:00"}
        """

    /// Captured `GET /api/v1/workouts/:id/exercise-entries`: one set
    /// logged against Squat. The stale `workout_block_id` proves
    /// the join pointer does not survive a workout edit — resume
    /// must match on the stable `block_id` + `exercise_id` pair.
    private let resumeBody = """
        [{"id":"e1","exercise_id":"ex-1","exercise_name":"Squat","exercise_type":"strength","reps":5,"weight":100.0,"notes":"","rest_time":90,"duration_seconds":0,"distance_meters":0.0,"avg_heart_rate":0,"calories_burned":0.0,"workout_id":"wo-1","block_id":"blk-1","workout_block_id":"stale-join","created_at":"2026-09-14T12:00:00.000000+01:00"}]
        """

    private var suiteName: String = ""
    private var defaults: UserDefaults!

    override func setUp() {
        super.setUp()
        suiteName = "workout-player-tests-\(UUID().uuidString)"
        defaults = UserDefaults(suiteName: suiteName)!
    }

    override func tearDown() {
        StubURLProtocol.handler = nil
        defaults.removePersistentDomain(forName: suiteName)
        super.tearDown()
    }

    private func makeAPI() -> APIClient {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        let session = URLSession(configuration: config)
        return APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            session: session,
            tokenProvider: { "test-token" }
        )
    }

    private func respond(status: Int = 200, body: String) -> (HTTPURLResponse, Data) {
        let response = HTTPURLResponse(
            url: URL(string: "http://localhost:8080/api/v1/")!,
            statusCode: status,
            httpVersion: nil,
            headerFields: ["Content-Type": "application/json"]
        )!
        return (response, Data(body.utf8))
    }

    private func stubPlayerPaths() {
        StubURLProtocol.handler = { [resumeBody, withItemsBody] request in
            if request.url?.absoluteString.contains("exercise-entries") == true {
                return self.respond(body: resumeBody)
            }
            return self.respond(body: withItemsBody)
        }
    }

    private func decodeWorkout() throws -> WorkoutWithItemsDTO {
        try APIClient.jsonDecoder.decode(
            WorkoutWithItemsDTO.self,
            from: Data(withItemsBody.utf8)
        )
    }

    // MARK: - Wire shapes

    func testWithItemsDecodesBlocksWithItems() throws {
        let workout = try decodeWorkout()
        XCTAssertEqual(workout.blocks.count, 1)
        XCTAssertEqual(workout.blocks[0].items.count, 2)
        XCTAssertEqual(workout.blocks[0].items[0].exerciseName, "Squat")
        XCTAssertEqual(workout.blocks[0].items[0].targetText, "3x5")
    }

    func testWithItemsToleratesMissingItemsKey() throws {
        // Older servers omit `items` (and `block_description`):
        // the player must decode to an empty row, not throw.
        let body = """
            {"id":"wo-1","name":"Day 1","description":"","scheduled_date":"2026-09-14","status":"planned","blocks":[{"id":"wb-1","block_id":"blk-1","block_name":"Push","block_type":"standard","position":0,"status":"pending","item_count":0}],"created_at":"2026-09-14T11:09:48.050784+01:00","updated_at":"2026-09-14T11:09:48.050784+01:00"}
            """
        let workout = try APIClient.jsonDecoder.decode(
            WorkoutWithItemsDTO.self,
            from: Data(body.utf8)
        )
        XCTAssertEqual(workout.blocks[0].items, [])
    }

    func testEntryDecodesWithoutLinkKeys() throws {
        // Entries cached by older app builds (or logged outside a
        // workout) carry no link keys: they decode to nil links.
        let body = """
            {"id":"e1","exercise_id":"ex-1","exercise_name":"Squat","exercise_type":"strength","reps":5,"weight":100.0,"notes":"","rest_time":90,"duration_seconds":0,"distance_meters":0.0,"avg_heart_rate":0,"calories_burned":0.0,"created_at":"2026-09-14T12:00:00.000000+01:00"}
            """
        let entry = try APIClient.jsonDecoder.decode(
            ExerciseEntryDTO.self,
            from: Data(body.utf8)
        )
        XCTAssertNil(entry.workoutID)
        XCTAssertNil(entry.blockID)
        XCTAssertNil(entry.workoutBlockID)
    }

    func testCreateSetInputEncodesLinkIDs() throws {
        let input = CreateSetInput(
            reps: 5, weight: 100, restTime: 90,
            workoutID: "wo-1", blockID: "blk-1", workoutBlockID: "wb-1"
        )
        let data = try APIClient.jsonEncoder.encode(input)
        let json = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        XCTAssertEqual(json?["workout_id"] as? String, "wo-1")
        XCTAssertEqual(json?["block_id"] as? String, "blk-1")
        XCTAssertEqual(json?["workout_block_id"] as? String, "wb-1")
    }

    // MARK: - Resume + progress

    func testLoadSeedsLoggedCountsByStableIDs() async throws {
        stubPlayerPaths()
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        XCTAssertNil(store.errorMessage)
        // Squat (bi-1) has 1 linked set; Bench (bi-2) has none.
        // The stale join pointer is ignored — the match is on
        // block_id + exercise_id.
        XCTAssertEqual(store.loggedCount(itemID: "bi-1"), 1)
        XCTAssertEqual(store.loggedCount(itemID: "bi-2"), 0)
        XCTAssertTrue(store.isItemDone(itemID: "bi-1"))
        XCTAssertFalse(store.isItemDone(itemID: "bi-2"))
        XCTAssertFalse(store.isBlockReady(store.workout!.blocks[0]))
    }

    func testSkipCountsTowardCompletion() async throws {
        stubPlayerPaths()
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        // Bench skipped (e.g. no equipment): the block is ready to
        // mark done even though only Squat logged sets.
        store.toggleSkip(itemID: "bi-2")
        XCTAssertTrue(store.isItemDone(itemID: "bi-2"))
        XCTAssertTrue(store.isBlockReady(store.workout!.blocks[0]))
        let progress = store.overallProgress()
        XCTAssertEqual(progress.done, 2)
        XCTAssertEqual(progress.total, 2)
    }

    func testDraftsPersistAcrossRestarts() async throws {
        stubPlayerPaths()
        let first = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await first.load()
        var rows = first.drafts["bi-2"] ?? []
        XCTAssertFalse(rows.isEmpty)
        rows[0].reps = 8
        rows[0].weightText = "60"
        first.setDrafts(rows, for: "bi-2")
        first.toggleSkip(itemID: "bi-1")

        // A fresh store (same on-device suite, e.g. after a kill)
        // restores the half-typed rows and skips on load.
        let second = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await second.load()
        XCTAssertEqual(second.drafts["bi-2"]?.first?.reps, 8)
        XCTAssertEqual(second.drafts["bi-2"]?.first?.weightText, "60")
        XCTAssertTrue(second.skippedItemIDs.contains("bi-1"))
    }

    func testSetInputMapping() {
        // Strength drafts map reps/weight/rest with the linkage.
        var strength = SetDraft(distanceUnit: "km")
        strength.reps = 5
        strength.weightText = "100"
        strength.restSeconds = 90
        let strengthInput = strength.setInput(workoutID: "wo-1", blockID: "blk-1", workoutBlockID: "wb-1")
        XCTAssertEqual(strengthInput.reps, 5)
        XCTAssertEqual(strengthInput.workoutID, "wo-1")
        XCTAssertEqual(strengthInput.blockID, "blk-1")
        XCTAssertEqual(strengthInput.workoutBlockID, "wb-1")

        // Cardio drafts map duration/distance (km to metres).
        var cardio = SetDraft(distanceUnit: "km")
        cardio.durationMinutesText = "25"
        cardio.distanceText = "5"
        let cardioInput = cardio.setInput(workoutID: "wo-1", blockID: "blk-1", workoutBlockID: "wb-1")
        XCTAssertEqual(cardioInput.durationSeconds, 1500)
        XCTAssertEqual(cardioInput.distanceMeters, 5000, accuracy: 0.001)
        XCTAssertEqual(cardioInput.reps, 0)
    }
}
