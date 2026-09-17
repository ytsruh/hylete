import HealthKit
import XCTest
@testable import Hylete

/// Unit tests for the single-type Health workout inference.
/// Pure count math — no HealthKit store, safe on Simulator.
final class WorkoutHealthActivityMapperTests: XCTestCase {
    func testEmptyIsOther() {
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(strength: 0, cardio: 0, other: 0),
            .other
        )
    }

    func testOtherOnlyIsOther() {
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(strength: 0, cardio: 0, other: 3),
            .other
        )
    }

    func testStrengthOnlyIsTraditional() {
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(strength: 5, cardio: 0, other: 1),
            .traditionalStrengthTraining
        )
    }

    func testCardioOnlyIsHIIT() {
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(strength: 0, cardio: 2, other: 0),
            .highIntensityIntervalTraining
        )
    }

    func testMixedTieBreaksToCardio() {
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(strength: 2, cardio: 2, other: 0),
            .highIntensityIntervalTraining
        )
    }

    func testStrengthMajorityIsTraditional() {
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(strength: 4, cardio: 1, other: 0),
            .traditionalStrengthTraining
        )
    }

    func testItemsConvenienceCountsExerciseTypes() {
        let items = [
            BlockItemDTO(id: "a", exerciseID: "e1", exerciseName: "Squat", exerciseType: "strength", position: 0, targetText: ""),
            BlockItemDTO(id: "b", exerciseID: "e2", exerciseName: "Run", exerciseType: "cardio", position: 1, targetText: ""),
            BlockItemDTO(id: "c", exerciseID: "e3", exerciseName: "Run", exerciseType: "CARDIO", position: 2, targetText: ""),
        ]
        XCTAssertEqual(
            WorkoutHealthActivityMapper.infer(items: items),
            .highIntensityIntervalTraining
        )
    }

    func testOutdoorEligibility() {
        XCTAssertTrue(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.running))
        XCTAssertTrue(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.cycling))
        XCTAssertTrue(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.walking))
        XCTAssertFalse(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.traditionalStrengthTraining))
        XCTAssertFalse(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.highIntensityIntervalTraining))
        XCTAssertFalse(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.rowing))
        XCTAssertFalse(WorkoutHealthActivityMapper.isOutdoorRouteEligible(.other))
    }

    func testSelectableTypesContainsOutdoorOptions() {
        let types = WorkoutHealthActivityMapper.selectableTypes
        XCTAssertTrue(types.contains(.running))
        XCTAssertTrue(types.contains(.traditionalStrengthTraining))
    }
}
