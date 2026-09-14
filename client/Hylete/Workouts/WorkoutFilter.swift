import SwiftUI

/// Shared workout-status filter used by the Workouts list. `all`
/// disables status filtering; the other cases match
/// `WorkoutSummaryDTO.status` by raw value.
enum WorkoutStatusFilter: String, CaseIterable, Identifiable {
    case all
    case planned
    case inProgress = "in_progress"
    case completed
    case skipped

    var id: String { rawValue }

    /// User-facing label. Mirrors `WorkoutStatusDTO.displayName`
    /// with an "All" option to disable filtering.
    var displayName: String {
        switch self {
        case .all: return "All"
        case .planned: return "Planned"
        case .inProgress: return "In Progress"
        case .completed: return "Completed"
        case .skipped: return "Skipped"
        }
    }
}

/// Pure workout list filter — name search (case-insensitive,
/// trimmed) combined with the status filter. Extracted from the
/// views so it can be unit-tested without hosting SwiftUI.
func filterWorkouts(
    _ workouts: [WorkoutSummaryDTO],
    search: String,
    statusFilter: WorkoutStatusFilter
) -> [WorkoutSummaryDTO] {
    let query = search.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    return workouts.filter { workout in
        let matchesQuery = query.isEmpty || workout.name.lowercased().contains(query)
        let matchesStatus = statusFilter == .all || workout.status.rawValue == statusFilter.rawValue
        return matchesQuery && matchesStatus
    }
}

/// Horizontally scrolling single-select chip filter for
/// `WorkoutsListView`. A `ScrollView` + `HStack` instead of a segmented
/// `Picker` so all five options (All + four statuses) stay legible on
/// narrow screens — segments squeeze to fit and truncate, chips
/// scroll. Styling mirrors the weekday picker chips in
/// `WorkoutDuplicateSheet`.
struct WorkoutStatusFilterView: View {
    @Binding var selection: WorkoutStatusFilter

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: DSSpacing.xs) {
                ForEach(WorkoutStatusFilter.allCases) { filter in
                    Button {
                        selection = filter
                    } label: {
                        Text(filter.displayName)
                            .font(.subheadline.weight(.semibold))
                            .padding(.horizontal, DSSpacing.md)
                            .padding(.vertical, DSSpacing.xs)
                            .background(
                                Capsule(style: .continuous)
                                    .fill(selection == filter ? DSColors.accent : DSColors.surfaceElevated)
                            )
                            .foregroundStyle(selection == filter ? DSColors.onPrimary : DSColors.text)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Filter by workout status: \(filter.displayName)")
                    .accessibilityAddTraits(selection == filter ? .isSelected : [])
                }
            }
            .padding(.horizontal)
        }
        .accessibilityLabel("Filter by workout status")
    }
}
