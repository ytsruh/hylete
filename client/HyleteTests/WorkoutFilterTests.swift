import XCTest
@testable import Hylete

/// Tests for the shared workout list filter behind
/// `WorkoutsListView`. Combines a case-insensitive name search with
/// an All/Planned/In Progress/Completed/Skipped status filter,
/// mirroring the exercise catalogue filter behaviour.
final class WorkoutFilterTests: XCTestCase {

    private func makeWorkout(id: String, name: String, status: WorkoutStatusDTO) -> WorkoutSummaryDTO {
        WorkoutSummaryDTO(
            id: id, name: name, description: "", scheduledDate: "2026-09-14",
            status: status, blockCount: 2, doneCount: 0,
            createdAt: Date(), updatedAt: Date()
        )
    }

    private var catalogue: [WorkoutSummaryDTO] {
        [
            makeWorkout(id: "1", name: "Monday Strength", status: .planned),
            makeWorkout(id: "2", name: "Friday Conditioning", status: .completed),
            makeWorkout(id: "3", name: "Monday Recovery", status: .skipped),
            makeWorkout(id: "4", name: "Wednesday Engine", status: .inProgress),
        ]
    }

    func testAllFilterWithEmptySearchReturnsEverything() {
        let result = filterWorkouts(catalogue, search: "", statusFilter: .all)
        XCTAssertEqual(result.map(\.id), ["1", "2", "3", "4"])
    }

    func testSearchIsCaseInsensitiveAndTrimmed() {
        let result = filterWorkouts(catalogue, search: "  MONDAY ", statusFilter: .all)
        XCTAssertEqual(Set(result.map(\.id)), ["1", "3"])
    }

    func testStatusFilterAlone() {
        XCTAssertEqual(
            filterWorkouts(catalogue, search: "", statusFilter: .planned).map(\.id),
            ["1"]
        )
        XCTAssertEqual(
            filterWorkouts(catalogue, search: "", statusFilter: .completed).map(\.id),
            ["2"]
        )
        XCTAssertEqual(
            filterWorkouts(catalogue, search: "", statusFilter: .skipped).map(\.id),
            ["3"]
        )
        XCTAssertEqual(
            filterWorkouts(catalogue, search: "", statusFilter: .inProgress).map(\.id),
            ["4"]
        )
    }

    func testSearchAndStatusCombine() {
        let result = filterWorkouts(catalogue, search: "monday", statusFilter: .planned)
        XCTAssertEqual(result.map(\.id), ["1"])

        let empty = filterWorkouts(catalogue, search: "monday", statusFilter: .completed)
        XCTAssertTrue(empty.isEmpty)
    }
}
