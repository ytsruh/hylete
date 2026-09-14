import SwiftUI

/// Full-screen Workout Player. Presented from `WorkoutDetailView`'s
/// Start button, it walks the user through every planned block and
/// exercise in order, logging sets as they go.
///
/// Each planned exercise gets type-appropriate editors (reusing
/// `SetRowEditor`): strength items get repeatable set rows,
/// cardio items a single session row, each with collapsible
/// notes. There is one call to action per block — "Mark as Done"
/// first POSTs every item's valid rows (each POST carries
/// `workout_id`/`block_id`/`workout_block_id`, creating normal
/// exercise entries: history, charts, and exports all include
/// them), then marks the block done. A failed submit aborts
/// before the status write. Done blocks keep the button (as "Log
/// additional sets") while they hold valid drafts, so sets typed
/// after reopening a finished workout can still be posted.
/// Half-typed rows stay on screen and survive background/kill via
/// the store's on-device snapshot.
///
/// Progress is hybrid: per-item logged counts plus local skips
/// (zero sets, e.g. no equipment or injury) drive a "ready to mark
/// done" hint, but the block pending/done/skipped check-off stays
/// manual — the source of truth, written through `WorkoutStore`.
/// The header's "% complete" is blocks-based (done or skipped over
/// total blocks) for the same reason. Done/skipped blocks
/// auto-collapse so the screen stays focused on remaining work;
/// the chevron always overrides for the session. Finish flips the
/// workout to completed (partial completion is allowed: pending
/// blocks may remain).
///
/// The screen idle timer is disabled while the player is open
/// (long rests can outlast the user's autolock) and restored on
/// dismiss.
struct WorkoutPlayerView: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var workoutStore: WorkoutStore
    @StateObject private var player: WorkoutPlayerStore

    let workoutID: String

    @State private var didRequestLoad: Bool = false
    @State private var showingFinishConfirm: Bool = false
    @State private var isFinishing: Bool = false
    /// Collapsed block ids (tap the chevron to fold long blocks).
    @State private var collapsedBlockIDs: Set<String> = []
    /// Block ids with a status write in flight (disables the Done
    /// button and shows a spinner so double-taps can't race).
    @State private var markingBlockIDs: Set<String> = []

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
                // Centered confirmation, matching the Delete
                // workout/block pattern elsewhere in the app
                // (a confirmationDialog would dock to the bottom
                // of the screen instead).
                .alert("Finish this workout?", isPresented: $showingFinishConfirm) {
                    Button("Finish workout") { Task { await finish() } }
                    Button("Keep going", role: .cancel) {}
                } message: {
                    Text("The workout is marked completed. Pending blocks may remain — partial completion is allowed.")
                }
                .task {
                    // Long rests between sets can outlast the
                    // user's autolock — keep the screen awake while
                    // the player is open. Released on disappear so
                    // Finish/Close can never leak it on.
                    TimerWakeLock.acquire()
                    guard !didRequestLoad else { return }
                    didRequestLoad = true
                    await player.load()
                    // Fold finished work away: done/skipped blocks
                    // start collapsed (pending blocks stay open).
                    // The chevron still overrides per block.
                    if let workout = player.workout {
                        collapsedBlockIDs = Set(workout.blocks
                            .filter { $0.status == .done || $0.status == .skipped }
                            .map(\.id))
                    }
                }
                .onDisappear {
                    TimerWakeLock.release()
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
        let progress = player.blockProgress()
        let percent = progress.total > 0 ? progress.done * 100 / progress.total : 0
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
            // Plan description (e.g. session goal) — collapsed by
            // default, hidden when the workout has none.
            if !workout.description.isEmpty {
                descriptionDisclosure(workout.description)
            }
            if progress.total > 0 {
                ProgressView(value: Double(progress.done), total: Double(progress.total))
                Text("\(percent)% complete")
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
        let isCollapsed = collapsedBlockIDs.contains(block.id)
        return VStack(alignment: .leading, spacing: DSSpacing.sm) {
            HStack(alignment: .top, spacing: DSSpacing.sm) {
                Button {
                    withAnimation {
                        if isCollapsed {
                            collapsedBlockIDs.remove(block.id)
                        } else {
                            collapsedBlockIDs.insert(block.id)
                        }
                    }
                } label: {
                    HStack(alignment: .top, spacing: DSSpacing.xs) {
                        Image(systemName: "chevron.right")
                            .font(.footnote.weight(.semibold))
                            .foregroundStyle(DSColors.textSecondary)
                            .rotationEffect(.degrees(isCollapsed ? 0 : 90))
                            .padding(.top, 4)
                        VStack(alignment: .leading, spacing: 2) {
                            Text(block.blockName)
                                .font(.headline)
                                .foregroundStyle(DSColors.text)
            Text(blockSubtitle(for: block))
                                .font(.subheadline)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                    }
                }
                .buttonStyle(.plain)
                .accessibilityLabel(isCollapsed ? "Expand \(block.blockName)" : "Collapse \(block.blockName)")
                Spacer()
                blockStatusMenu(block)
            }
            if !isCollapsed {
                Divider().background(DSColors.separator)
                // Plan description from the block catalogue (e.g.
                // coaching cues) — same collapsible treatment as
                // the workout description, hidden when empty.
                if !block.blockDescription.isEmpty {
                    descriptionDisclosure(block.blockDescription)
                }
                if block.items.isEmpty {
                    Text("This block's exercises are unavailable — you can still mark the block skipped.")
                        .font(.footnote)
                        .foregroundStyle(DSColors.textSecondary)
                }
                ForEach(block.items) { item in
                    itemView(item)
                    if item.id != block.items.last?.id {
                        Divider().background(DSColors.separator)
                    }
                }
            }
            // The block's single call to action — the status chip
            // menu alone wasn't discoverable, and a separate Log
            // button proved duplicative. Tapping first autosubmits
            // every item's valid rows, then performs the same
            // status-only Done write as the menu's Done row (a
            // no-op when already done). Manual check-off stays the
            // source of truth.
            //
            // Done blocks keep the button while they hold valid
            // unsubmitted drafts: reopening a finished workout
            // leaves its blocks done (only the workout status
            // flips), so without this, sets typed into a done
            // block could never be posted. Compact solid-primary
            // chrome — prominent, but one size below the
            // full-size "Finish workout" button.
            if block.status != .done || player.hasValidDrafts(in: block) {
                Button {
                    Task { await markBlockDone(block) }
                } label: {
                    if markingBlockIDs.contains(block.id) || player.isLoggingAny(in: block) {
                        ProgressView()
                            .tint(.white)
                            .frame(maxWidth: .infinity)
                    } else {
                        Text(block.status == .done ? "Log additional sets" : "Mark as Done")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.dsPrimaryCompact)
                .disabled(markingBlockIDs.contains(block.id) || player.isLoggingAny(in: block))
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

    /// Autosubmits the block's valid drafts, then marks it done
    /// via the shared store (status-only write) and refreshes the
    /// player's local statuses so the header and hints catch up
    /// without refetching items. A failed submit aborts before
    /// the status write — nothing is marked done unlogged, and
    /// the error stays on screen for retry.
    private func markBlockDone(_ block: WorkoutBlockDetailDTO) async {
        markingBlockIDs.insert(block.id)
        defer { markingBlockIDs.remove(block.id) }
        guard await player.logAllValid(in: block) else { return }
        await workoutStore.setBlockStatus(
            workoutID: workoutID,
            workoutBlockID: block.id,
            status: .done
        )
        await player.refreshBlockStatuses(from: workoutStore)
        // Fold the finished block away on success (a failed
        // submit or status write above leaves it open).
        if workoutStore.errorMessage == nil {
            withAnimation {
                collapsedBlockIDs.insert(block.id)
            }
        }
    }

    /// Collapsed-by-default "Description" disclosure, shared by
    /// the workout header and the block bodies (same label,
    /// styling, and behaviour in both places). The content text
    /// is pinned leading — without the explicit frame it drifts
    /// to the centre of the card width.
    private func descriptionDisclosure(_ text: String) -> some View {
        DisclosureGroup("Description") {
            Text(text)
                .font(.body)
                .foregroundStyle(DSColors.text)
                .frame(maxWidth: .infinity, alignment: .leading)
                .multilineTextAlignment(.leading)
        }
        .font(.footnote.weight(.semibold))
        .foregroundStyle(DSColors.textSecondary)
        .tint(DSColors.accent)
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
                        // Keep collapse in sync with check-offs:
                        // finished blocks fold away, reopened ones
                        // unfold. The chevron still overrides after.
                        if workoutStore.errorMessage == nil {
                            withAnimation {
                                if status == .done || status == .skipped {
                                    collapsedBlockIDs.insert(block.id)
                                } else {
                                    collapsedBlockIDs.remove(block.id)
                                }
                            }
                        }
                    }
                } label: {
                    Label(
                        status.displayName,
                        systemImage: block.status == status ? "checkmark" : "circle"
                    )
                }
            }
        } label: {
            // Secondary badge chrome (inverted fill + on-color
            // text, mirroring the cardio type pill) — the status
            // reads from the label, not from brand colour.
            Text(block.status.displayName)
                .font(.caption.weight(.semibold))
                .padding(.horizontal, DSSpacing.sm)
                .padding(.vertical, DSSpacing.xxs + 2)
                .background(Capsule().fill(DSColors.secondary))
                .foregroundStyle(DSColors.onSecondary)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Mark \(block.blockName) \(block.status.displayName)")
    }

    // MARK: - Items

    private func itemView(_ item: BlockItemDTO) -> some View {
        let skipped = player.skippedItemIDs.contains(item.id)
        let logged = player.loggedCount(itemID: item.id)
        // No type badge: strength vs cardio is already evident
        // from the editors below (set rows vs session row), and
        // the block subtitle carries the block type.
        return VStack(alignment: .leading, spacing: DSSpacing.xs) {
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
                // Collapsible notes ride along with the item's
                // next log call (Mark as Done autosubmits).
                // Hidden while skipped — a skipped exercise logs
                // nothing, so there is nothing to attach notes
                // to. The text is kept in the store, so
                // unskipping restores it.
                notesDisclosure(item)
            } else {
                Text("No sets will be logged for this exercise.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(.vertical, DSSpacing.xs)
    }

    /// Collapsible per-exercise notes, mirroring `NewSetView`'s
    /// notes section. Collapsed by default so it costs no screen
    /// room; every keystroke persists to the on-device snapshot.
    private func notesDisclosure(_ item: BlockItemDTO) -> some View {
        DisclosureGroup("Notes") {
            TextField(
                "Optional — e.g. felt easy, left knee niggle",
                text: notesBinding(for: item.id),
                axis: .vertical
            )
            .font(.body)
            .foregroundStyle(DSColors.text)
        }
        .font(.footnote.weight(.semibold))
        .foregroundStyle(DSColors.textSecondary)
        .tint(DSColors.accent)
    }

    private func notesBinding(for itemID: String) -> Binding<String> {
        Binding(
            get: { player.notes[itemID] ?? "" },
            set: { player.setNotes($0, for: itemID) }
        )
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
                // Same affordance as `NewSetView.setsSection`: a
                // Label with the plus.circle icon and no custom
                // font, so the row reads identically in both
                // places. The plain style + accent keeps it
                // legible on the card surface (a Form row gets
                // that tint for free). Pinned leading to match the
                // exercise-entry form alignment.
                Button {
                    player.addDraftRow(itemID: item.id)
                } label: {
                    Label("Add set", systemImage: "plus.circle")
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .buttonStyle(.plain)
                .foregroundStyle(DSColors.accent)
            }
        }
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

    /// Marks the workout completed via the status-only endpoint
    /// (plan and block check-offs untouched), clears the on-device
    /// drafts, and dismisses. Partial completion is allowed —
    /// pending blocks stay pending.
    private func finish() async {
        guard !isFinishing else { return }
        isFinishing = true
        defer { isFinishing = false }
        await workoutStore.setStatus(id: workoutID, status: .completed)
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
