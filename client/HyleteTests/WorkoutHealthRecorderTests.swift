import CoreLocation
import HealthKit
import XCTest
@testable import Hylete

/// Exercises the LIVE recorder's pause/resume state machine.
/// Pause/resume are pure state mutation (no HealthKit calls —
/// events are built once at save time), so this must never
/// crash. On a Simulator without Health data the session
/// simply never starts; the test then asserts the no-op path
/// still lands back at idle.
@MainActor
final class WorkoutHealthRecorderTests: XCTestCase {
    func testLivePauseResumeDiscard() async {
        let recorder = LiveWorkoutHealthRecorder()
        await recorder.start(activityType: .traditionalStrengthTraining, enablesRoute: false)
        if HKHealthStore.isHealthDataAvailable() {
            XCTAssertEqual(recorder.state, .active)
            recorder.pause()
            XCTAssertEqual(recorder.state, .paused)
            // Double-pause is a safe no-op (previously crashed).
            recorder.pause()
            XCTAssertEqual(recorder.state, .paused)
            recorder.resume()
            XCTAssertEqual(recorder.state, .active)
            // Double-resume is a safe no-op.
            recorder.resume()
            XCTAssertEqual(recorder.state, .active)
        }
        recorder.discard()
        XCTAssertEqual(recorder.state, .idle)
    }

    // MARK: - Rolling pace

    /// Steady movement in the window → seconds-per-km pace;
    /// single fix, standstill, and stale fixes → nil.
    func testRollingPace() {
        let now = Date()
        func fix(latOffset: Double, secondsAgo: Double) -> CLLocation {
            CLLocation(
                coordinate: CLLocationCoordinate2D(latitude: latOffset, longitude: 0),
                altitude: 0,
                horizontalAccuracy: 5,
                verticalAccuracy: 5,
                timestamp: now.addingTimeInterval(-secondsAgo)
            )
        }
        // 0.001° latitude ≈ 111m over 25s ≈ 225 s/km (3:45/km).
        let moving = [fix(latOffset: 0.001, secondsAgo: 25), fix(latOffset: 0, secondsAgo: 0)]
        let pace = LiveWorkoutHealthRecorder.rollingPaceSecPerKm(locations: moving, now: now)
        XCTAssertNotNil(pace)
        XCTAssertEqual(pace!, 225, accuracy: 8)
        // Single fix → nil.
        XCTAssertNil(LiveWorkoutHealthRecorder.rollingPaceSecPerKm(locations: [moving[0]], now: now))
        // No movement → nil (never an infinite pace).
        let still = [fix(latOffset: 0, secondsAgo: 10), fix(latOffset: 0, secondsAgo: 0)]
        XCTAssertNil(LiveWorkoutHealthRecorder.rollingPaceSecPerKm(locations: still, now: now))
        // Fixes outside the window → nil.
        let stale = [fix(latOffset: 0.001, secondsAgo: 60), fix(latOffset: 0, secondsAgo: 45)]
        XCTAssertNil(LiveWorkoutHealthRecorder.rollingPaceSecPerKm(locations: stale, now: now))
    }
}
