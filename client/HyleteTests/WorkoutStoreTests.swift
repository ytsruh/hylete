import Foundation
import XCTest
@testable import Hylete

/// Stubs `URLSession` at the protocol layer so `APIClient` (and the
/// stores above it) can be exercised with byte-exact server
/// responses. The bodies below were captured from a live server
/// (`GET /api/v1/workouts?from=&to=`, `GET /workouts/:id`, `POST
/// /workouts/:id/duplicate`) — if the wire format drifts, these
/// tests fail exactly where the app would show its error state.
final class StubURLProtocol: URLProtocol {
    /// Maps an incoming request to a canned response. Throw to
    /// simulate a transport failure.
    static var handler: ((URLRequest) throws -> (HTTPURLResponse, Data))?

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        do {
            guard let handler = Self.handler else {
                throw URLError(.unknown)
            }
            let (response, data) = try handler(request)
            client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
            client?.urlProtocol(self, didLoad: data)
            client?.urlProtocolDidFinishLoading(self)
        } catch {
            client?.urlProtocol(self, didFailWithError: error)
        }
    }

    override func stopLoading() {}
}

/// Tests for `WorkoutStore` against recorded server bytes: week load
/// (the dashboard calendar's path), detail, and duplicate all run
/// the production `APIClient` JSON decoder plus the store logic, so
/// a wire-format mismatch fails here instead of as an error state in
/// the app.
@MainActor
final class WorkoutStoreTests: XCTestCase {

    /// Captured `GET /api/v1/workouts?from=2026-09-14&to=2026-09-21`.
    private let rangeBody = """
        {"workouts":[{"id":"e992cf57-de90-4474-9664-6f674b76368b","name":"Monday","description":"Heavy","scheduled_date":"2026-09-14","status":"planned","block_count":1,"done_count":0,"created_at":"2026-09-14T11:09:48.050784+01:00","updated_at":"2026-09-14T11:09:48.050784+01:00"}]}
        """

    /// Captured `GET /api/v1/workouts/:id`.
    private let detailBody = """
        {"id":"e992cf57-de90-4474-9664-6f674b76368b","name":"Monday","description":"Heavy","scheduled_date":"2026-09-14","status":"planned","blocks":[{"id":"6e4daf50-f3bc-43ef-b047-99750ef57aa6","block_id":"8f7160f9-1973-4063-8fa5-6f0a701a80cd","block_name":"Push","block_description":"","block_type":"standard","position":0,"status":"pending","item_count":1}],"created_at":"2026-09-14T11:09:48.050784+01:00","updated_at":"2026-09-14T11:09:48.050784+01:00"}
        """

    private func makeStore() -> WorkoutStore {
        let config = URLSessionConfiguration.ephemeral
        config.protocolClasses = [StubURLProtocol.self]
        let session = URLSession(configuration: config)
        let api = APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            session: session,
            tokenProvider: { "test-token" }
        )
        return WorkoutStore(api: api)
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

    override func tearDown() {
        StubURLProtocol.handler = nil
        super.tearDown()
    }

    func testWeekLoadBucketsRecordedRange() async throws {
        let store = makeStore()
        StubURLProtocol.handler = { [rangeBody] request in
            let url = try XCTUnwrap(request.url?.absoluteString)
            XCTAssertTrue(url.contains("workouts?from="), "unexpected path: \(url)")
            return self.respond(body: rangeBody)
        }
        let monday = try XCTUnwrap(WorkoutDates.date(from: "2026-09-14"))
        await store.ensureWeekLoaded(for: monday)
        XCTAssertNil(store.errorMessage)
        let dayKey = CalendarMath.startOfDay(monday)
        let bucketed = try XCTUnwrap(store.workoutsByDay[dayKey])
        XCTAssertEqual(bucketed.count, 1)
        XCTAssertEqual(bucketed[0].name, "Monday")
        XCTAssertEqual(bucketed[0].progressLabel, "0/1 blocks")
    }

    func testDetailDecodesRecordedBytes() async throws {
        let store = makeStore()
        StubURLProtocol.handler = { [detailBody] _ in self.respond(body: detailBody) }
        let fetched = await store.detail(id: "e992cf57-de90-4474-9664-6f674b76368b")
        let workout = try XCTUnwrap(fetched)
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(workout.name, "Monday")
        XCTAssertEqual(workout.status, .planned)
        XCTAssertEqual(workout.blocks.count, 1)
        XCTAssertEqual(workout.blocks[0].blockName, "Push")
        XCTAssertEqual(workout.blocks[0].status, .pending)
    }

    func testDuplicateKeepsName() async throws {
        let store = makeStore()
        let dupBody = detailBody
            .replacingOccurrences(of: "2026-09-14", with: "2026-09-21")
        StubURLProtocol.handler = { _ in self.respond(status: 201, body: dupBody) }
        let duplicated = await store.duplicate(
            id: "e992cf57-de90-4474-9664-6f674b76368b",
            scheduledDate: "2026-09-21"
        )
        let created = try XCTUnwrap(duplicated)
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(created.name, "Monday")
        XCTAssertEqual(store.summaries.first?.name, "Monday")
    }

    func testServerErrorSurfacesMessage() async {
        let store = makeStore()
        StubURLProtocol.handler = { _ in
            self.respond(status: 404, body: "{\"error\":\"workout not found\"}")
        }
        let workout = await store.detail(id: "missing")
        XCTAssertNil(workout)
        XCTAssertEqual(store.errorMessage, "workout not found")
    }

    func testDuplicateBatchMergesRows() async throws {
        let store = makeStore()
        // Two recorded-style copies under a {"workouts": [...]} envelope.
        let copy = { (id: String, date: String) in
            self.detailBody
                .replacingOccurrences(of: "e992cf57-de90-4474-9664-6f674b76368b", with: id)
                .replacingOccurrences(of: "2026-09-14", with: date)
        }
        let batchBody = "{\"workouts\":[\(copy("id-1", "2026-09-21")),\(copy("id-2", "2026-09-28"))]}"
        StubURLProtocol.handler = { request in
            let url = try XCTUnwrap(request.url?.absoluteString)
            XCTAssertTrue(url.contains("duplicate-batch"), "unexpected path: \(url)")
            return self.respond(status: 201, body: batchBody)
        }
        let batched = await store.duplicateBatch(
            id: "e992cf57-de90-4474-9664-6f674b76368b",
            dates: ["2026-09-21", "2026-09-28"]
        )
        let created = try XCTUnwrap(batched)
        XCTAssertNil(store.errorMessage)
        XCTAssertEqual(created.count, 2)
        XCTAssertEqual(store.summaries.count, 2)
        XCTAssertNotNil(store.details["id-1"])
        XCTAssertNotNil(store.details["id-2"])
    }

    func testDuplicateBatchErrorSurfacesMessage() async {
        let store = makeStore()
        StubURLProtocol.handler = { _ in
            self.respond(status: 400, body: "{\"error\":\"at most 50 workouts can be created per batch\"}")
        }
        let created = await store.duplicateBatch(id: "x", dates: ["2026-09-21"])
        XCTAssertNil(created)
        XCTAssertEqual(store.errorMessage, "at most 50 workouts can be created per batch")
        XCTAssertTrue(store.summaries.isEmpty)
    }
}
