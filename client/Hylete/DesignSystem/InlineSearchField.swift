import SwiftUI

/// Inline search field that lives in the view hierarchy (as opposed
/// to `.searchable`, which renders in the navigation bar and therefore
/// always sits above content). Used by the Blocks and Workouts lists
/// so the order is description → search → filter chips → rows.
///
/// Styling mirrors `DSTextFieldStyle` (elevated-surface rounded field
/// with a hairline separator) with a magnifying-glass affordance and
/// a clear button, so it reads as a search field in both modes.
struct InlineSearchField: View {
    @Binding var text: String
    let prompt: String

    var body: some View {
        HStack(spacing: DSSpacing.xs) {
            Image(systemName: "magnifyingglass")
                .foregroundStyle(DSColors.textSecondary)
                .accessibilityHidden(true)
            TextField(prompt, text: $text)
                .textFieldStyle(.plain)
                .submitLabel(.search)
                .autocorrectionDisabled()
                .textInputAutocapitalization(.never)
                .accessibilityLabel(prompt)
            if !text.isEmpty {
                Button {
                    text = ""
                } label: {
                    Image(systemName: "xmark.circle.fill")
                        .foregroundStyle(DSColors.textSecondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Clear search")
            }
        }
        .padding(.horizontal, DSSpacing.sm)
        .padding(.vertical, DSSpacing.xs)
        .background(
            // Fully rounded pill to match the filter chips
            // (`Capsule` in `BlockTypeFilterView` /
            // `WorkoutStatusFilterView`).
            Capsule(style: .continuous)
                .fill(DSColors.surfaceElevated)
        )
        .overlay(
            Capsule(style: .continuous)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
    }
}
