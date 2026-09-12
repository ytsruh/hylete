import SwiftUI

/// Shared block-tree editor used by the workout editor. Owns the
/// block drafts, the per-block exercise picker flow, and the
/// add-block menu; callers wrap it in their own `Form` section
/// (headers/footers differ per surface). Positions are assigned
/// server-side by index, so drafts only track order in the array.
struct BlockTreeEditor: View {
    @Binding var blocks: [WorkoutBlockDraft]
    let exercises: [ExerciseDTO]
    let weightUnit: String
    let distanceUnit: String

    /// Which block the exercise picker is adding an item to.
    /// Nil means the picker is closed. Wrapped so
    /// `.sheet(item:)` has an `Identifiable` to drive off.
    @State private var pickerTarget: PickerTarget?
    @State private var pickedExerciseID: String?

    var body: some View {
        ForEach($blocks) { $block in
            BlockDraftEditor(
                block: $block,
                weightUnit: weightUnit,
                distanceUnit: distanceUnit,
                onAddExercise: { pickerTarget = PickerTarget(blockID: block.id) },
                onDeleteBlock: { blocks.removeAll { $0.id == block.id } }
            )
        }
        Menu {
            ForEach(WorkoutBlockType.allCases, id: \.self) { type in
                Button(type.displayName) {
                    blocks.append(WorkoutBlockDraft(type: type))
                }
            }
        } label: {
            Label("Add block", systemImage: "plus.circle")
        }
        .sheet(item: $pickerTarget) { target in
            ExercisePickerSheet(
                exercises: exercises,
                selectedExerciseID: $pickedExerciseID
            )
            .onDisappear {
                // The picker dismisses itself on selection;
                // attach the pick to its block here so
                // cancel leaves the tree untouched.
                if let exerciseID = pickedExerciseID,
                   let exercise = exercises.first(where: { $0.id == exerciseID }) {
                    appendItem(exercise, to: target.blockID)
                }
                pickedExerciseID = nil
            }
        }
    }

    private func appendItem(_ exercise: ExerciseDTO, to blockID: UUID) {
        guard let index = blocks.firstIndex(where: { $0.id == blockID }) else { return }
        // One exercise per block: re-adding the same exercise
        // would split its prescribed sets across two items.
        guard !blocks[index].items.contains(where: { $0.exercise.id == exercise.id }) else { return }
        blocks[index].items.append(WorkoutItemDraft(exercise: exercise, distanceUnit: distanceUnit))
    }
}

/// Identifiable wrapper around a block id so the exercise
/// picker's `.sheet(item:)` knows which block receives the pick.
private struct PickerTarget: Identifiable, Equatable {
    let blockID: UUID
    var id: UUID { blockID }
}

// MARK: - Drafts

/// Editable block: type, rounds, type-specific params, and its
/// prescribed items.
struct WorkoutBlockDraft: Identifiable, Equatable {
    let id = UUID()
    var type: WorkoutBlockType = .straight
    var rounds: Int = 1
    var restSeconds: Int = 0
    var intervalSeconds: Int = 0
    var timeCapSeconds: Int = 0
    var items: [WorkoutItemDraft] = []

    /// A block is prescribable when its type params check out
    /// and it carries at least one item (the server rejects
    /// empty item lists). Items themselves are always valid —
    /// targets are optional.
    var isValid: Bool {
        switch type {
        case .emom where intervalSeconds <= 0:
            return false
        case .amrap where timeCapSeconds <= 0:
            return false
        default:
            break
        }
        guard !items.isEmpty else { return false }
        return true
    }

    /// First problem for the block footer, or nil when valid.
    var problem: String? {
        switch type {
        case .emom where intervalSeconds <= 0:
            return "EMOM blocks need an interval above 0 seconds."
        case .amrap where timeCapSeconds <= 0:
            return "AMRAP blocks need a time cap above 0 seconds."
        default:
            break
        }
        if items.isEmpty {
            return "Add at least one exercise to this block."
        }
        return nil
    }

    func asRequest() -> WorkoutBlockInputRequest {
        WorkoutBlockInputRequest(
            type: type,
            // Straight blocks always run a single round — the
            // server normalises this too, but sending it clean
            // keeps the payload honest.
            rounds: type == .straight ? 1 : max(rounds, 1),
            restBetweenRoundsSeconds: restSeconds,
            intervalSeconds: intervalSeconds,
            timeCapSeconds: timeCapSeconds,
            items: items.map { $0.asRequest() }
        )
    }
}

/// Editable prescribed item: the linked exercise plus its target
/// values as form-friendly optionals/strings (blank = unset, so
/// fresh rows don't trip validation before the user types).
/// Mirrors `SetDraft`'s shape but speaks prescription ("target")
/// rather than logged values.
struct WorkoutItemDraft: Identifiable, Equatable {
    let id = UUID()
    var exercise: ExerciseDTO
    var targetSets: Int = 3
    var targetReps: Int?
    var weightText: String = ""
    var restSeconds: Int?
    var durationMinutesText: String = ""
    var distanceText: String = ""
    /// The unit `distanceText` is interpreted in. Captured at
    /// creation so a mid-edit settings change can't silently
    /// reinterpret a half-typed value (same pattern as
    /// `SetDraft.distanceUnit`).
    var distanceUnit: String = "km"
    var avgHeartRate: Int?
    var caloriesText: String = ""

    var isCardio: Bool { exercise.type.lowercased() == "cardio" }

    /// Parsed target weight, or nil while blank/unparseable.
    /// 0 is valid (bodyweight work).
    var weightValue: Double? {
        weightText.isEmpty ? nil : Double(weightText)
    }

    /// Parsed target duration in seconds, or nil while
    /// blank/unparseable.
    var durationSecondsValue: Double? {
        guard !durationMinutesText.isEmpty, let minutes = Double(durationMinutesText) else { return nil }
        return minutes * 60
    }

    /// Parsed target distance in metres (converted from
    /// `distanceUnit`), or nil while blank/unparseable.
    var distanceMetersValue: Double? {
        guard !distanceText.isEmpty, let value = Double(distanceText) else { return nil }
        switch distanceUnit {
        case "mi": return value * 1609.344
        default: return value * 1000
        }
    }

    var caloriesValue: Double? {
        caloriesText.isEmpty ? nil : Double(caloriesText)
    }

    /// An item is always prescribable once its exercise is
    /// attached — targets are fully optional (an exercise with
    /// no numbers is an open prescription). The editor's Save
    /// gating only needs the exercise, which the draft requires
    /// at construction.
    var isValid: Bool { true }

    /// Builds the request. Blank fields encode as zeros (the
    /// server reads all-zero targets as an open prescription).
    func asRequest() -> WorkoutItemInputRequest {
        if isCardio {
            return WorkoutItemInputRequest(
                exerciseID: exercise.id,
                targetSets: targetSets,
                targetDurationSeconds: Int(durationSecondsValue ?? 0),
                targetDistanceMeters: distanceMetersValue ?? 0,
                targetAvgHeartRate: avgHeartRate ?? 0,
                targetCalories: caloriesValue ?? 0
            )
        }
        return WorkoutItemInputRequest(
            exerciseID: exercise.id,
            targetSets: targetSets,
            targetReps: targetReps ?? 0,
            targetWeight: weightValue ?? 0,
            targetRestSeconds: restSeconds ?? 0
        )
    }
}

/// Seeds an item draft from a persisted item's targets. Exercises
/// that no longer exist in the catalogue resolve to nil so the
/// caller can drop the row instead of crashing.
func workoutItemDraft(
    exerciseID: String,
    targetSets: Int,
    targetReps: Int,
    targetWeight: Double,
    targetRestSeconds: Int,
    targetDurationSeconds: Int,
    targetDistanceMeters: Double,
    targetAvgHeartRate: Int,
    targetCalories: Double,
    exercises: [ExerciseDTO],
    distanceUnit: String
) -> WorkoutItemDraft? {
    guard let exercise = exercises.first(where: { $0.id == exerciseID }) else {
        return nil
    }
    return WorkoutItemDraft(
        exercise: exercise,
        targetSets: targetSets,
        targetReps: targetReps == 0 ? nil : targetReps,
        weightText: targetWeight == 0 ? "" : String(targetWeight),
        restSeconds: targetRestSeconds == 0 ? nil : targetRestSeconds,
        durationMinutesText: targetDurationSeconds == 0
            ? "" : String(Double(targetDurationSeconds) / 60),
        distanceText: targetDistanceMeters == 0
            ? "" : formatTargetDistance(targetDistanceMeters, unit: distanceUnit),
        distanceUnit: distanceUnit,
        avgHeartRate: targetAvgHeartRate == 0 ? nil : targetAvgHeartRate,
        caloriesText: targetCalories == 0 ? "" : String(targetCalories)
    )
}

/// Renders a metre value in the given unit for seeding the
/// distance field ("5.2" for km, raw miles otherwise).
func formatTargetDistance(_ meters: Double, unit: String) -> String {
    switch unit {
    case "mi": return String(meters / 1609.344)
    default: return String(meters / 1000)
    }
}

// MARK: - Block editor

/// One block card inside a tree editor: type picker, rounds +
/// type-specific params, the item list, and the add-exercise
/// affordance.
struct BlockDraftEditor: View {
    @Binding var block: WorkoutBlockDraft
    let weightUnit: String
    let distanceUnit: String
    let onAddExercise: () -> Void
    let onDeleteBlock: () -> Void

    var body: some View {
        Section {
            HStack {
                Menu {
                    ForEach(WorkoutBlockType.allCases, id: \.self) { type in
                        Button(type.displayName) {
                            block.type = type
                            // Straight ignores rounds — reset so a
                            // retargeted block can't carry a stale
                            // count into its payload.
                            if type == .straight {
                                block.rounds = 1
                            }
                        }
                    }
                } label: {
                    HStack {
                        Text(block.type.displayName)
                            .foregroundStyle(DSColors.text)
                        Image(systemName: "chevron.up.chevron.down")
                            .font(.caption)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
                .accessibilityLabel("Block type")
                Spacer()
                Button(role: .destructive) {
                    onDeleteBlock()
                } label: {
                    Image(systemName: Icons.trash)
                        .foregroundStyle(DSColors.destructive)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Delete block")
            }

            if block.type != .straight {
                Stepper("Rounds: \(block.rounds)", value: $block.rounds, in: 1...100)
            }
            switch block.type {
            case .superset, .circuit:
                SecondsField(label: "Rest between rounds", value: $block.restSeconds)
            case .emom:
                SecondsField(label: "Interval (seconds)", value: $block.intervalSeconds)
            case .amrap:
                SecondsField(label: "Time cap (seconds)", value: $block.timeCapSeconds)
            case .straight:
                EmptyView()
            }

            ForEach($block.items) { $item in
                ItemDraftEditor(item: $item, weightUnit: weightUnit, distanceUnit: distanceUnit)
            }
            .onDelete { indexSet in
                block.items.remove(atOffsets: indexSet)
            }

            Button {
                onAddExercise()
            } label: {
                Label("Add exercise", systemImage: "plus.circle")
            }
        } header: {
            Text("Block")
        } footer: {
            if let problem = block.problem {
                Text(problem)
            }
        }
    }
}

/// Integer seconds field. Values are clamped at zero so a stray
/// minus can never reach the payload.
private struct SecondsField: View {
    let label: String
    @Binding var value: Int

    var body: some View {
        HStack {
            Text(label)
                .foregroundStyle(DSColors.text)
            Spacer()
            TextField("0", value: Binding(
                get: { value },
                set: { value = max($0, 0) }
            ), format: .number)
                .keyboardType(.numberPad)
                .multilineTextAlignment(.trailing)
                .frame(width: 80)
        }
    }
}

// MARK: - Item editor

/// One prescribed item: exercise header with a type chip, a sets
/// stepper, and the type-appropriate target fields (strength:
/// reps/weight/rest; cardio: duration/distance + optional
/// HR/calories). Removal is the parent's swipe-to-delete.
private struct ItemDraftEditor: View {
    @Binding var item: WorkoutItemDraft
    let weightUnit: String
    let distanceUnit: String

    var body: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack {
                Text(item.exercise.name)
                    .font(.headline)
                    .foregroundStyle(DSColors.text)
                    .lineLimit(1)
                ExerciseTypeChip(type: item.exercise.type)
                Spacer()
            }

            Stepper("Sets: \(max(item.targetSets, 1))", value: $item.targetSets, in: 1...100)

            if item.isCardio {
                VStack(spacing: DSSpacing.xs) {
                    HStack(spacing: DSSpacing.xs) {
                        field(label: "Duration (min)") {
                            TextField("0", text: $item.durationMinutesText)
                                .keyboardType(.decimalPad)
                                .textFieldStyle(.ds)
                        }
                        field(label: "Distance (\(distanceUnit))") {
                            TextField("0.00", text: $item.distanceText)
                                .keyboardType(.decimalPad)
                                .textFieldStyle(.ds)
                        }
                    }
                    HStack(spacing: DSSpacing.xs) {
                        field(label: "Avg HR (bpm)") {
                            TextField("—", value: Binding(
                                get: { item.avgHeartRate ?? 0 },
                                set: { item.avgHeartRate = $0 == 0 ? nil : $0 }
                            ), format: .number)
                                .keyboardType(.numberPad)
                                .textFieldStyle(.ds)
                        }
                        field(label: "Calories (kcal)") {
                            TextField("—", text: $item.caloriesText)
                                .keyboardType(.decimalPad)
                                .textFieldStyle(.ds)
                        }
                    }
                }
            } else {
                HStack(spacing: DSSpacing.xs) {
                    field(label: "Reps") {
                        TextField("0", value: Binding(
                            get: { item.targetReps ?? 0 },
                            set: { item.targetReps = $0 == 0 ? nil : $0 }
                        ), format: .number)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Weight (\(weightUnit))") {
                        TextField("0.0", text: $item.weightText)
                            .keyboardType(.decimalPad)
                            .textFieldStyle(.ds)
                    }
                    field(label: "Rest (s)") {
                        TextField("0", value: Binding(
                            get: { item.restSeconds ?? 0 },
                            set: { item.restSeconds = $0 == 0 ? nil : $0 }
                        ), format: .number)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.ds)
                    }
                }
            }
        }
        .padding(.vertical, DSSpacing.xs)
    }

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
