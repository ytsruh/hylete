import XCTest
@testable import Hylete

/// Tests for the shared block list filter behind `BlocksListView`.
/// Combines a case-insensitive name search with an All/Standard/
/// Circuit/AMRAP/EMOM type filter, mirroring the exercise catalogue
/// filter behaviour.
final class BlockFilterTests: XCTestCase {

    private func makeBlock(id: String, name: String, type: BlockTypeDTO) -> BlockSummaryDTO {
        BlockSummaryDTO(
            id: id, name: name, description: "", type: type,
            rounds: 0, restSeconds: 0, timeCapSeconds: 0, intervalSeconds: 0,
            itemCount: 1, createdAt: Date(), updatedAt: Date()
        )
    }

    private var catalogue: [BlockSummaryDTO] {
        [
            makeBlock(id: "1", name: "Push Day", type: .standard),
            makeBlock(id: "2", name: "Morning Circuit", type: .circuit),
            makeBlock(id: "3", name: "Burpee AMRAP", type: .amrap),
            makeBlock(id: "4", name: "EMOM Engine", type: .emom),
            makeBlock(id: "5", name: "Push Circuit", type: .circuit),
        ]
    }

    func testAllFilterWithEmptySearchReturnsEverything() {
        let result = filterBlocks(catalogue, search: "", typeFilter: .all)
        XCTAssertEqual(result.map(\.id), ["1", "2", "3", "4", "5"])
    }

    func testSearchIsCaseInsensitiveAndTrimmed() {
        let result = filterBlocks(catalogue, search: "  PUSH ", typeFilter: .all)
        XCTAssertEqual(Set(result.map(\.id)), ["1", "5"])
    }

    func testTypeFilterAlone() {
        XCTAssertEqual(
            filterBlocks(catalogue, search: "", typeFilter: .circuit).map(\.id),
            ["2", "5"]
        )
        XCTAssertEqual(
            filterBlocks(catalogue, search: "", typeFilter: .amrap).map(\.id),
            ["3"]
        )
        XCTAssertEqual(
            filterBlocks(catalogue, search: "", typeFilter: .emom).map(\.id),
            ["4"]
        )
        XCTAssertEqual(
            filterBlocks(catalogue, search: "", typeFilter: .standard).map(\.id),
            ["1"]
        )
    }

    func testSearchAndTypeCombine() {
        let result = filterBlocks(catalogue, search: "push", typeFilter: .circuit)
        XCTAssertEqual(result.map(\.id), ["5"])

        let empty = filterBlocks(catalogue, search: "push", typeFilter: .amrap)
        XCTAssertTrue(empty.isEmpty)
    }
}
