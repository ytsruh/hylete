import SwiftUI

/// Create / edit sheet for a block. Sections: details (name,
/// description, kind), kind-specific config (rounds / rest /
/// time cap / interval — only the fields the chosen kind uses),
/// and the ordered exercise list (each row = exercise + one
/// free-text target field).
///
/// Save POSTs (create) or PUTs (edit, items fully replaced) via
/// the shared `BlockStore`. Validation mirrors the server so
/// Save stays disabled until the body is valid.
struct BlockEditorView: View {
    enum Mode {
        case create
        case edit(BlockDTO)
        /// A copy pre-fill: seeds the form from an existing
        /// block (name suffixed " copy") but saves as a brand
        /// new block — nothing is written until Save.
        case duplicate(BlockDTO)
    }

    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    let mode: Mode
    /// Shared with `BlocksListView` / `BlockDetailView` so the
    /// list and detail pick up the new / edited row in place.
    @ObservedObject var store: BlockStore
    /// Fired after a duplicate is saved (never for plain
    /// create or edit) so the presenter can navigate — e.g.
    /// the detail view pops back to the list instead of
    /// lingering on the original block.
    let onDuplicateSaved: (() -> Void)?

    init(mode: Mode, store: BlockStore, onDuplicateSaved: (() -> Void)? = nil) {
        self.mode = mode
        self._store = ObservedObject(wrappedValue: store)
        self.onDuplicateSaved = onDuplicateSaved
    }

    // MARK: - Draft state

    /// One planned exercise in the form. `id` is a local
    /// UUID (server ids arrive only after save); position is
    /// the array order.
    private struct DraftItem: Identifiable, Equatable {
        let id: UUID
        var exercise: ExerciseDTO
        var targetText: String
    }

    @State private var name: String = ""
    @State private var description: String = ""
    @State private var kind: BlockTypeDTO = .standard
    @State private var rounds: Int = 3
    @State private var restSeconds: Int = 60
    @State private var capMinutes: Int = 10
    @State private var intervalSeconds: Int = 60
    @State private var drafts: [DraftItem] = []

    @State private var exercises: [ExerciseDTO] = []
    @State private var exercisesFailed: Bool = false
    @State private var showingPicker: Bool = false
    @State private var pickedExerciseID: String?

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?
    @State private var didSeed: Bool = false

    private var isEditing: Bool {
        if case .edit = mode { return true }
        return false
    }

    private var isDuplicate: Bool {
        if case .duplicate = mode { return true }
        return false
    }

    /// The block the form is seeded from (edit and duplicate
    /// modes). Nil for create.
    private var sourceBlock: BlockDTO? {
        switch mode {
        case .create:
            return nil
        case .edit(let block), .duplicate(let block):
            return block
        }
    }

    private var trimmedName: String {
        name.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Mirrors the server's kind rules (`validateBlockFields`):
    /// standard carries no config; every other kind requires
    /// its own fields. Steppers already bound the ranges, so
    /// this is a final gate rather than the primary control.
    private var canSave: Bool {
        guard !isSaving else { return false }
        guard trimmedName.count >= 1, trimmedName.count <= 100 else { return false }
        guard description.count <= 1000 else { return false }
        guard drafts.count >= 1, drafts.count <= 20 else { return false }
        guard drafts.allSatisfy({ $0.targetText.count <= 500 }) else { return false }
        switch kind {
        case .standard:
            return true
        case .circuit:
            return rounds >= 1 && rounds <= 100
        case .amrap:
            return capMinutes >= 1 && capMinutes <= 1440
        case .emom:
            return rounds >= 1 && rounds <= 240
        }
    }

    var body: some View {
        NavigationStack {
            Form {
                detailsSection
                configSection
                exercisesSection
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
            .navigationTitle(isEditing ? "Edit Block" : isDuplicate ? "Duplicate Block" : "New Block")
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
            .alert("Delete this block?", isPresented: $showingDeleteConfirm) {
                Button("Delete", role: .destructive) {
                    Task { await deleteAndDismiss() }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This permanently removes the block and its planned exercises. Logged sets are unaffected.")
            }
            .sheet(isPresented: $showingPicker) {
                ExercisePickerSheet(exercises: exercises, selectedExerciseID: $pickedExerciseID)
            }
            .onChange(of: pickedExerciseID) { _, newID in
                guard let newID, let exercise = exercises.first(where: { $0.id == newID }) else { return }
                drafts.append(DraftItem(id: UUID(), exercise: exercise, targetText: ""))
                pickedExerciseID = nil
            }
            .task {
                await loadExercises()
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
            TextField("Description (optional)", text: $description, axis: .vertical)
                .lineLimit(1...4)
            Picker("Kind", selection: $kind) {
                ForEach(BlockTypeDTO.allCases, id: \.self) { kind in
                    Text(kind.displayName).tag(kind)
                }
            }
            .pickerStyle(.segmented)
            .onChange(of: kind) { _, _ in applyKindDefaults() }
        } header: {
            Text("Details")
        } footer: {
            Text(kindFooter)
        }
    }

    private var kindFooter: String {
        switch kind {
        case .standard:
            return "Exercises in order. Up to 20."
        case .circuit:
            return "Rotate through every exercise per round."
        case .amrap:
            return "As many rounds as possible before the cap."
        case .emom:
            return "Start the next exercise every interval."
        }
    }

    /// Only the current kind's fields render — standard shows
    /// no config at all. Switching kinds resets the hidden
    /// fields to their defaults so a stale value can never
    /// leak into the payload (`applyKindDefaults` zeroes what
    /// the kind doesn't use at save time too).
    @ViewBuilder
    private var configSection: some View {
        switch kind {
        case .standard:
            EmptyView()
        case .circuit:
            Section("Circuit") {
                Stepper("Rounds: \(rounds)", value: $rounds, in: 1...100)
                Stepper("Rest between rounds: \(restSeconds)s", value: $restSeconds, in: 0...3600, step: 15)
            }
        case .amrap:
            Section("AMRAP") {
                Stepper("Time cap: \(capMinutes) min", value: $capMinutes, in: 1...1440)
            }
        case .emom:
            Section("EMOM") {
                Stepper("Minutes: \(rounds)", value: $rounds, in: 1...240)
                Stepper("Every: \(intervalSeconds)s", value: $intervalSeconds, in: 15...3600, step: 15)
            }
        }
    }

    private var exercisesSection: some View {
        Section {
            if drafts.isEmpty {
                Button {
                    showingPicker = true
                } label: {
                    Label("Add exercise", systemImage: Icons.addSet)
                }
                .disabled(exercises.isEmpty)
            } else {
                ForEach($drafts) { $draft in
                    VStack(alignment: .leading, spacing: 4) {
                        Text(draft.exercise.name)
                            .font(.body)
                            .foregroundStyle(DSColors.text)
                        TextField("Target (optional) — e.g. 3x5 @ 100kg", text: $draft.targetText)
                            .font(.subheadline)
                    }
                    .padding(.vertical, DSSpacing.xxs)
                }
                .onDelete { drafts.remove(atOffsets: $0) }
                .onMove { drafts.move(fromOffsets: $0, toOffset: $1) }
                Button {
                    showingPicker = true
                } label: {
                    Label("Add exercise", systemImage: Icons.addSet)
                }
                .disabled(exercises.isEmpty || drafts.count >= 20)
            }
            if exercisesFailed {
                Text("Could not load exercises — check your connection and reopen the editor.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.destructive)
            }
        } header: {
            HStack {
                Text("Exercises")
                Spacer()
                if !drafts.isEmpty {
                    EditButton()
                        .font(.subheadline)
                }
            }
        } footer: {
            Text("1–20 exercises. Drag to reorder with Edit. Targets are free text up to 500 characters.")
        }
    }

    private var deleteSection: some View {
        Section {
            Button(role: .destructive) {
                showingDeleteConfirm = true
            } label: {
                HStack {
                    Spacer()
                    Text("Delete block")
                    Spacer()
                }
            }
        }
    }

    @State private var showingDeleteConfirm: Bool = false

    // MARK: - Save / Delete

    private func save() async {
        errorMessage = nil
        guard canSave else {
            errorMessage = "Give the block a name and add at least 1 exercise."
            return
        }
        isSaving = true
        defer { isSaving = false }

        let trimmedDescription = description.trimmingCharacters(in: .whitespacesAndNewlines)
        let items = drafts.map {
            BlockItemRequest(exerciseID: $0.exercise.id, targetText: $0.targetText.trimmingCharacters(in: .whitespacesAndNewlines))
        }
        switch mode {
        case .create, .duplicate:
            let created = await store.create(CreateBlockRequest(
                name: trimmedName,
                description: trimmedDescription,
                type: kind,
                rounds: kind == .circuit || kind == .emom ? rounds : 0,
                restSeconds: kind == .circuit ? restSeconds : 0,
                timeCapSeconds: kind == .amrap ? capMinutes * 60 : 0,
                intervalSeconds: kind == .emom ? intervalSeconds : 0,
                items: items
            ))
            if created == nil {
                errorMessage = store.errorMessage ?? "Could not save the block."
                return
            }
            if isDuplicate {
                onDuplicateSaved?()
            }
        case .edit(let block):
            await store.update(id: block.id, request: UpdateBlockRequest(
                name: trimmedName,
                description: trimmedDescription,
                type: kind,
                rounds: kind == .circuit || kind == .emom ? rounds : 0,
                restSeconds: kind == .circuit ? restSeconds : 0,
                timeCapSeconds: kind == .amrap ? capMinutes * 60 : 0,
                intervalSeconds: kind == .emom ? intervalSeconds : 0,
                items: items
            ))
            if let msg = store.errorMessage {
                errorMessage = msg
                return
            }
        }
        dismiss()
    }

    private func deleteAndDismiss() async {
        guard case .edit(let block) = mode else { return }
        isSaving = true
        defer { isSaving = false }
        await store.delete(id: block.id)
        if store.errorMessage != nil {
            errorMessage = store.errorMessage
            return
        }
        dismiss()
    }

    // MARK: - Loading / seeding

    private func loadExercises() async {
        do {
            exercises = try await env.api.listExercises()
        } catch {
            exercisesFailed = true
        }
    }

    /// Resets hidden config to kind defaults when the picker
    /// changes so stale values never leak into the payload.
    private func applyKindDefaults() {
        switch kind {
        case .standard:
            rounds = 0
            restSeconds = 0
            capMinutes = 10
            intervalSeconds = 60
        case .circuit:
            if rounds < 1 { rounds = 3 }
        case .amrap:
            break
        case .emom:
            if rounds < 1 { rounds = 10 }
        }
    }

    /// Populates the form from the existing block on first
    /// appear (edit and duplicate modes). Duplicate suffixed
    /// the name with " copy" and saves as a new block. Runs
    /// after the exercise catalog loads so drafts resolve to
    /// full `ExerciseDTO`s; items whose exercise vanished from
    /// the catalog keep their server-provided name via a
    /// synthesized row.
    private func seedIfNeeded() {
        guard !didSeed else { return }
        didSeed = true
        guard let block = sourceBlock else { return }
        name = isDuplicate ? "\(block.name) copy" : block.name
        description = block.description
        kind = block.type
        switch block.type {
        case .standard:
            break
        case .circuit:
            rounds = block.rounds
            restSeconds = block.restSeconds
        case .amrap:
            capMinutes = max(1, block.timeCapSeconds / 60)
        case .emom:
            rounds = block.rounds
            intervalSeconds = block.intervalSeconds
        }
        drafts = block.items.map { item in
            let exercise = exercises.first(where: { $0.id == item.exerciseID })
                ?? ExerciseDTO(
                    id: item.exerciseID,
                    name: item.exerciseName,
                    description: "",
                    videoURL: "",
                    imgURL: "",
                    imageURL: "",
                    type: item.exerciseType
                )
            return DraftItem(id: UUID(), exercise: exercise, targetText: item.targetText)
        }
    }
}

#Preview("Create") {
    BlockEditorView(
        mode: .create,
        store: BlockStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        ))
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
