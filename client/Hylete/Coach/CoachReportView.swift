import SwiftUI

/// Renders one stored weekly Coach report. Pure function of the
/// DTO — used for both the latest screen and history detail, so
/// both always show the same sections in the same order.
///
/// The payload is rendered verbatim (pre-phrased by the server
/// prompt); the client never re-computes or re-interprets it.
public struct CoachReportView: View {
    public let report: CoachReportDTO

    public init(report: CoachReportDTO) {
        self.report = report
    }

    public var body: some View {
        List {
            Section {
                Text(report.payload.summary)
            } header: {
                Text("Week of \(weekTitle)")
            }

            if !report.payload.recommendations.isEmpty {
                Section("Next week") {
                    ForEach(Array(report.payload.recommendations.enumerated()), id: \.offset) { index, rec in
                        HStack(alignment: .top, spacing: DSSpacing.xs) {
                            Text("\(index + 1).")
                                .font(.body.weight(.semibold))
                                .foregroundStyle(DSColors.accent)
                            Text(rec)
                        }
                    }
                }
            }

            if !report.payload.progressPerGoal.isEmpty {
                Section("Goals") {
                    ForEach(report.payload.progressPerGoal, id: \.goal) { line in
                        VStack(alignment: .leading, spacing: 2) {
                            Text(line.goal)
                                .font(.body.weight(.medium))
                            Text(line.status)
                                .font(.subheadline)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                    }
                }
            }

            if !report.payload.prs.isEmpty {
                bulletSection(title: "Highlights", items: report.payload.prs, icon: "trophy")
            }

            if !report.payload.stalling.isEmpty {
                bulletSection(title: "Stalling", items: report.payload.stalling, icon: "exclamationmark.triangle")
            }

            Section("Trends") {
                trendRow(label: "Volume", value: report.payload.trends.volume)
                trendRow(label: "Frequency", value: report.payload.trends.frequency)
                trendRow(label: "Bodyweight", value: report.payload.trends.bodyweight)
                trendRow(label: "Adherence", value: report.payload.adherence)
            }

            if !report.payload.recoverySignals.isEmpty {
                bulletSection(title: "Recovery", items: report.payload.recoverySignals, icon: "heart")
            }
        }
        .navigationTitle("Weekly review")
        .navigationBarTitleDisplayMode(.inline)
    }

    // MARK: - Helpers

    private var weekTitle: String {
        let fmt = DateFormatter()
        fmt.dateStyle = .medium
        fmt.timeStyle = .none
        return fmt.string(from: report.periodStart)
    }

    private func trendRow(label: String, value: String) -> some View {
        HStack {
            Text(label)
                .foregroundStyle(DSColors.text)
            Spacer()
            Text(value)
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.trailing)
        }
    }

    private func bulletSection(title: String, items: [String], icon: String) -> some View {
        Section(title) {
            ForEach(items, id: \.self) { item in
                HStack(alignment: .top, spacing: DSSpacing.xs) {
                    Image(systemName: icon)
                        .foregroundStyle(DSColors.accent)
                    Text(item)
                }
            }
        }
    }
}

/// One review card: week + generation date + summary + action
/// count. Shared by the Coach tab list and the archive list so
/// both always render identically; the detail screen above
/// carries the full breakdown.
public struct CoachCardView: View {
    public let report: CoachReportDTO

    public init(report: CoachReportDTO) {
        self.report = report
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text("Week of \(Self.weekFormatter.string(from: report.periodStart))")
                .font(.headline)
            Text(Self.weekFormatter.string(from: report.createdAt))
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
            Text(report.payload.summary)
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
                .lineLimit(3)
            if !report.payload.recommendations.isEmpty {
                Text(actionCountText(report.payload.recommendations.count))
                    .font(.footnote.weight(.medium))
                    .foregroundStyle(DSColors.accent)
            }
        }
        .padding(.vertical, DSSpacing.xs)
    }

    /// "1 action for next week" vs "3 actions for next week".
    private func actionCountText(_ count: Int) -> String {
        count == 1 ? "1 action for next week" : "\(count) actions for next week"
    }

    private static let weekFormatter: DateFormatter = {
        let fmt = DateFormatter()
        fmt.dateStyle = .medium
        fmt.timeStyle = .none
        return fmt
    }()
}
