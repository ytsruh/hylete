import SwiftUI

/// Weight-tab stats block. A 2-column `StatsGrid` of
/// `StatCard`s with the iOS-native icon-disk treatment
/// used elsewhere in the app (Personal-Best / Last-Set
/// in `ExerciseHistoryView`). Mirrors the same 2×2 shape
/// the web app uses (Current / Target / Progress / Gap)
/// but renders each piece as its own tile so the page
/// reads as a system of cards rather than a web-style
/// hero block.
///
/// The card is hidden by the parent when the user has no
/// target weight set, so this view assumes `target` is
/// non-optional. The percent is clamped to `[0, 100]`
/// inside `progressPercent` so "overshooting" the goal
/// caps at 100% rather than rendering a confusing 105%.
struct WeightProgressCard: View {
    let current: Double
    let target: Double
    let percent: Double
    let weightUnit: String

    /// "2.4 kg to go" / "0.5 kg to lose" / "At target" —
    /// the gap line sits in its own tile so the iOS
    /// layout stays balanced. Direction-flips phrasing
    /// match the web's `weightGapLine` helper.
    private var gapLine: String {
        let diff = target - current
        if abs(diff) < 0.05 {
            return "At target"
        }
        let absDiff = abs(diff)
        let formatted = String(format: "%.1f %@", absDiff, weightUnit)
        return diff > 0 ? "\(formatted) to go" : "\(formatted) to lose"
    }

    /// Percent formatted as an integer. Decimals are
    /// dropped because the `StatCard` value style reads
    /// best as a clean whole number and the precision is
    /// misleading at this scale anyway.
    private var formattedPercent: String {
        "\(Int(percent.rounded()))%"
    }

    var body: some View {
        StatsGrid {
            StatCard(
                label: "Current",
                value: String(format: "%.1f %@", current, weightUnit),
                icon: "figure.stand"
            )
            StatCard(
                label: "Target",
                value: String(format: "%.1f %@", target, weightUnit),
                icon: "target"
            )
            StatCard(
                label: "Progress",
                value: formattedPercent,
                icon: "chart.line.uptrend.xyaxis"
            )
            StatCard(
                label: gapLine.contains("to") ? "Remaining" : "Status",
                value: gapLine,
                icon: gapLine == "At target" ? "checkmark.seal.fill" : "arrow.right"
            )
        }
    }
}
