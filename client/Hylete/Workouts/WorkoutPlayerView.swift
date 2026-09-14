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
/// the store's on-device snapshot. The tappable "X logged" row
/// opens the exercise's sets for this workout (history sheet with
/// edit + delete); the store retains the full entries, not just
/// counts.
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
    @EnvironmentObject private var env: AppEnvironment
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
    /// Planned item whose logged-sets history sheet is open.
    /// `BlockItemDTO` is `Identifiable`, so the sheet binds by
    /// item and always reads live buckets from the store.
    @State private var historyItem: BlockItemDTO?

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
                .sheet(item: $historyItem) { item in
                    WorkoutLoggedSetsSheet(player: player, item: item)
                        .environmentObject(env)
                        .environmentObject(authStore)
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
                Spacer()
                // Workout status as a solid primary badge (accent
                // fill + on-color text, mirroring the strength
                // type pill) — the block badges stay secondary so
                // the workout state owns the brand colour.
                Text(workout.status.displayName)
                    .font(.caption.weight(.semibold))
                    .padding(.horizontal, DSSpacing.sm)
                    .padding(.vertical, DSSpacing.xxs + 2)
                    .background(Capsule().fill(DSColors.accent))
                    .foregroundStyle(DSColors.onPrimary)
            }
            // Plan description (e.g. session goal) — collapsed by
            // default, hidden when the workout has none.
            if !workout.description.isEmpty {
                descriptionDisclosure(workout.description)
            }
            if progress.total > 0 {
                ProgressView(value: Double(progress.done), total: Double(progress.total))
                // Bottom line: completion left, block counts
                // right-aligned against it.
                HStack {
                    Text("\(percent)% complete")
                    Spacer()
                    Text(workout.progressLabel)
                }
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
            // Row 1: collapse toggle (chevron + name) and the
            // status menu.
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
                        Text(block.blockName)
                            .font(.headline)
                            .foregroundStyle(DSColors.text)
                    }
                }
                .buttonStyle(.plain)
                .accessibilityLabel(isCollapsed ? "Expand \(block.blockName)" : "Collapse \(block.blockName)")
                Spacer()
                blockStatusMenu(block)
            }
            // Row 2: block type left, logged counts right. Lives in
            // the card's full-width stack (not inside the toggle
            // button) — a Spacer inside a Button label collapses,
            // so right-alignment only works out here.
            HStack {
                Text(block.blockType.displayName)
                Spacer()
                Text(loggedLabel(for: block))
            }
            .font(.subheadline)
            .foregroundStyle(DSColors.textSecondary)
            // Row 3: time structure under the type (timed types
            // only) — its own line, so no separator ever dangles
            // after it or wraps awkwardly on narrow screens.
            if !block.timingSummary.isEmpty {
                Text(block.timingSummary)
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
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

    /// "X/Y logged" for the header's right-aligned counts slot.
    /// The type and time structure render as their own rows, so
    /// this carries counts only.
    private func loggedLabel(for block: WorkoutBlockDetailDTO) -> String {
        let logged = block.items.filter { player.isItemDone(itemID: $0.id) }.count
        return "\(logged)/\(block.items.count) logged"
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
            // Title row with Skip docked to it — the toggle reads
            // as part of the exercise header rather than floating
            // a row below the counts.
            HStack(alignment: .top, spacing: DSSpacing.xs) {
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
                Button(skipped ? "Unskip" : "Skip") {
                    player.toggleSkip(itemID: item.id)
                }
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
                .disabled(player.isLogging(itemID: item.id))
            }
            // The logged count opens this exercise's sets for
            // the workout (history sheet with edit + delete).
            // Accent + chevron mark it tappable. Rendered only
            // when something is logged, so the row takes no
            // space otherwise.
            if logged > 0 {
                Button {
                    historyItem = item
                } label: {
                    HStack(spacing: 2) {
                        Text(logged == 1 ? "1 logged" : "\(logged) logged")
                        Image(systemName: "chevron.right")
                    }
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(DSColors.accent)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Show logged sets for \(item.exerciseName)")
                .accessibilityHint("Opens the set history for this workout")
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
                Text("Skipped — no sets will be logged for this exercise.")
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

/// One planned exercise's logged sets for this workout, newest
/// first, with swipe-to-edit and delete. Opened from the
/// player's tappable "X logged" row. Entries come from the
/// player store's retained buckets (resume fetch + everything
/// logged this session), so no extra fetch is needed and newly
/// logged sets appear immediately.
///
/// Editing reuses `EditSetView` (it performs the PUT; the save
/// callback splices the server-confirmed row into the bucket).
/// Deletes are pessimistic — the row leaves only on a confirmed
/// API delete, so counts can never desync. Read-only callers
/// (the detail view) render `HistorySetRow`s directly instead.
private struct WorkoutLoggedSetsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var player: WorkoutPlayerStore
    let item: BlockItemDTO

    @State private var editingEntry: ExerciseEntryDTO?
    @State private var entryPendingDelete: ExerciseEntryDTO?
    @State private var showingDeleteConfirm: Bool = false

    private var entries: [ExerciseEntryDTO] {
        player.loggedEntries(itemID: item.id)
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(entries) { entry in
                    HistorySetRow(
                        entry: entry,
                        weightUnit: weightUnit,
                        distanceUnit: distanceUnit,
                        showsNotes: true
                    )
                    .opacity(player.isDeleting(entryID: entry.id) ? 0.5 : 1)
                    .swipeActions(edge: .leading, allowsFullSwipe: true) {
                        Button {
                            editingEntry = entry
                        } label: {
                            Label("Edit", systemImage: Icons.edit)
                                .labelStyle(.iconOnly)
                        }
                        .tint(Color(.systemGray2))
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                        Button(role: .destructive) {
                            entryPendingDelete = entry
                            showingDeleteConfirm = true
                        } label: {
                            Label("Delete", systemImage: Icons.trash)
                                .labelStyle(.iconOnly)
                        }
                    }
                }
            }
            .navigationTitle(item.exerciseName)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
            .sheet(item: $editingEntry) { entry in
                EditSetView(exerciseEntry: entry) { updated in
                    player.updateLoggedEntry(updated)
                }
                .environmentObject(env)
                .environmentObject(authStore)
            }
            .alert(
                item.isCardio ? "Delete this session?" : "Delete this set?",
                isPresented: $showingDeleteConfirm,
                presenting: entryPendingDelete
            ) { entry in
                Button("Delete", role: .destructive) {
                    Task { await player.deleteLoggedEntry(entry) }
                }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text(item.isCardio
                    ? "This will permanently remove the session from this workout's history."
                    : "This will permanently remove the set from this workout's history.")
            }
            // The last set deleted empties the sheet's reason to
            // exist — close it so the player (whose "X logged"
            // row is now gone) is what the user sees.
            .onChange(of: entries) { _, newEntries in
                if newEntries.isEmpty {
                    dismiss()
                }
            }
        }
    }

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
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
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
