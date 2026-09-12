import SwiftUI

/// Detail view for one workout. Header carries the title and
/// description; the blocks section shows each linked block in
/// order; the schedule section lists every planned day (with
/// per-day delete). Edit opens the editor sheet; duplicate
/// copies server-side without opening the editor; delete
/// confirms then pops back to the list.
///
/// The workout loads via `WorkoutStore.detail(id:)` (cached,
/// so returning from the editor is instant and already fresh —
/// `update` rewrites the cache).
struct WorkoutDetailView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var store: WorkoutStore
    @ObservedObject var blockStore: BlockStore

    let workoutID: String
    /// Fired after a successful duplicate with the new copy.
    /// The detail still dismisses itself; the caller can
    /// additionally navigate (e.g. the dashboard jumps to
    /// the Workouts tab to show the copy). Nil keeps the
    /// plain dismiss-back behaviour.
    let onDuplicate: ((WorkoutDTO) -> Void)?

    init(
        store: WorkoutStore,
        blockStore: BlockStore,
        workoutID: String,
        onDuplicate: ((WorkoutDTO) -> Void)? = nil
    ) {
        self._store = ObservedObject(wrappedValue: store)
        self._blockStore = ObservedObject(wrappedValue: blockStore)
        self.workoutID = workoutID
        self.onDuplicate = onDuplicate
    }

    @State private var showingEditor: Bool = false
    @State private var showingDelete: Bool = false
    @State private var didRequestLoad: Bool = false

    private var workout: WorkoutDTO? {
        store.details[workoutID]
    }

    var body: some View {
        content
            .navigationTitle(workout?.title ?? "Workout")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if workout != nil {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("Edit") { showingEditor = true }
                    }
                }
            }
            .sheet(isPresented: $showingEditor) {
                if let workout {
                    WorkoutEditorView(
                        mode: .edit(workout),
                        store: store,
                        blockStore: blockStore
                    )
                    .environmentObject(env)
                }
            }
            .alert("Delete this workout?", isPresented: $showingDelete) {
                Button("Delete", role: .destructive) {
                    Task {
                        await store.delete(id: workoutID)
                        if store.errorMessage == nil {
                            dismiss()
                        }
                    }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This permanently removes the workout and its planned days. Blocks and logged sets are unaffected.")
            }
            .task {
                guard !didRequestLoad else { return }
                didRequestLoad = true
                await store.detail(id: workoutID)
            }
    }

    @ViewBuilder
    private var content: some View {
        if let workout {
            loadedView(workout)
        } else if let error = store.errorMessage {
            VStack(spacing: DSSpacing.md) {
                Image(systemName: Icons.warning)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.destructive)
                Text(error)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") {
                    Task { await store.detail(id: workoutID, refresh: true) }
                }
                .buttonStyle(.dsSecondary)
            }
            .padding(DSSpacing.lg)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func loadedView(_ workout: WorkoutDTO) -> some View {
        List {
            if !workout.description.isEmpty {
                Section {
                    Text(workout.description)
                        .font(.body)
                        .foregroundStyle(DSColors.text)
                }
            }

            Section {
                ForEach(workout.blocks) { link in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(link.blockName)
                            .font(.body)
                            .foregroundStyle(DSColors.text)
                        if !link.blockType.isEmpty {
                            Text(link.blockType.capitalized)
                                .font(.subheadline)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                    }
                    .padding(.vertical, DSSpacing.xxs)
                }
            } header: {
                Text(blockCountLabel(workout.blocks.count))
            }

            Section {
                if workout.assignments.isEmpty {
                    Text("Not scheduled yet — tap Edit to plan it onto your calendar.")
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                } else {
                    ForEach(workout.assignments.sorted { $0.scheduledDate < $1.scheduledDate }) { assignment in
                        HStack {
                            Text(friendlyDate(assignment.scheduledDate))
                                .font(.body)
                                .foregroundStyle(DSColors.text)
                            Spacer()
                            Button {
                                Task {
                                    await store.deleteAssignment(workoutID: workoutID, assignmentID: assignment.id)
                                }
                            } label: {
                                Image(systemName: "xmark.circle")
                                    .foregroundStyle(DSColors.textSecondary)
                            }
                            .buttonStyle(.plain)
                            .accessibilityLabel("Remove \(assignment.scheduledDate)")
                        }
                        .padding(.vertical, DSSpacing.xxs)
                    }
                }
            } header: {
                Text("Scheduled days")
            }

            // Full-width action buttons in the DesignSystem
            // primary/secondary idiom: Duplicate is the safe,
            // reversible action (secondary chrome), Delete is
            // filled destructive. Keeps the two visually
            // distinct so a thumb aiming for Duplicate never
            // lands on Delete.
            Section {
                Button {
                    Task {
                        if let copy = await store.duplicate(id: workoutID),
                           store.errorMessage == nil {
                            if let onDuplicate {
                                // Hand the copy to the caller,
                                // which pops this detail via
                                // state in the same batch as
                                // its own navigation. No
                                // dismiss() here: as an
                                // environment action it races
                                // the tab switch and flashes
                                // the dashboard for a frame.
                                onDuplicate(copy)
                            } else {
                                dismiss()
                            }
                        }
                    }
                } label: {
                    Text("Duplicate workout")
                }
                .buttonStyle(.dsSecondary)

                Button(role: .destructive) {
                    showingDelete = true
                } label: {
                    Text("Delete workout")
                }
                .font(.headline)
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(DSColors.destructive)
                .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
            }
        }
        .listStyle(.automatic)
    }

    private func blockCountLabel(_ count: Int) -> String {
        count == 1 ? "1 block" : "\(count) blocks"
    }

    /// "2026-09-14" becomes "Mon 14 Sep"; falls back to the
    /// raw string when parsing fails (defensive — the server
    /// validates on write).
    private func friendlyDate(_ yyyyMMdd: String) -> String {
        guard let date = WorkoutAssignmentDTO.dayFormatter.date(from: yyyyMMdd) else {
            return yyyyMMdd
        }
        return date.formatted(date: .abbreviated, time: .omitted)
    }
}

#Preview {
    NavigationStack {
        WorkoutDetailView(
            store: WorkoutStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )),
            blockStore: BlockStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )),
            workoutID: "preview"
        )
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
