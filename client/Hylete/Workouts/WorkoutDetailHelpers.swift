import Foundation

/// Pure helpers over `WorkoutDetailDTO` for the detail view's
/// progress lines. Extracted (like `filterExercises`) so they can
/// be unit-tested without hosting SwiftUI.

extension WorkoutDetailDTO {
    /// Entries logged against one planned item, in logging order.
    func entries(for workoutItemID: String) -> [ExerciseEntryDTO] {
        loggedEntries.filter { $0.workoutItemID == workoutItemID }
    }

    /// How many of the block's items have at least one logged
    /// entry. Drives the "2/3 exercises logged" line.
    func loggedItemCount(in block: WorkoutBlockDTO) -> Int {
        block.items.filter { !entries(for: $0.id).isEmpty }.count
    }

    /// The round a fresh log against the item should default
    /// to: the first round with no entries yet, or the last
    /// round when every round has entries (extra work still
    /// counts). Round 0 (unrounded) entries don't advance the
    /// suggestion — they were logged outside the round scheme.
    func suggestedRound(for workoutItemID: String, rounds: Int) -> Int {
        let logged = Set(entries(for: workoutItemID).map { $0.roundNumber })
        return suggestedWorkoutRound(loggedRounds: logged, rounds: rounds)
    }
}

/// First round in 1...rounds with no logged entry, or `rounds`
/// when all are covered. `rounds <= 1` always yields 1 (the
/// caller hides the picker for straight blocks and sends 0).
/// Pure so the default can be unit-tested.
func suggestedWorkoutRound(loggedRounds: Set<Int>, rounds: Int) -> Int {
    guard rounds > 1 else { return 1 }
    return (1...rounds).first(where: { !loggedRounds.contains($0) }) ?? rounds
}

/// One-line prescription summary for an item: "3 × 8 @ 60.0 kg"
/// for strength, "25:00 · 5.20 km" for cardio, "Open target" when
/// no numbers were prescribed. Mirrors the server's
/// `ExerciseEntry.Summary` shape so targets read like the logged
/// sets they prescribe.
func workoutItemTargetSummary(
    _ item: WorkoutItemDTO,
    weightUnit: String,
    distanceUnit: String
) -> String {
    if item.isPrescribed {
        if item.isCardio {
            return "\(formatWorkoutDuration(item.targetDurationSeconds)) · \(formatWorkoutDistance(item.targetDistanceMeters, unit: distanceUnit))"
        }
        return "\(item.targetSets) × \(item.targetReps) @ \(String(format: "%.1f %@", item.targetWeight, weightUnit))"
    }
    return "Open target"
}

/// M:SS, switching to H:MM:SS past an hour ("25:00", "1:05:30").
/// Same shape as `SetRow.formattedDuration`.
func formatWorkoutDuration(_ seconds: Int) -> String {
    let s = max(0, seconds)
    let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
    if h > 0 {
        return String(format: "%d:%02d:%02d", h, m, sec)
    }
    return String(format: "%02d:%02d", m, sec)
}

/// Distance converted from stored metres to the user's unit.
func formatWorkoutDistance(_ meters: Double, unit: String) -> String {
    let km = meters / 1000.0
    if unit == "mi" {
        return String(format: "%.2f mi", km / 1.609344)
    }
    return String(format: "%.2f km", km)
}

/// One-line schedule summary for list rows: "7 Sep, 18:00–19:00",
/// "Starts 7 Sep, 18:00" (end missing), or "" when unscheduled.
/// Day and time render in the device locale; the range dash keeps
/// the row to a single line.
func workoutScheduleSummary(start: Date?, end: Date?) -> String {
    guard let start else { return "" }
    let day = DateFormatter.cachedDay.string(from: start)
    let time = DateFormatter.cachedTime.string(from: start)
    guard let end else {
        return "\(day), \(time)"
    }
    return "\(day), \(time)–\(DateFormatter.cachedTime.string(from: end))"
}

/// "18:00–19:00" clock range for dashboard banners (the
/// calendar already says which day it is). Start-only renders
/// just the start time; nil start renders "".
func workoutClockRange(start: Date?, end: Date?) -> String {
    guard let start else { return "" }
    let time = DateFormatter.cachedWorkoutTime.string(from: start)
    guard let end else { return time }
    return "\(time)–\(DateFormatter.cachedWorkoutTime.string(from: end))"
}

private extension DateFormatter {
    /// "7 Sep" style day label shared by workout rows.
    static let cachedDay: DateFormatter = {
        let f = DateFormatter()
        f.dateStyle = .medium
        f.timeStyle = .none
        return f
    }()

    /// "18:00" style time label shared by workout rows.
    static let cachedTime: DateFormatter = {
        let f = DateFormatter()
        f.dateStyle = .none
        f.timeStyle = .short
        return f
    }()

    /// Clock-only label for banner ranges. Separate instance so
    /// the shared row formatter is never reconfigured.
    static let cachedWorkoutTime: DateFormatter = {
        let f = DateFormatter()
        f.dateStyle = .none
        f.timeStyle = .short
        return f
    }()
}
