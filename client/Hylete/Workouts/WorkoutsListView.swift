import SwiftUI

/// The Workouts list (Beta). A plain `List` of the user's
/// planned workouts, newest first. Tapping a row pushes the
/// detail view; leading swipe duplicates (server-side, so the
/// copy keeps the block composition without a round-trip
/// through the editor); trailing swipe deletes.
///
/// Stack-less content: the More hub's `NavigationStack`
/// provides the single stack — never wrap this view. Sheets
/// (the editor) keep their own stacks.
///
/// All networking and state lives in `WorkoutStore`; this view
/// is purely presentational.
struct WorkoutsListView: View {
    @EnvironmentObject private var env: AppEnvironment
    @ObservedObject var store: WorkoutStore
    @ObservedObject var blockStore: BlockStore

    @State private var showingNewWorkout: Bool = false
    @State private var deletingWorkout: WorkoutSummaryDTO?
    @State private var showingDeleteAlert: Bool = false

    var body: some View {
        content
            .navigationTitle("Workouts")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showingNewWorkout = true
                    } label: {
                        Image(systemName: Icons.addSet)
                    }
                    .accessibilityLabel("Add workout")
                }
            }
            .sheet(isPresented: $showingNewWorkout) {
                WorkoutEditorView(mode: .create, store: store, blockStore: blockStore)
                    .environmentObject(env)
            }
            .alert("Delete this workout?", isPresented: $showingDeleteAlert) {
                Button("Delete", role: .destructive) {
                    if let workout = deletingWorkout {
                        Task { await store.delete(id: workout.id) }
                    }
                }
                Button("Cancel", role: .cancel) {
                    deletingWorkout = nil
                }
            } message: {
                Text("This permanently removes the workout and its planned days. Blocks and logged sets are unaffected.")
            }
            .task { await store.load() }
            .refreshable { await store.load() }
    }

    @ViewBuilder
    private var content: some View {
        if store.isLoading && store.workouts.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = store.errorMessage, store.workouts.isEmpty {
            errorState(error)
        } else if store.workouts.isEmpty {
            emptyState
        } else {
            loadedList
        }
    }

    private var loadedList: some View {
        List {
            Section {
                ForEach(store.workouts) { workout in
                    NavigationLink {
                        WorkoutDetailView(store: store, blockStore: blockStore, workoutID: workout.id)
                    } label: {
                        workoutRow(for: workout)
                    }
                    .swipeActions(edge: .leading, allowsFullSwipe: false) {
                        Button {
                            Task { await store.duplicate(id: workout.id) }
                        } label: {
                            Label("Duplicate", systemImage: "plus.square.on.square")
                        }
                        .tint(DSColors.accent)
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                        Button(role: .destructive) {
                            deletingWorkout = workout
                            showingDeleteAlert = true
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
            } header: {
                Text("Plans made of blocks, scheduled onto your calendar")
                    .textCase(nil)
            }
        }
        .listStyle(.automatic)
    }

    /// Two-line row: title + "N blocks" subtitle, plus the
    /// next planned day when the detail is already cached.
    private func workoutRow(for workout: WorkoutSummaryDTO) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(workout.title)
                .font(.body)
                .foregroundStyle(DSColors.text)
            Text(workoutSubtitle(for: workout))
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    private func workoutSubtitle(for workout: WorkoutSummaryDTO) -> String {
        let blocks = workout.blockCount == 1 ? "1 block" : "\(workout.blockCount) blocks"
        if let next = store.details[workout.id]?.assignments
            .map(\.scheduledDate).sorted().first {
            return "\(blocks) · Next \(next)"
        }
        return blocks
    }

    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            ZStack {
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surfaceElevated)
                Image(systemName: "calendar.badge.clock")
                    .font(.system(size: 24))
                    .foregroundStyle(DSColors.text)
            }
            .frame(width: 48, height: 48)

            Text("No workouts yet")
                .font(.title3.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text("Combine blocks into a planned workout — a push day, a deload week, a race build-up — then schedule it onto your calendar.")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Text("Tap + to plan your first workout.")
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func errorState(_ message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Try again") {
                Task { await store.load() }
            }
            .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

#Preview {
    NavigationStack {
        WorkoutsListView(
            store: WorkoutStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )),
            blockStore: BlockStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            ))
        )
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
