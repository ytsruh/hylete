import SwiftUI

/// Single-line goal row content. Pure presentational — no
/// callbacks, no buttons. The row is wrapped in a `Button`
/// by `GoalsListView` (tap → open the editor) and given
/// `.swipeActions` for the complete / reopen gestures, so
/// this view is just the data: status dot, title, inline
/// date text, and an optional "Today" / "Nd" badge.
///
/// Mirrors the web's `GoalCard` from
/// `internal/views/goals/list.templ` — same anatomy, same
/// single-line layout, just without the inline action
/// buttons (which live behind a swipe on iOS per the
/// platform convention used by Mail / Notes / Reminders).
///
/// When `goal.isCompleted` flips, the row's strikethrough
/// animates via the `.animation` modifier at the bottom of
/// `body`. The parent doesn't have to drive it — it just
/// toggles the value.
struct GoalRow: View {
    let goal: GoalDTO

    var body: some View {
        HStack(spacing: DSSpacing.sm) {
            statusDot
            title
            Spacer(minLength: DSSpacing.sm)
            dateText
            if let badge = daysUntilTargetBadge {
                daysBadge(badge)
            }
        }
        .opacity(goal.isCompleted ? 0.65 : 1)
        .animation(.easeInOut(duration: 0.2), value: goal.isCompleted)
    }

    // MARK: - Subviews

    /// Filled accent dot for active goals; muted dot for
    /// completed. Mirrors the web's `bg-primary` /
    /// `bg-muted-foreground` status dot.
    private var statusDot: some View {
        Circle()
            .fill(goal.isCompleted ? DSColors.textSecondary.opacity(0.4) : DSColors.accent)
            .frame(width: 8, height: 8)
    }

    /// Title truncates with an ellipsis so a long goal name
    /// never wraps the row. Strikethrough + dimmer foreground
    /// when complete mirrors the web's `line-through` style.
    private var title: some View {
        Text(goal.title)
            .font(.body.weight(.medium))
            .foregroundStyle(goal.isCompleted ? DSColors.textSecondary : DSColors.text)
            .strikethrough(goal.isCompleted, color: DSColors.textSecondary)
            .lineLimit(1)
            .truncationMode(.tail)
    }

    /// Inline date string — joins "Started X", "Target X",
    /// "Ended X" (or "Done X" when complete) with middle dots
    /// so it reads the same way the web's `GoalDateInline`
    /// does. Hidden when the goal has no dates at all.
    @ViewBuilder
    private var dateText: some View {
        if let text = inlineDateText {
            Text(text)
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
                .lineLimit(1)
        }
    }

    /// Small accent-coloured "Today" / "Nd" pill rendered
    /// only when the target date is in the future. Mirrors
    /// the web's `bg-primary text-primary-foreground` badge
    /// in `list.templ:221-227`.
    private func daysBadge(_ text: String) -> some View {
        Text(text)
            .font(.caption2.weight(.semibold).monospacedDigit())
            .foregroundStyle(DSColors.onPrimary)
            .padding(.horizontal, DSSpacing.xs)
            .padding(.vertical, 2)
            .background(
                Capsule().fill(DSColors.accent)
            )
    }

    // MARK: - Date helpers

    /// Builds the inline "Started 12 May · Target 01 Jul" text
    /// for the row. Returns nil when the goal has no dates so
    /// the parent can skip rendering the text view entirely.
    /// Mirrors the order and short-date format the web uses
    /// (`shortGoalDate` in `list.templ:362`).
    private var inlineDateText: String? {
        var parts: [String] = []
        if let start = goal.startDate {
            parts.append("Started \(Self.short(start))")
        }
        if let target = goal.targetDate {
            parts.append("Target \(Self.short(target))")
        }
        if let end = goal.endDate {
            parts.append("Ended \(Self.short(end))")
        }
        if goal.isCompleted, let done = goal.completedAt {
            parts.append("Done \(Self.short(done))")
        }
        return parts.isEmpty ? nil : parts.joined(separator: " · ")
    }

    /// "Today" / "Nd" pill text. Mirrors
    /// `models.Goal.DaysUntilTarget` — truncated to whole
    /// calendar days so a target later today reads as 0
    /// ("Today") rather than 1.
    private var daysUntilTargetBadge: String? {
        guard !goal.isCompleted, let target = goal.targetDate else { return nil }
        let calendar = Calendar.current
        let start = calendar.startOfDay(for: Date())
        let end = calendar.startOfDay(for: target)
        let days = calendar.dateComponents([.day], from: start, to: end).day ?? 0
        guard days >= 0 else { return nil }
        return days == 0 ? "Today" : "\(days)d"
    }

    /// Short "DD MMM" format (e.g. "12 May") matching the
    /// web's `shortGoalDate`.
    private static func short(_ date: Date) -> String {
        date.formatted(.dateTime.day().month(.abbreviated))
    }
}

#Preview {
    let active = GoalDTO(
        id: "g1",
        title: "Bench 100kg for 5 reps",
        description: "",
        startDate: Date().addingTimeInterval(-7 * 24 * 3600),
        targetDate: Date().addingTimeInterval(14 * 24 * 3600),
        endDate: nil,
        completedAt: nil,
        createdAt: Date(),
        updatedAt: Date()
    )
    let today = GoalDTO(
        id: "g2",
        title: "Run a 5k",
        description: "",
        startDate: Date().addingTimeInterval(-30 * 24 * 3600),
        targetDate: Date(),
        endDate: nil,
        completedAt: nil,
        createdAt: Date(),
        updatedAt: Date()
    )
    let completed = GoalDTO(
        id: "g3",
        title: "Stretch every morning for two weeks",
        description: "",
        startDate: Date().addingTimeInterval(-30 * 24 * 3600),
        targetDate: Date().addingTimeInterval(-1 * 24 * 3600),
        endDate: nil,
        completedAt: Date().addingTimeInterval(-2 * 24 * 3600),
        createdAt: Date(),
        updatedAt: Date()
    )
    return List {
        Section("Active") {
            GoalRow(goal: active)
            GoalRow(goal: today)
        }
        Section("Completed") {
            GoalRow(goal: completed)
        }
    }
}