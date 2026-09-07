import SwiftUI

/// One stat card in a 2-column "Stats" grid. Layout: a small
/// accent-tinted icon disk on the leading edge, a label
/// above, and the value rendered in a large bold monospaced
/// digit beneath. Mirrors the pattern `ExerciseHistoryView`
/// uses for its Personal-Best / Last-Set cards; extracted to
/// `DesignSystem` so the Weight tab (and any future stats-
/// heavy surface) can reuse the same shape.
///
/// The card uses the standard rounded surface + separator
/// stroke style, identical to the dashboard's donut card
/// and the row cards in the Weight / Goals lists, so the
/// page reads as one consistent system. Icons are always
/// rendered in the brand accent (and sit on a soft
/// accent-tinted disk) so the card row has a consistent
/// colour rhythm; text uses `DSColors.text` so the value
/// reads cleanly on the surface background.
public struct StatCard: View {
    public let label: String
    public let value: String
    public let icon: String

    public init(label: String, value: String, icon: String) {
        self.label = label
        self.value = value
        self.icon = icon
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack(spacing: DSSpacing.xs) {
                Image(systemName: icon)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(DSColors.accent)
                    .frame(width: 28, height: 28)
                    .background(
                        Circle()
                            .fill(DSColors.accent.opacity(0.12))
                    )
                Text(label)
                    .font(.caption)
                    .foregroundStyle(DSColors.text)
                    .textCase(nil)
            }
            Text(value)
                .font(.title3.weight(.bold).monospacedDigit())
                .foregroundStyle(DSColors.text)
                .lineLimit(1)
                .minimumScaleFactor(0.8)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(.horizontal, DSSpacing.sm)
        .padding(.vertical, DSSpacing.xs)
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

/// 2-column grid for the StatCard pattern. Behaviourally
/// identical to SwiftUI's `LazyVGrid` but kept as a named
/// type so callers don't have to repeat the column layout
/// at every call site, and so the design system's "stats
/// grid" idiom is searchable. Uses `LazyVGrid` internally
/// so the page stays smooth even with more cards.
public struct StatsGrid<Content: View>: View {
    let columns: [GridItem]
    let spacing: CGFloat
    @ViewBuilder let content: () -> Content

    public init(
        columns: [GridItem] = [
            GridItem(.flexible(), spacing: DSSpacing.sm),
            GridItem(.flexible(), spacing: DSSpacing.sm),
        ],
        spacing: CGFloat = DSSpacing.sm,
        @ViewBuilder content: @escaping () -> Content
    ) {
        self.columns = columns
        self.spacing = spacing
        self.content = content
    }

    public var body: some View {
        LazyVGrid(columns: columns, alignment: .leading, spacing: spacing) {
            content()
        }
    }
}

#Preview {
    VStack(spacing: DSSpacing.md) {
        StatsGrid {
            StatCard(label: "Current", value: "68.3 kg", icon: "figure.stand")
            StatCard(label: "Target", value: "69.0 kg", icon: "target")
        }
        StatsGrid {
            StatCard(label: "Progress", value: "99%", icon: "chart.line.uptrend.xyaxis")
            StatCard(label: "To go", value: "0.7 kg", icon: "arrow.right")
        }
    }
    .padding()
    .background(DSColors.background)
}
