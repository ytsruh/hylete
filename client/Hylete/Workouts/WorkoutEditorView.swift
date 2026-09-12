import SwiftUI

/// Create / edit sheet for a dated workout. Create mode builds a
/// workout from scratch: header (name, notes, scheduled window)
/// plus an inline block tree via the shared `BlockTreeEditor`.
/// (Copies arrive through duplicate / the plan-ahead sheet —
/// this editor never takes a source.) Edit mode edits the header
/// only: the tree is fixed at creation, and status moves through
/// the detail view's actions so the server owns `completed_at`.
///
/// Schedule inputs use compact pickers with a clear affordance
/// (same pattern as `GoalEditorView.dateRow`): empty fields send
/// nil for an unscheduled draft.
struct WorkoutEditorView: View {
    enum Mode {
        case create
        case edit(workoutID: String)
    }

    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    let mode: Mode
    /// Shared with `WorkoutsListView` so the list picks up the
    /// new / edited row (the store reloads itself on create).
    @ObservedObject var store: WorkoutStore

    @State private var name: String = ""
    @State private var notes: String = ""
    @State private var scheduledStart: Date?
    @State private var scheduledEnd: Date?
    @State private var blocks: [WorkoutBlockDraft] = []
    @State private var exercises: [ExerciseDTO] = []

    @State private var isLoading: Bool = false
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    private var isEditing: Bool {
        if case .edit = mode { return true }
        return false
    }

    private var trimmedName: String {
        name.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Save is enabled when the name is in range, the schedule
    /// isn't inverted, and (create only) every block is fully
    /// prescribed.
    private var canSave: Bool {
        guard !isSaving, !isLoading else { return false }
        guard trimmedName.count >= 1, trimmedName.count <= 200 else { return false }
        if let start = scheduledStart, let end = scheduledEnd, end < start {
            return false
        }
        if !isEditing {
            return blocks.allSatisfy { $0.isValid }
        }
        return true
    }

    private var weightUnit: String {
        env.authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        env.authStore.currentUser?.distanceUnit ?? "km"
    }

    var body: some View {
        NavigationStack {
            Group {
                if isLoading {
                    ProgressView()
                        .frame(maxWidth: .infinity, maxHeight: .infinity)
                } else {
                    Form {
                        nameSection
                        notesSection
                        scheduleSection
                        if !isEditing {
                            treeSection
                        }
                        if let errorMessage {
                            Section {
                                Text(errorMessage)
                                    .font(.footnote)
                                    .foregroundStyle(DSColors.destructive)
                            }
                        }
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
        }
        .task { await initialLoad() }
    }

    // MARK: - Sections

    private var nameSection: some View {
        Section {
            TextField("Name", text: $name, axis: .vertical)
                .lineLimit(1...3)
                .textInputAutocapitalization(.sentences)
        } header: {
            Text("Name")
        } footer: {
            Text("Up to 200 characters.")
        }
    }

    private var notesSection: some View {
        Section {
            TextField("Notes (optional)", text: $notes, axis: .vertical)
                .lineLimit(1...4)
        } header: {
            Text("Notes")
        }
    }

    private var scheduleSection: some View {
        Section {
            scheduleRow(label: "Starts", binding: $scheduledStart)
            scheduleRow(label: "Ends", binding: $scheduledEnd)
        } header: {
            Text("Schedule")
        } footer: {
            if let start = scheduledStart, let end = scheduledEnd, end < start {
                Text("The end must not be before the start.")
            } else {
                Text("Both are optional. Leave them empty for an unscheduled workout.")
            }
        }
    }

    /// Single compact datetime row with a clear affordance. The
    /// binding is `Binding<Date?>` so the empty state is part of
    /// the model and clears send nil.
    @ViewBuilder
    private func scheduleRow(label: String, binding: Binding<Date?>) -> some View {
        HStack {
            Text(label)
                .foregroundStyle(DSColors.text)
            Spacer()
            if let date = binding.wrappedValue {
                DatePicker(
                    label,
                    selection: Binding(
                        get: { date },
                        set: { binding.wrappedValue = $0 }
                    ),
                    displayedComponents: [.date, .hourAndMinute]
                )
                .labelsHidden()
                Button {
                    binding.wrappedValue = nil
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(DSColors.textSecondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear \(label.lowercased()) time")
            } else {
                Button {
                    binding.wrappedValue = .now
                } label: {
                    Text("Set")
                        .font(.subheadline)
                        .foregroundStyle(DSColors.accent)
                }
                .buttonStyle(.plain)
            }
        }
    }

    private var treeSection: some View {
        Section {
            BlockTreeEditor(
                blocks: $blocks,
                exercises: exercises,
                weightUnit: weightUnit,
                distanceUnit: distanceUnit
            )
        } header: {
            Text("Exercises")
        } footer: {
            Text("Workouts carry their own exercises. To train the same session again, duplicate it or plan it onto more dates from the workout list. Targets are optional — add exercises now and prescribe numbers whenever you like.")
        }
    }

    // MARK: - Data

    private func initialLoad() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        if case .edit(let id) = mode {
            // Header-only seed: the tree is fixed at creation
            // and never edited here.
            guard let detail = await store.loadDetail(id: id) else {
                errorMessage = store.errorMessage ?? "Could not load the workout."
                return
            }
            guard name.isEmpty else { return }
            name = detail.name
            notes = detail.notes
            scheduledStart = detail.scheduledStart
            scheduledEnd = detail.scheduledEnd
        } else {
            do {
                exercises = try await env.api.listExercises()
            } catch {
                errorMessage = "Could not load the exercise list."
            }
        }
    }

    // MARK: - Save

    private func save() async {
        errorMessage = nil
        guard canSave else { return }
        isSaving = true
        defer { isSaving = false }

        let trimmedNotes = notes.trimmingCharacters(in: .whitespacesAndNewlines)
        switch mode {
        case .create:
            let created = await store.create(CreateWorkoutRequest(
                name: trimmedName,
                notes: trimmedNotes,
                scheduledStart: scheduledStart,
                scheduledEnd: scheduledEnd,
                blocks: blocks.map { $0.asRequest() }
            ))
            if created == nil {
                errorMessage = store.errorMessage ?? "Could not save the workout."
                return
            }
        case .edit(let id):
            await store.update(id: id, request: UpdateWorkoutRequest(
                name: trimmedName,
                notes: trimmedNotes,
                scheduledStart: scheduledStart,
                scheduledEnd: scheduledEnd
            ))
            if store.errorMessage != nil {
                errorMessage = store.errorMessage
                return
            }
        }
        dismiss()
    }
}

#Preview("Create") {
    WorkoutEditorView(
        mode: .create,
        store: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        ))
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
