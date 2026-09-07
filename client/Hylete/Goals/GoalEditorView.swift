import SwiftUI

/// Create / edit sheet for a goal. Mirrors the web's
/// `/goals/new` and `/goals/:id/edit` forms
/// (`internal/views/goals/form.templ`): title, optional
/// description, and three optional date fields. Save POSTs
/// (create) or PUTs (edit) via the shared `GoalStore`.
///
/// Date inputs use `.datePickerStyle(.compact)` — a single
/// inline row that expands on tap. This keeps the form
/// compact (matches the web's `<input type="date">`) instead
/// of swallowing the screen with three full calendars. Each
/// date has its own "Clear" affordance so the user can send
/// `nil` without losing the field.
struct GoalEditorView: View {
    enum Mode {
        case create
        case edit(GoalDTO)
    }

    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    let mode: Mode
    /// Shared with `GoalsListView` so the list picks up the
    /// new / edited row in place (optimistic insertion +
    /// server confirmation).
    @ObservedObject var store: GoalStore

    @State private var title: String = ""
    @State private var description: String = ""
    @State private var startDate: Date? = nil
    @State private var targetDate: Date? = nil
    @State private var endDate: Date? = nil

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    private var isEditing: Bool {
        if case .edit = mode { return true }
        return false
    }

    private var trimmedTitle: String {
        title.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var canSave: Bool {
        guard !isSaving else { return false }
        let count = trimmedTitle.count
        return count >= 1 && count <= 200
    }

    var body: some View {
        NavigationStack {
            Form {
                titleSection
                descriptionSection
                datesSection
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
            .navigationTitle(isEditing ? "Edit Goal" : "New Goal")
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
            .confirmationDialog(
                "Delete this goal?",
                isPresented: $showingDeleteConfirm,
                titleVisibility: .visible
            ) {
                Button("Delete goal", role: .destructive) {
                    Task { await deleteAndDismiss() }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This permanently removes the goal and cannot be undone.")
            }
            .onAppear { seedIfNeeded() }
        }
    }

    // MARK: - Sections

    private var titleSection: some View {
        Section {
            TextField("Title", text: $title, axis: .vertical)
                .lineLimit(1...3)
                .textInputAutocapitalization(.sentences)
        } header: {
            Text("Title")
        } footer: {
            Text("Up to 200 characters.")
        }
    }

    private var descriptionSection: some View {
        Section {
            TextField("Notes (optional)", text: $description, axis: .vertical)
                .lineLimit(1...6)
        } header: {
            Text("Description")
        } footer: {
            Text("Up to 2000 characters.")
        }
    }

    /// All three date fields in one section. Each field is
    /// always visible — the user toggles "set" by tapping
    /// the picker (which expands it) or the "Clear" button
    /// (which removes the date). Empty fields send `nil` so
    /// the server clears that column.
    private var datesSection: some View {
        Section {
            dateRow(
                label: "Start",
                binding: $startDate
            )
            dateRow(
                label: "Target",
                binding: $targetDate
            )
            dateRow(
                label: "End",
                binding: $endDate
            )
        } header: {
            Text("Dates")
        } footer: {
            Text("All three are optional. Tap a date to set it; tap the × to clear it.")
        }
    }

    /// Single compact date row. `binding` is `Binding<Date?>`
    /// so the empty/clear state is part of the model. Renders
    /// a compact picker when set, a "Set" placeholder when
    /// nil.
    @ViewBuilder
    private func dateRow(label: String, binding: Binding<Date?>) -> some View {
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
                    displayedComponents: .date
                )
                .labelsHidden()
                .datePickerStyle(.compact)
                Button {
                    binding.wrappedValue = nil
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(DSColors.textSecondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear \(label.lowercased()) date")
            } else {
                Button {
                    binding.wrappedValue = Calendar.current.startOfDay(for: .now)
                } label: {
                    Text("Set")
                        .font(.subheadline)
                        .foregroundStyle(DSColors.accent)
                }
                .buttonStyle(.plain)
            }
        }
    }

    /// Delete button lives on the edit form only (matches
    /// the web's edit page header — `form.templ:42-55`). The
    /// list view intentionally has no delete affordance.
    private var deleteSection: some View {
        Section {
            Button(role: .destructive) {
                showingDeleteConfirm = true
            } label: {
                HStack {
                    Spacer()
                    Text("Delete goal")
                    Spacer()
                }
            }
        }
    }

    @State private var showingDeleteConfirm: Bool = false

    // MARK: - Save / Delete

    private func save() async {
        errorMessage = nil
        let trimmed = trimmedTitle
        guard trimmed.count >= 1, trimmed.count <= 200 else {
            errorMessage = "Title must be between 1 and 200 characters."
            return
        }
        isSaving = true
        defer { isSaving = false }

        let request = CreateGoalRequest(
            title: trimmed,
            description: description.trimmingCharacters(in: .whitespacesAndNewlines),
            startDate: startDate,
            targetDate: targetDate,
            endDate: endDate
        )
        // `GoalStore` methods are non-throwing: they report
        // failure via a nil result / `errorMessage`, so no
        // do/catch is needed here.
        switch mode {
        case .create:
            let created = await store.create(request)
            if created == nil {
                errorMessage = store.errorMessage ?? "Could not save the goal."
                return
            }
        case .edit(let goal):
            let update = UpdateGoalRequest(
                title: request.title,
                description: request.description,
                startDate: request.startDate,
                targetDate: request.targetDate,
                endDate: request.endDate
            )
            await store.update(id: goal.id, request: update)
            if let msg = store.errorMessage {
                errorMessage = msg
                return
            }
        }
        dismiss()
    }

    private func deleteAndDismiss() async {
        guard case .edit(let goal) = mode else { return }
        isSaving = true
        defer { isSaving = false }
        await store.delete(id: goal.id)
        if store.errorMessage != nil {
            errorMessage = store.errorMessage
            return
        }
        dismiss()
    }

    // MARK: - Seeding

    /// Populates the form from the existing goal on first
    /// appear (edit mode only). Skipped when the title is
    /// already populated so a SwiftUI re-render mid-edit
    /// doesn't wipe the user's typing.
    private func seedIfNeeded() {
        guard case .edit(let goal) = mode else { return }
        guard title.isEmpty else { return }
        title = goal.title
        description = goal.description
        startDate = goal.startDate
        targetDate = goal.targetDate
        endDate = goal.endDate
    }
}

#Preview("Create") {
    GoalEditorView(
        mode: .create,
        store: GoalStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        ))
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}

#Preview("Edit") {
    GoalEditorView(
        mode: .edit(GoalDTO(
            id: "g1",
            title: "Bench 100kg",
            description: "Working sets",
            startDate: Date(),
            targetDate: Date().addingTimeInterval(14 * 24 * 3600),
            endDate: nil,
            completedAt: nil,
            createdAt: Date(),
            updatedAt: Date()
        )),
        store: GoalStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        ))
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}