import XCTest
@testable import Hylete

/// Tests for the Blocks DTOs, the iOS mirror of the server's
/// `BlockDTO` / `BlockSummaryDTO` in `internal/routes/api_dto.go`.
/// Covers the golden decode of a full block (every config key
/// present), the list envelope, and the `configSummary` labels
/// the detail header renders per kind.
final class BlockTests: XCTestCase {

    private var fullBlockJSON: String {
        """
        {"id":"b1","name":"Push Day","description":"Chest + shoulders",
         "type":"circuit","rounds":4,"rest_seconds":90,
         "time_cap_seconds":0,"interval_seconds":0,
         "items":[
          {"id":"i1","exercise_id":"ex-1","exercise_name":"Bench Press",
           "exercise_type":"strength","position":0,"target_text":"3x5 @ 100kg"},
          {"id":"i2","exercise_id":"ex-2","exercise_name":"Overhead Press",
           "exercise_type":"strength","position":1,"target_text":""}],
         "created_at":"2026-09-12T10:00:00Z","updated_at":"2026-09-12T10:00:00Z"}
        """
    }

    func testDecodesFullBlock() throws {
        let block = try APIClient.jsonDecoder.decode(
            BlockDTO.self, from: Data(fullBlockJSON.utf8))
        XCTAssertEqual(block.id, "b1")
        XCTAssertEqual(block.name, "Push Day")
        XCTAssertEqual(block.type, .circuit)
        XCTAssertEqual(block.rounds, 4)
        XCTAssertEqual(block.restSeconds, 90)
        XCTAssertEqual(block.items.count, 2)
        XCTAssertEqual(block.items[0].exerciseName, "Bench Press")
        XCTAssertEqual(block.items[0].targetText, "3x5 @ 100kg")
        XCTAssertEqual(block.items[1].position, 1)
        XCTAssertFalse(block.items[0].isCardio)
        XCTAssertEqual(block.configSummary, "4 rounds · 90s rest")
    }

    func testDecodesBlocksEnvelope() throws {
        let json = """
        {"blocks":[
          {"id":"b1","name":"Push","description":"","type":"standard",
           "rounds":0,"rest_seconds":0,"time_cap_seconds":0,"interval_seconds":0,
           "item_count":3,
           "created_at":"2026-09-12T10:00:00Z","updated_at":"2026-09-12T10:00:00Z"}]}
        """
        let response = try APIClient.jsonDecoder.decode(
            BlocksResponse.self, from: Data(json.utf8))
        XCTAssertEqual(response.blocks.count, 1)
        XCTAssertEqual(response.blocks[0].itemCount, 3)
        XCTAssertEqual(response.blocks[0].type.displayName, "Standard")
    }

    func testConfigSummaries() {
        let base = BlockDTO(
            id: "b", name: "n", description: "", type: .standard,
            rounds: 0, restSeconds: 0, timeCapSeconds: 0, intervalSeconds: 0,
            items: [], createdAt: Date(), updatedAt: Date())
        XCTAssertEqual(base.configSummary, "")

        func with(type: BlockTypeDTO, rounds: Int, rest: Int, cap: Int, interval: Int) -> BlockDTO {
            BlockDTO(
                id: "b", name: "n", description: "", type: type,
                rounds: rounds, restSeconds: rest, timeCapSeconds: cap,
                intervalSeconds: interval, items: [],
                createdAt: Date(), updatedAt: Date())
        }
        XCTAssertEqual(
            with(type: .amrap, rounds: 0, rest: 0, cap: 600, interval: 0).configSummary,
            "10:00 cap")
        XCTAssertEqual(
            with(type: .emom, rounds: 12, rest: 0, cap: 0, interval: 60).configSummary,
            "Every 60s × 12")
    }

    func testBlockTypeDisplayNames() {
        XCTAssertEqual(BlockTypeDTO.standard.displayName, "Standard")
        XCTAssertEqual(BlockTypeDTO.circuit.displayName, "Circuit")
        XCTAssertEqual(BlockTypeDTO.amrap.displayName, "AMRAP")
        XCTAssertEqual(BlockTypeDTO.emom.displayName, "EMOM")
    }
}
