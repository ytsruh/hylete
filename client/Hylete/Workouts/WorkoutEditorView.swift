import HealthKit
import SwiftUI

/// Create / edit sheet for a workout. Sections: details (name,
/// scheduled date, description, status on edit, Health activity
/// type), and the ordered block list (each row = block name +
/// kind subtitle, reorderable).
///
/// Save POSTs (create) or PUTs (edit, blocks fully replaced with
/// statuses reset to pending) via the shared `WorkoutStore`.
/// Validation mirrors the server so Save stays disabled until the
/// body is valid. The block catalogue loads from `blockStore`
/// (refreshing it when empty) so the picker always offers current
/// blocks.
struct WorkoutEditorView: View {
    enum Mode {
        case create
        case edit(WorkoutDTO)
    }

    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    let mode: Mode
    /// Shared with `WorkoutsListView` / `WorkoutDetailView` so the
    /// list and detail pick up the new / edited row in place.
    @ObservedObject var store: WorkoutStore
    /// Supplies the block catalogue for the picker.
    @ObservedObject var blockStore: BlockStore

    init(mode: Mode, store: WorkoutStore, blockStore: BlockStore) {
        self.mode = mode
        self._store = ObservedObject(wrappedValue: store)
        self._blockStore = ObservedObject(wrappedValue: blockStore)
    }

    // MARK: - Draft state

    /// One planned block in the form. `id` is a local UUID (server
    /// join ids arrive only after save); position is the array order.
    private struct DraftBlock: Identifiable, Equatable {
        let id: UUID
        var block: BlockSummaryDTO
    }

    @State private var name: String = ""
    @State private var scheduledDate: Date = Date()
    @State private var description: String = ""
    @State private var status: WorkoutStatusDTO = .planned
    /// Apple Health activity type, saved with the workout so the
    /// player auto-starts the right session. Seeded from the
    /// server value on edit (blanket default on create, matching
    /// the server).
    @State private var healthActivityType: HKWorkoutActivityType = .traditionalStrengthTraining
    @State private var drafts: [DraftBlock] = []

    @State private var showingPicker: Bool = false
    @State private var pickedBlockID: String?
    @State private var blocksFailed: Bool = false

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?
    @State private var didSeed: Bool = false

    private var isEditing: Bool {
        if case .edit = mode { return true }
        return false
    }

    /// The workout the form is seeded from (edit mode). Nil for create.
    private var sourceWorkout: WorkoutDTO? {
        switch mode {
        case .create:
            return nil
        case .edit(let workout):
            return workout
        }
    }

    private var trimmedName: String {
        name.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Mirrors the server's rules (`validateWorkoutFields`): name
    /// 1–100, description ≤1000, 1–20 blocks.
    private var canSave: Bool {
        guard !isSaving else { return false }
        guard trimmedName.count >= 1, trimmedName.count <= 100 else { return false }
        guard description.count <= 1000 else { return false }
        guard drafts.count >= 1, drafts.count <= 20 else { return false }
        return true
    }

    var body: some View {
        NavigationStack {
            Form {
                detailsSection
                blocksSection
                if isEditing {
                    deleteSection
                }
                if let errorMessage {
                    Section {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            }
            .navigationTitle(isEditing ? "Edit Workout" : "New Workout")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                        .disabled(isSaving)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await save() }
                    } label: {
                        if isSaving {
                            ProgressView()
                        } else {
                            Text("Save").bold()
                        }
                    }
                    .disabled(!canSave)
                }
            }
            .alert("Delete this workout?", isPresented: $showingDeleteConfirm) {
                Button("Delete", role: .destructive) {
                    Task { await deleteAndDismiss() }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This permanently removes the workout and its plan. Logged sets are unaffected.")
            }
            .sheet(isPresented: $showingPicker) {
                BlockPickerSheet(blocks: blockStore.blocks, selectedBlockID: $pickedBlockID)
            }
            .onChange(of: pickedBlockID) { _, newID in
                guard let newID, let block = blockStore.blocks.first(where: { $0.id == newID }) else { return }
                drafts.append(DraftBlock(id: UUID(), block: block))
                pickedBlockID = nil
            }
            .task {
                await loadBlocks()
                seedIfNeeded()
            }
        }
    }

    // MARK: - Sections

    private var detailsSection: some View {
        Section {
            TextField("Name", text: $name, axis: .vertical)
                .lineLimit(1...2)
                .textInputAutocapitalization(.sentences)
            DatePicker("Date", selection: $scheduledDate, displayedComponents: .date)
            TextField("Description (optional)", text: $description, axis: .vertical)
                .lineLimit(1...4)
            if isEditing {
                Picker("Status", selection: $status) {
                    ForEach(WorkoutStatusDTO.allCases, id: \.self) { status in
                        Text(status.displayName).tag(status)
                    }
                }
            }
            Picker("Activity type", selection: $healthActivityType) {
                ForEach(WorkoutHealthActivityMapper.selectableTypes, id: \.rawValue) { type in
                    Text(WorkoutHealthActivityMapper.displayName(for: type)).tag(type)
                }
            }
        } header: {
            Text("Details")
        } footer: {
            Text(isEditing
                ? "Changing the block list resets every block to pending."
                : "New workouts start as Planned with every block pending.")
        }
    }

    private var blocksSection: some View {
        Section {
            if drafts.isEmpty {
                Button {
                    showingPicker = true
                } label: {
                    Label("Add block", systemImage: Icons.addSet)
                }
                .disabled(blockStore.blocks.isEmpty)
            } else {
                ForEach($drafts) { $draft in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(draft.block.name)
                            .font(.body)
                            .foregroundStyle(DSColors.text)
                        Text(blockSubtitle(for: draft.block))
                            .font(.subheadline)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    .padding(.vertical, DSSpacing.xxs)
                }
                .onDelete { drafts.remove(atOffsets: $0) }
                .onMove { drafts.move(fromOffsets: $0, toOffset: $1) }
                Button {
                    showingPicker = true
                } label: {
                    Label("Add block", systemImage: Icons.addSet)
                }
                .disabled(blockStore.blocks.isEmpty || drafts.count >= 20)
            }
            if blocksFailed {
                Text("Could not load blocks — check your connection and reopen the editor.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.destructive)
            }
        } header: {
            HStack {
                Text("Blocks")
                Spacer()
                if !drafts.isEmpty {
                    EditButton()
                        .font(.subheadline)
                }
            }
        } footer: {
            Text("1–20 blocks. Drag to reorder with Edit — the order is the training order.")
        }
    }

    private func blockSubtitle(for block: BlockSummaryDTO) -> String {
        let exercises = block.itemCount == 1 ? "1 exercise" : "\(block.itemCount) exercises"
        return "\(block.type.displayName) · \(exercises)"
    }

    private var deleteSection: some View {
        Section {
            Button(role: .destructive) {
                showingDeleteConfirm = true
            } label: {
                HStack {
                    Text("Delete workout")
                }
            }
        }
    }

    @State private var showingDeleteConfirm: Bool = false

    // MARK: - Save / Delete

    private func save() async {
        errorMessage = nil
        guard canSave else {
            errorMessage = "Give the workout a name and add at least 1 block."
            return
        }
        isSaving = true
        defer { isSaving = false }

        let trimmedDescription = description.trimmingCharacters(in: .whitespacesAndNewlines)
        let dayString = WorkoutDates.dayString(from: scheduledDate)
        let blocks = drafts.map { WorkoutBlockRequest(blockID: $0.block.id) }
        switch mode {
        case .create:
            let created = await store.create(CreateWorkoutRequest(
                name: trimmedName,
                description: trimmedDescription,
                scheduledDate: dayString,
                status: nil,
                healthActivityType: WorkoutHealthActivityMapper.key(for: healthActivityType),
                blocks: blocks
            ))
            if created == nil {
                errorMessage = store.errorMessage ?? "Could not save the workout."
                return
            }
        case .edit(let workout):
            await store.update(id: workout.id, request: UpdateWorkoutRequest(
                name: trimmedName,
                description: trimmedDescription,
                scheduledDate: dayString,
                status: status,
                healthActivityType: WorkoutHealthActivityMapper.key(for: healthActivityType),
                blocks: blocks
            ))
            if let msg = store.errorMessage {
                errorMessage = msg
                return
            }
        }
        dismiss()
    }

    private func deleteAndDismiss() async {
        guard case .edit(let workout) = mode else { return }
        isSaving = true
        defer { isSaving = false }
        await store.delete(id: workout.id)
        if store.errorMessage != nil {
            errorMessage = store.errorMessage
            return
        }
        dismiss()
    }

    // MARK: - Loading / seeding

    private func loadBlocks() async {
        if blockStore.blocks.isEmpty {
            await blockStore.load()
        }
        blocksFailed = blockStore.blocks.isEmpty && blockStore.errorMessage != nil
    }

    /// Populates the form from the existing workout on first appear
    /// (edit mode). Blocks resolve against the loaded catalogue;
    /// blocks missing from the catalogue keep their server-provided
    /// name via a synthesized summary row.
    private func seedIfNeeded() {
        guard !didSeed else { return }
        didSeed = true
        guard let workout = sourceWorkout else { return }
        name = workout.name
        description = workout.description
        status = workout.status
        // Server key wins; unknown/missing keys fall back to the
        // blanket default (never inference here — the explicit
        // row must round-trip exactly what the server holds).
        healthActivityType = WorkoutHealthActivityMapper.activityType(forKey: workout.healthActivityType)
            ?? .traditionalStrengthTraining
        if let date = WorkoutDates.date(from: workout.scheduledDate) {
            scheduledDate = date
        }
        drafts = workout.blocks.map { wb in
            let summary = blockStore.blocks.first(where: { $0.id == wb.blockID })
                ?? BlockSummaryDTO(
                    id: wb.blockID,
                    name: wb.blockName,
                    description: wb.blockDescription,
                    type: wb.blockType,
                    rounds: 0,
                    restSeconds: 0,
                    timeCapSeconds: 0,
                    intervalSeconds: 0,
                    itemCount: wb.itemCount,
                    createdAt: Date(),
                    updatedAt: Date()
                )
            return DraftBlock(id: UUID(), block: summary)
        }
    }
}

#Preview("Create") {
    WorkoutEditorView(
        mode: .create,
        store: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )),
        blockStore: BlockStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        ))
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
