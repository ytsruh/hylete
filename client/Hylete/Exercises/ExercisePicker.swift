import SwiftUI

/// Shared exercise-type filter used by both the Exercises tab and the
/// NewSet picker sheet. `all` disables type filtering; the other cases
/// match `ExerciseDTO.type` case-insensitively.
enum ExerciseTypeFilter: String, CaseIterable, Identifiable {
    case all
    case strength
    case cardio
    case other

    var id: String { rawValue }

    /// User-facing label. Mirrors the web type dropdown
    /// ("All types" / "Strength" / "Cardio" / "Other").
    var displayName: String {
        switch self {
        case .all: return "All"
        case .strength: return "Strength"
        case .cardio: return "Cardio"
        case .other: return "Other"
        }
    }
}

/// Pure exercise catalogue filter — name + alias search (case-insensitive,
/// trimmed) combined with the type filter. Extracted from the views so
/// it can be unit-tested without hosting SwiftUI.
func filterExercises(
    _ exercises: [ExerciseDTO],
    search: String,
    typeFilter: ExerciseTypeFilter
) -> [ExerciseDTO] {
    let query = search.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    return exercises.filter { exercise in
        let matchesQuery = query.isEmpty
            || exercise.name.lowercased().contains(query)
            || exercise.aliases.lowercased().contains(query)
        let matchesType = typeFilter == .all || exercise.type.lowercased() == typeFilter.rawValue
        return matchesQuery && matchesType
    }
}

/// Segmented type filter control shared by `ExerciseListView` and
/// `ExercisePickerSheet` so both surfaces filter identically.
struct ExerciseTypeFilterView: View {
    @Binding var selection: ExerciseTypeFilter

    var body: some View {
        Picker("Type", selection: $selection) {
            ForEach(ExerciseTypeFilter.allCases) { filter in
                Text(filter.displayName).tag(filter)
            }
        }
        .pickerStyle(.segmented)
        .accessibilityLabel("Filter by exercise type")
    }
}

/// Searchable exercise picker sheet used by `NewSetView`. Mirrors the
/// web exercise list: a search field plus an All/Strength/Cardio/Other
/// type filter over rows showing the name and type chip. Tapping a row
/// selects the exercise and dismisses; the caller owns the selection
/// binding so cancel leaves the form unchanged.
struct ExercisePickerSheet: View {
    let exercises: [ExerciseDTO]
    @Binding var selectedExerciseID: String?
    @Environment(\.dismiss) private var dismiss

    @State private var search: String = ""
    @State private var typeFilter: ExerciseTypeFilter = .all

    private var filtered: [ExerciseDTO] {
        filterExercises(exercises, search: search, typeFilter: typeFilter)
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                ExerciseTypeFilterView(selection: $typeFilter)
                    .padding(.horizontal)
                    .padding(.top, DSSpacing.sm)
                    .padding(.bottom, DSSpacing.xs)
                content
            }
            .navigationTitle("Choose Exercise")
            .navigationBarTitleDisplayMode(.inline)
            .searchable(text: $search, prompt: "Search exercises")
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
    }

    @ViewBuilder
    private var content: some View {
        if filtered.isEmpty {
            VStack(spacing: DSSpacing.md) {
                Spacer()
                Image(systemName: Icons.exercises)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.textSecondary)
                Text("No exercises match your search.")
                    .foregroundStyle(DSColors.textSecondary)
                    .multilineTextAlignment(.center)
                Spacer()
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List {
                ForEach(filtered) { exercise in
                    Button {
                        selectedExerciseID = exercise.id
                        dismiss()
                    } label: {
                        HStack {
                            ExerciseRow(exercise: exercise)
                            Spacer()
                            if exercise.id == selectedExerciseID {
                                Image(systemName: "checkmark")
                                    .foregroundStyle(DSColors.accent)
                                    .accessibilityLabel("Selected")
                            }
                        }
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .listRowSeparator(.hidden)
                }
            }
            .listStyle(.insetGrouped)
            .listRowSeparator(.hidden)
        }
    }
}

#Preview {
    ExercisePickerSheet(
        exercises: [
            ExerciseDTO(
                id: "1", name: "Bench Press", description: "", videoURL: "",
                imgURL: "", imageURL: "", type: "strength"
            ),
            ExerciseDTO(
                id: "2", name: "Running", description: "", videoURL: "",
                imgURL: "", imageURL: "", type: "cardio"
            ),
            ExerciseDTO(
                id: "3", name: "Yoga Flow", description: "", videoURL: "",
                imgURL: "", imageURL: "", type: "other"
            ),
        ],
        selectedExerciseID: .constant("1")
    )
}
