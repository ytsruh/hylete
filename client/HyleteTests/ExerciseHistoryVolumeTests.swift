import XCTest
@testable import Hylete

/// Tests for the Best Volume stat card on the per-exercise history
/// screen (`ExerciseHistoryView`). Covers the display formatter and
/// the `HistoryStatsDTO.bestSetVolume` decoding contract, including
/// back-compat with server builds that omit `best_set_volume`.
final class ExerciseHistoryVolumeTests: XCTestCase {

    // MARK: - Formatter

    func testBestVolumeTextFormatsWithoutDecimals() {
        // 8 reps × 100 kg.
        XCTAssertEqual(ExerciseHistoryView.bestVolumeText(800, unit: "kg"), "800 kg")
        XCTAssertEqual(ExerciseHistoryView.bestVolumeText(800, unit: "lbs"), "800 lbs")
    }

    func testBestVolumeTextZeroReadsAsEmDash() {
        XCTAssertEqual(ExerciseHistoryView.bestVolumeText(0, unit: "kg"), "—")
    }

    // MARK: - Decoding

    func testDecodesBestSetVolume() throws {
        let json = """
            {"max_weight": 110, "best_set_volume": 550, \
            "best_pace_sec_per_km": 0, "longest_distance_meters": 0}
            """.data(using: .utf8)!
        let stats = try JSONDecoder().decode(HistoryStatsDTO.self, from: json)
        XCTAssertEqual(stats.bestSetVolume, 550)
    }

    func testMissingBestSetVolumeDecodesAsZero() throws {
        // Older server builds omit the key entirely — the card
        // renders "—" instead of failing the decode.
        let json = """
            {"max_weight": 110, \
            "best_pace_sec_per_km": 0, "longest_distance_meters": 0}
            """.data(using: .utf8)!
        let stats = try JSONDecoder().decode(HistoryStatsDTO.self, from: json)
        XCTAssertEqual(stats.bestSetVolume, 0)
        XCTAssertEqual(ExerciseHistoryView.bestVolumeText(stats.bestSetVolume, unit: "kg"), "—")
    }
}
