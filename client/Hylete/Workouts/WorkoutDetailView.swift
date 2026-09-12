import SwiftUI

/// The beta workout detail, pushed from `WorkoutsListView`. Shows
/// the header (schedule, status, notes), the block tree with
/// per-item targets and logged progress, the ad-hoc entries, and
/// the status actions (complete / reopen / cancel / delete) plus
/// the plan-again and duplicate actions. Read-only apart from
/// status: the tree is fixed at creation, header edits live in
/// `WorkoutEditorView`.
///
/// Per-item logging arrives in the next slice — this view owns the
/// `noteLoggedEntries` refresh it will call on dismiss.
///
/// Stack-less content: the list's `NavigationStack` provides the
/// single stack.
struct WorkoutDetailView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    let workoutID: String
    @ObservedObject var store: WorkoutStore

    @State private var showingEditor: Bool = false
    @State private var showingDeleteConfirm: Bool = false
    @State private var showingPlanAhead: Bool = false
    /// Pending log sheet target (per-item prefilled or ad-hoc).
    /// Nil means no sheet.
    @State private var loggingTarget: LoggingTarget?

    private var detail: WorkoutDetailDTO? {
        store.detail(for: workoutID)
    }

    private var weightUnit: String {
        env.authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        env.authStore.currentUser?.distanceUnit ?? "km"
    }

    var body: some View {
        Group {
            if let detail {
                loadedDetail(detail)
            } else if let error = store.errorMessage {
                errorState(error)
            } else {
                ProgressView()
                    .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
        }
        .navigationTitle(detail?.name ?? "Workout")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    showingEditor = true
                } label: {
                    Image(systemName: Icons.edit)
                }
                .accessibilityLabel("Edit workout")
                .disabled(detail == nil)
            }
        }
        .sheet(isPresented: $showingEditor) {
            WorkoutEditorView(mode: .edit(workoutID: workoutID), store: store)
                .environmentObject(env)
        }
        .sheet(isPresented: $showingPlanAhead) {
            if let detail {
                PlanAheadSheet(workoutID: detail.id, workoutName: detail.name, workoutStore: store)
            }
        }
        .sheet(item: $loggingTarget) { target in
            if let detail {
                NewSetView(
                    initialExerciseID: target.exerciseID,
                    workoutID: detail.id,
                    workoutItemID: target.workoutItemID,
                    roundNumber: target.roundNumber,
                    blockRounds: target.blockRounds,
                    workoutName: detail.name,
                    targetHint: target.targetHint,
                    onSaved: {
                        Task { await store.noteLoggedEntries(workoutID: detail.id) }
                    }
                )
                .environmentObject(env)
            }
        }
        .confirmationDialog(
            "Delete this workout?",
            isPresented: $showingDeleteConfirm,
            titleVisibility: .visible
        ) {
            Button("Delete workout", role: .destructive) {
                Task { await deleteAndPop() }
            }
            Button("Cancel", role: .cancel) {}
        } message: {
            Text("Sets already logged into this workout are kept — they become standalone entries.")
        }
        .task { await store.loadDetail(id: workoutID) }
    }

    // MARK: - Detail

    @ViewBuilder
    private func loadedDetail(_ detail: WorkoutDetailDTO) -> some View {
        List {
            Section {
                headerCard(detail)
                statusActions(detail)
            }
            ForEach(detail.blocks) { block in
                Section {
                    blockView(block, detail: detail)
                } header: {
                    Text(blockHeaderTitle(block))
                }
            }
            if !detail.adHocEntries.isEmpty {
                Section {
                    ForEach(detail.adHocEntries) { entry in
                        SetRow(entry: entry, weightUnit: weightUnit, distanceUnit: distanceUnit)
                    }
                } header: {
                    Text("Extra sets")
                } footer: {
                    Text("Logged into this workout without a planned exercise.")
                }
            }
            Section {
                Button {
                    loggingTarget = LoggingTarget(
                        exerciseID: nil,
                        workoutItemID: nil,
                        roundNumber: 0,
                        blockRounds: 0,
                        targetHint: nil
                    )
                } label: {
                    Label("Log extra set", systemImage: "plus.circle")
                }
            } footer: {
                Text("Logs a set into this workout without picking a planned exercise first.")
            }
        }
        .listStyle(.automatic)
        .refreshable {
            await store.loadDetail(id: workoutID, force: true)
        }
    }

    private func headerCard(_ detail: WorkoutDetailDTO) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack {
                statusChip(detail.status)
                Spacer()
            }
            let schedule = workoutScheduleSummary(start: detail.scheduledStart, end: detail.scheduledEnd)
            if !schedule.isEmpty {
                Label(schedule, systemImage: Icons.calendar)
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
            }
            if !detail.notes.isEmpty {
                Text(detail.notes)
                    .font(.body)
                    .foregroundStyle(DSColors.text)
            }
            if detail.isCompleted, let completedAt = detail.completedAt {
                Text("Completed \(completedAt, style: .date)")
                    .font(.caption)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(.vertical, DSSpacing.xs)
    }

    @ViewBuilder
    private func statusChip(_ status: String) -> some View {
        switch status {
        case "completed":
            Text("Completed")
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.success)
        case "cancelled":
            Text("Cancelled")
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
        default:
            Text("Planned")
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.accent)
        }
    }

    /// Primary status action plus plan-again, duplicate, and
    /// the destructive delete. The secondary transitions (reopen
    /// a completed workout, re-plan a cancelled one) live here
    /// too so every state can move without leaving the view.
    @ViewBuilder
    private func statusActions(_ detail: WorkoutDetailDTO) -> some View {
        Button {
            showingPlanAhead = true
        } label: {
            HStack {
                Spacer()
                Label("Plan again…", systemImage: Icons.calendar)
                    .bold()
                Spacer()
            }
        }
        .buttonStyle(.dsSecondary)
        if detail.status == "planned" {
            Button {
                Task { await store.complete(id: workoutID) }
            } label: {
                HStack {
                    Spacer()
                    Text("Mark complete")
                        .bold()
                    Spacer()
                }
            }
            .buttonStyle(.dsPrimary)
            Button(role: .destructive) {
                Task { await store.cancel(id: workoutID) }
            } label: {
                HStack {
                    Spacer()
                    Text("Cancel workout")
                    Spacer()
                }
            }
        } else {
            Button {
                Task { await store.reopen(id: workoutID) }
            } label: {
                HStack {
                    Spacer()
                    Text(detail.status == "cancelled" ? "Re-plan" : "Reopen")
                        .bold()
                    Spacer()
                }
            }
            .buttonStyle(.dsSecondary)
        }
        Button(role: .destructive) {
            showingDeleteConfirm = true
        } label: {
            HStack {
                Spacer()
                Text("Delete workout")
                Spacer()
            }
        }
        Button {
            Task { await store.duplicate(id: workoutID) }
        } label: {
            HStack {
                Spacer()
                Label("Duplicate", systemImage: Icons.duplicate)
                Spacer()
            }
        }
    }

    // MARK: - Blocks & items

    private func blockHeaderTitle(_ block: WorkoutBlockDTO) -> String {
        var parts = [block.type.displayName]
        if block.type != .straight {
            parts.append("· \(block.rounds) rounds")
        }
        return parts.joined(separator: " ")
    }

    @ViewBuilder
    private func blockView(_ block: WorkoutBlockDTO, detail: WorkoutDetailDTO) -> some View {
        let logged = detail.loggedItemCount(in: block)
        if !block.items.isEmpty {
            Text("\(logged)/\(block.items.count) logged")
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
        }
        ForEach(block.items) { item in
            itemView(item, in: block, detail: detail)
        }
    }

    @ViewBuilder
    private func itemView(_ item: WorkoutItemDTO, in block: WorkoutBlockDTO, detail: WorkoutDetailDTO) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack {
                Text(item.exerciseName)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                Spacer()
                let logged = detail.entries(for: item.id).count
                if logged > 0 {
                    Text("\(logged) logged")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(DSColors.success)
                } else {
                    Text(workoutItemTargetSummary(item, weightUnit: weightUnit, distanceUnit: distanceUnit))
                        .font(.callout.monospacedDigit())
                        .foregroundStyle(DSColors.textSecondary)
                }
                Button {
                    // Straight blocks log unrounded (round 0);
                    // multi-round blocks default to the first
                    // round with nothing logged yet.
                    let round = block.rounds > 1
                        ? detail.suggestedRound(for: item.id, rounds: block.rounds)
                        : 0
                    loggingTarget = LoggingTarget(
                        exerciseID: item.exerciseID,
                        workoutItemID: item.id,
                        roundNumber: round,
                        blockRounds: block.rounds,
                        targetHint: workoutItemTargetSummary(
                            item, weightUnit: weightUnit, distanceUnit: distanceUnit)
                    )
                } label: {
                    Image(systemName: "plus.circle")
                        .foregroundStyle(DSColors.accent)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Log \(item.exerciseName)")
            }
            // Logged sets render beneath their planned item with
            // a round badge when the set belongs to a round.
            ForEach(detail.entries(for: item.id)) { entry in
                HStack(alignment: .firstTextBaseline) {
                    if entry.roundNumber > 0 {
                        Text("R\(entry.roundNumber)")
                            .font(.caption.weight(.semibold))
                            .foregroundStyle(DSColors.accent)
                            .frame(width: 28, alignment: .leading)
                    }
                    SetRow(entry: entry, weightUnit: weightUnit, distanceUnit: distanceUnit)
                }
            }
        }
        .padding(.vertical, DSSpacing.xs)
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
                Task { await store.loadDetail(id: workoutID, force: true) }
            }
            .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func deleteAndPop() async {
        await store.delete(id: workoutID)
        if store.errorMessage == nil {
            dismiss()
        }
    }
}

/// Pending `NewSetView` configuration: per-item prefilled
/// logging (exercise + item + round) or workout-level ad-hoc
/// logging (exercise picked in the form, no item, round 0).
private struct LoggingTarget: Identifiable, Equatable {
    let id = UUID()
    let exerciseID: String?
    let workoutItemID: String?
    let roundNumber: Int
    let blockRounds: Int
    let targetHint: String?
}

#Preview {
    NavigationStack {
        WorkoutDetailView(workoutID: "w-1", store: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
