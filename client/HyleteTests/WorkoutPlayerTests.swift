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
        // mark done even though only Squat logged sets. The block
        // itself is still pending, so blocks-based progress is 0/1.
        store.toggleSkip(itemID: "bi-2")
        XCTAssertTrue(store.isItemDone(itemID: "bi-2"))
        XCTAssertTrue(store.isBlockReady(store.workout!.blocks[0]))
        let progress = store.blockProgress()
        XCTAssertEqual(progress.done, 0)
        XCTAssertEqual(progress.total, 1)
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

    /// Regression test for the "unexpected response" player bug:
    /// concurrent decodes used to race on one shared date
    /// formatter's mutable options and intermittently fail. Hammer
    /// both player payloads from parallel tasks — every decode
    /// must succeed. Run with Thread Sanitizer to prove the race
    /// is gone (it flagged the old shared-mutable-formatter code).
    func testConcurrentDecodesAlwaysSucceed() async throws {
        let workoutData = Data(withItemsBody.utf8)
        let resumeData = Data(resumeBody.utf8)
        try await withThrowingTaskGroup(of: Void.self) { group in
            for _ in 0..<50 {
                group.addTask {
                    let workout = try APIClient.jsonDecoder.decode(
                        WorkoutWithItemsDTO.self,
                        from: workoutData
                    )
                    XCTAssertEqual(workout.blocks.count, 1)
                }
                group.addTask {
                    let entries = try APIClient.jsonDecoder.decode(
                        [ExerciseEntryDTO].self,
                        from: resumeData
                    )
                    XCTAssertEqual(entries.count, 1)
                }
            }
            try await group.waitForAll()
        }
    }

    // MARK: - Autosubmit + notes

    /// Lock-guarded POST-body capture. The stub handler runs on a
    /// URLSession thread, so plain array appends would race.
    private final class PostCapture: @unchecked Sendable {
        private let lock = NSLock()
        private var _bodies: [[String: Any]] = []

        func append(_ json: [String: Any]) {
            lock.lock()
            defer { lock.unlock() }
            _bodies.append(json)
        }

        var bodies: [[String: Any]] {
            lock.lock()
            defer { lock.unlock() }
            return _bodies
        }
    }

    /// Reads a captured request's body. Stubs at the
    /// `URLProtocol` layer may receive the body as a stream
    /// instead of `httpBody` — without this, POST captures are
    /// silently empty while the requests still succeed.
    private func stubBody(of request: URLRequest) -> Data? {
        if let body = request.httpBody, !body.isEmpty { return body }
        guard let stream = request.httpBodyStream else { return nil }
        stream.open()
        defer { stream.close() }
        var data = Data()
        while stream.hasBytesAvailable {
            var buffer = [UInt8](repeating: 0, count: 4096)
            let count = stream.read(&buffer, maxLength: buffer.count)
            if count <= 0 { break }
            data.append(buffer, count: count)
        }
        return data.isEmpty ? nil : data
    }

    /// Player GET stubs plus POST capture. `postStatus`/`failExercise`
    /// let failure tests fail one item's submit while the other
    /// succeeds: `failExercise` names the `exercise_id` whose POST
    /// returns `postStatus`.
    private func stubPlayerPathsCapturingPosts(
        posts: PostCapture,
        postStatus: Int = 201,
        failExercise: String? = nil
    ) {
        StubURLProtocol.handler = { [resumeBody, withItemsBody] request in
            if request.httpMethod == "POST" {
                let body = self.stubBody(of: request) ?? Data()
                if let json = try? JSONSerialization.jsonObject(with: body) as? [String: Any] {
                    posts.append(json)
                }
                let exerciseID = (try? JSONSerialization.jsonObject(with: body) as? [String: Any])
                    .flatMap { $0["exercise_id"] as? String }
                if let failExercise, exerciseID == failExercise {
                    return self.respond(status: postStatus, body: #"{"error":"boom"}"#)
                }
                // One created entry per POST keeps `loggedCounts`
                // math exact (+1 per successful submit).
                return self.respond(status: postStatus, body: resumeBody)
            }
            if request.url?.absoluteString.contains("exercise-entries") == true {
                return self.respond(body: resumeBody)
            }
            return self.respond(body: withItemsBody)
        }
    }

    private func validStrengthDraft(reps: Int, weight: String) -> SetDraft {
        var row = SetDraft(distanceUnit: "kg")
        row.reps = reps
        row.weightText = weight
        row.restSeconds = 90
        return row
    }

    func testLogAllValidSubmitsEachItemWithValidDrafts() async throws {
        let posts = PostCapture()
        stubPlayerPathsCapturingPosts(posts: posts)
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        XCTAssertNil(store.errorMessage)
        store.setDrafts([validStrengthDraft(reps: 5, weight: "100")], for: "bi-1")
        store.setDrafts([validStrengthDraft(reps: 8, weight: "60")], for: "bi-2")
        store.setNotes("  felt strong  ", for: "bi-1")

        let ok = await store.logAllValid(in: store.workout!.blocks[0])

        XCTAssertTrue(ok)
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(posts.bodies.count, 2)
        // Notes ride on the item's own log call (trimmed);
        // the other item posts empty notes.
        let squatPost = posts.bodies.first { $0["exercise_id"] as? String == "ex-1" }
        XCTAssertEqual(squatPost?["notes"] as? String, "felt strong")
        let benchPost = posts.bodies.first { $0["exercise_id"] as? String == "ex-2" }
        XCTAssertEqual(benchPost?["notes"] as? String, "")
        // Resume seeded bi-1 with 1; each POST adds one more.
        XCTAssertEqual(store.loggedCount(itemID: "bi-1"), 2)
        XCTAssertEqual(store.loggedCount(itemID: "bi-2"), 1)
    }

    func testLogAllValidSkipsSkippedItemsAndEmptyDrafts() async throws {
        let posts = PostCapture()
        stubPlayerPathsCapturingPosts(posts: posts)
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        store.toggleSkip(itemID: "bi-2")
        // bi-1 keeps its empty starter draft: nothing submittable,
        // so Done proceeds with zero requests (skip-only path).
        let ok = await store.logAllValid(in: store.workout!.blocks[0])

        XCTAssertTrue(ok)
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(posts.bodies.count, 0)
    }

    func testLogAllValidAbortsOnFirstFailure() async throws {
        let posts = PostCapture()
        stubPlayerPathsCapturingPosts(posts: posts, postStatus: 500, failExercise: "ex-1")
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        store.setDrafts([validStrengthDraft(reps: 5, weight: "100")], for: "bi-1")
        store.setDrafts([validStrengthDraft(reps: 8, weight: "60")], for: "bi-2")

        // bi-1 sorts before bi-2: its failure must stop the run
        // before bi-2 is attempted (the Done handler then skips
        // the status write on `false`).
        let ok = await store.logAllValid(in: store.workout!.blocks[0])

        XCTAssertFalse(ok)
        XCTAssertNotNil(store.errorMessage)
        XCTAssertEqual(posts.bodies.count, 1)
        XCTAssertEqual(store.loggedCount(itemID: "bi-1"), 1)
        XCTAssertEqual(store.loggedCount(itemID: "bi-2"), 0)
    }

    func testNotesTrimCapAndPersist() async throws {
        stubPlayerPaths()
        let first = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await first.load()
        first.setNotes("   ", for: "bi-1")
        XCTAssertNil(first.notes["bi-1"])
        first.setNotes("  easy  ", for: "bi-1")
        XCTAssertEqual(first.notes["bi-1"], "easy")
        first.setNotes(String(repeating: "x", count: 600), for: "bi-2")
        XCTAssertEqual(first.notes["bi-2"]?.count, 500)

        let second = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await second.load()
        XCTAssertEqual(second.notes["bi-1"], "easy")
        XCTAssertEqual(second.notes["bi-2"]?.count, 500)
    }

    func testOldSnapshotWithoutNotesKeyStillDecodes() async throws {
        // Snapshots written before notes existed carry no `notes`
        // key: they must decode with empty notes, not throw.
        defaults.set(
            Data(#"{"drafts":{},"skipped":[]}"#.utf8),
            forKey: "hylete.workout-player.wo-1"
        )
        stubPlayerPaths()
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(store.notes, [:])
    }

    // MARK: - Done-block resubmission

    /// Regression test for the reopened-workout bug: flipping a
    /// completed workout back to In Progress leaves its blocks
    /// done (only the workout status changes), and done blocks
    /// had no submit button — newly typed sets could never post.
    /// The server accepts links to done blocks (it validates link
    /// ownership, never statuses), so the client just needs the
    /// `hasValidDrafts` gate and the unchanged `logAllValid` path.
    func testLogAllValidSubmitsToDoneBlock() async throws {
        let posts = PostCapture()
        stubPlayerPathsCapturingPosts(posts: posts)
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        XCTAssertNil(store.errorMessage)
        // Reopened session: the block reads done, the user types
        // fresh sets into it.
        var doneBlock = store.workout!.blocks[0]
        doneBlock = WorkoutBlockDetailDTO(
            id: doneBlock.id,
            blockID: doneBlock.blockID,
            blockName: doneBlock.blockName,
            blockType: doneBlock.blockType,
            position: doneBlock.position,
            status: .done,
            itemCount: doneBlock.itemCount,
            items: doneBlock.items
        )
        store.setDrafts([validStrengthDraft(reps: 5, weight: "100")], for: "bi-1")

        XCTAssertTrue(store.hasValidDrafts(in: doneBlock))
        let ok = await store.logAllValid(in: doneBlock)

        XCTAssertTrue(ok)
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(posts.bodies.count, 1)
        XCTAssertEqual(posts.bodies.first?["exercise_id"] as? String, "ex-1")
    }

    func testHasValidDraftsIgnoresEmptyAndSkipped() async throws {
        stubPlayerPaths()
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        let block = store.workout!.blocks[0]
        // Starter drafts are empty: nothing submittable.
        XCTAssertFalse(store.hasValidDrafts(in: block))
        // A skipped item's valid rows don't count either.
        store.setDrafts([validStrengthDraft(reps: 5, weight: "100")], for: "bi-1")
        store.setDrafts([validStrengthDraft(reps: 8, weight: "60")], for: "bi-2")
        store.toggleSkip(itemID: "bi-1")
        store.toggleSkip(itemID: "bi-2")
        XCTAssertFalse(store.hasValidDrafts(in: block))
        store.toggleSkip(itemID: "bi-1")
        XCTAssertTrue(store.hasValidDrafts(in: block))
    }

    // MARK: - Blocks-based progress

    private func detailBlock(status: WorkoutBlockStatusDTO) -> WorkoutBlockDetailDTO {
        WorkoutBlockDetailDTO(
            id: "wb-\(status.rawValue)",
            blockID: "blk-1",
            blockName: "Push",
            blockType: .standard,
            position: 0,
            status: status,
            itemCount: 0,
            items: []
        )
    }

    func testBlockProgressCountsDoneAndSkipped() {
        // Pending blocks don't move the number; done and skipped
        // both count as finished.
        let blocks: [WorkoutBlockDetailDTO] = [
            detailBlock(status: .pending),
            detailBlock(status: .done),
            detailBlock(status: .skipped),
        ]
        XCTAssertEqual(WorkoutPlayerStore.completedBlockCount(in: blocks), 2)
        XCTAssertEqual(WorkoutPlayerStore.completedBlockCount(in: []), 0)
    }

    func testBlockProgressFromLoadIsZeroOfOne() async throws {
        stubPlayerPaths()
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        await store.load()
        // Single pending block: 0/1, i.e. "0% complete".
        // Item-level activity (logged resume sets, skips) does not
        // move the headline number — only block check-offs do.
        store.toggleSkip(itemID: "bi-2")
        let progress = store.blockProgress()
        XCTAssertEqual(progress.done, 0)
        XCTAssertEqual(progress.total, 1)
    }

    func testBlockProgressWithNoWorkoutIsZero() {
        let store = WorkoutPlayerStore(workoutID: "wo-1", api: makeAPI(), defaults: defaults)
        XCTAssertEqual(store.blockProgress().done, 0)
        XCTAssertEqual(store.blockProgress().total, 0)
    }

    func testBlockTimingDecodes() throws {
        // The player fetch carries each type's time config; the
        // subtitle helper turns it into the header line.
        let body = """
            {"id":"wo-1","name":"Day 1","description":"","scheduled_date":"2026-09-14","status":"planned","blocks":[{"id":"wb-1","block_id":"blk-1","block_name":"Metcon","block_description":"","block_type":"emom","position":0,"status":"pending","item_count":0,"items":[],"rounds":12,"rest_seconds":0,"time_cap_seconds":0,"interval_seconds":60}],"created_at":"2026-09-14T11:09:48.050784+01:00","updated_at":"2026-09-14T11:09:48.050784+01:00"}
            """
        let workout = try APIClient.jsonDecoder.decode(
            WorkoutWithItemsDTO.self,
            from: Data(body.utf8)
        )
        let block = workout.blocks[0]
        XCTAssertEqual(block.rounds, 12)
        XCTAssertEqual(block.intervalSeconds, 60)
        XCTAssertEqual(block.timingSummary, "12 rounds × Every 60s")
    }

    func testBlockTimingMissingKeysDefaultToZero() throws {
        // Servers predating the timing keys (and standard blocks)
        // decode to zeros with an empty summary — never a throw.
        let workout = try decodeWorkout()
        let block = workout.blocks[0]
        XCTAssertEqual(block.rounds, 0)
        XCTAssertEqual(block.restSeconds, 0)
        XCTAssertEqual(block.timeCapSeconds, 0)
        XCTAssertEqual(block.intervalSeconds, 0)
        XCTAssertEqual(block.timingSummary, "")
    }

    func testTimingSummaryPerType() {
        func block(type: BlockTypeDTO, rounds: Int, rest: Int, cap: Int, interval: Int) -> WorkoutBlockDetailDTO {
            WorkoutBlockDetailDTO(
                id: "wb-1", blockID: "blk-1", blockName: "B",
                blockType: type, position: 0, status: .pending,
                itemCount: 0, rounds: rounds, restSeconds: rest,
                timeCapSeconds: cap, intervalSeconds: interval
            )
        }
        XCTAssertEqual(block(type: .circuit, rounds: 4, rest: 90, cap: 0, interval: 0).timingSummary, "4 rounds · 90s rest")
        XCTAssertEqual(block(type: .amrap, rounds: 0, rest: 0, cap: 600, interval: 0).timingSummary, "10 mins")
        XCTAssertEqual(block(type: .emom, rounds: 12, rest: 0, cap: 0, interval: 60).timingSummary, "12 rounds × Every 60s")
        XCTAssertEqual(block(type: .standard, rounds: 0, rest: 0, cap: 0, interval: 0).timingSummary, "")
    }

    func testBlockDescriptionDecodes() throws {
        // (Plain escaped strings: `#""...""#` raw literals swallow
        // a quote at each boundary and corrupt the JSON.)
        let body = withItemsBody.replacingOccurrences(
            of: "\"block_description\":\"\"",
            with: "\"block_description\":\"Rest 2 min between rounds\""
        )
        let workout = try APIClient.jsonDecoder.decode(
            WorkoutWithItemsDTO.self,
            from: Data(body.utf8)
        )
        XCTAssertEqual(workout.blocks[0].blockDescription, "Rest 2 min between rounds")
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
