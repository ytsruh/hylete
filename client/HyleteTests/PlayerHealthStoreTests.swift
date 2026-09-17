import HealthKit
import XCTest
@testable import Hylete

/// Tests for the Health session owner. Drives start/pause/save
/// through `MockWorkoutHealthRecorder` + `MockHealthStore` — no
/// device needed.
@MainActor
final class PlayerHealthStoreTests: XCTestCase {
    private func strengthItems() -> [BlockItemDTO] {
        [
            BlockItemDTO(id: "i1", exerciseID: "e1", exerciseName: "Squat", exerciseType: "strength", position: 0, targetText: ""),
            BlockItemDTO(id: "i2", exerciseID: "e2", exerciseName: "Bench", exerciseType: "strength", position: 1, targetText: ""),
        ]
    }

    func testInitInfersStrengthDefault() {
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: MockWorkoutHealthRecorder(),
            provider: MockHealthStore(status: .granted)
        )
        XCTAssertEqual(store.selectedType, .traditionalStrengthTraining)
        XCTAssertFalse(store.hasSession)
    }

    func testStartRequestsRecorderWithRouteGating() async {
        let recorder = MockWorkoutHealthRecorder()
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: recorder,
            provider: MockHealthStore(status: .granted)
        )
        await store.start()
        XCTAssertEqual(recorder.starts.count, 1)
        XCTAssertEqual(recorder.starts.first?.type, .traditionalStrengthTraining)
        XCTAssertEqual(recorder.starts.first?.route, false)
        XCTAssertTrue(store.hasSession)
    }

    func testOutdoorTypeArmsRoute() async {
        let recorder = MockWorkoutHealthRecorder()
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: recorder,
            provider: MockHealthStore(status: .granted)
        )
        store.selectedType = .running
        XCTAssertTrue(store.armsRouteOnStart)
        await store.start()
        XCTAssertEqual(recorder.starts.first?.route, true)
    }

    func testDeniedWriteSkipsWithoutStarting() async {
        let recorder = MockWorkoutHealthRecorder()
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: recorder,
            provider: MockHealthStore(status: .granted, writeStatus: .denied)
        )
        await store.start()
        XCTAssertTrue(recorder.starts.isEmpty)
        XCTAssertNil(store.errorMessage)
    }

    /// The recorder refusing to start (stays `.idle`) must
    /// surface an error — Start must never appear to do nothing.
    func testRecorderStayingIdleSurfacesError() async {
        let recorder = MockWorkoutHealthRecorder()
        recorder.refusesStart = true
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: recorder,
            provider: MockHealthStore(status: .granted)
        )
        await store.start()
        XCTAssertEqual(recorder.starts.count, 1)
        XCTAssertNotNil(store.errorMessage)
        XCTAssertFalse(store.errorMessage!.isEmpty)
    }

    func testEndAndSaveIsNoopWhenIdle() async {
        let recorder = MockWorkoutHealthRecorder()
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: recorder,
            provider: MockHealthStore(status: .granted)
        )
        let saved = await store.endAndSave()
        XCTAssertTrue(saved)
        XCTAssertTrue(recorder.saves.isEmpty)
    }

    func testAdoptProfileSetsMaxHeartRate() {
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: MockWorkoutHealthRecorder(),
            provider: MockHealthStore(status: .granted)
        )
        XCTAssertNil(store.maxHeartRate)
        store.adoptProfile(dateOfBirth: "1996-03-04")
        XCTAssertNotNil(store.maxHeartRate)
        // Clearing the DOB clears the estimate (tile hides).
        store.adoptProfile(dateOfBirth: nil)
        XCTAssertNil(store.maxHeartRate)
        store.adoptProfile(dateOfBirth: "not-a-date")
        XCTAssertNil(store.maxHeartRate)
    }

    func testPauseResumeMirror() async {
        let recorder = MockWorkoutHealthRecorder()
        let store = PlayerHealthStore(
            workoutID: "w1",
            items: strengthItems(),
            recorder: recorder,
            provider: MockHealthStore(status: .granted)
        )
        await store.start()
        store.pause()
        XCTAssertEqual(store.recorderState, .paused)
        store.resume()
        XCTAssertEqual(store.recorderState, .active)
    }
}
