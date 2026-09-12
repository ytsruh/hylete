import SwiftUI

/// The lists shown under the calendar strip for the
/// currently selected day: planned workouts first, then the
/// logged sets. Read-only navigation surface: set rows tap
/// through to the exercise's history view, planned rows to
/// the workout's detail view. Editing and deleting stay in
/// the per-exercise history view and the workout detail —
/// this list intentionally has no swipe actions so browsing
/// around the calendar can't trigger destructive changes by
/// accident.
///
/// Mirrors the card styling of the old `DashboardSetList` and
/// reuses `SetRow` for the set rows.
struct SelectedDaySetList: View {
    /// The day being displayed (start-of-day instant from the
    /// calendar). Used only for the empty-state copy.
    let date: Date
    let entries: [ExerciseEntryDTO]
    /// Planned workouts for the day (from the schedule
    /// endpoint), rendered above the logged sets.
    let planned: [WorkoutAssignmentDTO]
    /// `true` while the containing week's entries are being
    /// fetched; shows an inline spinner instead of the
    /// empty-state so freshly-paged weeks don't flash "no sets".
    let isLoading: Bool
    let weightUnit: String
    let distanceUnit: String
    /// Map from `exerciseID` to the full `ExerciseDTO` needed to
    /// push the history view (resolved once at dashboard load).
    let exerciseLookup: [String: ExerciseDTO]
    /// The selected planned workout to push. Owned by the
    /// dashboard, which pushes the shared detail view for the
    /// id via `navigationDestination(item:)`. Plain state so
    /// opening and closing the detail batches atomically
    /// with everything else (no `dismiss()` race).
    @Binding var openWorkoutID: String?

    init(
        date: Date,
        entries: [ExerciseEntryDTO],
        planned: [WorkoutAssignmentDTO],
        isLoading: Bool,
        weightUnit: String,
        distanceUnit: String,
        exerciseLookup: [String: ExerciseDTO],
        openWorkoutID: Binding<String?>
    ) {
        self.date = date
        self.entries = entries
        self.planned = planned
        self.isLoading = isLoading
        self.weightUnit = weightUnit
        self.distanceUnit = distanceUnit
        self.exerciseLookup = exerciseLookup
        self._openWorkoutID = openWorkoutID
    }

    var body: some View {
        if isLoading && entries.isEmpty && planned.isEmpty {
            HStack {
                Spacer()
                ProgressView()
                Spacer()
            }
            .padding(.vertical, DSSpacing.lg)
        } else if entries.isEmpty && planned.isEmpty {
            VStack(spacing: DSSpacing.xxs) {
                Text(emptyTitle)
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                Text("Tap an exercise to view its full history.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, DSSpacing.lg)
        } else {
            VStack(spacing: DSSpacing.xs) {
                if !planned.isEmpty {
                    plannedCard
                }
                if !entries.isEmpty {
                    setsCard
                }
            }
        }
    }

    /// Planned-workout card: one row per planned workout.
    /// Tapping a row sets the dashboard's pushed id (which
    /// pushes the shared detail view); the whole row is the
    /// hit area.
    private var plannedCard: some View {
        VStack(spacing: 0) {
            ForEach(Array(planned.enumerated()), id: \.element.id) { index, assignment in
                Button {
                    openWorkoutID = assignment.workoutID
                } label: {
                    HStack(spacing: DSSpacing.xs) {
                        Image(systemName: "calendar.badge.clock")
                            .foregroundStyle(DSColors.accent)
                        Text(assignment.workoutTitle)
                            .font(.body)
                            .foregroundStyle(DSColors.text)
                        Spacer()
                        Image(systemName: "chevron.right")
                            .font(.footnote)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    // Matches `SetRow`'s vertical rhythm so
                    // planned and logged rows feel the same.
                    .padding(.vertical, DSSpacing.sm)
                    // Stretch the hit area to the full row so a
                    // tap anywhere (not just the chevron)
                    // pushes the workout detail.
                    .contentShape(Rectangle())
                }
                .buttonStyle(.plain)
                if index < planned.count - 1 {
                    Divider()
                        .background(DSColors.separator)
                        .padding(.leading, DSSpacing.md)
                }
            }
        }
        .padding(.horizontal, DSSpacing.md)
        .background(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .fill(DSColors.surface)
        )
        .overlay(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
    }

    private var setsCard: some View {
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

    /// "No sets yet" on today, otherwise the friendly date.
    private var emptyTitle: String {
        if CalendarMath.isSameDay(date, Date()) {
            return "No activity logged today"
        }
        return "No activity on \(date.formatted(date: .abbreviated, time: .omitted))"
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
        planned: [
            WorkoutAssignmentDTO(
                id: "a1", workoutID: "w1",
                workoutTitle: "Push Day", scheduledDate: "2026-09-14"
            ),
        ],
        isLoading: false,
        weightUnit: "kg",
        distanceUnit: "km",
        exerciseLookup: [:],
        openWorkoutID: .constant(nil)
    )
    .padding(DSSpacing.md)
}
