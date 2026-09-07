import Foundation

/// View-model behind the beta-gated Health tab and the Profile
/// connect row. Owns the link lifecycle (availability →
/// authorization → fetch) and publishes the latest readings;
/// views only render `status` + `readings` and forward taps.
///
/// Mass/distance formatting follows the user's Hylete profile
/// units, captured at init from `authStore.currentUser` so the
/// view-model stays a pure function of (provider, units).
@MainActor
public final class HealthViewModel: ObservableObject {
    /// Lifecycle state the views switch on.
    public enum Status: Equatable {
        case notRequested
        case loading
        case loaded
        case denied
        case unavailable
        case error(String)
    }

    @Published public private(set) var status: Status = .notRequested
    @Published public private(set) var readings: [HealthMetric: HealthSample] = [:]
    @Published public private(set) var lastUpdated: Date?

    private let provider: HealthDataProvider
    private let weightUnit: String
    private let distanceUnit: String

    public init(
        provider: HealthDataProvider = LiveHealthStore(),
        weightUnit: String = "kg",
        distanceUnit: String = "km"
    ) {
        self.provider = provider
        self.weightUnit = weightUnit
        self.distanceUnit = distanceUnit
        if !provider.isAvailable() {
            status = .unavailable
        } else if provider.authorizationStatus() == .granted {
            status = .loading
        }
    }

    /// Display string for one metric ("—" when Health has no
    /// data for it).
    public func text(for metric: HealthMetric) -> String {
        metric.formatted(
            value: readings[metric]?.value,
            weightUnit: weightUnit,
            distanceUnit: distanceUnit
        )
    }

    /// Presents the system permission sheet, then fetches.
    /// Denial is not an error — it lands on `.denied` with an
    /// "Open Settings" CTA in the view. A *thrown* failure
    /// (unreachable store, restricted device, Simulator without
    /// Health) lands on `.error` with a graceful message that
    /// keeps the underlying detail for diagnosis.
    public func connect() async {
        guard provider.isAvailable() else { status = .unavailable; return }
        status = .loading
        do {
            try await provider.requestAuthorization()
        } catch {
            status = .error(Self.connectFailureMessage(for: error))
            return
        }
        await refresh()
    }

    /// Human-readable connect failure. Keeps the underlying
    /// system detail in parentheses (so Simulator / restriction
    /// issues stay diagnosable) while reassuring the user that
    /// Hylete itself is unaffected — the Health link is
    /// display-only and nothing else depends on it.
    static func connectFailureMessage(for error: Error) -> String {
        let detail = (error as NSError).localizedDescription
        return "Apple Health couldn't be reached (\(detail)). " +
            "Your Hylete data is unaffected — this can happen on " +
            "a Simulator or a device with Health restricted. " +
            "Try again."
    }

    /// Re-fetches every metric best-effort. Returned samples
    /// are ground truth and always win: any data at all resolves
    /// to `.loaded`, even if the (advisory, Simulator-flaky)
    /// authorization signal claims denied. Only a completely
    /// empty fetch falls back to the status signal — and a
    /// granted-but-empty fetch is `.loaded` ("no data yet"),
    /// never an error.
    public func refresh() async {
        guard provider.isAvailable() else { status = .unavailable; return }
        if status != .loading { status = .loading }
        let fetched = await provider.fetchReadings()
        readings = fetched
        lastUpdated = Date()
        if !fetched.isEmpty {
            status = .loaded
            return
        }
        switch provider.authorizationStatus() {
        case .denied:
            status = .denied
        case .unavailable:
            status = .unavailable
        case .notRequested:
            status = .notRequested
        case .granted:
            status = .loaded
        }
    }
}
