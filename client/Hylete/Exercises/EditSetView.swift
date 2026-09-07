import SwiftUI

/// Edit sheet for a single exercise entry. The exercise is
/// read-only (you can't change which exercise an entry
/// belongs to) and the user can adjust the type-appropriate
/// metrics, notes, and date:
///   - strength entries: reps / weight / rest
///   - cardio entries: duration (min) / distance (preferred
///     unit) plus optional avg heart rate and calories
///
/// Presented by the per-exercise history view's swipe-edit
/// action. On Save the form PUTs the update via
/// `APIClient.updateExerciseEntry` and calls `onUpdated`
/// with the new server-confirmed `ExerciseEntryDTO` so
/// the parent can splice it into the visible page without
/// re-fetching the whole list.
struct EditSetView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    /// The entry as it currently lives on the page. Used
    /// to pre-populate the form fields and as the identity
    /// for the PUT request.
    let exerciseEntry: ExerciseEntryDTO

    /// Called after a successful save with the
    /// server-confirmed entry (which may differ slightly
    /// from the local copy — e.g. server-normalised date).
    let onUpdated: (ExerciseEntryDTO) -> Void

    // Strength fields.
    @State private var repsText: String = ""
    @State private var weightText: String = ""
    @State private var restText: String = ""

    // Cardio fields.
    @State private var durationMinutesText: String = ""
    @State private var distanceText: String = ""
    @State private var heartRateText: String = ""
    @State private var caloriesText: String = ""

    @State private var notes: String = ""
    @State private var createdAt: Date = Date()
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    private var isCardio: Bool { exerciseEntry.isCardio }

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }

    private var parsedReps: Int? {
        let trimmed = repsText.trimmingCharacters(in: .whitespaces)
        return trimmed.isEmpty ? nil : Int(trimmed)
    }

    private var parsedWeight: Double? {
        let trimmed = weightText.trimmingCharacters(in: .whitespaces)
        return trimmed.isEmpty ? nil : Double(trimmed)
    }

    private var parsedRest: Int {
        let trimmed = restText.trimmingCharacters(in: .whitespaces)
        return Int(trimmed) ?? 0
    }

    /// Parsed cardio duration in whole seconds (decimal minutes input).
    private var parsedDurationSeconds: Int? {
        let trimmed = durationMinutesText.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, let minutes = Double(trimmed) else { return nil }
        return Int((minutes * 60).rounded())
    }

    /// Parsed distance converted to metres from the user's unit.
    private var parsedDistanceMeters: Double? {
        let trimmed = distanceText.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, let value = Double(trimmed) else { return nil }
        switch distanceUnit {
        case "mi": return value * 1609.344
        default:   return value * 1000
        }
    }

    private var parsedHeartRate: Int {
        let trimmed = heartRateText.trimmingCharacters(in: .whitespaces)
        return Int(trimmed) ?? 0
    }

    private var parsedCalories: Double {
        let trimmed = caloriesText.trimmingCharacters(in: .whitespaces)
        return Double(trimmed) ?? 0
    }

    /// Type-aware validity mirroring the server's rules: strength
    /// needs reps ≥ 1 (weight can be 0); cardio needs a positive
    /// duration and a positive distance.
    private var canSave: Bool {
        guard !isSaving else { return false }
        if isCardio {
            guard let seconds = parsedDurationSeconds, seconds > 0 else { return false }
            guard let meters = parsedDistanceMeters, meters > 0 else { return false }
            return true
        }
        guard let reps = parsedReps, reps > 0 else { return false }
        guard let weight = parsedWeight, weight >= 0 else { return false }
        return true
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
            .navigationTitle("Edit Set")
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
        .task { populateFromEntry() }
    }

    // MARK: - Sections

    /// Exercise is read-only on the edit form. The web's
    /// `EditExerciseEntryForm` also disables this field —
    /// a set can't be re-assigned to a different exercise
    /// because the server's update endpoint validates the
    /// `exercise_id` against the entry's existing
    /// relationship.
    private var exerciseSection: some View {
        Section("Exercise") {
            HStack {
                Text(exerciseEntry.exerciseName)
                    .foregroundStyle(DSColors.text)
                Spacer()
                Image(systemName: "lock.fill")
                    .font(.caption2)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
    }

    /// Metric editor, branched by the entry's type: strength shows
    /// reps / weight / rest in one row; cardio shows duration /
    /// distance in the first row and the optional heart rate /
    /// calories pair below it.
    @ViewBuilder
    private var setsSection: some View {
        if isCardio {
            Section("Session") {
                HStack(spacing: DSSpacing.xs) {
                    field(label: "Duration (min)") {
                        TextField("0", text: $durationMinutesText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Distance (\(distanceUnit))") {
                        TextField("0.00", text: $distanceText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                }
                HStack(spacing: DSSpacing.xs) {
                    field(label: "Avg HR (bpm)") {
                        TextField("—", text: $heartRateText)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Calories (kcal)") {
                        TextField("—", text: $caloriesText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                }
            }
        } else {
            Section("Set") {
                HStack(spacing: DSSpacing.xs) {
                    field(label: "Reps") {
                        TextField("0", text: $repsText)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Weight (\(weightUnit))") {
                        TextField("0.0", text: $weightText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Rest (s)") {
                        TextField("0", text: $restText)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.ds)
                    }
                }
            }
        }
    }

    /// One labelled field cell — shared caption-over-input styling
    /// for both layouts.
    @ViewBuilder
    private func field<Content: View>(label: String, @ViewBuilder content: () -> Content) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(label)
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
            content()
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
            DatePicker(
                "Date & Time",
                selection: $createdAt,
                displayedComponents: [.date, .hourAndMinute]
            )
        }
    }

    // MARK: - Data

    /// Hydrates the editable state from the entry passed
    /// in. Runs once when the sheet appears.
    private func populateFromEntry() {
        notes = exerciseEntry.notes
        createdAt = exerciseEntry.createdAt
        if isCardio {
            durationMinutesText = String(format: "%.1f", Double(exerciseEntry.durationSeconds) / 60.0)
            let km = exerciseEntry.distanceMeters / 1000.0
            let displayKm = distanceUnit == "mi" ? km / 1.609344 : km
            distanceText = String(format: "%.2f", displayKm)
            heartRateText = exerciseEntry.avgHeartRate > 0 ? String(exerciseEntry.avgHeartRate) : ""
            caloriesText = exerciseEntry.caloriesBurned > 0 ? String(format: "%.0f", exerciseEntry.caloriesBurned) : ""
        } else {
            repsText = String(exerciseEntry.reps)
            weightText = String(format: "%.1f", exerciseEntry.weight)
            restText = exerciseEntry.restTime > 0 ? String(exerciseEntry.restTime) : ""
        }
    }

    private func save() async {
        errorMessage = nil
        if isCardio {
            guard let seconds = parsedDurationSeconds, seconds > 0,
                  let meters = parsedDistanceMeters, meters > 0 else {
                errorMessage = "Duration and distance are required for cardio sessions."
                return
            }
            isSaving = true
            defer { isSaving = false }
            await submit(UpdateExerciseEntryRequest(
                exerciseID: exerciseEntry.exerciseID,
                notes: notes,
                reps: 0,
                weight: 0,
                restTime: 0,
                durationSeconds: seconds,
                distanceMeters: meters,
                avgHeartRate: parsedHeartRate,
                caloriesBurned: parsedCalories,
                createdAt: createdAt
            ))
        } else {
            guard let reps = parsedReps, reps > 0,
                  let weight = parsedWeight, weight >= 0 else {
                errorMessage = "Reps must be at least 1 and weight must be a non-negative number."
                return
            }
            isSaving = true
            defer { isSaving = false }
            await submit(UpdateExerciseEntryRequest(
                exerciseID: exerciseEntry.exerciseID,
                notes: notes,
                reps: reps,
                weight: weight,
                restTime: parsedRest,
                createdAt: createdAt
            ))
        }
    }

    /// Shared PUT + result plumbing for both metric modes.
    private func submit(_ request: UpdateExerciseEntryRequest) async {
        do {
            let updated = try await env.api.updateExerciseEntry(id: exerciseEntry.id, request: request)
            onUpdated(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your changes."
        }
    }
}

#Preview {
    EditSetView(
        exerciseEntry: ExerciseEntryDTO(
            id: "entry-1",
            exerciseID: "ex-1",
            exerciseName: "Squat",
            exerciseType: "strength",
            reps: 5,
            weight: 80,
            notes: "Felt strong",
            restTime: 120,
            createdAt: Date()
        ),
        onUpdated: { _ in }
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
