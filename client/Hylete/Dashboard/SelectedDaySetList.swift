import SwiftUI

/// The list of sets shown under the calendar strip for the
/// currently selected day. Read-only navigation surface: rows
/// tap through to the exercise's history view. Editing and
/// deleting stay in the per-exercise history view — this list
/// intentionally has no swipe actions so browsing around the
/// calendar can't trigger destructive changes by accident.
///
/// Mirrors the card styling of the old `DashboardSetList` and
/// reuses `SetRow` for the row itself.
struct SelectedDaySetList: View {
    /// The day being displayed (start-of-day instant from the
    /// calendar). Used only for the empty-state copy.
    let date: Date
    let entries: [ExerciseEntryDTO]
    /// `true` while the containing week's entries are being
    /// fetched; shows an inline spinner instead of the
    /// empty-state so freshly-paged weeks don't flash "no sets".
    let isLoading: Bool
    let weightUnit: String
    let distanceUnit: String
    /// Map from `exerciseID` to the full `ExerciseDTO` needed to
    /// push the history view (resolved once at dashboard load).
    let exerciseLookup: [String: ExerciseDTO]

    var body: some View {
        if isLoading && entries.isEmpty {
            HStack {
                Spacer()
                ProgressView()
                Spacer()
            }
            .padding(.vertical, DSSpacing.lg)
        } else if entries.isEmpty {
            VStack(spacing: DSSpacing.xxs) {
                Text(emptyTitle)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                Text("Tap a set's exercise to view its full history.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, DSSpacing.lg)
        } else {
            VStack(spacing: 0) {
                ForEach(Array(entries.enumerated()), id: \.element.id) { index, entry in
                    rowLink(for: entry)
                    if index < entries.count - 1 {
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

    /// "No sets yet" on today, otherwise the friendly date.
    private var emptyTitle: String {
        if CalendarMath.isSameDay(date, Date()) {
            return "No sets logged today"
        }
        return "No sets on \(date.formatted(date: .abbreviated, time: .omitted))"
    }

    @ViewBuilder
    private func rowLink(for entry: ExerciseEntryDTO) -> some View {
        if let exercise = exerciseLookup[entry.exerciseID] {
            NavigationLink(value: exercise) {
                SetRow(entry: entry, weightUnit: weightUnit, distanceUnit: distanceUnit)
            }
            .buttonStyle(.plain)
        } else {
            // The exercise was deleted server-side after the
            // set was logged. Show the row read-only so the
            // user can still see the set without crashing on
            // a missing destination.
            SetRow(entry: entry, weightUnit: weightUnit, distanceUnit: distanceUnit)
                .opacity(0.6)
        }
    }
}

#Preview("With entries") {
    SelectedDaySetList(
        date: Date(),
        entries: [
            ExerciseEntryDTO(
                id: "1", exerciseID: "ex-1", exerciseName: "Bench Press",
                reps: 5, weight: 100, notes: "", restTime: 120,
                createdAt: Date()
            ),
        ],
        isLoading: false,
        weightUnit: "kg",
        distanceUnit: "km",
        exerciseLookup: [:]
    )
    .padding(DSSpacing.md)
}
