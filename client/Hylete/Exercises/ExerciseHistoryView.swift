import Charts
import SwiftUI

/// The per-exercise history screen. Pushed onto the
/// `ExerciseListView` navigation stack when the user taps
/// an exercise. The page is anchored by a hero image (or
/// placeholder), followed by a "Watch Video" button when
/// one exists, a "Details" disclosure that carries the
/// description and type chip, a Swift Charts progress
/// chart (area-filled, gradient line, PR reference line,
/// and visible x-axis), a 2-column grid of stat cards
/// (Personal Best highlighted), and a paginated history
/// list. The nav-bar `+` button opens `NewSetView`
/// pre-set to this exercise so the user can log a new set
/// without leaving the context.
///
/// Everything metric-bearing branches on the exercise's
/// type: strength plots heaviest-weight-per-day with a max-
/// weight PR; cardio plots fastest-pace-per-day with the
/// best-pace PR, and rows/stat cards render duration ·
/// distance instead of reps × weight.
struct ExerciseHistoryView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.openURL) private var openURL

    let exercise: ExerciseDTO

    /// `true` when this screen belongs to a cardio exercise —
    /// drives the stat cards, chart metric, and row rendering.
    private var isCardio: Bool { exercise.type.lowercased() == "cardio" }

    @State private var page: HistoryPageDTO?
    @State private var chartPoints: [ChartPoint] = []
    @State private var isLoading: Bool = true
    @State private var errorMessage: String?
    @State private var currentPage: Int = 1
    @State private var selectedDate: Date?
    @State private var isPresentingNewSet: Bool = false
    @State private var isDetailsExpanded: Bool = false
    @State private var entryPendingDelete: ExerciseEntryDTO?
    @State private var editingEntry: ExerciseEntryDTO?
    @State private var showingDeleteConfirm: Bool = false

    var body: some View {
        content
            .navigationTitle(exercise.name)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        isPresentingNewSet = true
                    } label: {
                        Image(systemName: Icons.addSet)
                            .font(.body.weight(.semibold))
                    }
                    .accessibilityLabel("Add set")
                }
            }
            .sheet(isPresented: $isPresentingNewSet) {
                NewSetView(initialExerciseID: exercise.id)
                    .environmentObject(env)
                    .environmentObject(authStore)
                    .onDisappear {
                        // Paging-style refresh: the user just
                        // logged a set, so reload the first
                        // page and the chart so the new row
                        // appears at the top.
                        currentPage = 1
                        Task { await load(refresh: true) }
                    }
            }
            .sheet(item: $editingEntry) { entry in
                EditSetView(exerciseEntry: entry) { updated in
                    updateEntry(updated)
                }
                .environmentObject(env)
                .environmentObject(authStore)
            }
            .alert(
                isCardio ? "Delete this session?" : "Delete this set?",
                isPresented: $showingDeleteConfirm,
                presenting: entryPendingDelete
            ) { entry in
                Button("Delete", role: .destructive) {
                    Task { await deleteEntry(entry) }
                }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text(isCardio
                     ? "This will permanently remove the session from your history."
                     : "This will permanently remove the set from your history.")
            }
            .task(id: currentPage) { await load(refresh: false) }
    }

    @ViewBuilder
    private var content: some View {
        if isLoading && page == nil {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let errorMessage, page == nil {
            VStack(spacing: DSSpacing.md) {
                Image(systemName: Icons.warning)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.destructive)
                Text(errorMessage)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") { Task { await load(refresh: false) } }
                    .buttonStyle(.dsSecondary)
            }
            .padding()
        } else if let page, page.entries.isEmpty {
            VStack(spacing: 0) {
                heroImage
                // Outside a List a `Section` renders with no
                // card background, so the Details disclosure
                // gets the app's standard rounded surface
                // card applied manually to match the grouped
                // look of the non-empty branch.
                actionsContent
                    .padding(.vertical, DSSpacing.sm)
                    .padding(.horizontal, DSSpacing.md)
                    .background(
                        RoundedRectangle(
                            cornerRadius: DSSpacing.cornerRadius,
                            style: .continuous
                        )
                        .fill(DSColors.surface)
                    )
                    .overlay(
                        RoundedRectangle(
                            cornerRadius: DSSpacing.cornerRadius,
                            style: .continuous
                        )
                        .stroke(DSColors.separator, lineWidth: 0.5)
                    )
                    .padding(.top, DSSpacing.md)
                Spacer()
                emptyState
            }
            .padding(.horizontal, DSSpacing.md)
        } else {
            List {
                // Hero image lives in its own transparent
                // section so it sits flush against the
                // screen edges instead of being wrapped in
                // a List section card. The standard section
                // padding is removed via `.listRowInsets`
                // so the image uses the full width.
                Section {
                    heroImage
                }
                .listRowBackground(Color.clear)
                .listRowSeparator(.hidden)
                .listRowInsets(EdgeInsets(top: 0, leading: 0, bottom: 0, trailing: 0))

                actionsSection
                statsSection
                chartSection
                Section(isCardio ? "Sessions" : "History") {
                    ForEach(page?.entries ?? []) { entry in
                        HistorySetRow(entry: entry, weightUnit: weightUnit, distanceUnit: distanceUnit)
                            // Leading edge → Edit. Neutral
                            // grey tint (not the brand
                            // accent) so the two swipe
                            // directions read as
                            // left=neutral-edit,
                            // right=destructive-delete.
                            .swipeActions(edge: .leading, allowsFullSwipe: true) {
                                Button {
                                    editingEntry = entry
                                } label: {
                                    Label("Edit", systemImage: Icons.edit)
                                        .labelStyle(.iconOnly)
                                }
                                .tint(Color(.systemGray2))
                            }
                            // Trailing edge → Delete. Full
                            // swipe goes through the
                            // confirmation dialog below —
                            // never deletes directly.
                            .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                                Button(role: .destructive) {
                                    entryPendingDelete = entry
                                    showingDeleteConfirm = true
                                } label: {
                                    Label("Delete", systemImage: Icons.trash)
                                        .labelStyle(.iconOnly)
                                }
                            }
                    }
                }
                paginationSection
            }
            .listStyle(.insetGrouped)
            // Compact section spacing keeps the inter-section
            // gaps tight everywhere — the user asked for the
            // Progress→History gap to be the reference, and
            // `.compact` matches that visual rhythm.
            .listSectionSpacing(.compact)
            // Pull the top of the scroll content up so the
            // hero image sits closer to the navigation bar.
            .contentMargins(.top, 0, for: .scrollContent)
        }
    }

    /// "Watch Video" button and the "Details" `DisclosureGroup`
    /// (which holds the description and the type chip), wrapped
    /// in a List `Section` so it picks up the inset-grouped
    /// card background when this screen has entries. The image
    /// is rendered separately so this section is just the
    /// page's primary actions and metadata.
    @ViewBuilder
    private var actionsSection: some View {
        Section {
            actionsContent
        }
    }

    /// The raw contents of `actionsSection` without the List
    /// `Section` wrapper — also used by the empty-state branch,
    /// where there is no List and the card background is
    /// applied manually instead.
    @ViewBuilder
    private var actionsContent: some View {
        VStack(alignment: .leading, spacing: DSSpacing.sm) {
            if exercise.hasVideo {
                Button {
                    if let url = URL(string: exercise.videoURL) {
                        openURL(url)
                    }
                } label: {
                    Label("Watch Video", systemImage: Icons.play)
                        .font(.subheadline.weight(.semibold))
                }
                .buttonStyle(.dsSecondary)
                .frame(maxWidth: .infinity)
            }
            DisclosureGroup(isExpanded: $isDetailsExpanded) {
                VStack(alignment: .leading, spacing: DSSpacing.xs) {
                    if !exercise.description.isEmpty {
                        Text(exercise.description)
                            .font(.subheadline)
                            .foregroundStyle(DSColors.textSecondary)
                    } else {
                        Text("No description available")
                            .font(.subheadline)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    ExerciseTypeChip(type: exercise.type)
                }
                .padding(.top, DSSpacing.xs)
                .frame(maxWidth: .infinity, alignment: .leading)
            } label: {
                Text(isDetailsExpanded ? "Hide details" : "Details")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                    .textCase(nil)
            }
        }
    }

    /// Item driving the full-screen image viewer. Non-nil
    /// presents `ImageViewer` over everything else; nil dismisses.
    private struct ViewerImage: Identifiable {
        let id = UUID()
        let url: URL
    }

    @State private var fullScreenImage: ViewerImage?

    /// Width cap for the hero banner: the frame is exactly 3:1
    /// (BannerImage), so capping width keeps the whole photo
    /// visible — ~353pt iPhone rows render ~353x118pt full-width,
    /// while wider iPad / Pro Max rows get a centred 360x120pt
    /// card instead of a heavily cropped strip. Mirrors the web's
    /// `max-w-[624px]` cap on `components.BannerImage` in
    /// `internal/views/exercise/history.templ`; uploads should be
    /// 3:1 (e.g. 3000x1000). Both the image and the no-image
    /// placeholder use it so empty states match.
    private let heroMaxWidth: CGFloat? = 360

    private var heroImage: some View {
        // A nil URL renders the component's own placeholder tile,
        // so the no-image state goes through the identical 3:1
        // layout path as a loaded photo and the two can never
        // diverge. Tapping (when an image exists) opens the
        // full-screen zoomable viewer; with no image the empty
        // URL makes the tap a no-op.
        BannerImage(
            url: exercise.hasImage ? URL(string: exercise.imageURL) : nil,
            accessibilityLabel: exercise.name,
            placeholderIcon: Icons.exercises
        )
        .frame(maxWidth: heroMaxWidth)
        .contentShape(Rectangle())
        .onTapGesture {
            // viewerImageURL prefers the hi-res original and
            // falls back to the display image.
            if let url = URL(string: exercise.viewerImageURL) {
                fullScreenImage = ViewerImage(url: url)
            }
        }
        .accessibilityAddTraits(.isButton)
        .accessibilityHint("Opens the image full screen")
        .fullScreenCover(item: $fullScreenImage) { item in
            ImageViewer(url: item.url)
        }
    }

    @ViewBuilder
    private var chartSection: some View {
        if !chartPoints.isEmpty {
            Section(isCardio ? "Pace Progress" : "Progress") {
                Chart {
                    ForEach(chartPoints) { point in
                        // Area fill rendered first so the line
                        // and points sit on top. The fill is
                        // explicitly bounded below by the
                        // lowest data point (via `yStart:`)
                        // so the gradient doesn't bleed past
                        // the data and cover the y-axis tick
                        // labels at the bottom of the chart.
                        AreaMark(
                            x: .value("Date", point.date),
                            yStart: .value("Baseline", chartYBaseline),
                            yEnd: .value(chartMetricName, point.value)
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
                            y: .value(chartMetricName, point.value)
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
                            y: .value(chartMetricName, point.value)
                        )
                        .foregroundStyle(DSColors.accent)
                        .symbolSize(isMostRecentPoint(point) ? 160 : 50)

                        // Highlight marks drawn in chart
                        // coordinate space (RuleMark / PointMark
                        // must live inside `Chart { ... }` — the
                        // .chartOverlay content is a plain View
                        // and can only host the floating card).
                        if let selectedDate,
                           let nearest = nearestPoint(to: selectedDate),
                           point.id == nearest.id {
                            RuleMark(x: .value("Selected", nearest.date))
                                .foregroundStyle(DSColors.textSecondary.opacity(0.35))
                                .lineStyle(StrokeStyle(lineWidth: 1, dash: [3, 3]))
                                .zIndex(-1)
                            PointMark(
                                x: .value("Date", nearest.date),
                                y: .value(chartMetricName, nearest.value)
                            )
                            .foregroundStyle(DSColors.accent)
                            .symbolSize(160)
                        }
                    }

                    // PR reference line lives alongside the
                    // series marks so it shares the same y
                    // axis. Drawn after the series so it
                    // overlays the line — the dashed stroke
                    // keeps it visually distinct from the
                    // data line. For cardio the "PR" is the
                    // fastest pace, so the domain direction
                    // flips but the annotation reads the same.
                    if let pr = personalBest {
                        RuleMark(y: .value("PR", pr))
                            .foregroundStyle(DSColors.accent.opacity(0.55))
                            .lineStyle(StrokeStyle(lineWidth: 1, dash: [4, 4]))
                            .zIndex(-1)
                            .annotation(position: .top, alignment: .leading) {
                                Text("PR")
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
                .frame(height: 220)
                .chartYScale(domain: chartYDomain)
                .chartYAxis {
                    AxisMarks(position: .leading, values: .automatic(desiredCount: 4)) { value in
                        AxisGridLine()
                        AxisTick()
                        AxisValueLabel {
                            if let v = value.as(Double.self) {
                                Text(axisLabel(for: v))
                                    .font(.caption2.monospacedDigit())
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
                .chartXSelection(value: $selectedDate)
                .chartOverlay { proxy in
                    if let selectedDate,
                       let nearest = nearestPoint(to: selectedDate),
                       let xPos = proxy.position(forX: nearest.date),
                       let yPos = proxy.position(forY: nearest.value) {
                        // Floating card overlay. The proxy
                        // returns positions in the chart's
                        // content area; we clamp the x so the
                        // card never runs off the chart edges.
                        GeometryReader { geometry in
                            let cardHalfWidth: CGFloat = 50
                            let minX = cardHalfWidth + 4
                            let maxX = geometry.size.width - cardHalfWidth - 4
                            let clampedX = min(max(xPos, minX), maxX)
                            let clampedY = max(yPos - 28, 20)

                            chartTooltip(for: nearest)
                                .position(x: clampedX, y: clampedY)
                        }
                    }
                }
            }
        }
    }

    /// Y-axis domain for the progress chart. Rounds the data's
    /// min/max out to a "nice" step and then pads by 10% on
    /// each side so the line doesn't kiss the top and bottom
    /// edges. The nice-step rounding keeps the axis from
    /// snapping to the next round number below the data (e.g.
    /// anchoring to 10 when the data actually starts at 11).
    private var chartYDomain: ClosedRange<Double> {
        let values = chartPoints.map(\.value)
        let rawLo = values.min() ?? 0
        let rawHi = values.max() ?? 0
        let step = Self.niceStep(for: rawHi - rawLo)
        let lo = (rawLo / step).rounded(.down) * step
        let hi = (rawHi / step).rounded(.up) * step
        let pad = max((hi - lo) * 0.1, step)
        return (lo - pad)...(hi + pad)
    }

    /// Picks a "nice" tick step (1, 2.5, 5, 10, …) sized to
    /// the data's range so axis labels read as clean numbers.
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

    /// Returns the chart point whose date is closest to the
    /// given target, or nil if the chart has no data. Used by
    /// the tooltip to snap the user's touch to a real
    /// observation.
    private func nearestPoint(to date: Date) -> ChartPoint? {
        chartPoints.min { lhs, rhs in
            abs(lhs.date.timeIntervalSince(date)) < abs(rhs.date.timeIntervalSince(date))
        }
    }

    /// The user's lifetime personal best for this exercise, in
    /// chart units: heaviest weight for strength, fastest pace
    /// (lowest sec/km) for cardio. Prefers the type-appropriate
    /// `HistoryStatsDTO` value and falls back to the chart points.
    private var personalBest: Double? {
        if isCardio {
            if let stats = page?.stats, stats.bestPaceSecPerKm > 0 {
                return stats.bestPaceSecPerKm
            }
            return chartPoints.map(\.value).min()
        }
        if let stats = page?.stats, stats.maxWeight > 0 {
            return stats.maxWeight
        }
        return chartPoints.map(\.value).max()
    }

    /// Lower bound for the area fill. Anchoring the fill
    /// to the lowest data point (rather than the y-axis
    /// baseline, which includes padding) keeps the
    /// gradient from bleeding into the y-axis tick-label
    /// row at the bottom of the chart.
    private var chartYBaseline: Double {
        chartPoints.map(\.value).min() ?? 0
    }

    /// `true` when the point is the most recent (latest
    /// date) on the chart. Used by the chart to render the
    /// most recent point as a larger accent dot so the eye
    /// lands on "today's status" without inspecting the
    /// line.
    private func isMostRecentPoint(_ point: ChartPoint) -> Bool {
        guard let latest = chartPoints.map(\.date).max() else { return false }
        return point.date == latest
    }

    /// Small floating card showing the date and the metric value
    /// (weight or pace) for the highlighted chart point.
    /// Positioned by the caller via `.position(x:y:)` so the card
    /// sits above the dot.
    @ViewBuilder
    private func chartTooltip(for point: ChartPoint) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(point.date.formatted(.dateTime.day().month(.abbreviated).year()))
                .font(.caption2)
                .foregroundStyle(DSColors.textSecondary)
            Text(metricText(point.value))
                .font(.caption.weight(.semibold).monospacedDigit())
                .foregroundStyle(DSColors.text)
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
    }

    /// Formats a raw chart value for axis ticks and tooltips:
    /// "85 kg" for strength, "4:58 /km" for cardio pace.
    private func metricText(_ value: Double) -> String {
        if isCardio {
            return Self.paceText(value, unit: distanceUnit)
        }
        return String(format: "%.1f %@", value, weightUnit)
    }

    private func axisLabel(for value: Double) -> String {
        metricText(value)
    }

    /// Renders a seconds-per-unit pace as "M:SS /unit".
    static func paceText(_ secPerKm: Double, unit: String) -> String {
        let seconds = unit == "mi" ? secPerKm * 1.609344 : secPerKm
        let s = Int(seconds.rounded())
        return String(format: "%d:%02d /%@", s / 60, s % 60, unit == "mi" ? "mi" : "km")
    }

    @ViewBuilder
    private var statsSection: some View {
        if let stats = page?.stats {
            // Outer Section card is intentionally removed
            // (`.listRowBackground(Color.clear)` + no
            // row separator) so the individual stat cards
            // sit on the page background rather than
            // appearing as "cards inside a card". No
            // section header — the stat cards' labels are
            // self-describing, so a "Stats" title would be
            // redundant.
            Section {
                StatsGrid(
                    columns: [
                        GridItem(.flexible(), spacing: DSSpacing.sm),
                        GridItem(.flexible(), spacing: DSSpacing.sm),
                    ],
                    spacing: DSSpacing.sm
                ) {
                    if isCardio {
                        // Cardio personal best = fastest pace ever.
                        StatCard(
                            label: "Best Pace",
                            value: stats.bestPaceSecPerKm > 0
                                ? Self.paceText(stats.bestPaceSecPerKm, unit: distanceUnit)
                                : "—",
                            icon: Icons.trophy
                        )
                        .gridCellColumns(2)

                        if let lastSet = stats.lastSet {
                            StatCard(
                                label: "Last Session",
                                value: lastSessionValue(lastSet),
                                icon: Icons.dumbbellSmall
                            )
                            StatCard(
                                label: "Longest Distance",
                                value: longestDistanceValue(stats),
                                icon: Icons.calendar
                            )
                        }
                    } else {
                        StatCard(
                            label: "Personal Best",
                            value: String(format: "%.1f %@", stats.maxWeight, weightUnit),
                            icon: Icons.trophy
                        )
                        .gridCellColumns(2)

                        if let lastSet = stats.lastSet {
                            StatCard(
                                label: "Last Set",
                                value: "\(lastSet.reps) × \(String(format: "%.1f", lastSet.weight)) \(weightUnit)",
                                icon: Icons.dumbbellSmall
                            )
                            StatCard(
                                label: "Last Activity",
                                value: lastSet.createdAt.formatted(.dateTime.day().month().year()),
                                icon: Icons.calendar
                            )
                        }
                    }
                }
            }
            .listRowBackground(Color.clear)
            .listRowSeparator(.hidden)
        }
    }

    /// "25:00 · 5.20 km" summary for the cardio Last Session card,
    /// mirroring SetRow's cardio rendering.
    private func lastSessionValue(_ entry: ExerciseEntryDTO) -> String {
        let duration = SetRow.formattedDuration(entry.durationSeconds)
        return "\(duration) · \(distanceText(entry.distanceMeters))"
    }

    /// The user's longest-ever distance for this exercise, rendered
    /// in their preferred unit; "—" when nothing qualifies yet.
    private func longestDistanceValue(_ stats: HistoryStatsDTO) -> String {
        guard stats.longestDistanceMeters > 0 else { return "—" }
        return distanceText(stats.longestDistanceMeters)
    }

    /// Converts stored metres into the user's preferred unit string.
    private func distanceText(_ meters: Double) -> String {
        let km = meters / 1000.0
        if distanceUnit == "mi" {
            return String(format: "%.2f mi", km / 1.609344)
        }
        return String(format: "%.2f km", km)
    }

    @ViewBuilder
    private var paginationSection: some View {
        HStack(spacing: DSSpacing.sm) {
            Button { currentPage = max(1, currentPage - 1) } label: {
                Image(systemName: "chevron.left")
                    .font(.footnote.weight(.semibold))
            }
            .buttonStyle(.plain)
            .foregroundStyle(DSColors.textSecondary)
            .disabled(!(page?.hasPrev ?? false))

            if isLoading {
                ProgressView()
                    .scaleEffect(0.8)
            } else {
                Text("Page \(currentPage)")
                    .font(.footnote.weight(.semibold).monospacedDigit())
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(minWidth: 60)
            }

            Button { currentPage += 1 } label: {
                Image(systemName: "chevron.right")
                    .font(.footnote.weight(.semibold))
            }
            .buttonStyle(.plain)
            .foregroundStyle(DSColors.textSecondary)
            .disabled(!(page?.hasNext ?? false))
        }
        .frame(maxWidth: .infinity)
        .padding(.vertical, DSSpacing.xs)
    }

    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.chartEmpty)
                .font(.system(size: 48))
                .foregroundStyle(DSColors.textSecondary)
            Text(isCardio ? "No sessions yet" : "No sets yet")
                .font(.title3.weight(.semibold))
            Text(isCardio
                 ? "Log a session from the Today tab to start tracking \(exercise.name)."
                 : "Log a set from the Today tab to start tracking \(exercise.name).")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Button {
                isPresentingNewSet = true
            } label: {
                Label(isCardio ? "Log First Session" : "Log First Set", systemImage: Icons.addSet)
                    .font(.headline)
            }
            .buttonStyle(.dsPrimary)
            .padding(.horizontal, DSSpacing.lg)
            .padding(.top, DSSpacing.sm)
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }

    /// The chart's y-axis semantic, used as the mark series name so
    /// accessibility labels read correctly per exercise type.
    private var chartMetricName: String {
        isCardio ? "Pace" : "Max weight"
    }

    // MARK: - Data

    /// `refresh` triggers a full reload of the chart and the
    /// first page — used when the user just saved a new set
    /// from the embedded `NewSetView` sheet so the new row
    /// appears at the top immediately.
    ///
    /// `@MainActor` because this mutates `@State` (`isLoading`,
    /// `page`, `errorMessage`) and delegates to `loadChart`,
    /// which also writes state. Same rationale as the
    /// dashboard's pinned networking methods.
    @MainActor
    private func load(refresh: Bool) async {
        let isFirstLoad = page == nil
        if isFirstLoad || refresh { isLoading = true }
        defer { isLoading = false }
        do {
            let result = try await env.api.getExerciseHistory(id: exercise.id, page: currentPage)
            page = result
            // The chart is a lifetime view of all sets, so it
            // only needs to be fetched once — not on every
            // page change. After a save we want it refreshed
            // though, so `refresh` triggers a re-fetch.
            if isFirstLoad || refresh {
                await loadChart()
            }
        } catch let error as APIError {
            if case .unauthorized = error { return }
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load history."
        }
    }

    /// `@MainActor` because this mutates `@State`
    /// (`chartPoints`, `selectedDate`).
    @MainActor
    private func loadChart() async {
        do {
            let entries = try await env.api.getExerciseChartData(id: exercise.id)
            chartPoints = Self.chartPoints(from: entries, isCardio: isCardio)
        } catch {
            // Non-fatal: history is still useful without the chart.
            chartPoints = []
        }
        // The chart selection (via `chartXSelection`) references
        // a point in the PREVIOUS data. Keeping it attached
        // across a reload — page change, save, or delete — is a
        // known iOS 17 SwiftUI Charts crash; dropping it just
        // closes the floating tooltip.
        selectedDate = nil
    }

    // MARK: - Row actions

    /// Removes a set after the user confirms via the
    /// swipe-to-delete confirmation dialog. Optimistic —
    /// the row disappears from the local page immediately
    /// and the API call rolls it back (via a full reload)
    /// if it fails. The chart is refreshed too because
    /// the PR or the last-set point may have changed.
    private func deleteEntry(_ entry: ExerciseEntryDTO) async {
        guard let current = page else { return }
        let filteredEntries = current.entries.filter { $0.id != entry.id }
        page = HistoryPageDTO(
            exerciseType: current.exerciseType,
            entries: filteredEntries,
            stats: current.stats,
            page: current.page,
            hasPrev: current.hasPrev,
            hasNext: current.hasNext
        )
        do {
            try await env.api.deleteExerciseEntry(id: entry.id)
        } catch let error as APIError {
            if case .unauthorized = error { return }
            errorMessage = error.errorDescription
            // Rollback by reloading the current page so the
            // row reappears with the rest of the server's
            // truth.
            await load(refresh: true)
            return
        } catch {
            errorMessage = "Could not delete the set."
            await load(refresh: true)
            return
        }
        // Refresh the lifetime chart (PR / last-set may have
        // changed) and reload the current page so the stats
        // block in the header stays in sync.
        await loadChart()
        await load(refresh: true)
    }

    /// Splices a server-confirmed edited entry back into
    /// the visible page and refreshes the chart so the
    /// trend, PR reference line, and stat cards reflect
    /// the new values.
    private func updateEntry(_ updated: ExerciseEntryDTO) {
        guard let current = page else { return }
        let updatedEntries = current.entries.map { entry in
            entry.id == updated.id ? updated : entry
        }
        page = HistoryPageDTO(
            exerciseType: current.exerciseType,
            entries: updatedEntries,
            stats: current.stats,
            page: current.page,
            hasPrev: current.hasPrev,
            hasNext: current.hasNext
        )
        Task {
            await loadChart()
            await load(refresh: true)
        }
    }

    /// Aggregates the full entry list into one point per
    /// calendar day: the heaviest set of the day for strength,
    /// the fastest pace of the day for cardio (entries without
    /// a derivable pace are skipped). Matches the web chart's
    /// per-day reduction so the two platforms agree.
    private static func chartPoints(from entries: [ExerciseEntryDTO], isCardio: Bool) -> [ChartPoint] {
        let calendar = Calendar.current
        let grouped = Dictionary(grouping: entries.filter { entry in
            isCardio ? entry.paceSecPerKm != nil : true
        }) { entry in
            calendar.startOfDay(for: entry.createdAt)
        }
        return grouped
            .map { day, entries -> ChartPoint in
                if isCardio {
                    // Fastest pace of the day wins (lowest sec/km).
                    let best = entries.compactMap(\.paceSecPerKm).min() ?? 0
                    return ChartPoint(date: day, value: best)
                }
                return ChartPoint(date: day, value: entries.map(\.weight).max() ?? 0)
            }
            .sorted { $0.date < $1.date }
    }
}

/// One point on the progress chart. `value` is type-dependent:
/// heaviest weight (kg) for strength, fastest pace (sec/km) for
/// cardio — the view owns the interpretation.
private struct ChartPoint: Identifiable {
    let date: Date
    let value: Double
    var id: TimeInterval { date.timeIntervalSince1970 }
}

/// A single row in the exercise-history list. Intentionally
/// narrower than the dashboard's `SetRow`: the exercise name
/// is implicit (we're already inside that exercise's screen),
/// notes are dropped, and the time of day is hidden in favour
/// of the date so rows scan cleanly when paging through
/// history. Cardio entries render duration · distance instead
/// of reps × weight.
private struct HistorySetRow: View {
    let entry: ExerciseEntryDTO
    let weightUnit: String
    let distanceUnit: String

    var body: some View {
        HStack(alignment: .firstTextBaseline) {
            Text(entry.createdAt, format: .dateTime.day().month(.abbreviated).year())
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
            Spacer()
            Text(summaryText)
                .font(.body.weight(.semibold).monospacedDigit())
                .foregroundStyle(DSColors.text)
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    private var summaryText: String {
        if entry.isCardio {
            let km = entry.distanceMeters / 1000.0
            let distance = distanceUnit == "mi"
                ? String(format: "%.2f mi", km / 1.609344)
                : String(format: "%.2f km", km)
            return "\(SetRow.formattedDuration(entry.durationSeconds)) · \(distance)"
        }
        return "\(entry.reps) × \(String(format: "%.1f", entry.weight)) \(weightUnit)"
    }
}

#Preview {
    NavigationStack {
        ExerciseHistoryView(exercise: ExerciseDTO(
            id: "ex-1",
            name: "Squat",
            description: "Compound lower-body exercise.",
            videoURL: "",
            imgURL: "",
            imageURL: "",
            type: "strength"
        ))
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
        .environmentObject(AuthStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
    }
}
