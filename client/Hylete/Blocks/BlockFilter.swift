import SwiftUI

/// Shared block-type filter used by the Blocks list. `all` disables
/// type filtering; the other cases match `BlockSummaryDTO.type`
/// by raw value.
enum BlockTypeFilter: String, CaseIterable, Identifiable {
    case all
    case standard
    case circuit
    case amrap
    case emom

    var id: String { rawValue }

    /// User-facing label. Mirrors `BlockTypeDTO.displayName`
    /// with an "All" option to disable filtering.
    var displayName: String {
        switch self {
        case .all: return "All"
        case .standard: return "Standard"
        case .circuit: return "Circuit"
        case .amrap: return "AMRAP"
        case .emom: return "EMOM"
        }
    }
}

/// Pure block list filter — name search (case-insensitive, trimmed)
/// combined with the type filter. Extracted from the views so it can
/// be unit-tested without hosting SwiftUI.
func filterBlocks(
    _ blocks: [BlockSummaryDTO],
    search: String,
    typeFilter: BlockTypeFilter
) -> [BlockSummaryDTO] {
    let query = search.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
    return blocks.filter { block in
        let matchesQuery = query.isEmpty || block.name.lowercased().contains(query)
        let matchesType = typeFilter == .all || block.type.rawValue == typeFilter.rawValue
        return matchesQuery && matchesType
    }
}

/// Horizontally scrolling single-select chip filter for
/// `BlocksListView`. A `ScrollView` + `HStack` instead of a segmented
/// `Picker` so all five options (All + four types) stay legible on
/// narrow screens — segments squeeze to fit and truncate, chips
/// scroll. Styling mirrors the weekday picker chips in
/// `WorkoutDuplicateSheet`.
struct BlockTypeFilterView: View {
    @Binding var selection: BlockTypeFilter

    var body: some View {
        ScrollView(.horizontal, showsIndicators: false) {
            HStack(spacing: DSSpacing.xs) {
                ForEach(BlockTypeFilter.allCases) { filter in
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
                    .accessibilityLabel("Filter by block type: \(filter.displayName)")
                    .accessibilityAddTraits(selection == filter ? .isSelected : [])
                }
            }
            .padding(.horizontal)
        }
        .accessibilityLabel("Filter by block type")
    }
}
