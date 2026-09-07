import SwiftUI

/// One row summarising a single exercise entry. Used by the
/// dashboard's per-day calendar list (and available anywhere a
/// compact set/session summary is needed). Shows the exercise
/// name, notes, and a one-line metric summary — reps × weight
/// for strength entries, duration · distance for cardio — with
/// time of day and date intentionally omitted to keep rows
/// scannable inside day-scoped lists.
struct SetRow: View {
    let entry: ExerciseEntryDTO
    let weightUnit: String
    let distanceUnit: String

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            VStack(alignment: .leading, spacing: DSSpacing.xxs) {
                Text(entry.exerciseName)
                    .font(.body.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                if !entry.notes.isEmpty {
                    Text(entry.notes)
                        .font(.footnote)
                        .foregroundStyle(DSColors.textSecondary)
                        .lineLimit(2)
                }
            }
            Spacer()
            Text(summaryText)
                .font(.body.weight(.semibold).monospacedDigit())
                .foregroundStyle(DSColors.text)
        }
        .padding(.vertical, DSSpacing.sm)
        .padding(.horizontal, DSSpacing.md)
        .contentShape(Rectangle())
    }

    /// Type-aware one-line summary: "5 × 100.0 kg" for strength,
    /// "25:00 · 5.20 km" for cardio (mirrors the web's Details column).
    private var summaryText: String {
        if entry.isCardio {
            return "\(Self.formattedDuration(entry.durationSeconds)) · \(formattedDistance)"
        }
        return "\(entry.reps) × \(formattedWeight)"
    }

    private var formattedWeight: String {
        String(format: "%.1f %@", entry.weight, weightUnit)
    }

    /// Distance converted from stored metres to the user's unit.
    private var formattedDistance: String {
        let km = entry.distanceMeters / 1000.0
        if distanceUnit == "mi" {
            return String(format: "%.2f mi", km / 1.609344)
        }
        return String(format: "%.2f km", km)
    }

    /// M:SS, switching to H:MM:SS past an hour ("25:00", "1:05:30").
    static func formattedDuration(_ seconds: Int) -> String {
        let s = max(0, seconds)
        let h = s / 3600, m = (s % 3600) / 60, sec = s % 60
        if h > 0 {
            return String(format: "%d:%02d:%02d", h, m, sec)
        }
        return String(format: "%02d:%02d", m, sec)
    }
}
