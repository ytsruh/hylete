import SwiftUI

/// Single row for the weight list. Layout (left → right):
///
///   [ weight + photo badge ]   [ date ]
///
/// The row is deliberately photo-less: thumbnails were removed when
/// weight entries grew front/side/back slots (a single thumb could
/// no longer represent the entry, and loading images in the scroll
/// path cost bandwidth). Entries with photos show a small count
/// badge instead; photos live in the editor and the comparison
/// picker. Matches the `ExerciseRow` pattern in
/// `ExerciseListView.swift`.
///
/// Notes are deliberately not shown here — they only appear
/// when the user taps into the editor (`onTap`).
struct WeightRow: View {
    let entry: WeightEntryDTO
    let weightUnit: String

    /// Tapping the row opens the edit sheet.
    let onTap: () -> Void

    init(
        entry: WeightEntryDTO,
        weightUnit: String,
        onTap: @escaping () -> Void
    ) {
        self.entry = entry
        self.weightUnit = weightUnit
        self.onTap = onTap
    }

    /// UK-formatted date (DD/MM/YY) — matches the web's
    /// `FormattedDate` so the iOS view reads identically to
    /// the web table.
    private var formattedDate: String {
        entry.createdAt.formatted(.dateTime
            .day(.twoDigits)
            .month(.twoDigits)
            .year(.twoDigits)
        )
    }

    var body: some View {
        HStack(spacing: DSSpacing.md) {
            HStack(spacing: DSSpacing.xs) {
                Text(entry.formattedWeight(in: weightUnit))
                    .font(.subheadline.weight(.semibold).monospacedDigit())
                    .foregroundStyle(DSColors.text)
                if entry.photoCount > 0 {
                    photoBadge
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)

            Text(formattedDate)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.text)
        }
        .padding(.vertical, DSSpacing.md)
        .contentShape(Rectangle())
        .onTapGesture {
            onTap()
        }
        .accessibilityLabel("\(entry.formattedWeight(in: weightUnit)), \(formattedDate)")
    }

    /// Small badge showing how many of the front/side/back slots
    /// hold a photo. Keeps photo presence visible in the list
    /// without loading any image bytes.
    private var photoBadge: some View {
        HStack(spacing: 2) {
            Image(systemName: "photo")
                .font(.system(size: 10))
            Text("\(entry.photoCount)")
                .font(.caption2.monospacedDigit())
        }
        .foregroundStyle(DSColors.textSecondary)
        .accessibilityLabel("\(entry.photoCount) photos")
    }
}
