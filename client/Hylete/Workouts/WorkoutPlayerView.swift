import SwiftUI

/// Full-screen Workout Player. Presented from `WorkoutDetailView`'s
/// Start button, it walks the user through every planned block and
/// exercise in order, logging sets as they go.
///
/// Each planned exercise gets type-appropriate editors (reusing
/// `SetRowEditor`): strength items log repeatable set rows,
/// cardio items log a single session row. Only valid rows POST —
/// each POST carries `workout_id`/`block_id`/`workout_block_id`,
/// creating normal exercise entries (history, charts, and exports
/// all include them). Half-typed rows stay on screen and survive
/// background/kill via the store's on-device snapshot.
///
/// Progress is hybrid: per-item logged counts plus local skips
/// (zero sets, e.g. no equipment or injury) drive a "ready to mark
/// done" hint, but the block pending/done/skipped check-off stays
/// manual — the source of truth, written through `WorkoutStore`.
/// Finish flips the workout to completed (partial completion is
/// allowed: pending blocks may remain).
struct WorkoutPlayerView: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var workoutStore: WorkoutStore
    @StateObject private var player: WorkoutPlayerStore

    let workoutID: String

    @State private var didRequestLoad: Bool = false
    @State private var showingFinishConfirm: Bool = false
    @State private var isFinishing: Bool = false

    init(workoutID: String, workoutStore: WorkoutStore, player: WorkoutPlayerStore) {
        self.workoutID = workoutID
        self.workoutStore = workoutStore
        _player = StateObject(wrappedValue: player)
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle(player.workout?.name ?? "Workout")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Close") { dismiss() }
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("Finish") { showingFinishConfirm = true }
                            .bold()
                            .disabled(isFinishing || player.workout == nil)
                    }
                }
                .confirmationDialog(
                    "Finish this workout?",
                    isPresented: $showingFinishConfirm,
                    titleVisibility: .visible
                ) {
                    Button("Finish workout") { Task { await finish() } }
                    Button("Keep going", role: .cancel) {}
                } message: {
                    Text("The workout is marked completed. Pending blocks may remain — partial completion is allowed.")
                }
                .task {
                    guard !didRequestLoad else { return }
                    didRequestLoad = true
                    await player.load()
                }
        }
    }

    @ViewBuilder
    private var content: some View {
        if let workout = player.workout {
            loadedView(workout)
        } else if player.isLoading {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = player.errorMessage {
            VStack(spacing: DSSpacing.md) {
                Image(systemName: Icons.warning)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.destructive)
                Text(error)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") {
                    Task { await player.load() }
                }
                .buttonStyle(.dsSecondary)
                Button("Close") { dismiss() }
                    .buttonStyle(.plain)
                    .foregroundStyle(DSColors.textSecondary)
            }
            .padding(DSSpacing.lg)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func loadedView(_ workout: WorkoutWithItemsDTO) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: DSSpacing.md) {
                headerCard(workout)
                if let error = player.errorMessage {
                    Text(error)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                        .padding(.horizontal, DSSpacing.xs)
                }
                ForEach(workout.blocks) { block in
                    blockCard(block)
                }
                finishSection
            }
            .padding(DSSpacing.md)
        }
        .background(DSColors.background.ignoresSafeArea())
    }

    // MARK: - Header

    private func headerCard(_ workout: WorkoutWithItemsDTO) -> some View {
        let progress = player.overallProgress()
        return VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack {
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
            if progress.total > 0 {
                ProgressView(value: Double(progress.done), total: Double(progress.total))
                Text("\(progress.done) of \(progress.total) exercises logged")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
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

    // MARK: - Blocks

    private func blockCard(_ block: WorkoutBlockDetailDTO) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.sm) {
            HStack(alignment: .top, spacing: DSSpacing.sm) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(block.blockName)
                        .font(.headline)
                        .foregroundStyle(DSColors.text)
                    Text(blockSubtitle(for: block))
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                }
                Spacer()
                blockStatusMenu(block)
            }
            if player.isBlockReady(block) && block.status == .pending {
                Text("All exercises logged or skipped — ready to mark done.")
                    .font(.footnote.weight(.semibold))
                    .foregroundStyle(DSColors.accent)
            }
            Divider().background(DSColors.separator)
            if block.items.isEmpty {
                Text("This block's exercises are unavailable — you can still mark the block skipped.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
            ForEach(block.items) { item in
                itemView(item, in: block)
                if item.id != block.items.last?.id {
                    Divider().background(DSColors.separator)
                }
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

    private func blockSubtitle(for block: WorkoutBlockDetailDTO) -> String {
        let logged = block.items.filter { player.isItemDone(itemID: $0.id) }.count
        return "\(block.blockType.displayName) · \(logged)/\(block.items.count) logged"
    }

    private func blockStatusMenu(_ block: WorkoutBlockDetailDTO) -> some View {
        Menu {
            ForEach(WorkoutBlockStatusDTO.allCases, id: \.self) { status in
                Button {
                    Task {
                        await workoutStore.setBlockStatus(
                            workoutID: workoutID,
                            workoutBlockID: block.id,
                            status: status
                        )
                        await player.refreshBlockStatuses(from: workoutStore)
                    }
                } label: {
                    Label(
                        status.displayName,
                        systemImage: block.status == status ? "checkmark" : "circle"
                    )
                }
            }
        } label: {
            Text(block.status.displayName)
                .font(.caption.weight(.semibold))
                .padding(.horizontal, DSSpacing.sm)
                .padding(.vertical, DSSpacing.xxs + 2)
                .background(Capsule().fill(DSColors.accent.opacity(0.15)))
                .foregroundStyle(DSColors.accent)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Mark \(block.blockName) \(block.status.displayName)")
    }

    // MARK: - Items

    private func itemView(_ item: BlockItemDTO, in block: WorkoutBlockDetailDTO) -> some View {
        let skipped = player.skippedItemIDs.contains(item.id)
        let logged = player.loggedCount(itemID: item.id)
        return VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack(spacing: DSSpacing.xs) {
                VStack(alignment: .leading, spacing: 2) {
                    Text(item.exerciseName)
                        .font(.body.weight(.semibold))
                        .foregroundStyle(DSColors.text)
                    if !item.targetText.isEmpty {
                        Text(item.targetText)
                            .font(.subheadline)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
                Spacer()
                ExerciseTypeChip(type: item.exerciseType)
            }
            HStack(spacing: DSSpacing.xs) {
                if logged > 0 {
                    Text(logged == 1 ? "1 logged" : "\(logged) logged")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(DSColors.accent)
                }
                if skipped {
                    Text("Skipped")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(DSColors.textSecondary)
                }
                Spacer()
                Button(skipped ? "Unskip" : "Skip") {
                    player.toggleSkip(itemID: item.id)
                }
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
                .disabled(player.isLogging(itemID: item.id))
            }
            if !skipped {
                editorRows(item)
                Button {
                    Task { await player.log(item: item, in: block) }
                } label: {
                    if player.isLogging(itemID: item.id) {
                        ProgressView()
                            .frame(maxWidth: .infinity)
                    } else {
                        Text(item.isCardio ? "Log session" : "Log sets")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.dsSecondary)
                .disabled(player.isLogging(itemID: item.id) || !hasValidDraft(item))
            } else {
                Text("No sets will be logged for this exercise.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(.vertical, DSSpacing.xs)
    }

    private func editorRows(_ item: BlockItemDTO) -> some View {
        // Editor writes go through `setDrafts(for:)` so every
        // keystroke persists to the on-device snapshot.
        let rows = Binding(
            get: { player.drafts[item.id] ?? [] },
            set: { player.setDrafts($0, for: item.id) }
        )
        let firstRow = Binding(
            get: { player.drafts[item.id]?.first ?? SetDraft(distanceUnit: distanceUnit) },
            set: { player.setDrafts([$0], for: item.id) }
        )
        return VStack(spacing: DSSpacing.xs) {
            if item.isCardio {
                // Cardio is a one-shot session, not repeated sets:
                // exactly one fixed row, no add/remove affordances
                // (mirrors `NewSetView`).
                SetRowEditor(
                    set: firstRow,
                    weightUnit: weightUnit,
                    distanceUnit: distanceUnit,
                    isCardioMode: true
                )
            } else {
                ForEach(rows) { $row in
                    HStack(alignment: .top, spacing: DSSpacing.xs) {
                        SetRowEditor(
                            set: $row,
                            weightUnit: weightUnit,
                            distanceUnit: distanceUnit,
                            isCardioMode: false
                        )
                        Button {
                            player.removeDraftRow(itemID: item.id, id: row.id)
                        } label: {
                            Image(systemName: "minus.circle")
                                .foregroundStyle(DSColors.textSecondary)
                        }
                        .buttonStyle(.plain)
                        .padding(.top, DSSpacing.sm)
                        .accessibilityLabel("Remove set row")
                    }
                }
                Button {
                    player.addDraftRow(itemID: item.id)
                } label: {
                    Label("Add set", systemImage: "plus.circle")
                        .font(.footnote)
                }
                .buttonStyle(.plain)
                .foregroundStyle(DSColors.accent)
            }
        }
    }

    private func hasValidDraft(_ item: BlockItemDTO) -> Bool {
        (player.drafts[item.id] ?? []).contains { $0.isValid(isCardioMode: item.isCardio) }
    }

    // MARK: - Finish

    private var finishSection: some View {
        Button {
            showingFinishConfirm = true
        } label: {
            if isFinishing {
                ProgressView()
                    .frame(maxWidth: .infinity)
                    .frame(height: 48)
            } else {
                Text("Finish workout")
                    .frame(maxWidth: .infinity)
            }
        }
        .buttonStyle(.dsPrimary)
        .disabled(isFinishing)
    }

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }

    /// Marks the workout completed (status-only update, plan
    /// untouched), clears the on-device drafts, and dismisses.
    /// Partial completion is allowed — pending blocks stay pending.
    private func finish() async {
        guard !isFinishing else { return }
        isFinishing = true
        defer { isFinishing = false }
        await workoutStore.detail(id: workoutID, refresh: true)
        guard let current = workoutStore.details[workoutID] else { return }
        await workoutStore.update(
            id: workoutID,
            request: UpdateWorkoutRequest(
                name: current.name,
                description: current.description,
                scheduledDate: current.scheduledDate,
                status: .completed,
                blocks: current.blocks.map { WorkoutBlockRequest(blockID: $0.blockID) }
            )
        )
        if workoutStore.errorMessage == nil {
            player.clearSnapshot()
            dismiss()
        }
    }
}

#Preview {
    WorkoutPlayerView(
        workoutID: "preview",
        workoutStore: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )),
        player: WorkoutPlayerStore(
            workoutID: "preview",
            api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )
        )
    )
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
