import XCTest
@testable import Hylete

/// Tests for the Workouts DTOs, API paths, and stores — the iOS
/// mirror of `internal/routes/api_workouts.go` plus the
/// exercise-entry linkage fields. Covers golden decodes (workout
/// detail with logged/ad-hoc split), the old-server leniency
/// rules (missing linkage keys, unknown block types), snake_case
/// request encoding, and the store's optimistic status
/// transitions (including rollback).
final class WorkoutTests: XCTestCase {

    // MARK: - Fixtures

    private var workoutDetailJSON: String {
        """
        {"id":"w-1","name":"Monday legs","notes":"","status":"planned",
         "scheduled_start":"2026-09-07T18:00:00Z","scheduled_end":"2026-09-07T19:00:00Z",
         "blocks":[{"id":"wb-1","type":"superset","position":0,"rounds":3,
          "rest_between_rounds_seconds":90,"interval_seconds":0,"time_cap_seconds":0,
          "items":[{"id":"wi-1","exercise_id":"ex-1","exercise_name":"Squat",
           "exercise_type":"strength","position":0,"target_sets":3,"target_reps":8,
           "target_weight":60.0,"target_rest_seconds":120,"target_duration_seconds":0,
           "target_distance_meters":0,"target_avg_heart_rate":0,"target_calories":0}]}],
         "logged_entries":[{"id":"e-1","exercise_id":"ex-1","exercise_name":"Squat",
          "exercise_type":"strength","reps":5,"weight":100.0,"notes":"","rest_time":120,
          "duration_seconds":0,"distance_meters":0,"avg_heart_rate":0,"calories_burned":0,
          "workout_id":"w-1","workout_item_id":"wi-1","round_number":2,
          "created_at":"2026-09-07T18:10:00Z"}],
         "ad_hoc_entries":[],
         "created_at":"2026-09-01T10:00:00Z","updated_at":"2026-09-01T10:00:00Z"}
        """
    }

    // MARK: - Golden decodes

    func testDecodesWorkoutDetailWithTreeAndEntrySplit() throws {
        let detail = try APIClient.jsonDecoder.decode(
            WorkoutDetailDTO.self, from: Data(workoutDetailJSON.utf8))
        XCTAssertEqual(detail.id, "w-1")
        XCTAssertEqual(detail.status, "planned")
        XCTAssertFalse(detail.isCompleted)
        XCTAssertNotNil(detail.scheduledStart)
        XCTAssertEqual(detail.blocks.count, 1)
        let block = detail.blocks[0]
        XCTAssertEqual(block.type, .superset)
        XCTAssertEqual(block.rounds, 3)
        XCTAssertEqual(block.restBetweenRoundsSeconds, 90)
        let item = block.items[0]
        XCTAssertEqual(item.exerciseName, "Squat")
        XCTAssertFalse(item.isCardio)
        XCTAssertEqual(item.targetSets, 3)
        XCTAssertEqual(item.targetReps, 8)
        XCTAssertEqual(item.targetWeight, 60.0)
        XCTAssertEqual(detail.loggedEntries.count, 1)
        XCTAssertTrue(detail.adHocEntries.isEmpty)
        XCTAssertEqual(detail.allLoggedEntries.count, 1)
        let entry = detail.loggedEntries[0]
        XCTAssertEqual(entry.workoutID, "w-1")
        XCTAssertEqual(entry.workoutItemID, "wi-1")
        XCTAssertEqual(entry.roundNumber, 2)
    }

    func testDecodesWorkoutHeaderDates() throws {
        let json = """
        {"id":"w-2","name":"Evening run","notes":"","status":"completed",
         "completed_at":"2026-09-07T19:30:00Z",
         "created_at":"2026-09-01T10:00:00Z","updated_at":"2026-09-07T19:30:00Z"}
        """
        let workout = try APIClient.jsonDecoder.decode(
            WorkoutDTO.self, from: Data(json.utf8))
        XCTAssertTrue(workout.isCompleted)
        XCTAssertFalse(workout.isPlanned)
        XCTAssertNil(workout.scheduledStart)
        XCTAssertNotNil(workout.completedAt)
    }

    // MARK: - Old-server leniency

    func testUnknownBlockTypeFallsBackToStraight() throws {
        let json = """
        {"id":"wb-9","type":"tabata","position":0,"rounds":8,
         "rest_between_rounds_seconds":0,"interval_seconds":0,"time_cap_seconds":0,
         "items":[]}
        """
        let block = try APIClient.jsonDecoder.decode(
            WorkoutBlockDTO.self, from: Data(json.utf8))
        XCTAssertEqual(block.type, .straight)
    }

    func testEntryWithoutLinkageKeysDecodesAsStandalone() throws {
        // Pre-workouts server payload: no workout keys at all.
        let json = """
        {"id":"e-9","exercise_id":"ex-1","exercise_name":"Squat",
         "exercise_type":"strength","reps":5,"weight":100.0,"notes":"",
         "rest_time":120,"duration_seconds":0,"distance_meters":0,
         "avg_heart_rate":0,"calories_burned":0,
         "created_at":"2026-09-07T18:10:00Z"}
        """
        let entry = try APIClient.jsonDecoder.decode(
            ExerciseEntryDTO.self, from: Data(json.utf8))
        XCTAssertNil(entry.workoutID)
        XCTAssertNil(entry.workoutItemID)
        XCTAssertEqual(entry.roundNumber, 0)
    }

    func testBlockTypeDisplayNames() {
        XCTAssertEqual(WorkoutBlockType.straight.displayName, "Straight")
        XCTAssertEqual(WorkoutBlockType.superset.displayName, "Superset")
        XCTAssertEqual(WorkoutBlockType.emom.displayName, "EMOM")
        XCTAssertEqual(WorkoutBlockType.amrap.displayName, "AMRAP")
    }

    // MARK: - Request encoding

    func testCreateWorkoutRequestEncodesSnakeCase() throws {
        let request = CreateWorkoutRequest(
            name: "Lower A",
            notes: "legs",
            blocks: [WorkoutBlockInputRequest(
                type: .superset, rounds: 3, restBetweenRoundsSeconds: 90,
                items: [WorkoutItemInputRequest(
                    exerciseID: "ex-1", targetSets: 3, targetReps: 8, targetWeight: 60)]
            )]
        )
        let raw = try APIClient.jsonEncoder.encode(request)
        let body = String(data: raw, encoding: .utf8) ?? ""
        XCTAssertTrue(body.contains("\"name\":\"Lower A\""), body)
        XCTAssertTrue(body.contains("\"type\":\"superset\""), body)
        XCTAssertTrue(body.contains("\"rest_between_rounds_seconds\":90"), body)
        XCTAssertTrue(body.contains("\"exercise_id\":\"ex-1\""), body)
        XCTAssertTrue(body.contains("\"target_sets\":3"), body)
        XCTAssertTrue(body.contains("\"target_reps\":8"), body)
        XCTAssertFalse(body.contains("template_id"), body)
    }

    func testBulkRequestEncodesInstances() throws {
        let start = Date(timeIntervalSince1970: 1_757_000_000)
        let request = BulkCreateWorkoutsRequest(
            workoutID: "w-1",
            instances: [BulkWorkoutInstanceRequest(scheduledStart: start)]
        )
        let raw = try APIClient.jsonEncoder.encode(request)
        let body = String(data: raw, encoding: .utf8) ?? ""
        XCTAssertTrue(body.contains("\"workout_id\":\"w-1\""), body)
        XCTAssertTrue(body.contains("\"instances\":[{"), body)
    }

    func testDuplicateRequestDefaults() throws {
        let request = DuplicateWorkoutRequest()
        let raw = try APIClient.jsonEncoder.encode(request)
        let body = String(data: raw, encoding: .utf8) ?? ""
        // Blank name (server becomes "Copy of <source>") and no
        // schedule bounds (server inherits the source window).
        XCTAssertTrue(body.contains("\"name\":\"\""), body)
        XCTAssertFalse(body.contains("scheduled_start"), body)
        XCTAssertFalse(body.contains("scheduled_end"), body)
    }

    func testEntryCreateOmitsNilLinkageButSendsRound() throws {
        let standalone = CreateExerciseEntriesRequest(
            exerciseID: "ex-1", notes: "", createdAt: nil,
            sets: [CreateSetInput(reps: 5, weight: 100, restTime: 0)]
        )
        let rawStandalone = try APIClient.jsonEncoder.encode(standalone)
        let bodyStandalone = String(data: rawStandalone, encoding: .utf8) ?? ""
        XCTAssertFalse(bodyStandalone.contains("workout_id"), bodyStandalone)
        XCTAssertFalse(bodyStandalone.contains("workout_item_id"), bodyStandalone)
        XCTAssertTrue(bodyStandalone.contains("\"round_number\":0"), bodyStandalone)

        let linked = CreateExerciseEntriesRequest(
            exerciseID: "ex-1", notes: "", createdAt: nil,
            sets: [CreateSetInput(reps: 5, weight: 100, restTime: 0)],
            workoutID: "w-1", workoutItemID: "wi-1", roundNumber: 2
        )
        let rawLinked = try APIClient.jsonEncoder.encode(linked)
        let bodyLinked = String(data: rawLinked, encoding: .utf8) ?? ""
        XCTAssertTrue(bodyLinked.contains("\"workout_id\":\"w-1\""), bodyLinked)
        XCTAssertTrue(bodyLinked.contains("\"workout_item_id\":\"wi-1\""), bodyLinked)
        XCTAssertTrue(bodyLinked.contains("\"round_number\":2"), bodyLinked)
    }

    // MARK: - Stores (stubbed network)

    func testWorkoutStoreCompleteRollsBackOnFailure() async {
        let listJSON = """
        [{"id":"w-1","name":"Monday legs","notes":"","status":"planned",
          "created_at":"2026-09-01T10:00:00Z","updated_at":"2026-09-01T10:00:00Z"}]
        """
        StubURLProtocol.stub("GET /api/v1/workouts", status: 200, json: listJSON)
        let store = await WorkoutStore(api: StubURLProtocol.api())
        await store.load()
        let planned = await store.plannedWorkouts
        XCTAssertEqual(planned.count, 1)

        // Server rejects the transition: the row must snap back
        // to planned and the error must surface.
        StubURLProtocol.stub(
            "POST /api/v1/workouts/w-1/complete",
            status: 500, json: "{\"error\":\"boom\"}"
        )
        await store.complete(id: "w-1")
        let stillPlanned = await store.plannedWorkouts
        XCTAssertEqual(stillPlanned.count, 1)
        let message = await store.errorMessage
        XCTAssertEqual(message, "boom")
    }

    func testWorkoutStoreBulkCreateReloadsList() async {
        StubURLProtocol.stub("GET /api/v1/workouts", status: 200, json: "[]")
        let store = await WorkoutStore(api: StubURLProtocol.api())
        await store.load()
        let empty = await store.workouts
        XCTAssertTrue(empty.isEmpty)

        let createdJSON = """
        [{"id":"w-1","name":"Lower A","notes":"","status":"planned",
          "created_at":"2026-09-01T10:00:00Z","updated_at":"2026-09-01T10:00:00Z"}]
        """
        StubURLProtocol.stub("POST /api/v1/workouts/bulk", status: 201, json: createdJSON)
        StubURLProtocol.stub("GET /api/v1/workouts", status: 200, json: createdJSON)
        let created = await store.bulkCreate(
            BulkCreateWorkoutsRequest(workoutID: "w-9", instances: [BulkWorkoutInstanceRequest()])
        )
        XCTAssertEqual(created?.count, 1)
        let reloaded = await store.workouts
        XCTAssertEqual(reloaded.count, 1)
    }

    func testWorkoutStoreDuplicateReloadsList() async {
        StubURLProtocol.stub("GET /api/v1/workouts", status: 200, json: "[]")
        let store = await WorkoutStore(api: StubURLProtocol.api())
        await store.load()

        let dupJSON = workoutDetailJSON
            .replacingOccurrences(of: "\"w-1\"", with: "\"w-2\"")
            .replacingOccurrences(of: "\"Monday legs\"", with: "\"Copy of Monday legs\"")
        StubURLProtocol.stub("POST /api/v1/workouts/w-1/duplicate", status: 201, json: dupJSON)
        StubURLProtocol.stub(
            "GET /api/v1/workouts", status: 200,
            json: "[{\"id\":\"w-2\",\"name\":\"Copy of Monday legs\",\"notes\":\"\",\"status\":\"planned\",\"created_at\":\"2026-09-01T10:00:00Z\",\"updated_at\":\"2026-09-01T10:00:00Z\"}]"
        )
        let duplicated = await store.duplicate(id: "w-1")
        XCTAssertEqual(duplicated?.name, "Copy of Monday legs")
        let reloaded = await store.workouts
        XCTAssertEqual(reloaded.count, 1)
        let cached = await store.detail(for: "w-2")
        XCTAssertNotNil(cached)
    }

    // MARK: - Editor drafts

    private var squatExercise: ExerciseDTO {
        ExerciseDTO(
            id: "ex-1", name: "Squat", description: "",
            videoURL: "", imgURL: "", imageURL: "",
            type: "strength"
        )
    }

    private var runExercise: ExerciseDTO {
        ExerciseDTO(
            id: "ex-run", name: "Run", description: "",
            videoURL: "", imgURL: "", imageURL: "",
            type: "cardio"
        )
    }

    func testItemDraftStrengthTargetsOptional() {
        // A blank draft is an open prescription: valid, and it
        // encodes as all-zero targets.
        let draft = WorkoutItemDraft(exercise: squatExercise, distanceUnit: "kg")
        XCTAssertTrue(draft.isValid)
        let open = draft.asRequest()
        XCTAssertEqual(open.targetReps, 0)
        XCTAssertEqual(open.targetWeight, 0)

        var prescribed = WorkoutItemDraft(exercise: squatExercise, distanceUnit: "kg")
        prescribed.targetReps = 8
        prescribed.weightText = "60"
        XCTAssertTrue(prescribed.isValid)
        let request = prescribed.asRequest()
        XCTAssertEqual(request.targetSets, 3)
        XCTAssertEqual(request.targetReps, 8)
        XCTAssertEqual(request.targetWeight, 60.0)
    }

    func testItemDraftCardioConversion() {
        var draft = WorkoutItemDraft(exercise: runExercise, distanceUnit: "mi")
        XCTAssertTrue(draft.isValid, "blank cardio draft is an open prescription")

        draft.durationMinutesText = "25"
        draft.distanceText = "3.1"
        let request = draft.asRequest()
        XCTAssertEqual(request.targetDurationSeconds, 1500)
        XCTAssertEqual(request.targetDistanceMeters, 3.1 * 1609.344, accuracy: 0.01)
    }

    func testBlockDraftValidationAndProblems() {
        var block = WorkoutBlockDraft(type: .superset, rounds: 3)
        XCTAssertFalse(block.isValid)
        XCTAssertNotNil(block.problem, "empty block must name its problem")

        // Exercise-only items complete a block — no numbers needed.
        block.items = [WorkoutItemDraft(exercise: squatExercise, distanceUnit: "km")]
        XCTAssertTrue(block.isValid)
        XCTAssertNil(block.problem)

        var emom = WorkoutBlockDraft(type: .emom)
        emom.items = [WorkoutItemDraft(exercise: squatExercise, distanceUnit: "km")]
        XCTAssertFalse(emom.isValid, "EMOM without an interval is invalid")
        emom.intervalSeconds = 60
        XCTAssertTrue(emom.isValid)
    }

    func testBlockDraftStraightForcesSingleRound() {
        var block = WorkoutBlockDraft(type: .straight, rounds: 5)
        block.items = [WorkoutItemDraft(exercise: squatExercise, distanceUnit: "km")]
        block.items[0].targetReps = 5
        block.items[0].weightText = "100"
        XCTAssertTrue(block.isValid, "straight ignores round extras")
        XCTAssertEqual(block.asRequest().rounds, 1)
    }

    // MARK: - Detail helpers

    func testTargetSummaryFormats() {
        let strength = WorkoutItemDTO(
            id: "wi-1", exerciseID: "ex-1", exerciseName: "Squat",
            exerciseType: "strength", position: 0,
            targetSets: 3, targetReps: 8, targetWeight: 60
        )
        XCTAssertEqual(
            workoutItemTargetSummary(strength, weightUnit: "kg", distanceUnit: "km"),
            "3 × 8 @ 60.0 kg"
        )
        let cardio = WorkoutItemDTO(
            id: "wi-2", exerciseID: "ex-run", exerciseName: "Run",
            exerciseType: "cardio", position: 1, targetSets: 1,
            targetDurationSeconds: 1500, targetDistanceMeters: 5200
        )
        XCTAssertEqual(
            workoutItemTargetSummary(cardio, weightUnit: "kg", distanceUnit: "km"),
            "25:00 · 5.20 km"
        )
        XCTAssertEqual(
            workoutItemTargetSummary(cardio, weightUnit: "kg", distanceUnit: "mi"),
            "25:00 · 3.23 mi"
        )
    }

    func testTargetSummaryOpenPrescription() {
        let open = WorkoutItemDTO(
            id: "wi-3", exerciseID: "ex-1", exerciseName: "Squat",
            exerciseType: "strength", position: 2, targetSets: 3
        )
        XCTAssertFalse(open.isPrescribed)
        XCTAssertEqual(
            workoutItemTargetSummary(open, weightUnit: "kg", distanceUnit: "km"),
            "Open target"
        )
        let prescribed = WorkoutItemDTO(
            id: "wi-1", exerciseID: "ex-1", exerciseName: "Squat",
            exerciseType: "strength", position: 0,
            targetSets: 3, targetReps: 8, targetWeight: 60
        )
        XCTAssertTrue(prescribed.isPrescribed)
    }

    func testDetailEntryGrouping() throws {
        let detail = try APIClient.jsonDecoder.decode(
            WorkoutDetailDTO.self, from: Data(workoutDetailJSON.utf8))
        XCTAssertEqual(detail.entries(for: "wi-1").count, 1)
        XCTAssertTrue(detail.entries(for: "wi-missing").isEmpty)

        let block = WorkoutBlockDTO(
            id: "wb-1", type: .superset, position: 0, rounds: 3,
            items: [
                WorkoutItemDTO(id: "wi-1", exerciseID: "ex-1", exerciseName: "Squat", position: 0, targetSets: 3),
                WorkoutItemDTO(id: "wi-2", exerciseID: "ex-1", exerciseName: "Squat", position: 1, targetSets: 3),
            ]
        )
        XCTAssertEqual(detail.loggedItemCount(in: block), 1,
                       "only wi-1 has a logged entry")
    }

    func testScheduleSummary() {
        XCTAssertEqual(workoutScheduleSummary(start: nil, end: nil), "")
        // Exact strings are locale-dependent; assert structure.
        let start = Date(timeIntervalSince1970: 1_757_000_000)
        let end = start.addingTimeInterval(3600)
        let ranged = workoutScheduleSummary(start: start, end: end)
        XCTAssertTrue(ranged.contains("–"), ranged)
        let openEnded = workoutScheduleSummary(start: start, end: nil)
        XCTAssertFalse(openEnded.isEmpty)
        XCTAssertFalse(openEnded.contains("–"), openEnded)
    }

    // MARK: - Plan-ahead dates

    private var utcCalendar: Calendar {
        var calendar = Calendar(identifier: .gregorian)
        calendar.timeZone = TimeZone(identifier: "UTC")!
        return calendar
    }

    private func utcDate(_ year: Int, _ month: Int, _ day: Int, _ hour: Int, _ minute: Int = 0) -> Date {
        var components = DateComponents()
        components.year = year
        components.month = month
        components.day = day
        components.hour = hour
        components.minute = minute
        return utcCalendar.date(from: components)!
    }

    func testNextWeekdayIncludesTodayWhenAhead() {
        // 2026-09-07 is a Monday — the test asserts it so the
        // fixture can never silently rot.
        let monday = utcDate(2026, 9, 7, 10)
        XCTAssertEqual(utcCalendar.component(.weekday, from: monday), 2)

        let dates = nextWeekdayDates(
            weekday: 2, count: 3, from: monday,
            hour: 18, minute: 0, duration: 3600,
            calendar: utcCalendar
        )
        XCTAssertEqual(dates.count, 3)
        XCTAssertEqual(dates[0].start, utcDate(2026, 9, 7, 18))
        XCTAssertEqual(dates[1].start, utcDate(2026, 9, 14, 18))
        XCTAssertEqual(dates[2].start, utcDate(2026, 9, 21, 18))
        XCTAssertEqual(dates[0].end, dates[0].start.addingTimeInterval(3600))
    }

    func testNextWeekdaySkipsTodayWhenPassed() {
        let mondayEvening = utcDate(2026, 9, 7, 20)
        let dates = nextWeekdayDates(
            weekday: 2, count: 2, from: mondayEvening,
            hour: 18, minute: 0, duration: 3600,
            calendar: utcCalendar
        )
        XCTAssertEqual(dates.count, 2)
        XCTAssertEqual(dates[0].start, utcDate(2026, 9, 14, 18))
        XCTAssertEqual(dates[1].start, utcDate(2026, 9, 21, 18))
    }

    func testNextWeekdayMidweekTarget() {
        let monday = utcDate(2026, 9, 7, 10)
        let dates = nextWeekdayDates(
            weekday: 4, count: 2, from: monday,
            hour: 7, minute: 30, duration: 5400,
            calendar: utcCalendar
        )
        XCTAssertEqual(dates.count, 2)
        XCTAssertEqual(dates[0].start, utcDate(2026, 9, 9, 7, 30))
        XCTAssertEqual(dates[0].end, dates[0].start.addingTimeInterval(5400))
        XCTAssertEqual(dates[1].start, utcDate(2026, 9, 16, 7, 30))
    }

    func testNextWeekdayRejectsBadInput() {
        let monday = utcDate(2026, 9, 7, 10)
        XCTAssertTrue(nextWeekdayDates(
            weekday: 0, count: 3, from: monday,
            hour: 18, minute: 0, duration: 3600,
            calendar: utcCalendar
        ).isEmpty)
        XCTAssertTrue(nextWeekdayDates(
            weekday: 2, count: 0, from: monday,
            hour: 18, minute: 0, duration: 3600,
            calendar: utcCalendar
        ).isEmpty)
    }

    func testClockRange() {
        XCTAssertEqual(workoutClockRange(start: nil, end: nil), "")
        let start = utcDate(2026, 9, 7, 18)
        XCTAssertFalse(workoutClockRange(start: start, end: nil).isEmpty)
        XCTAssertFalse(workoutClockRange(start: start, end: nil).contains("–"))
        let ranged = workoutClockRange(start: start, end: start.addingTimeInterval(3600))
        XCTAssertTrue(ranged.contains("–"), ranged)
    }

    // MARK: - Logging

    func testSuggestedRound() {
        XCTAssertEqual(suggestedWorkoutRound(loggedRounds: [], rounds: 3), 1)
        XCTAssertEqual(suggestedWorkoutRound(loggedRounds: [1], rounds: 3), 2)
        XCTAssertEqual(suggestedWorkoutRound(loggedRounds: [2], rounds: 3), 1)
        XCTAssertEqual(suggestedWorkoutRound(loggedRounds: [1, 2, 3], rounds: 3), 3,
                       "full rounds fall back to the last round for extra work")
        XCTAssertEqual(suggestedWorkoutRound(loggedRounds: [0], rounds: 3), 1,
                       "unrounded entries don't advance the suggestion")
        XCTAssertEqual(suggestedWorkoutRound(loggedRounds: [], rounds: 1), 1)
    }

    func testUpdateRequestPreservesLinkage() throws {
        // `EditSetView` passes the entry's links through so an
        // edit never unlinks a logged set.
        let request = UpdateExerciseEntryRequest(
            exerciseID: "ex-1", notes: "", reps: 5, weight: 100, restTime: 0,
            createdAt: nil,
            workoutID: "w-1", workoutItemID: "wi-1", roundNumber: 2
        )
        let raw = try APIClient.jsonEncoder.encode(request)
        let body = String(data: raw, encoding: .utf8) ?? ""
        XCTAssertTrue(body.contains("\"workout_id\":\"w-1\""), body)
        XCTAssertTrue(body.contains("\"workout_item_id\":\"wi-1\""), body)
        XCTAssertTrue(body.contains("\"round_number\":2"), body)
    }
}

// MARK: - Stub transport

/// `URLProtocol` stub that serves canned JSON per
/// "METHOD path" key. Lets store tests run the real
/// `APIClient` + real store logic with no network.
final class StubURLProtocol: URLProtocol {
    struct Stub {
        let status: Int
        let json: String
    }

    private static var stubs: [String: Stub] = [:]

    @discardableResult
    static func stub(_ key: String, status: Int, json: String) -> APIClient {
        stubs[key] = Stub(status: status, json: json)
        return api()
    }

    static func api() -> APIClient {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        return APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            session: URLSession(configuration: config),
            tokenProvider: { "test-token" }
        )
    }

    override class func canInit(with request: URLRequest) -> Bool { true }

    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let key = "\(request.httpMethod ?? "") \(request.url?.path ?? "")"
        let stub = Self.stubs[key] ?? Stub(status: 404, json: "{\"error\":\"not stubbed: \(key)\"}")
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: stub.status,
            httpVersion: nil,
            headerFields: ["Content-Type": "application/json"]
        )!
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        client?.urlProtocol(self, didLoad: Data(stub.json.utf8))
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}
