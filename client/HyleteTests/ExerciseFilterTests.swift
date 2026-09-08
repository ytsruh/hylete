import XCTest
@testable import Hylete

/// Tests for the shared exercise catalogue filter behind
/// `ExerciseListView` and `ExercisePickerSheet`. The filter combines a
/// case-insensitive name search with an All/Strength/Cardio/Other type
/// filter, mirroring the web exercise list behaviour.
final class ExerciseFilterTests: XCTestCase {

    private func makeExercise(id: String, name: String, type: String) -> ExerciseDTO {
        ExerciseDTO(
            id: id, name: name, description: "", videoURL: "",
            imgURL: "", imageURL: "", type: type
        )
    }

    private var catalogue: [ExerciseDTO] {
        [
            makeExercise(id: "1", name: "Bench Press", type: "strength"),
            makeExercise(id: "2", name: "Running", type: "cardio"),
            makeExercise(id: "3", name: "Yoga Flow", type: "other"),
            makeExercise(id: "4", name: "Press Up", type: "strength"),
        ]
    }

    func testAllFilterWithEmptySearchReturnsEverything() {
        let result = filterExercises(catalogue, search: "", typeFilter: .all)
        XCTAssertEqual(result.map(\.id), ["1", "2", "3", "4"])
    }

    func testSearchIsCaseInsensitiveAndTrimmed() {
        let result = filterExercises(catalogue, search: "  PRESS ", typeFilter: .all)
        XCTAssertEqual(Set(result.map(\.id)), ["1", "4"])
    }

    func testTypeFilterAlone() {
        XCTAssertEqual(
            filterExercises(catalogue, search: "", typeFilter: .strength).map(\.id),
            ["1", "4"]
        )
        XCTAssertEqual(
            filterExercises(catalogue, search: "", typeFilter: .cardio).map(\.id),
            ["2"]
        )
        XCTAssertEqual(
            filterExercises(catalogue, search: "", typeFilter: .other).map(\.id),
            ["3"]
        )
    }

    func testSearchAndTypeCombine() {
        let result = filterExercises(catalogue, search: "press", typeFilter: .strength)
        XCTAssertEqual(Set(result.map(\.id)), ["1", "4"])

        let cardioOnly = filterExercises(catalogue, search: "press", typeFilter: .cardio)
        XCTAssertTrue(cardioOnly.isEmpty)
    }

    func testTypeMatchingIsCaseInsensitive() {
        let mixed = [makeExercise(id: "9", name: "Cycle", type: "Cardio")]
        let result = filterExercises(mixed, search: "", typeFilter: .cardio)
        XCTAssertEqual(result.map(\.id), ["9"])
    }
}
