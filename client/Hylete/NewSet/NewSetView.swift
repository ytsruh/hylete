import SwiftUI

/// The "log an entry" sheet. Picker for the exercise, then
/// type-appropriate entry rows — strength mode offers a repeatable
/// list of set rows (reps/weight/rest); cardio mode shows exactly one
/// fixed session row (duration/distance + optional HR/calories),
/// because a cardio workout is logged as a single session rather than
/// broken into sets. A notes field and optional timestamp round out
/// the form. Tapping save POSTs the batch in one request so all
/// entries share the same exercise, notes, and timestamp.
///
/// The row editor branches on the selected exercise's type:
/// a cardio exercise swaps the strength fields for duration +
/// distance (+ optional heart rate / calories), mirroring the
/// server-side validation rules (cardio requires duration and
/// distance; strength requires reps).
///
/// `initialExerciseID`, when supplied, pre-selects an
/// exercise in the picker once the catalogue has loaded.
/// Used by the per-exercise history view's `+` button —
/// mirrors the web's
/// `/exercises/:id/new` route, which renders the same form
/// with `preselectedExerciseID` set so the user lands
/// ready to log a set for the exercise they came from.
struct NewSetView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    /// Preselected exercise id. `nil` falls back to the
    /// existing "auto-pick the first exercise" behaviour so
    /// the dashboard's `+` button is unchanged.
    private let initialExerciseID: String?

    @State private var exercises: [ExerciseDTO] = []
    @State private var selectedExerciseID: String?
    @State private var sets: [SetDraft] = [SetDraft()]
    @State private var notes: String = ""
    @State private var timestamp: Date = Date()
    @State private var includeTimestamp: Bool = false
    @State private var isLoadingExercises: Bool = true
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    init(initialExerciseID: String? = nil) {
        self.initialExerciseID = initialExerciseID
    }

    /// The currently selected exercise, resolved once the catalogue
    /// has loaded. Drives the strength-vs-cardio row editor.
    private var selectedExercise: ExerciseDTO? {
        guard let selectedExerciseID else { return nil }
        return exercises.first { $0.id == selectedExerciseID }
    }

    /// `true` when the selected exercise is cardio — the row editor
    /// shows duration/distance fields instead of reps/weight/rest.
    private var isCardioMode: Bool {
        selectedExercise?.type.lowercased() == "cardio"
    }

    /// The user's preferred distance unit ("km"/"mi"); cardio
    /// distances are typed in this unit and converted to metres on
    /// save.
    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }

    var body: some View {
        NavigationStack {
            Form {
                exerciseSection
                setsSection
                notesSection
                timestampSection
                if let errorMessage {
                    Section {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            }
            .navigationTitle(isCardioMode ? "New Session" : "New Set")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
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
                    .disabled(isSaving || !canSave)
                }
            }
            // Crossing the strength/cardio boundary replaces the draft
            // rows: cardio logs exactly one session (multi-set doesn't
            // apply), and stale strength values must not leak into a
            // cardio row or vice versa. Switching between two exercises
            // of the SAME mode keeps whatever rows are already typed.
            .onChange(of: isCardioMode) { _, _ in resetSetsForMode() }
        }
        .task { await loadExercises() }
    }

    // MARK: - Sections

    private var exerciseSection: some View {
        Section("Exercise") {
            if isLoadingExercises {
                HStack { ProgressView(); Text("Loading…") }
            } else if exercises.isEmpty {
                Text("No exercises available. Ask an admin to add some.")
                    .foregroundStyle(DSColors.textSecondary)
            } else {
                Picker("Exercise", selection: $selectedExerciseID) {
                    Text("Select…").tag(String?.none)
                    ForEach(exercises) { exercise in
                        Text(exercise.name).tag(String?.some(exercise.id))
                    }
                }
                .pickerStyle(.menu)
                // Menu pickers default to the accent tint
                // (brand orange in this app). Override to
                // the default text colour so the selected
                // exercise name reads as neutral body text
                // instead of a coloured chip.
                .tint(DSColors.text)
            }
        }
    }

    private var setsSection: some View {
        Section {
            if isCardioMode {
                // Cardio is a one-shot session, not repeated sets:
                // exactly one fixed row, no add/remove affordances.
                if !sets.isEmpty {
                    SetRowEditor(
                        set: $sets[0],
                        weightUnit: authStore.currentUser?.weightUnit ?? "kg",
                        distanceUnit: distanceUnit,
                        isCardioMode: true
                    )
                }
            } else {
                ForEach($sets) { $set in
                    SetRowEditor(
                        set: $set,
                        weightUnit: authStore.currentUser?.weightUnit ?? "kg",
                        distanceUnit: distanceUnit,
                        isCardioMode: false
                    )
                }
                .onDelete { indexSet in
                    sets.remove(atOffsets: indexSet)
                    if sets.isEmpty {
                        sets.append(SetDraft(distanceUnit: distanceUnit))
                    }
                }
                Button {
                    sets.append(SetDraft(distanceUnit: distanceUnit))
                } label: {
                    Label("Add set", systemImage: "plus.circle")
                }
            }
        } header: {
            Text(isCardioMode ? "Session" : "Sets")
        } footer: {
            Text(isCardioMode
                 ? "A cardio entry is a single session. Duration and distance are required; heart rate and calories are optional."
                 : "Swipe a row to remove it. Each set is saved separately but shares this exercise, notes, and timestamp.")
        }
    }

    private var notesSection: some View {
        Section("Notes") {
            TextField("Optional", text: $notes, axis: .vertical)
                .lineLimit(1...4)
        }
    }

    private var timestampSection: some View {
        Section("When") {
            Toggle("Set a custom time", isOn: $includeTimestamp)
            if includeTimestamp {
                DatePicker(
                    "Time",
                    selection: $timestamp,
                    displayedComponents: [.date, .hourAndMinute]
                )
            } else {
                Text("Will be saved with the current time.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
    }

    // MARK: - Validation

    private var canSave: Bool {
        guard selectedExerciseID != nil else { return false }
        return sets.contains { $0.isValid(isCardioMode: isCardioMode) }
    }

    // MARK: - Data

    /// Replaces the draft rows whenever the form crosses the
    /// strength/cardio boundary so the fields on screen always match
    /// the new mode — a half-typed weight must not leak into a cardio
    /// session row and vice versa. Cardio always lands on exactly one
    /// fresh row; strength starts from one empty row. Keeps one draft
    /// ready to fill.
    private func resetSetsForMode() {
        sets = [SetDraft(distanceUnit: distanceUnit)]
    }

    private func loadExercises() async {
        defer { isLoadingExercises = false }
        do {
            exercises = try await env.api.listExercises()
            // Honour the caller's preselection — when the
            // user lands here from the per-exercise history
            // view's `+` button we want this exercise
            // already chosen. Falls back to the first
            // exercise on the dashboard for a faster happy
            // path there.
            if selectedExerciseID == nil {
                if let initialExerciseID,
                   exercises.contains(where: { $0.id == initialExerciseID }) {
                    selectedExerciseID = initialExerciseID
                } else {
                    selectedExerciseID = exercises.first?.id
                }
            }
        } catch {
            errorMessage = "Could not load the exercise list."
        }
    }

    private func save() async {
        errorMessage = nil
        guard let exerciseID = selectedExerciseID else { return }
        // Build the payload directly so we can use the
        // non-nil values once (no force-unwraps) and so the
        // filter + map is a single pass. Cardio drafts carry
        // their distance in the user's preferred unit; the API
        // expects metres.
        let validSets: [CreateSetInput] = sets.compactMap { draft in
            if isCardioMode {
                guard let seconds = draft.durationSecondsValue, seconds > 0,
                      let meters = draft.distanceMetersValue, meters > 0 else {
                    return nil
                }
                return CreateSetInput(
                    reps: 0,
                    weight: 0,
                    restTime: 0,
                    durationSeconds: Int(seconds),
                    distanceMeters: meters,
                    avgHeartRate: draft.avgHeartRate ?? 0,
                    caloriesBurned: draft.caloriesValue ?? 0
                )
            }
            guard let reps = draft.reps, reps > 0,
                  let weight = draft.weightValue, weight >= 0 else {
                return nil
            }
            return CreateSetInput(
                reps: reps,
                weight: weight,
                restTime: draft.restSeconds ?? 0
            )
        }
        guard !validSets.isEmpty else { return }
        isSaving = true
        defer { isSaving = false }
        do {
            _ = try await env.api.createExerciseEntries(
                CreateExerciseEntriesRequest(
                    exerciseID: exerciseID,
                    notes: notes,
                    createdAt: includeTimestamp ? timestamp : nil,
                    sets: validSets
                )
            )
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your set."
        }
    }
}

/// One row in the new-set form. In strength mode the meaningful
/// fields are reps / weight / rest; in cardio mode they are
/// duration (minutes) / distance (in the user's preferred unit)
/// plus optional heart rate and calories. Empty values are kept
/// as `nil` / `""` so freshly added rows don't trigger validation
/// errors before the user types.
struct SetDraft: Identifiable, Equatable {
    let id = UUID()

    // Strength fields.
    var reps: Int?
    var weightText: String = ""
    var restSeconds: Int?

    // Cardio fields.
    /// Decimal minutes ("42.5" = 42m30s); converted to seconds on save.
    var durationMinutesText: String = ""
    /// Typed in `distanceUnit`; converted to metres on save.
    var distanceText: String = ""
    var avgHeartRate: Int?
    var caloriesText: String = ""
    /// The unit `distanceText` is interpreted in ("km"/"mi"). Captured
    /// at draft creation so a mid-edit settings change can't silently
    /// reinterpret a half-typed value.
    var distanceUnit: String = "km"

    /// Numeric weight for the API. `nil` while the user
    /// hasn't typed anything, otherwise the parsed value
    /// (0 is valid for bodyweight exercises).
    var weightValue: Double? {
        get { weightText.isEmpty ? nil : Double(weightText) }
    }

    /// Parsed duration in seconds, or nil while blank/unparseable.
    var durationSecondsValue: Double? {
        guard !durationMinutesText.isEmpty, let minutes = Double(durationMinutesText) else { return nil }
        return minutes * 60
    }

    /// Parsed distance in metres (converted from `distanceUnit`),
    /// or nil while blank/unparseable.
    var distanceMetersValue: Double? {
        guard !distanceText.isEmpty, let value = Double(distanceText) else { return nil }
        switch distanceUnit {
        case "mi": return value * 1609.344
        default:   return value * 1000
        }
    }

    /// Parsed calories, or nil while blank/unparseable.
    var caloriesValue: Double? {
        get { caloriesText.isEmpty ? nil : Double(caloriesText) }
    }

    /// A row is "valid" when the metric pair required by the active
    /// mode is present: strength needs reps (weight can be 0 for
    /// bodyweight work); cardio needs both a positive duration and a
    /// positive distance, mirroring the server's validation. Heart
    /// rate and calories stay optional in both cases.
    func isValid(isCardioMode: Bool) -> Bool {
        if isCardioMode {
            guard let seconds = durationSecondsValue, seconds > 0 else { return false }
            guard let meters = distanceMetersValue, meters > 0 else { return false }
            return true
        }
        guard let reps, reps > 0 else { return false }
        guard let w = weightValue, w >= 0 else { return false }
        return true
    }
}

/// Editor row for a single `SetDraft`. Strength mode lays three
/// text fields out horizontally (reps / weight / rest) so the whole
/// row fits on one screen even on the smallest iPhone. Cardio mode
/// stacks two horizontal pairs (duration / distance, then the
/// optional heart rate / calories) because five columns cannot fit.
struct SetRowEditor: View {
    @Binding var set: SetDraft
    let weightUnit: String
    let distanceUnit: String
    let isCardioMode: Bool

    var body: some View {
        if isCardioMode {
            VStack(spacing: DSSpacing.xs) {
                HStack(spacing: DSSpacing.xs) {
                    field(label: "Duration (min)") {
                        TextField("0", text: $set.durationMinutesText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Distance (\(distanceUnit))") {
                        TextField("0.00", text: $set.distanceText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                }
                HStack(spacing: DSSpacing.xs) {
                    field(label: "Avg HR (bpm)") {
                        TextField("—", value: Binding(
                            get: { set.avgHeartRate ?? 0 },
                            set: { set.avgHeartRate = $0 == 0 ? nil : $0 }
                        ), format: .number)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Calories (kcal)") {
                        TextField("—", text: $set.caloriesText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                }
            }
        } else {
            HStack(spacing: DSSpacing.xs) {
                field(label: "Reps") {
                    TextField("0", value: Binding(
                        get: { set.reps ?? 0 },
                        set: { set.reps = $0 == 0 ? nil : $0 }
                    ), format: .number)
                        .keyboardType(.numberPad)
                        .textFieldStyle(.ds)
                }
                field(label: "Weight (\(weightUnit))") {
                    TextField("0.0", text: $set.weightText)
                        .keyboardType(.decimalPad)
                        .textFieldStyle(.ds)
                }
                field(label: "Rest (s)") {
                    TextField("0", value: Binding(
                        get: { set.restSeconds ?? 0 },
                        set: { set.restSeconds = $0 == 0 ? nil : $0 }
                    ), format: .number)
                        .keyboardType(.numberPad)
                        .textFieldStyle(.ds)
                }
            }
        }
    }

    /// One labelled field cell — extracted so both layouts share the
    /// caption-over-input styling.
    @ViewBuilder
    private func field<Content: View>(label: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label)
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
            content()
        }
    }
}

#Preview {
    NewSetView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
        .environmentObject(AuthStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
}
