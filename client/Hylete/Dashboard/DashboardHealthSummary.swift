import SwiftUI

/// Beta-gated "today at a glance" vitals on the dashboard,
/// below the calendar. A fixed 6-metric subset rendered with
/// the same `StatsGrid`/`StatCard` language as the Health tab,
/// driven by the dashboard-owned `HealthViewModel`.
///
/// Visibility contract: cards only when readings are loaded
/// (or a slim spinner on first fetch). Every other state —
/// never-connected, denied, unavailable, error — renders
/// nothing; the Health tab owns all connect/denied UX and the
/// dashboard stays clean. Read-only like the tab: no sync, no
/// writes, no server calls.
struct DashboardHealthSummary: View {

    /// Today's dashboard metrics: steps, distance, active +
    /// resting energy, heart rate, resting heart rate. Distance
    /// follows the view-model's `distanceUnit` (profile unit
    /// passed down from the dashboard); the rest are
    /// unit-independent (count, kcal, bpm).
    static let metrics: [HealthMetric] = [
        .steps, .distance, .activeEnergy, .basalEnergy,
        .heartRate, .restingHeartRate,
    ]

    /// Whether a dashboard pull-to-refresh should also refresh
    /// vitals. `HealthViewModel.refresh()` unconditionally
    /// queries HealthKit, so it runs only for opted-in users
    /// with a live link (`.loading`/`.loaded`): never-connected
    /// beta users pay for no queries, and recovery from denied/
    /// error stays in the Health tab's explicit retry UX.
    static func shouldRefresh(
        status: HealthViewModel.Status,
        betaEnabled: Bool
    ) -> Bool {
        guard betaEnabled else { return false }
        return status == .loading || status == .loaded
    }

    @ObservedObject var viewModel: HealthViewModel

    var body: some View {
        Group {
            switch viewModel.status {
            case .loaded:
                loadedSection
            case .loading where viewModel.readings.isEmpty:
                ProgressView()
                    .frame(maxWidth: .infinity)
                    .padding(.vertical, DSSpacing.md)
            case .loading, .notRequested, .denied, .unavailable, .error:
                EmptyView()
            }
        }
        .task {
            if viewModel.status == .loading {
                await viewModel.refresh()
            }
        }
    }

    private var loadedSection: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text("Health Summary")
                .font(.headline)
                .foregroundStyle(DSColors.text)
                .padding(.horizontal, DSSpacing.xs)
            StatsGrid {
                ForEach(Self.metrics) { metric in
                    StatCard(
                        label: metric.title,
                        value: viewModel.text(for: metric),
                        icon: metric.systemImage,
                        footnote: viewModel.footnote(for: metric)
                    )
                }
            }
        }
    }
}

#Preview("Loaded") {
    DashboardHealthSummary(viewModel: HealthViewModel(provider: MockHealthStore(
        readings: [
            .steps: HealthSample(value: 8_432, date: Date()),
            .distance: HealthSample(value: 5_200, date: Date()),
            .activeEnergy: HealthSample(value: 420, date: Date()),
            .basalEnergy: HealthSample(value: 1_650, date: Date()),
            .heartRate: HealthSample(value: 72, date: Date()),
            .restingHeartRate: HealthSample(value: 58, date: Date()),
        ]
    )))
}
