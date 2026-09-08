import Foundation

/// Authorization state for the read-only HealthKit link.
///
/// Coarse union across every type the grid reads: granted when
/// *any* type is authorized, denied only when nothing is.
/// Apple never reveals *which* types were denied, and the
/// per-type status API is unreliable on Simulator, so this is
/// advisory only — actually-returned samples are ground truth
/// (see `HealthViewModel.refresh()`).
public enum HealthAuthStatus: Equatable {
    case unavailable
    case notRequested
    case granted
    case denied
}

/// One fetched vital: canonical-unit value plus the sample
/// date backing the card's "updated x ago" footnote. A missing
/// entry in the readings dictionary means "no data" (never
/// logged in Health); the views render `HealthMetric.formatted`
/// with nil → "—".
public struct HealthSample: Equatable {
    public let value: Double
    public let date: Date?

    public init(value: Double, date: Date? = nil) {
        self.value = value
        self.date = date
    }
}

/// Interface the Health UI accepts (per codebase convention:
/// packages define the interfaces they consume). The live
/// HealthKit implementation is `LiveHealthStore`; previews and
/// tests inject `MockHealthStore`.
public protocol HealthDataProvider {
    /// `false` on iPad / Simulator-without-Health / restricted
    /// devices (`HKHealthStore.isHealthDataAvailable()`).
    func isAvailable() -> Bool

    /// Coarse authorization state (see `HealthAuthStatus`).
    func authorizationStatus() -> HealthAuthStatus

    /// Presents the iOS system permission sheet (read-only).
    /// Resolves when the sheet dismisses; denial is NOT an
    /// error — re-check `authorizationStatus()` afterwards.
    func requestAuthorization() async throws

    /// Fetches every metric best-effort. Per-metric failures
    /// are swallowed (that metric is simply absent), so one
    /// missing type never fails the whole grid.
    func fetchReadings() async -> [HealthMetric: HealthSample]
}
