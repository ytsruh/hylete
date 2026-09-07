import SwiftUI

/// The "Weight" tab. Layout (top → bottom):
///
///   - Chart (only with ≥ 2 entries) showing the user's
///     body-weight trend, plus a horizontal "target" line
///     when one is set
///   - Progress card under the chart with the current
///     weight, target, percent, and remaining gap
///   - Date-grouped list of entries, newest first
///
/// Each row is tappable to open the editor.
struct WeightListView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var store: WeightStore

    @State private var showingNewWeight: Bool = false
    @State private var editingEntry: WeightEntryDTO?
    @State private var selectedPhotoIDs: [String] = []
    @State private var comparison: WeightCompareResponse?
    @State private var showingComparison: Bool = false
    @State private var comparisonError: String?
    @State private var isLoadingComparison: Bool = false

    /// Sorted chart points (oldest-first) so the chart
    /// draws left-to-right. Cached because the sort runs
    /// on every body evaluation otherwise.
    private var chartPoints: [WeightChart.ChartPoint] {
        store.entries
            .sorted { $0.createdAt < $1.createdAt }
            .map { WeightChart.ChartPoint(date: $0.createdAt, weight: $0.weight) }
    }

    /// The user's preferred unit. Falls back to "kg"
    /// when the user isn't signed in (the tab is hidden
    /// in that state, but the fallback keeps body
    /// evaluation safe).
    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    /// The user's target weight, if any. Drives the
    /// progress card and the chart's target line.
    private var targetWeight: Double? {
        authStore.currentUser?.targetWeight
    }

    /// The most recent weight value, for the progress
    /// card. Returns nil when the user has no entries so
    /// the parent can hide the card.
    private var currentWeight: Double? {
        store.latestEntry?.weight
    }

    /// Progress percent clamped to `[0, 100]`. Mirrors
    /// the web's `weightProgress` helper at
    /// `internal/views/weight/list.templ:315`.
    private var progressPercent: Double {
        guard let target = targetWeight, target > 0, let current = currentWeight else {
            return 0
        }
        let pct = current / target * 100
        return min(max(pct, 0), 100)
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Weight")
                .toolbar { toolbarContent }
                .sheet(isPresented: $showingNewWeight, onDismiss: {
                    Task { await store.load() }
                }) {
                    WeightEditorView(mode: .create, store: store)
                        .environmentObject(env)
                        .environmentObject(authStore)
                }
                .sheet(item: $editingEntry, onDismiss: {
                    Task { await store.load() }
                }) { entry in
                    WeightEditorView(mode: .edit(entry), store: store)
                        .environmentObject(env)
                        .environmentObject(authStore)
                }
                .sheet(isPresented: $showingComparison) {
                    if let comparison {
                        WeightComparisonView(
                            comparison: comparison,
                            weightUnit: weightUnit
                        )
                    }
                }
        }
        .task { await store.load() }
        .refreshable { await store.load() }
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Button {
                showingNewWeight = true
            } label: {
                Image(systemName: Icons.addSet)
                    .font(.body.weight(.semibold))
            }
            .accessibilityLabel("Log weight")
        }
    }

    // MARK: - Content states

    @ViewBuilder
    private var content: some View {
        if store.isLoading && store.entries.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = store.errorMessage, store.entries.isEmpty {
            errorState(error)
        } else if store.entries.isEmpty {
            emptyState
        } else {
            loadedList
        }
    }

    /// The main scroll view: chart + progress card (when
    /// applicable) + entries. The chart and progress sit
    /// in their original card chrome at the top; the
    /// entries live below in a card with the iOS-native
    /// "list" feel (rounded corners, separator strokes,
    /// thumbnail-on-left row design).
    private var loadedList: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: DSSpacing.md) {
                chartSection
                entriesSection
                comparisonBar
            }
            .padding(DSSpacing.md)
        }
        .background(DSColors.background.ignoresSafeArea())
    }

    @ViewBuilder
    private var chartSection: some View {
        if chartPoints.count >= 2 {
            VStack(spacing: DSSpacing.md) {
                // Card chrome around the chart so the
                // stacked layout reads as two equal-weight
                // sections (chart + progress) rather than a
                // bare plot sitting on the page background.
                WeightChart(
                    points: chartPoints,
                    weightUnit: weightUnit,
                    targetWeight: targetWeight
                )
                .padding(DSSpacing.md)
                .frame(maxWidth: .infinity)
                .background(
                    RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                        .fill(DSColors.surface)
                )
                .overlay(
                    RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                        .stroke(DSColors.separator, lineWidth: 0.5)
                )
                if let target = targetWeight, let current = currentWeight {
                    WeightProgressCard(
                        current: current,
                        target: target,
                        percent: progressPercent,
                        weightUnit: weightUnit
                    )
                }
            }
        }
    }

    /// Date-grouped list of entries, styled to feel like
    /// an iOS-native list card (rounded corners, hairline
    /// separators between rows). The row layout itself —
    /// thumbnail on the left, weight + date inline —
    /// matches the `ExerciseRow` pattern in
    /// `ExerciseListView` so the two list-heavy tabs read
    /// as part of the same iOS-native idiom.
    private var entriesSection: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text("History")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
                .padding(.horizontal, DSSpacing.md)
            VStack(spacing: 0) {
                ForEach(store.entries) { entry in
                    WeightRow(
                        entry: entry,
                        weightUnit: weightUnit,
                        onTap: { editingEntry = entry },
                        isSelected: selectedPhotoIDs.contains(entry.id),
                        onToggleSelection: entry.hasPhoto ? { togglePhotoSelection(entry.id) } : nil
                    )
                    if entry.id != store.entries.last?.id {
                        Divider()
                            .padding(.leading, DSSpacing.md + 60 + DSSpacing.md)
                    }
                }
            }
            .padding(.horizontal, DSSpacing.md)
            .background(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .stroke(DSColors.separator, lineWidth: 0.5)
            )
        }
    }

    @ViewBuilder
    private var comparisonBar: some View {
        if !selectedPhotoIDs.isEmpty {
            VStack(spacing: DSSpacing.sm) {
                HStack {
                    VStack(alignment: .leading, spacing: DSSpacing.xxs) {
                        Text("Compare photos")
                            .font(.subheadline.weight(.semibold))
                            .foregroundStyle(DSColors.text)
                        Text("\(selectedPhotoIDs.count) of 2 selected")
                            .font(.caption)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    Spacer()
                    Button("Clear") {
                        selectedPhotoIDs.removeAll()
                    }
                    .buttonStyle(.dsSecondary)
                    Button {
                        Task { await loadComparison() }
                    } label: {
                        if isLoadingComparison {
                            ProgressView()
                                .tint(.white)
                        } else {
                            Text("Compare")
                        }
                    }
                    .buttonStyle(.dsPrimary)
                    .disabled(selectedPhotoIDs.count != 2 || isLoadingComparison)
                }

                if let comparisonError {
                    Text(comparisonError)
                        .font(.caption)
                        .foregroundStyle(DSColors.destructive)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
            }
            .padding(DSSpacing.md)
            .background(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .stroke(DSColors.separator, lineWidth: 0.5)
            )
        }
    }

    /// Empty state. Encourages the user to log their first
    /// weight. Mirrors the web's `WeightEmptyState`
    /// template.
    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            ZStack {
                Circle()
                    .fill(DSColors.surfaceElevated)
                    .frame(width: 72, height: 72)
                Image(systemName: "figure.stand")
                    .font(.system(size: 32))
                    .foregroundStyle(DSColors.textSecondary)
            }
            Text("No weight entries yet")
                .font(.headline)
                .foregroundStyle(DSColors.text)
            Text("Start tracking your body weight by logging your first entry.")
                .font(.subheadline)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button {
                showingNewWeight = true
            } label: {
                Label("Log your first weight", systemImage: Icons.addSet)
            }
            .buttonStyle(.dsPrimary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(DSColors.background.ignoresSafeArea())
    }

    private func errorState(_ message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Try again") { Task { await store.load() } }
                .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .background(DSColors.background.ignoresSafeArea())
    }

    private func togglePhotoSelection(_ id: String) {
        comparisonError = nil
        if let index = selectedPhotoIDs.firstIndex(of: id) {
            selectedPhotoIDs.remove(at: index)
        } else if selectedPhotoIDs.count < 2 {
            selectedPhotoIDs.append(id)
        }
    }

    private func loadComparison() async {
        guard selectedPhotoIDs.count == 2 else { return }
        comparisonError = nil
        isLoadingComparison = true
        defer { isLoadingComparison = false }

        do {
            comparison = try await env.api.compareWeightEntries(
                a: selectedPhotoIDs[0],
                b: selectedPhotoIDs[1]
            )
            showingComparison = true
        } catch let error as APIError {
            comparisonError = error.errorDescription
        } catch {
            comparisonError = "Could not compare the selected photos."
        }
    }
}
