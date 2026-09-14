import SwiftUI

/// The workouts planned for the currently selected day, shown under
/// the calendar strip above the logged sets. Read-only navigation
/// surface: rows tap through to the workout's detail view.
///
/// Mirrors the card styling of `SelectedDaySetList` so the two day
/// sections read as one surface.
struct SelectedDayWorkoutList: View {
    /// The day being displayed (start-of-day instant from the
    /// calendar). Used only for the empty-state copy.
    let date: Date
    let workouts: [WorkoutSummaryDTO]
    /// `true` while the containing week's workouts are being
    /// fetched; shows an inline spinner instead of the empty-state
    /// so freshly-paged weeks don't flash "no workouts".
    let isLoading: Bool

    var body: some View {
        if isLoading && workouts.isEmpty {
            HStack {
                Spacer()
                ProgressView()
                Spacer()
            }
            .padding(.vertical, DSSpacing.lg)
        } else if workouts.isEmpty {
            VStack(spacing: DSSpacing.xxs) {
                Text(emptyTitle)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                Text("Plan training days in More → Workouts.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, DSSpacing.md)
        } else {
            VStack(alignment: .leading, spacing: DSSpacing.xs) {
                Text(workouts.count == 1 ? "1 workout planned" : "\(workouts.count) workouts planned")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                    .padding(.horizontal, DSSpacing.xs)
                VStack(spacing: 0) {
                    ForEach(Array(workouts.enumerated()), id: \.element.id) { index, workout in
                        NavigationLink(value: workout) {
                            workoutRow(for: workout)
                        }
                        .buttonStyle(.plain)
                        if index < workouts.count - 1 {
                            Divider()
                                .background(DSColors.separator)
                                .padding(.leading, DSSpacing.md)
                        }
                    }
                }
                .background(
                    RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                        .fill(DSColors.surface)
                )
                .overlay(
                    RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                        .stroke(DSColors.separator, lineWidth: 0.5)
                )
            }
        }
    }

    /// "No workout planned today" on today, otherwise the friendly date.
    private var emptyTitle: String {
        if CalendarMath.isSameDay(date, Date()) {
            return "No workout planned today"
        }
        return "No workout on \(date.formatted(date: .abbreviated, time: .omitted))"
    }

    private func workoutRow(for workout: WorkoutSummaryDTO) -> some View {
        HStack(spacing: DSSpacing.sm) {
            VStack(alignment: .leading, spacing: 2) {
                Text(workout.name)
                    .font(.body)
                    .foregroundStyle(DSColors.text)
                Text("\(workout.status.displayName) · \(workout.progressLabel)")
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
            }
            Spacer()
            Image(systemName: "chevron.right")
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(.horizontal, DSSpacing.md)
        .padding(.vertical, DSSpacing.sm)
        .contentShape(Rectangle())
    }
}

#Preview("With workouts") {
    NavigationStack {
        SelectedDayWorkoutList(
            date: Date(),
            workouts: [
                WorkoutSummaryDTO(
                    id: "1", name: "Monday Strength", description: "",
                    scheduledDate: "2026-09-14", status: .planned,
                    blockCount: 3, doneCount: 1,
                    createdAt: Date(), updatedAt: Date()
                ),
            ],
            isLoading: false
        )
    }
    .padding(DSSpacing.md)
}
