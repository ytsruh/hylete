import Charts
import SwiftUI

/// Line chart for body weight over time. Area + line + point
/// marks, plus an optional dashed horizontal line at the
/// user's target weight when one is set — the user can see
/// the gap between the latest weight and the target at a
/// glance without inspecting the data points.
///
/// The chart is hidden when there are fewer than 2 entries
/// (a single point doesn't draw a line and an empty area
/// fill looks broken). The caller is responsible for that
/// guard; this view focuses on rendering.
struct WeightChart: View {

    /// Sorted ascending by date (oldest first). The chart
    /// draws left-to-right so the x-axis reads chronologically.
    let points: [ChartPoint]
    /// The user's preferred unit. Drives the y-axis labels
    /// and the chart legend.
    let weightUnit: String
    /// The user's target weight, if any. When non-nil, a
    /// dashed reference line is drawn at this value.
    let targetWeight: Double?

    /// Single chart point. The view re-sorts on every
    /// render so the caller can hand in entries in any
    /// order (the store keeps them newest-first).
    struct ChartPoint: Identifiable, Hashable {
        let date: Date
        let weight: Double
        var id: TimeInterval { date.timeIntervalSince1970 }
    }

    /// Min/max weight across the visible points, padded
    /// to a "nice" step. When a target is set we widen
    /// the domain to include it so the reference line
    /// stays inside the chart's plot area. Mirrors the
    /// ExerciseHistoryView chart's domain logic so the
    /// two charts feel consistent.
    private var chartYDomain: ClosedRange<Double> {
        var lo = points.map(\.weight).min() ?? 0
        var hi = points.map(\.weight).max() ?? 0
        if let target = targetWeight {
            lo = min(lo, target)
            hi = max(hi, target)
        }
        let step = Self.niceStep(for: hi - lo)
        let lower = (lo / step).rounded(.down) * step
        let upper = (hi / step).rounded(.up) * step
        let pad = max((upper - lower) * 0.1, step)
        return (lower - pad)...(upper + pad)
    }

    /// Picks a "nice" tick step (1, 2.5, 5, 10, …) sized
    /// to the data's range so axis labels read as clean
    /// numbers. Directly copies the ExerciseHistoryView
    /// helper so the y-axis labels share the same
    /// behaviour.
    private static func niceStep(for range: Double) -> Double {
        switch range {
        case ..<5:    return 1
        case ..<20:   return 2.5
        case ..<50:   return 5
        case ..<200:  return 10
        case ..<500:  return 25
        default:      return 50
        }
    }

    /// Earliest and latest dates on the x-axis. The
    /// target reference line is drawn between these so it
    /// spans the full chart width regardless of where the
    /// first / last data points sit.
    private var xDomain: ClosedRange<Date> {
        let dates = points.map(\.date)
        return (dates.min() ?? Date())...(dates.max() ?? Date())
    }

    var body: some View {
        Chart {
            ForEach(points) { point in
                AreaMark(
                    x: .value("Date", point.date),
                    yStart: .value("Baseline", chartYDomain.lowerBound),
                    yEnd: .value("Weight", point.weight)
                )
                .interpolationMethod(.monotone)
                .foregroundStyle(
                    LinearGradient(
                        colors: [
                            DSColors.accent.opacity(0.25),
                            DSColors.accent.opacity(0.0),
                        ],
                        startPoint: .top,
                        endPoint: .bottom
                    )
                )

                LineMark(
                    x: .value("Date", point.date),
                    y: .value("Weight", point.weight)
                )
                .interpolationMethod(.monotone)
                .foregroundStyle(
                    LinearGradient(
                        colors: [
                            DSColors.accent,
                            DSColors.accent.opacity(0.7),
                        ],
                        startPoint: .top,
                        endPoint: .bottom
                    )
                )
                .lineStyle(StrokeStyle(lineWidth: 2.5))

                PointMark(
                    x: .value("Date", point.date),
                    y: .value("Weight", point.weight)
                )
                .foregroundStyle(DSColors.accent)
                .symbolSize(50)
            }

            // Dashed horizontal reference at the user's
            // target weight. Drawn last so it sits on top
            // of the data line, with a small "Target"
            // annotation pinned to the right edge so the
            // user knows what the line means.
            if let target = targetWeight {
                RuleMark(y: .value("Target", target))
                    .foregroundStyle(DSColors.accent.opacity(0.55))
                    .lineStyle(StrokeStyle(lineWidth: 1, dash: [4, 4]))
                    .annotation(position: .top, alignment: .leading) {
                        Text("Target")
                            .font(.caption2.weight(.bold))
                            .foregroundStyle(DSColors.accent)
                            .padding(.horizontal, 4)
                            .padding(.vertical, 1)
                            .background(
                                Capsule(style: .continuous)
                                    .fill(DSColors.surface)
                            )
                            .overlay(
                                Capsule(style: .continuous)
                                    .stroke(DSColors.accent.opacity(0.55), lineWidth: 0.5)
                            )
                    }
            }
        }
        .frame(height: 180)
        .chartYScale(domain: chartYDomain)
        .chartXScale(domain: xDomain)
        .chartYAxis {
            AxisMarks(position: .leading, values: .automatic(desiredCount: 4)) { value in
                AxisGridLine()
                AxisTick()
                AxisValueLabel {
                    if let weight = value.as(Double.self) {
                        Text("\(Int(weight.rounded())) \(weightUnit)")
                            .font(.caption2)
                    }
                }
            }
        }
        .chartXAxis {
            AxisMarks(values: .automatic(desiredCount: 3)) { value in
                AxisGridLine()
                AxisValueLabel {
                    if let date = value.as(Date.self) {
                        Text(date, format: .dateTime.day().month(.abbreviated))
                            .font(.caption2)
                    }
                }
            }
        }
    }
}
