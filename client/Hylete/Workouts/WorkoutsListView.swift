import SwiftUI

/// The beta "Workouts" list, pushed from the More hub's Training
/// section. Two sections in a plain `List`:
///   - **Upcoming** — planned workouts, always expanded
///   - **History** — completed + cancelled, collapsed by default
///
/// Tapping a row pushes the detail view. Leading swipe toggles
/// complete/reopen (full-swipe, the fast triage gesture);
/// trailing swipe plans more dates, duplicates, edits, or deletes.
/// Long-press offers the same actions for discoverability.
///
/// All networking and state lives in `WorkoutStore`; this view is
/// purely presentational. Stack-less content — the More hub's
/// `NavigationStack` provides the single stack.
struct WorkoutsListView: View {
    @EnvironmentObject private var env: AppEnvironment
    @ObservedObject var store: WorkoutStore

    @State private var showingNewWorkout: Bool = false
    @State private var editingWorkout: EditingWorkout?
    @State private var planningWorkout: PlanningWorkout?
    @State private var historyExpanded: Bool = false

    var body: some View {
        content
            .navigationTitle("Workouts")
            .toolbar { toolbarContent }
            .sheet(isPresented: $showingNewWorkout) {
                WorkoutEditorView(mode: .create, store: store)
                    .environmentObject(env)
            }
            .sheet(item: $editingWorkout) { editing in
                WorkoutEditorView(mode: .edit(workoutID: editing.id), store: store)
                    .environmentObject(env)
            }
            .sheet(item: $planningWorkout) { planning in
                PlanAheadSheet(workoutID: planning.id, workoutName: planning.name, workoutStore: store)
            }
            .task { await store.load() }
            .refreshable { await store.load() }
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Button {
                showingNewWorkout = true
            } label: {
                Image(systemName: Icons.addSet)
            }
            .accessibilityLabel("Add workout")
        }
    }

    // MARK: - Content states

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
            upcomingSection
            historySection
        }
        .listStyle(.automatic)
    }

    @ViewBuilder
    private var upcomingSection: some View {
        if !store.plannedWorkouts.isEmpty {
            Section {
                ForEach(store.plannedWorkouts) { workout in
                    row(for: workout)
                }
            } header: {
                sectionHeader("Upcoming", count: store.plannedWorkouts.count)
            }
        }
    }

    /// History uses a `DisclosureGroup` so it starts collapsed
    /// (matches the goals tab's completed accordion).
    @ViewBuilder
    private var historySection: some View {
        if !store.pastWorkouts.isEmpty {
            DisclosureGroup(isExpanded: $historyExpanded) {
                ForEach(store.pastWorkouts) { workout in
                    row(for: workout)
                }
            } label: {
                sectionHeader("History", count: store.pastWorkouts.count)
            }
        }
    }

    @ViewBuilder
    private func row(for workout: WorkoutDTO) -> some View {
        NavigationLink {
            WorkoutDetailView(workoutID: workout.id, store: store)
        } label: {
            WorkoutRow(workout: workout)
        }
        .contextMenu {
            Button {
                planningWorkout = PlanningWorkout(id: workout.id, name: workout.name)
            } label: {
                Label("Plan ahead…", systemImage: Icons.calendar)
            }
            Button {
                Task { await store.duplicate(id: workout.id) }
            } label: {
                Label("Duplicate", systemImage: Icons.duplicate)
            }
            Button {
                editingWorkout = EditingWorkout(id: workout.id)
            } label: {
                Label("Edit", systemImage: Icons.edit)
            }
            Button(role: .destructive) {
                Task { await store.delete(id: workout.id) }
            } label: {
                Label("Delete", systemImage: Icons.trash)
            }
        }
        .swipeActions(edge: .leading, allowsFullSwipe: true) {
            if workout.isCompleted {
                Button {
                    Task { await store.reopen(id: workout.id) }
                } label: {
                    Label("Reopen", systemImage: "arrow.uturn.backward")
                }
                .tint(DSColors.accent)
            } else if workout.isPlanned {
                Button {
                    Task { await store.complete(id: workout.id) }
                } label: {
                    Label("Complete", systemImage: "checkmark")
                }
                .tint(DSColors.accent)
            }
        }
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button {
                planningWorkout = PlanningWorkout(id: workout.id, name: workout.name)
            } label: {
                Label("Plan", systemImage: Icons.calendar)
            }
            .tint(DSColors.accent)
            Button {
                Task { await store.duplicate(id: workout.id) }
            } label: {
                Label("Duplicate", systemImage: Icons.duplicate)
            }
            .tint(.gray)
            Button {
                editingWorkout = EditingWorkout(id: workout.id)
            } label: {
                Label("Edit", systemImage: Icons.edit)
            }
            .tint(.gray)
            Button(role: .destructive) {
                Task { await store.delete(id: workout.id) }
            } label: {
                Label("Delete", systemImage: Icons.trash)
            }
        }
    }

    private func sectionHeader(_ title: String, count: Int) -> some View {
        HStack(spacing: DSSpacing.xs) {
            Text(title)
                .font(.headline)
                .foregroundStyle(DSColors.text)
            Text("·")
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
            Text("\(count)")
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
        }
        .textCase(nil)
    }

    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            ZStack {
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surfaceElevated)
                Image(systemName: Icons.workouts)
                    .font(.system(size: 24))
                    .foregroundStyle(DSColors.text)
            }
            .frame(width: 48, height: 48)

            Text("No workouts yet")
                .font(.title3.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text("Workouts group your exercises into a planned session. Build one, then plan it onto as many days as you like — or log a one-off.")
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

/// Identifiable wrapper around a workout id so the list's
/// `.sheet(item:)` can drive the edit sheet.
private struct EditingWorkout: Identifiable, Equatable {
    let id: String
}

/// Identifiable wrapper around a workout id + name so the
/// list's `.sheet(item:)` can drive the plan-ahead sheet.
private struct PlanningWorkout: Identifiable, Equatable {
    let id: String
    let name: String
}

/// One workout row: name, schedule line, and a status chip.
/// Cancelled rows show the chip but no leading swipe (reopen
/// lives on the detail view — the list keeps one gesture
/// pair per row).
private struct WorkoutRow: View {    let workout: WorkoutDTO

    var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Text(workout.name)
                    .font(.body)
                    .foregroundStyle(DSColors.text)
                Spacer()
                statusChip
            }
            let schedule = workoutScheduleSummary(start: workout.scheduledStart, end: workout.scheduledEnd)
            if !schedule.isEmpty {
                Text(schedule)
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    @ViewBuilder
    private var statusChip: some View {
        switch workout.status {
        case "completed":
            Text("Done")
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.success)
        case "cancelled":
            Text("Cancelled")
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
        default:
            EmptyView()
        }
    }
}

#Preview {
    NavigationStack {
        WorkoutsListView(store: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
