import Charts
import SwiftUI

/// Renders the dashboard's "Most Popular Exercises (7d)"
/// donut using Swift Charts' `SectorMark`. Mirrors the
/// web's Chart.js donut (cutout 60%, brand palette, "Other"
/// bucket, percentage tooltip) without the Chart.js
/// dependency. The data is passed in pre-bucketed so the
/// chart stays a pure presentation concern and
/// `popularExerciseBuckets(_:)` can be tested in isolation.
///
/// Tapping a slice selects it and shows a small floating
/// card just outside the slice's center, matching the
/// tooltip style used by `ExerciseHistoryView`'s line
/// chart. Tapping anywhere else on the chart clears the
/// selection (handled automatically by
/// `chartAngleSelection`).
///
/// Single-slice donuts are still valid (a solid disc) —
/// only the empty case is skipped, matching the web
/// component.
struct DashboardDonutChart: View {
    let buckets: [PopularExerciseBucket]

    @State private var selectedAngle: Double?

    var body: some View {
        if buckets.isEmpty {
            EmptyView()
        } else {
            Chart(buckets) { bucket in
                SectorMark(
                    angle: .value("Sets", bucket.count),
                    innerRadius: .ratio(0.6),
                    angularInset: 1
                )
                .cornerRadius(2)
                .foregroundStyle(bucket.color)
            }
            .frame(height: 180)
            .chartLegend(shouldShowLegend ? .visible : .hidden)
            .chartAngleSelection(value: $selectedAngle)
            .onChange(of: buckets) { _, _ in
                // A dashboard reappear or pull-to-refresh replaces
                // the bucket list. An angle the user previously
                // selected may no longer map to any slice in the
                // new data; keeping it attached across a data
                // change is a known iOS 17 SwiftUI Charts crash.
                // Clearing it simply closes the tooltip.
                selectedAngle = nil
            }
            .chartOverlay { proxy in
                if let bucket = selectedBucket {
                    GeometryReader { geometry in
                        if let plotFrame = proxy.plotFrame {
                            // Plot area is the rectangular region the
                            // chart actually draws into. For a sector
                            // chart with no axes it's the donut's
                            // bounding box; for the line chart the
                            // proxy returns the inset rectangle the
                            // points live in. Same API, same units.
                            let plotRect = geometry[plotFrame]
                            let center = CGPoint(x: plotRect.midX, y: plotRect.midY)
                            // The visible ring sits between the
                            // inner cutout (0.6) and the outer edge
                            // (1.0); 0.8 is the radial midpoint of
                            // that ring, which is the natural place
                            // to anchor a tooltip.
                            let radius = min(plotRect.width, plotRect.height) / 2 * 0.8
                            let cumulativeBefore = Self.cumulativeCount(of: bucket, in: buckets)
                            let sliceCenterValue = Double(cumulativeBefore) + Double(bucket.count) / 2
                            let total = Double(buckets.reduce(0) { $0 + $1.count })
                            // Swift Charts sectors start at 12
                            // o'clock (top) and increase clockwise,
                            // matching standard math convention with
                            // the y-axis flipped for screen space.
                            let angle = sliceCenterValue / total * 2 * .pi
                            let anchorX = center.x + radius * sin(angle)
                            let anchorY = center.y - radius * cos(angle)

                            // Clamp the card so it never runs off
                            // the chart's edges. Same approach as
                            // the line chart's tooltip.
                            let cardHalfWidth: CGFloat = 60
                            let cardHalfHeight: CGFloat = 22
                            let minX = cardHalfWidth + 4
                            let maxX = geometry.size.width - cardHalfWidth - 4
                            let minY = cardHalfHeight + 4
                            let maxY = geometry.size.height - cardHalfHeight - 4
                            let clampedX = min(max(anchorX, minX), maxX)
                            let clampedY = min(max(anchorY, minY), maxY)

                            TooltipCard(bucket: bucket, total: total)
                                .position(x: clampedX, y: clampedY)
                        }
                    }
                }
            }
        }
    }

    /// A single-slice legend is silly. Matches the web's
    /// `showLegend = !props.HideLegend && len(props.Labels) >= 2`.
    private var shouldShowLegend: Bool {
        buckets.count >= 2
    }

    /// Resolves `selectedAngle` (a value-axis position from
    /// `chartAngleSelection`) back to the bucket whose slice
    /// contains that value. The selection is inclusive on the
    /// start, exclusive on the end, so a touch on a slice's
    /// trailing edge falls into the *next* slice. A touch on
    /// the chart's outermost edge (value == total) returns the
    /// last bucket so the user can never be left without a
    /// tooltip mid-touch.
    private var selectedBucket: PopularExerciseBucket? {
        guard let selectedAngle else { return nil }
        var cumulative: Double = 0
        for bucket in buckets {
            let next = cumulative + Double(bucket.count)
            if selectedAngle >= cumulative && selectedAngle < next {
                return bucket
            }
            cumulative = next
        }
        if let last = buckets.last, selectedAngle == cumulative {
            return last
        }
        return nil
    }

    /// Sum of every bucket's count that appears before `bucket`
    /// in the array. Used to translate a bucket's "value
    /// position" into a sector angle. Iterates by id so two
    /// buckets with the same name (impossible today, but cheap
    /// to guard) are still treated distinctly.
    private static func cumulativeCount(
        of bucket: PopularExerciseBucket,
        in buckets: [PopularExerciseBucket]
    ) -> Int {
        var total = 0
        for b in buckets {
            if b.id == bucket.id { return total }
            total += b.count
        }
        return total
    }
}

/// Small floating card shown on top of the donut when a
/// slice is selected. Styling intentionally matches the
/// `chartTooltip` card on `ExerciseHistoryView`: rounded
/// surface fill, hairline border, and a soft drop shadow
/// so it feels like a callout, not a label.
private struct TooltipCard: View {
    let bucket: PopularExerciseBucket
    let total: Double

    var body: some View {
        let percent = total > 0
            ? Int((Double(bucket.count) / total * 100).rounded())
            : 0
        VStack(alignment: .leading, spacing: 2) {
            Text(bucket.name)
                .font(.caption.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text("\(bucket.count) (\(percent)%)")
                .font(.caption.monospacedDigit())
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(.horizontal, 8)
        .padding(.vertical, 6)
        .background(
            RoundedRectangle(cornerRadius: 8)
                .fill(DSColors.surface)
                .shadow(color: .black.opacity(0.15), radius: 4, y: 1)
        )
        .overlay(
            RoundedRectangle(cornerRadius: 8)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
        .fixedSize()
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(bucket.name), \(bucket.count) sets, \(percent) percent")
    }
}

#Preview {
    let sample: [PopularExerciseBucket] = [
        .init(name: "Squat", count: 12, color: Color(red: 0.961, green: 0.286, blue: 0.0), isOther: false),
        .init(name: "Bench Press", count: 8, color: Color(red: 0.965, green: 0.357, blue: 0.102), isOther: false),
        .init(name: "Deadlift", count: 6, color: Color(red: 0.969, green: 0.427, blue: 0.200), isOther: false),
        .init(name: "Other", count: 4, color: .gray, isOther: true),
    ]
    return DashboardDonutChart(buckets: sample)
        .padding()
        .background(DSColors.background)
}
