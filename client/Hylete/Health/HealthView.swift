import SwiftUI

/// Beta-gated "Health" tab: read-only Apple Health vitals.
///
/// Rendered only when the master beta switch is on (see
/// `MainTabView`) and wrapped in `BetaFeature` as a second
/// gate, so the surface disappears entirely for non-beta
/// users. Content is a sectioned `StatsGrid` of `StatCard`s —
/// the same card language as the Weight tab — with one card
/// per `HealthMetric`. Values are display-only; nothing is
/// written to HealthKit or to the Hylete server.
public struct HealthView: View {
    @StateObject private var viewModel: HealthViewModel

    public init(
        provider: HealthDataProvider = LiveHealthStore(),
        weightUnit: String = "kg",
        distanceUnit: String = "km"
    ) {
        _viewModel = StateObject(wrappedValue: HealthViewModel(
            provider: provider,
            weightUnit: weightUnit,
            distanceUnit: distanceUnit
        ))
    }

    public var body: some View {
        NavigationStack {
            content
                .navigationTitle("Health")
                .task {
                    if viewModel.status == .loading {
                        await viewModel.refresh()
                    }
                }
                .refreshable {
                    await viewModel.refresh()
                }
        }
    }

    @ViewBuilder
    private var content: some View {
        switch viewModel.status {
        case .notRequested:
            connectPrompt(message: "See your steps, heart rate, and body measurements alongside your workouts.")
        case .loading where viewModel.readings.isEmpty:
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        case .denied:
            deniedState
        case .unavailable:
            emptyState(
                title: "Apple Health isn't available",
                message: "This device doesn't support Apple Health, or access is restricted."
            )
        case .error(let message):
            emptyState(title: "Couldn't load Health data", message: message, showRetry: true)
        case .loading, .loaded:
            loadedGrid
        }
    }

    // MARK: - Loaded grid

    private var loadedGrid: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: DSSpacing.md) {
                if viewModel.readings.isEmpty && viewModel.status == .loaded {
                    Text("No Health data yet — log steps, workouts, or weigh-ins in Apple Health and pull to refresh.")
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                        .padding(.horizontal, DSSpacing.xs)
                }
                metricSection(title: "Activity", metrics: [
                    .steps, .distance, .activeEnergy,
                    .basalEnergy, .exerciseTime,
                ])
                metricSection(title: "Heart", metrics: [
                    .heartRate, .restingHeartRate,
                    .walkingHeartRateAverage, .heartRateVariability,
                    .cardioRecovery, .cardioFitness,
                ])
                metricSection(title: "Body", metrics: [
                    .weight, .bmi, .bodyFatPercentage, .leanBodyMass,
                ])
                metricSection(title: "Recovery", metrics: [.sleep])
                if let updated = viewModel.lastUpdated {
                    Text("Updated \(updated.formatted(date: .omitted, time: .shortened)) · Apple Health, read-only")
                        .font(.caption)
                        .foregroundStyle(DSColors.textSecondary)
                        .padding(.horizontal, DSSpacing.xs)
                }
            }
            .padding(DSSpacing.md)
        }
        .background(DSColors.background.ignoresSafeArea())
    }

    private func metricSection(title: String, metrics: [HealthMetric]) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text(title)
                .font(.headline)
                .foregroundStyle(DSColors.text)
                .padding(.horizontal, DSSpacing.xs)
            StatsGrid {
                ForEach(metrics) { metric in
                    StatCard(
                        label: metric.title,
                        value: viewModel.text(for: metric),
                        icon: metric.systemImage
                    )
                }
            }
        }
    }

    // MARK: - States

    private func connectPrompt(message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: "heart.text.square")
                .font(.largeTitle)
                .foregroundStyle(DSColors.accent)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Connect Apple Health") {
                Task { await viewModel.connect() }
            }
            .buttonStyle(.dsPrimary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    /// Access-off state (empty fetch + union reports denied).
    /// Shown only when nothing is granted — the connected-but-
    /// empty case renders the cards grid via `loadedGrid`
    /// instead. The softener covers a stale status signal: if
    /// access was just allowed, a live re-check clears this.
    private var deniedState: some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: "heart.slash")
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text("Apple Health access is off")
                .font(.headline)
            Text("Hylete can't read your vitals until you switch access back on in Settings. If you've just allowed access and still see this, try again.")
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Open Settings") {
                if let url = URL(string: UIApplication.openSettingsURLString) {
                    UIApplication.shared.open(url)
                }
            }
            .buttonStyle(.dsSecondary)
            Button("Try again") {
                Task { await viewModel.refresh() }
            }
            .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func emptyState(title: String, message: String, showRetry: Bool = false) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.textSecondary)
            Text(title).font(.headline)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            if showRetry {
                Button("Try again") {
                    Task { await viewModel.refresh() }
                }
                .buttonStyle(.dsSecondary)
            }
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

#Preview("Loaded") {
    HealthView(
        provider: MockHealthStore(readings: [
            .steps: HealthSample(value: 8_432, date: Date()),
            .heartRate: HealthSample(value: 72, date: Date()),
            .weight: HealthSample(value: 82.4, date: Date()),
            .sleep: HealthSample(value: 7.2, date: Date()),
        ]),
        weightUnit: "kg",
        distanceUnit: "km"
    )
}

#Preview("Connect") {
    HealthView(provider: MockHealthStore(status: .notRequested))
}
