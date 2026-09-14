import SwiftUI

/// Detail view for one workout. Header carries the name, scheduled
/// date, status chip, and description; the block list shows each
/// planned block in order with its completion state. Tapping a
/// block's status chip opens a menu to mark it pending/done/skipped.
///
/// The workout loads via `WorkoutStore.detail(id:)` (cached, so
/// returning from the editor is instant and already fresh —
/// `update` rewrites the cache). Edit opens the editor sheet;
/// delete confirms then pops back to the list.
struct WorkoutDetailView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var store: WorkoutStore
    @ObservedObject var blockStore: BlockStore

    let workoutID: String

    @State private var showingEditor: Bool = false
    @State private var showingDuplicate: Bool = false
    @State private var showingDelete: Bool = false
    @State private var showingPlayer: Bool = false
    @State private var didRequestLoad: Bool = false

    private var workout: WorkoutDTO? {
        store.details[workoutID]
    }

    var body: some View {
        content
            .navigationTitle(workout?.name ?? "Workout")
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
            .sheet(isPresented: $showingDuplicate) {
                if let workout {
                    WorkoutDuplicateSheet(
                        workout: workout,
                        store: store,
                        onDuplicated: { _ in
                            // The sheet dismisses itself first; then pop
                            // back to the list where the new copy is
                            // visible. (The list's own duplicate flow
                            // pushes the new detail instead — both end
                            // somewhere sensible.)
                            dismiss()
                        },
                        onBatchDuplicated: {
                            // Same landing as a one-off: back to the
                            // list, which already merged the new rows.
                            dismiss()
                        }
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
                Text("This permanently removes the workout and its plan. Logged sets are unaffected.")
            }
            .fullScreenCover(isPresented: $showingPlayer) {
                WorkoutPlayerView(
                    workoutID: workoutID,
                    workoutStore: store,
                    player: WorkoutPlayerStore(
                        workoutID: workoutID,
                        api: env.api,
                        weightUnit: authStore.currentUser?.weightUnit ?? "kg",
                        distanceUnit: authStore.currentUser?.distanceUnit ?? "km"
                    )
                )
                .environmentObject(env)
                .environmentObject(authStore)
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
            Section {
                HStack(spacing: DSSpacing.xs) {
                    Text(WorkoutDates.display(workout.scheduledDate))
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(DSColors.text)
                    Text("·")
                        .foregroundStyle(DSColors.textSecondary)
                    Text(workout.status.displayName)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(DSColors.accent)
                    Spacer()
                    Text(workout.progressLabel)
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                }
                if !workout.description.isEmpty {
                    Text(workout.description)
                        .font(.body)
                        .foregroundStyle(DSColors.text)
                }
                Picker("Status", selection: statusBinding(for: workout)) {
                    ForEach(WorkoutStatusDTO.allCases, id: \.self) { status in
                        Text(status.displayName).tag(status)
                    }
                }
            }

            // Primary entry to the Workout Player: full-screen
            // session that walks through every block, logging sets
            // as exercise entries. "Start" is UI state only — the
            // workout flips to in_progress on the first logged set.
            Section {
                Button {
                    showingPlayer = true
                } label: {
                    Text("Start workout")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.dsPrimary)
                .listRowSeparator(.hidden)
                .listRowBackground(Color.clear)
                .listRowInsets(EdgeInsets())
            }

            Section {
                ForEach(workout.blocks) { block in
                    HStack(alignment: .top, spacing: DSSpacing.sm) {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(block.blockName)
                                .font(.body)
                                .foregroundStyle(DSColors.text)
                            Text(blockSubtitle(for: block))
                                .font(.subheadline)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                        Spacer()
                        Menu {
                            ForEach(WorkoutBlockStatusDTO.allCases, id: \.self) { status in
                                Button {
                                    Task {
                                        await store.setBlockStatus(
                                            workoutID: workout.id,
                                            workoutBlockID: block.id,
                                            status: status
                                        )
                                    }
                                } label: {
                                    Label(
                                        status.displayName,
                                        systemImage: block.status == status ? "checkmark" : statusIcon(for: status)
                                    )
                                }
                            }
                        } label: {
                            Text(block.status.displayName)
                                .font(.caption.weight(.semibold))
                                .padding(.horizontal, DSSpacing.sm)
                                .padding(.vertical, DSSpacing.xxs + 2)
                                .background(
                                    Capsule().fill(statusColor(for: block.status).opacity(0.15))
                                )
                                .foregroundStyle(statusColor(for: block.status))
                        }
                        .buttonStyle(.plain)
                        .accessibilityLabel("Mark \(block.blockName) \(block.status.displayName)")
                    }
                    .padding(.vertical, DSSpacing.xxs)
                }
            } header: {
                Text(blockCountLabel(workout.blocks.count))
            }

            if let error = store.errorMessage {
                Section {
                    Text(error)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }

            // Full-width action buttons in the DesignSystem
            // primary/secondary idiom: Duplicate is the safe,
            // reversible action (secondary chrome), Delete is
            // filled destructive. Both live in a single row so
            // no list separator or inter-row padding sits
            // between them — just a tight stack spacing.
            Section {
                VStack(spacing: DSSpacing.xs) {
                    Button {
                        showingDuplicate = true
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
                .listRowSeparator(.hidden)
            }
        }
        .listStyle(.automatic)
    }

    /// Two-way binding for the status picker: writes go through the
    /// workout update endpoint (structure unchanged, status only).
    private func statusBinding(for workout: WorkoutDTO) -> Binding<WorkoutStatusDTO> {
        Binding(
            get: { workout.status },
            set: { newStatus in
                Task {
                    await store.update(
                        id: workout.id,
                        request: UpdateWorkoutRequest(
                            name: workout.name,
                            description: workout.description,
                            scheduledDate: workout.scheduledDate,
                            status: newStatus,
                            blocks: workout.blocks.map { WorkoutBlockRequest(blockID: $0.blockID) }
                        )
                    )
                }
            }
        )
    }

    private func blockSubtitle(for block: WorkoutBlockDTO) -> String {
        let exercises = block.itemCount == 1 ? "1 exercise" : "\(block.itemCount) exercises"
        return "\(block.blockType.displayName) · \(exercises)"
    }

    private func blockCountLabel(_ count: Int) -> String {
        count == 1 ? "1 block" : "\(count) blocks"
    }

    private func statusIcon(for status: WorkoutBlockStatusDTO) -> String {
        switch status {
        case .pending: return "circle"
        case .done: return "checkmark.circle"
        case .skipped: return "forward"
        }
    }

    private func statusColor(for status: WorkoutBlockStatusDTO) -> Color {
        switch status {
        case .pending: return DSColors.textSecondary
        case .done: return DSColors.accent
        case .skipped: return DSColors.destructive
        }
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
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
