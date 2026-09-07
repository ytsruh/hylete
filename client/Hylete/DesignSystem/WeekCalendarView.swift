import SwiftUI

/// A reusable one-week calendar strip: seven tappable days,
/// swipe or chevron navigation between weeks, a "Today"
/// shortcut, activity dots under days that have data, and a
/// caller-provided content area rendered beneath the strip for
/// the selected day.
///
/// The component is deliberately data-agnostic — it knows
/// nothing about workouts, weight entries, or the API. The
/// consumer owns all data:
///
///   * `isBusy` answers "should this day show an activity
///     dot?" (typically derived from the consumer's cache).
///   * `content` receives the **start-of-day** `Date` of the
///     selected day and renders anything — a list of sets, a
///     weigh-in card, multiple sections, or nothing.
///
/// Example:
///
///     WeekCalendarView(
///         selection: $selectedDate,
///         isBusy: { day in store.hasEntries(on: day) }
///     ) { day in
///         SetsForDay(date: day)
///     }
///
/// Weeks are unbounded in both directions (the past and the
/// future are both browsable); capping is the consumer's job,
/// not this component's. Week boundaries follow the device
/// locale (`Calendar.current.firstWeekday`).
public struct WeekCalendarView<DayContent: View>: View {

    // MARK: Inputs

    /// The selected day (any instant within the day; normalised
    /// internally to start-of-day for comparisons).
    @Binding private var selection: Date
    /// Answers whether a given day should show its activity dot.
    private let isBusy: (Date) -> Bool
    /// Rendered below the strip for the selected day. Receives
    /// the start-of-day instant of `selection`.
    private let content: (Date) -> DayContent

    // MARK: State

    /// Start-of-day for the first (leftmost) day of the week
    /// currently shown. Independent of `selection`: paging weeks
    /// doesn't change which day is selected, and selecting a day
    /// in another week re-syncs this via `onChange(of: selection)`.
    @State private var visibleWeekStart: Date

    // MARK: Init

    public init(
        selection: Binding<Date>,
        isBusy: @escaping (Date) -> Bool = { _ in false },
        @ViewBuilder content: @escaping (Date) -> DayContent
    ) {
        _selection = selection
        self.isBusy = isBusy
        self.content = content
        _visibleWeekStart = State(initialValue: CalendarMath.startOfWeek(for: selection.wrappedValue))
    }

    // MARK: Body

    public var body: some View {
        VStack(alignment: .leading, spacing: DSSpacing.sm) {
            header
            weekdayHeader
            dayStrip
            Divider().background(DSColors.separator)
            content(CalendarMath.startOfDay(selection))
        }
        .gesture(swipeToChangeWeek)
        .onChange(of: selection) { _, newSelection in
            // If `selection` moved outside the displayed week
            // (external change or a future multi-day jump),
            // follow it. Same-week taps keep the current page.
            let week = CalendarMath.startOfWeek(for: newSelection)
            if !CalendarMath.isSameDay(week, visibleWeekStart) {
                withAnimation(Self.weekChangeAnimation) {
                    visibleWeekStart = week
                }
            }
        }
    }

    // MARK: Header (month label + navigation)

    private var header: some View {
        HStack(spacing: DSSpacing.xs) {
            Button {
                changeWeek(by: -1)
            } label: {
                Image(systemName: "chevron.left")
                    .font(.body.weight(.semibold))
                    .frame(width: 32, height: 32)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Previous week")

            Text(monthLabel)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.text)
                .frame(maxWidth: .infinity)

            if showsTodayShortcut {
                Button {
                    jumpToToday()
                } label: {
                    Text("Today")
                        .font(.caption.weight(.semibold))
                        .padding(.horizontal, DSSpacing.sm)
                        .padding(.vertical, DSSpacing.xxs + 2)
                        .background(
                            // The app's standard inverted chip
                            // (same pairing as the cardio
                            // exercise-type pill): dark fill in
                            // light mode, light fill in dark mode.
                            Capsule().fill(DSColors.secondary)
                        )
                        .foregroundStyle(DSColors.onSecondary)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Jump to today")
                .transition(.opacity)
            }

            Button {
                changeWeek(by: 1)
            } label: {
                Image(systemName: "chevron.right")
                    .font(.body.weight(.semibold))
                    .frame(width: 32, height: 32)
                    .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Next week")
        }
        .foregroundStyle(DSColors.text)
    }

    /// "August 2026"-style label derived from the middle of the
    /// visible week, the conventional tie-breaker for weeks that
    /// straddle two months.
    private var monthLabel: String {
        guard let mid = Calendar.current.date(
            byAdding: .day, value: 3, to: visibleWeekStart
        ) else { return "" }
        return mid.formatted(.dateTime.month(.wide).year())
    }

    /// The Today shortcut only appears when the visible week
    /// doesn't already contain today — otherwise it's dead UI.
    private var showsTodayShortcut: Bool {
        !CalendarMath.days(inWeekOf: visibleWeekStart).contains { day in
            CalendarMath.isSameDay(day, Date())
        }
    }

    // MARK: Strip

    private var weekdayHeader: some View {
        HStack(spacing: 0) {
            ForEach(Array(CalendarMath.weekdaySymbols().enumerated()), id: \.offset) { _, symbol in
                Text(symbol)
                    .font(.caption2.weight(.medium))
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(maxWidth: .infinity)
            }
        }
    }

    private var dayStrip: some View {
        HStack(spacing: 0) {
            ForEach(Array(CalendarMath.days(inWeekOf: visibleWeekStart).enumerated()), id: \.element.timeIntervalSince1970) { _, day in
                dayCell(day)
            }
        }
    }

    private func dayCell(_ day: Date) -> some View {
        let isSelected = CalendarMath.isSameDay(day, selection)
        return Button {
            withAnimation(.easeInOut(duration: 0.15)) {
                selection = day
            }
        } label: {
            VStack(spacing: DSSpacing.xxs) {
                Text(day, format: .dateTime.day())
                    .font(.subheadline.weight(isSelected || isToday(day) ? .bold : .regular).monospacedDigit())
                    .foregroundStyle(textColor(for: day, isSelected: isSelected))
                    .frame(width: 34, height: 34)
                    .background(
                        Circle().fill(isSelected ? DSColors.accent : .clear)
                    )
                    .overlay(
                        // Today gets a ring when it isn't the
                        // selected day; when selected, the filled
                        // circle already says everything.
                        Circle()
                            .stroke(DSColors.accent, lineWidth: 1.5)
                            .opacity(!isSelected && isToday(day) ? 1 : 0)
                    )
                Circle()
                    .fill(DSColors.accent)
                    .frame(width: 4, height: 4)
                    .opacity(isBusy(day) ? 1 : 0)
            }
            .frame(maxWidth: .infinity)
            .padding(.vertical, DSSpacing.xxs)
            .contentShape(Rectangle())
        }
        .buttonStyle(.plain)
        .accessibilityLabel(Text(day.formatted(.dateTime.weekday(.wide).day().month(.wide))))
        .accessibilityAddTraits(isSelected ? [.isSelected] : [])
    }

    private func isToday(_ day: Date) -> Bool {
        CalendarMath.isSameDay(day, Date())
    }

    private func textColor(for day: Date, isSelected: Bool) -> Color {
        if isSelected { return DSColors.onPrimary }
        return isToday(day) ? DSColors.accent : DSColors.text
    }

    // MARK: Navigation

    private func changeWeek(by weeks: Int) {
        withAnimation(Self.weekChangeAnimation) {
            visibleWeekStart = CalendarMath.addWeeks(weeks, to: visibleWeekStart)
        }
    }

    private func jumpToToday() {
        let today = Date()
        withAnimation(Self.weekChangeAnimation) {
            visibleWeekStart = CalendarMath.startOfWeek(for: today)
            selection = CalendarMath.startOfDay(today)
        }
    }

    /// Horizontal-dominant swipes page between weeks. The
    /// distance/aspect thresholds keep vertical scrolling in the
    /// content area (and accidental diagonal drags) from paging.
    private var swipeToChangeWeek: some Gesture {
        DragGesture(minimumDistance: 24)
            .onEnded { value in
                let horizontal = value.translation.width
                let vertical = value.translation.height
                guard abs(horizontal) > abs(vertical), abs(horizontal) > 48 else { return }
                changeWeek(by: horizontal < 0 ? 1 : -1)
            }
    }
}

// MARK: - Previews

/// Shared timing for week-page transitions. Declared outside
/// the generic `WeekCalendarView` because Swift forbids static
/// stored properties on generic types.
private extension Animation {
    static let weekChange = Animation.easeInOut(duration: 0.2)
}

extension WeekCalendarView {
    /// Internal alias so the view body reads cleanly while the
    /// constant itself lives outside the generic type.
    fileprivate static var weekChangeAnimation: Animation { .weekChange }
}

#Preview("With workout-style content") {
    VStack(spacing: 0) {
        WeekCalendarView(
            selection: .constant(Date()),
            isBusy: { day in Calendar.current.component(.weekday, from: day) % 3 == 0 }
        ) { day in
            Text("Content for \(day.formatted(date: .long, time: .omitted))")
                .padding()
                .frame(maxWidth: .infinity, alignment: .leading)
        }
        Spacer()
    }
    .padding(DSSpacing.md)
    .background(DSColors.background)
}
