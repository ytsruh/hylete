import SwiftUI

/// Single row for the weight list. Layout (left → right):
///
///   [ thumbnail ]   [ weight ]   [ date ]
///
/// The thumbnail is filled with the entry's photo when
/// one exists; otherwise a muted placeholder tile with the
/// "figure.stand" icon is shown in the same slot so every
/// row has the same visual rhythm — matches the
/// `ExerciseRow` pattern in `ExerciseListView.swift`.
///
/// Notes are deliberately not shown here — they only appear
/// when the user taps into the editor (`onTap`).
struct WeightRow: View {
    let entry: WeightEntryDTO
    let weightUnit: String

    /// Tapping the row opens the edit sheet.
    let onTap: () -> Void

    /// Optional comparison selection action. It is only supplied
    /// for the weight history screen, leaving the row reusable in
    /// contexts that do not offer photo comparison.
    let isSelected: Bool
    let onToggleSelection: (() -> Void)?

    init(
        entry: WeightEntryDTO,
        weightUnit: String,
        onTap: @escaping () -> Void,
        isSelected: Bool = false,
        onToggleSelection: (() -> Void)? = nil
    ) {
        self.entry = entry
        self.weightUnit = weightUnit
        self.onTap = onTap
        self.isSelected = isSelected
        self.onToggleSelection = onToggleSelection
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
            if entry.hasPhoto, let onToggleSelection {
                selectableThumbnail(onToggleSelection: onToggleSelection)
            } else {
                thumbnail
            }

            Text(entry.formattedWeight(in: weightUnit))
                .font(.subheadline.weight(.semibold).monospacedDigit())
                .foregroundStyle(DSColors.text)
                .frame(maxWidth: .infinity, alignment: .leading)

            Text(formattedDate)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.text)
        }
        .padding(.vertical, DSSpacing.xxs)
        .contentShape(Rectangle())
        .onTapGesture {
            onTap()
        }
    }

    /// Photo thumbnails are the comparison selection controls.
    /// The selected outline and checkmark keep the state visible
    /// without adding a checkbox column.
    private func selectableThumbnail(onToggleSelection: @escaping () -> Void) -> some View {
        Button(action: onToggleSelection) {
            ZStack(alignment: .topTrailing) {
                thumbnail
                    .overlay(
                        RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
                            .stroke(isSelected ? BrandColors.brandOrange : .clear, lineWidth: 3)
                    )

                if isSelected {
                    Image(systemName: "checkmark")
                        .font(.caption2.weight(.bold))
                        .foregroundStyle(.white)
                        .frame(width: 20, height: 20)
                        .background(BrandColors.brandOrange, in: Circle())
                        .offset(x: 5, y: -5)
                }
            }
            .frame(width: 60, height: 40)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(isSelected ? "Deselect photo" : "Select photo for comparison")
        .accessibilityValue(isSelected ? "Selected" : "Not selected")
    }

    @ViewBuilder
    private var thumbnail: some View {
        if entry.hasPhoto, let url = URL(string: entry.photoURL) {
            AsyncImage(url: url) { phase in
                switch phase {
                case .success(let image):
                    image
                        .resizable()
                        .scaledToFill()
                case .failure, .empty:
                    placeholder
                @unknown default:
                    placeholder
                }
            }
            .frame(width: 60, height: 40)
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
        } else {
            placeholder
                .frame(width: 60, height: 40)
        }
    }

    /// Muted tile with the figure.stand icon shown when
    /// the entry has no photo. Keeps every row's leading
    /// edge at the same width so the weight / date column
    /// aligns across the list — matches the placeholder
    /// shape used by `ExerciseRow` for the catalogue list.
    private var placeholder: some View {
        RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
            .fill(DSColors.surfaceElevated)
            .overlay(
                Image(systemName: "figure.stand")
                    .font(.system(size: 16))
                    .foregroundStyle(DSColors.textSecondary)
            )
    }
}
