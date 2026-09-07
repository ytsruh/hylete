import HealthKit
import XCTest
@testable import Hylete

/// Tests for the beta-gated Health vitals surface. Formatting
/// and view-model cases run against `MockHealthStore` — no
/// device needed. The VO2-unit regression test touches the
/// real `HKUnit` construction (which previously raised an
/// uncatchable NSException) but performs no HealthKit queries,
/// so it is safe to run on a simulator with no Health data.
final class HealthTests: XCTestCase {

    // MARK: - Formatting

    func testNilFormatsAsEmDash() {
        XCTAssertEqual(HealthMetric.steps.formatted(value: nil), "—")
        XCTAssertEqual(HealthMetric.weight.formatted(value: nil), "—")
    }

    func testWeightFollowsProfileUnit() {
        // Canonical provider unit is kilograms.
        XCTAssertEqual(
            HealthMetric.weight.formatted(value: 80, weightUnit: "kg"),
            "80.0 kg"
        )
        XCTAssertEqual(
            HealthMetric.weight.formatted(value: 80, weightUnit: "lb"),
            "176.4 lb"
        )
    }

    func testDistanceFollowsProfileUnit() {
        // Canonical provider unit is metres.
        XCTAssertEqual(
            HealthMetric.distance.formatted(value: 1_000, distanceUnit: "km"),
            "1.00 km"
        )
        XCTAssertEqual(
            HealthMetric.distance.formatted(value: 1_609.344, distanceUnit: "mi"),
            "1.00 mi"
        )
    }

    func testFixedUnitMetrics() {
        XCTAssertEqual(HealthMetric.heartRate.formatted(value: 72.4), "72 bpm")
        XCTAssertEqual(HealthMetric.heartRateVariability.formatted(value: 41.6), "42 ms")
        XCTAssertEqual(HealthMetric.bodyFatPercentage.formatted(value: 18.25), "18.2 %")
        XCTAssertEqual(HealthMetric.bmi.formatted(value: 24.96), "25.0")
        XCTAssertEqual(HealthMetric.activeEnergy.formatted(value: 512.7), "513 kcal")
        XCTAssertEqual(HealthMetric.sleep.formatted(value: 7.25), "7.2 h")
        XCTAssertEqual(HealthMetric.exerciseTime.formatted(value: 32.4), "32 min")
    }

    func testStepsLabel() {
        XCTAssertTrue(HealthMetric.steps.formatted(value: 8_432).hasSuffix("steps"))
    }

    // MARK: - View model

    /// Granted + readings → loaded grid with formatted text.
    @MainActor
    func testRefreshLoadsReadings() async {
        let vm = HealthViewModel(
            provider: MockHealthStore(readings: [
                .steps: HealthSample(value: 1_000, date: Date()),
            ]),
            weightUnit: "kg",
            distanceUnit: "km"
        )
        await vm.refresh()
        XCTAssertEqual(vm.status, .loaded)
        XCTAssertTrue(vm.text(for: .steps).hasSuffix("steps"))
        XCTAssertEqual(vm.text(for: .heartRate), "—")
    }

    /// Denied + no data → denied state (Open Settings CTA).
    @MainActor
    func testDeniedWithNoDataMapsToDenied() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .denied))
        await vm.refresh()
        XCTAssertEqual(vm.status, .denied)
    }

    // MARK: - Samples are ground truth

    /// Returned samples always win over the advisory status
    /// signal: even a provider reporting `.denied` (stale or
    /// Simulator-flaky `authorizationStatus`) resolves to
    /// `.loaded` when queries actually return data. This is the
    /// regression test for the stuck-denied-screen loop.
    @MainActor
    func testReturnedDataOverridesDeniedStatus() async {
        let vm = HealthViewModel(provider: MockHealthStore(
            status: .denied,
            readings: [.steps: HealthSample(value: 1_000, date: Date())]
        ))
        await vm.refresh()
        XCTAssertEqual(vm.status, .loaded)
        XCTAssertTrue(vm.text(for: .steps).hasSuffix("steps"))
    }

    /// Granted but nothing logged in Health → loaded grid of
    /// "—" cards, never an error or denied screen.
    @MainActor
    func testGrantedWithNoDataMapsToLoaded() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .granted))
        await vm.refresh()
        XCTAssertEqual(vm.status, .loaded)
        XCTAssertEqual(vm.text(for: .steps), "—")
    }

    /// Never-requested + empty fetch → back to the connect
    /// prompt, not a dead-end denied screen.
    @MainActor
    func testNotRequestedWithNoDataMapsToNotRequested() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .notRequested))
        await vm.refresh()
        XCTAssertEqual(vm.status, .notRequested)
    }

    /// Unavailable device → unavailable state, no fetch.
    @MainActor
    func testUnavailableStaysUnavailable() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .unavailable))
        XCTAssertEqual(vm.status, .unavailable)
        await vm.refresh()
        XCTAssertEqual(vm.status, .unavailable)
    }

    // MARK: - VO2 unit regression

    /// `HKUnit(from: "ml/kg/min")` raised an uncatchable
    /// NSException out of `+[HKUnit unitFromString:]` and
    /// crashed `fetchReadings()` (via the Health tab's
    /// "Try again" button). The store now builds the unit from
    /// typed constructors — merely touching it here guards the
    /// regression, since the old code crashed the test runner.
    func testVO2UnitConstructionAndFormatting() {
        let expected = HKUnit.literUnit(with: .milli)
            .unitDivided(by: .gramUnit(with: .kilo))
            .unitDivided(by: .minute())
        XCTAssertEqual(LiveHealthStore.vo2Unit, expected)
        XCTAssertEqual(HealthMetric.cardioFitness.formatted(value: 45.0), "45.0 VO₂")
    }

    // MARK: - Graceful connect failure

    /// A *thrown* `requestAuthorization` (unreachable store,
    /// restricted device, Simulator without Health) must land
    /// on `.error` with a graceful message: underlying detail
    /// kept for diagnosis, user reassured Hylete is unaffected.
    /// Denial never throws, so this path is distinct from the
    /// `.denied` / "Open Settings" state.
    @MainActor
    func testConnectFailureIsGracefulError() async {
        let vm = HealthViewModel(provider: ThrowingHealthStore())
        await vm.connect()
        guard case .error(let message) = vm.status else {
            return XCTFail("expected .error, got \(vm.status)")
        }
        XCTAssertTrue(message.contains("simulated failure"))
        XCTAssertTrue(message.contains("unaffected"))
    }
}

/// Stub whose authorization always throws, simulating a
/// Simulator or restricted device where the Health store is
/// unreachable. Never touches HealthKit.
private struct ThrowingHealthStore: HealthDataProvider {
    func isAvailable() -> Bool { true }
    func authorizationStatus() -> HealthAuthStatus { .notRequested }
    func requestAuthorization() async throws {
        throw NSError(
            domain: "HealthTests",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: "simulated failure"]
        )
    }
    func fetchReadings() async -> [HealthMetric: HealthSample] { [:] }
}
