import HealthKit
import SwiftUI

/// Full-screen Workout Player. Presented from `WorkoutDetailView`'s
/// Start button, it walks the user through every planned block and
/// exercise in order, logging sets as they go.
///
/// Each planned exercise gets type-appropriate editors (reusing
/// `SetRowEditor`): strength items get repeatable set rows,
/// cardio items a single session row, each with collapsible
/// notes. There is one call to action per block — "Mark as Done"
/// first POSTs every item's valid rows (each POST carries
/// `workout_id`/`block_id`/`workout_block_id`, creating normal
/// exercise entries: history, charts, and exports all include
/// them), then marks the block done. A failed submit aborts
/// before the status write. Done blocks keep the button (as "Log
/// additional sets") while they hold valid drafts, so sets typed
/// after reopening a finished workout can still be posted.
/// Half-typed rows stay on screen and survive background/kill via
/// the store's on-device snapshot. The tappable "X logged" row
/// opens the exercise's sets for this workout (history sheet with
/// edit + delete); the store retains the full entries, not just
/// counts.
///
/// Progress is hybrid: per-item logged counts plus local skips
/// (zero sets, e.g. no equipment or injury) drive a "ready to mark
/// done" hint, but the block pending/done/skipped check-off stays
/// manual — the source of truth, written through `WorkoutStore`.
/// The header's "% complete" is blocks-based (done or skipped over
/// total blocks) for the same reason. Done/skipped blocks
/// auto-collapse so the screen stays focused on remaining work;
/// the chevron always overrides for the session. Finish flips the
/// workout to completed (partial completion is allowed: pending
/// blocks may remain).
///
/// The screen idle timer is disabled while the player is open
/// (long rests can outlast the user's autolock) and restored on
/// dismiss.
struct WorkoutPlayerView: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var workoutStore: WorkoutStore
    @StateObject private var player: WorkoutPlayerStore
    /// Shared app-scoped Health recording (owned by
    /// `MainTabView`, so a recording survives closing this
    /// player). This view only attaches to it per workout —
    /// see the load task. Separate from `player` so a Health
    /// denial or failure can never block set logging.
    @ObservedObject var healthStore: PlayerHealthStore
    /// The player-scoped single shared timer. Block header
    /// chips start timers here; the sticky pill and expanded
    /// sheet render this store. Never persisted — a timer is
    /// session UI, not workout data.
    @StateObject private var timerStore = PlayerTimerStore()
    /// Bridges the timer store's fire dates to fallback
    /// local notifications (background/locked-phone cue).
    /// Watches `timerStore.endDate` — one observation point
    /// for starts, pauses, resumes, round rolls, and resets.
    @StateObject private var timerNotifier = PlayerTimerNotifier()

    let workoutID: String

    @State private var didRequestLoad: Bool = false
    /// Set when this player opened while the shared Health
    /// store is recording a different workout. Auto-start is
    /// skipped and the banner below explains how to switch
    /// (end the active recording from the global pill).
    @State private var healthBusy: Bool = false
    @State private var showingFinishConfirm: Bool = false
    @State private var isFinishing: Bool = false
    /// Collapsed block ids (tap the chevron to fold long blocks).
    @State private var collapsedBlockIDs: Set<String> = []
    /// Block ids with a status write in flight (disables the Done
    /// button and shows a spinner so double-taps can't race).
    @State private var markingBlockIDs: Set<String> = []
    /// Planned item whose logged-sets history sheet is open.
    /// `BlockItemDTO` is `Identifiable`, so the sheet binds by
    /// item and always reads live buckets from the store.
    @State private var historyItem: BlockItemDTO?
    /// Exercise whose reference detail sheet is open (full
    /// `ExerciseHistoryView` in read-only mode: details,
    /// stats, chart, history — no entry actions). Set
    /// immediately to a lightweight fallback built from the
    /// planned item so the sheet opens without waiting for
    /// the catalogue; upgraded to the rich catalogue match
    /// in the background when it arrives (same id, so the
    /// history load is unaffected).
    @State private var exerciseDetail: ExerciseDTO?
    /// Catalogue cache for resolving planned items to full
    /// `ExerciseDTO`s. Loaded lazily on first tap (one fetch
    /// per player session) — there is no single-exercise GET
    /// endpoint, so the list is the only source.
    @State private var exerciseCatalog: [String: ExerciseDTO]?
    /// Expanded timer sheet visibility.
    @State private var showingTimerSheet: Bool = false
    /// Block whose plan config prefills the timer sheet's
    /// custom form. Set by a block's "Custom…" timer row, or
    /// nil when the sheet opens from the pill (the running
    /// timer already carries its block context).
    @State private var timerSetupBlock: WorkoutBlockDetailDTO?
    /// Hides the sticky pill after its completion auto-dismiss
    /// fires. Pill-only: the store stays complete so the
    /// auto-opened sheet is unaffected — a new start or reset
    /// re-shows the pill via the observers below.
    @State private var hideCompletedPill = false
    /// Generation for the auto-dismiss task. Bumped on every
    /// timer event so a new start/reset cancels a pending hide
    /// (the task additionally re-checks `isComplete`, so only
    /// a genuine completion can hide the pill).
    @State private var pillDismissGeneration = 0
    /// Paged player index: 0 = logging, 1 = Live Health stats.
    /// Swipeable full-screen (`TabView` page style) with dots
    /// plus an explicit link — swipe alone is undiscoverable
    /// mid-workout.
    @State private var selectedPage = 0

    init(workoutID: String, workoutStore: WorkoutStore, player: WorkoutPlayerStore, healthStore: PlayerHealthStore) {
        self.workoutID = workoutID
        self.workoutStore = workoutStore
        _player = StateObject(wrappedValue: player)
        self.healthStore = healthStore
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle(player.workout?.name ?? "Workout")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Close") { dismiss() }
                    }
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("Finish") { showingFinishConfirm = true }
                            .bold()
                            .disabled(isFinishing || player.workout == nil)
                    }
                }
                // Centered confirmation, matching the Delete
                // workout/block pattern elsewhere in the app
                // (a confirmationDialog would dock to the bottom
                // of the screen instead).
                .alert("Finish this workout?", isPresented: $showingFinishConfirm) {
                    Button("Finish workout") { Task { await finish() } }
                    Button("Keep going", role: .cancel) {}
                } message: {
                    Text("The workout is marked completed. Pending blocks may remain — partial completion is allowed.")
                }
                .sheet(item: $historyItem) { item in
                    WorkoutLoggedSetsSheet(player: player, item: item)
                        .environmentObject(env)
                        .environmentObject(authStore)
                }
                .sheet(item: $exerciseDetail) { exercise in
                    NavigationStack {
                        ExerciseHistoryView(exercise: exercise, allowsEntryActions: false)
                            .environmentObject(env)
                            .environmentObject(authStore)
                    }
                }
                .task {
                    // Long rests between sets can outlast the
                    // user's autolock — keep the screen awake while
                    // the player is open. Released on disappear so
                    // Finish/Close can never leak it on.
                    TimerWakeLock.acquire()
                    guard !didRequestLoad else { return }
                    didRequestLoad = true
                    await player.load()
                    // Fold finished work away: done/skipped blocks
                    // start collapsed (pending blocks stay open).
                    // The chevron still overrides per block.
                    if let workout = player.workout {
                        collapsedBlockIDs = Set(workout.blocks
                            .filter { $0.status == .done || $0.status == .skipped }
                            .map(\.id))
                        // Claim the shared Health store for this
                        // workout (type/DOB adoption runs inside
                        // on first claim). Reattaching to our own
                        // running session starts nothing new; a
                        // session for another workout blocks us
                        // with a banner instead of auto-starting.
                        switch healthStore.attach(
                            workoutID: workoutID,
                            items: workout.blocks.flatMap(\.items),
                            serverKey: workout.healthActivityType,
                            dateOfBirth: authStore.currentUser?.dateOfBirth
                        ) {
                        case .claimed:
                            // Auto-start tracking unless opted
                            // out in Profile. Runs once per
                            // player instance (didRequestLoad
                            // guard above); denial lands on the
                            // existing inline note, never a
                            // block.
                            if HealthAutoStart.isEnabled {
                                await healthStore.start()
                            }
                        case .reattached:
                            break
                        case .busy:
                            healthBusy = true
                        }
                    }
                }
                .onDisappear {
                    TimerWakeLock.release()
                }
        }
    }

    @ViewBuilder
    private var content: some View {
        if let workout = player.workout {
            loadedView(workout)
        } else if player.isLoading {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = player.errorMessage {
            VStack(spacing: DSSpacing.md) {
                Image(systemName: Icons.warning)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.destructive)
                Text(error)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") {
                    Task { await player.load() }
                }
                .buttonStyle(.dsSecondary)
                Button("Close") { dismiss() }
                    .buttonStyle(.plain)
                    .foregroundStyle(DSColors.textSecondary)
            }
            .padding(DSSpacing.lg)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func loadedView(_ workout: WorkoutWithItemsDTO) -> some View {
        // Paged player: page 0 logs sets, page 1 shows live
        // Apple Health stats. Full-screen swipe (`TabView` page
        // style) with dots plus explicit links — swipe alone is
        // undiscoverable mid-workout. The toolbar (Close/Finish)
        // and timer pill stay outside the pages so both share them.
        TabView(selection: $selectedPage) {
            logPage(workout)
                .tag(0)
            BetaFeature {
                liveHealthPage
            }
            .tag(1)
        }
        .tabViewStyle(.page(indexDisplayMode: .automatic))
        // Sticky live timer, pinned under the nav bar while a
        // timer runs. Outside the page content so logging
        // rows never shift under it; zero height when idle or
        // once a completed timer auto-dismisses below.
        .safeAreaInset(edge: .top, spacing: 0) {
            if timerStore.isActive && !hideCompletedPill {
                PlayerTimerPill(store: timerStore) {
                    timerSetupBlock = nil
                    showingTimerSheet = true
                }
            }
        }
        .sheet(isPresented: $showingTimerSheet) {
            PlayerTimerSheet(
                store: timerStore,
                setup: timerSetupBlock.map(PlayerTimerSetupContext.init(block:)),
                notifier: timerNotifier
            )
        }
        // Every fire-date change re-arms the fallback local
        // notification (starts, round rolls, resumes) or
        // cancels it (pauses, resets, completions).
        .onChange(of: timerStore.endDate) { _, newDate in
            timerNotifier.timerFireDateChanged(newDate, store: timerStore)
        }
        // Auto-dismisses the sticky pill a few seconds after
        // completion. A new start or reset clears the event,
        // which re-shows the pill and cancels the pending hide
        // via the generation key; the task's `isComplete`
        // re-check means only a genuine completion hides it.
        // The store itself is untouched, so the auto-opened
        // sheet keeps its complete state.
        .onChange(of: timerStore.lastEvent) { _, _ in
            hideCompletedPill = false
            pillDismissGeneration += 1
        }
        .task(id: pillDismissGeneration) {
            guard pillDismissGeneration > 0 else { return }
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled else { return }
            if timerStore.isComplete {
                hideCompletedPill = true
            }
        }
        .animation(.easeInOut(duration: 0.25), value: hideCompletedPill)
        .background(DSColors.background.ignoresSafeArea())
    }

    // MARK: - Paged content

    /// Page 0: the existing logging scroll (header + blocks +
    /// finish). The Live page is reached by swiping (page dots
    /// below indicate the second page).
    private func logPage(_ workout: WorkoutWithItemsDTO) -> some View {
        ScrollView {
            VStack(alignment: .leading, spacing: DSSpacing.md) {
                // Another workout owns the shared Health
                // recording — this player tracks nothing until
                // that session ends (see the global pill).
                if healthBusy && healthStore.isRecording && healthStore.claimedWorkoutID != workoutID {
                    Text("Apple Health is recording another workout. End it from the recording pill to track this one.")
                        .font(.footnote)
                        .foregroundStyle(DSColors.textSecondary)
                        .padding(DSSpacing.sm)
                        .frame(maxWidth: .infinity, alignment: .leading)
                        .background(
                            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                                .fill(DSColors.surface)
                        )
                }
                headerCard(workout)
                if let error = player.errorMessage {
                    Text(error)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                        .padding(.horizontal, DSSpacing.xs)
                }
                ForEach(workout.blocks) { block in
                    blockCard(block)
                }
                finishSection
            }
            .padding(DSSpacing.md)
        }
        .scrollDismissesKeyboard(.interactively)
    }

    /// Page 1: live workout recording. Explicit-start only —
    /// nothing records until the user taps Start — and
    /// permission-gated: denial shows an inline note while
    /// logging continues normally. Numbers-only v1 (no route
    /// map). Glanceable from across the gym: hero clock plus
    /// large tiles, no branding or instructional copy.
    private var liveHealthPage: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: DSSpacing.md) {
                if let error = healthStore.errorMessage {
                    Text(error)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
                switch healthStore.recorderState {
                case .idle:
                    liveIdleView
                case .active, .paused:
                    liveActiveView
                case .ended:
                    liveStatsView
                    Text("Saved")
                        .font(.caption.weight(.semibold))
                        .foregroundStyle(DSColors.textSecondary)
                        .frame(maxWidth: .infinity, alignment: .center)
                case .failed(let message):
                    Text(message)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
            .padding(DSSpacing.md)
        }
        .background(DSColors.background.ignoresSafeArea())
    }

    /// Idle Live page: inferred type + override menu and Start.
    /// One functional footnote (outdoor GPS vs indoor) so the
    /// location prompt never surprises; no branding copy.
    private var liveIdleView: some View {
        VStack(alignment: .leading, spacing: DSSpacing.sm) {
            Menu {
                ForEach(WorkoutHealthActivityMapper.selectableTypes, id: \.rawValue) { type in
                    Button {
                        healthStore.selectedType = type
                    } label: {
                        Label(
                            WorkoutHealthActivityMapper.displayName(for: type),
                            systemImage: healthStore.selectedType == type ? "checkmark" : "circle"
                        )
                    }
                }
            } label: {
                HStack {
                    Text(WorkoutHealthActivityMapper.displayName(for: healthStore.selectedType))
                        .font(.title2)
                        .foregroundStyle(DSColors.text)
                    Image(systemName: "chevron.up.chevron.down")
                        .font(.caption)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Workout type")
            if healthStore.armsRouteOnStart {
                Text("Outdoor — starting records a GPS route.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
            Button {
                Task { await healthStore.start() }
            } label: {
                if healthStore.isRequestingAuth {
                    ProgressView().frame(maxWidth: .infinity).frame(height: 48)
                } else {
                    Text("Start tracking").frame(maxWidth: .infinity)
                }
            }
            .buttonStyle(.dsPrimary)
            .disabled(healthStore.isRequestingAuth)
            // Access denied (or restricted): logging is
            // unaffected — this only explains why nothing is
            // recording, with a way back.
            if healthStore.healthSkipped {
                Text("Recording is off, so this workout isn't being tracked.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") {
                    Task { await healthStore.start() }
                }
                .buttonStyle(.dsSecondary)
            }
        }
        .padding(DSSpacing.md)
        .background(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .fill(DSColors.surface)
        )
        .overlay(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
    }

    /// Active Live page: hero clock plus large tiles, readable
    /// at arm's length mid-set. Pause/Resume only — saving
    /// happens through Finish, so this page carries no
    /// instructional copy.
    private var liveActiveView: some View {
        VStack(spacing: DSSpacing.md) {
            liveStatsView
            if healthStore.recorderState == .active {
                Button("Pause") { healthStore.pause() }
                    .buttonStyle(.dsSecondary)
            } else {
                Button("Resume") { healthStore.resume() }
                    .buttonStyle(.dsSecondary)
            }
        }
    }

    /// Hero elapsed clock over a 2-column tile grid: zone,
    /// heart rate, pace (outdoor, while moving), calories,
    /// distance when available. Values use monospaced digits so
    /// they never jump width as they tick.
    private var liveStatsView: some View {
        let stats = healthStore.liveStats
        let zone = HeartRateZones.zone(bpm: stats.heartRateBpm, maxHeartRate: healthStore.maxHeartRate)
        return VStack(spacing: DSSpacing.md) {
            Text(Self.elapsedLabel(stats.elapsedSeconds))
                .font(DSFont.monospacedDigits(64, weight: .bold))
                .foregroundStyle(DSColors.text)
                .frame(maxWidth: .infinity, alignment: .center)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
            StatsGrid {
                if let zone {
                    liveTile(
                        label: "Zone",
                        value: "\(zone)",
                        icon: "speedometer",
                        tint: Self.zoneTint(zone),
                        caption: HeartRateZones.name(for: zone)
                    )
                }
                liveTile(
                    label: "Heart rate",
                    value: stats.heartRateBpm.map { HealthMetric.heartRate.formatted(value: $0) } ?? "—",
                    icon: "heart.fill"
                )
                if healthStore.armsRouteOnStart, let pace = stats.currentPaceSecPerKm {
                    liveTile(
                        label: "Pace",
                        value: Self.paceLabel(secPerKm: pace, distanceUnit: distanceUnit),
                        icon: "gauge.with.dots.needle.67percent"
                    )
                }
                liveTile(
                    label: "Calories",
                    value: stats.activeEnergyKcal.map { HealthMetric.activeEnergy.formatted(value: $0) } ?? "—",
                    icon: "flame.fill"
                )
                if let meters = stats.distanceMeters {
                    liveTile(
                        label: "Distance",
                        value: HealthMetric.distance.formatted(value: meters, distanceUnit: distanceUnit),
                        icon: "map"
                    )
                }
            }
        }
    }

    /// Zone dot tint, mapped onto existing tokens only (no new
    /// colorsets): cool → hot across Z1…Z5. Light/dark variants
    /// come free with the tokens.
    private static func zoneTint(_ zone: Int) -> Color {
        switch zone {
        case 1: return DSColors.info
        case 2: return DSColors.success
        case 3: return DSColors.accent
        case 4: return DSColors.chart2
        default: return DSColors.destructive
        }
    }

    /// Oversized `StatCard`: same surface + tinted-disk idiom,
    /// but a 34pt monospaced value for gym-glance readability.
    private func liveTile(
        label: String,
        value: String,
        icon: String,
        tint: Color? = nil,
        caption: String? = nil
    ) -> some View {
        let color = tint ?? DSColors.accent
        return VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack(spacing: DSSpacing.xs) {
                Image(systemName: icon)
                    .font(.system(size: 14, weight: .semibold))
                    .foregroundStyle(color)
                    .frame(width: 28, height: 28)
                    .background(
                        Circle()
                            .fill(color.opacity(0.12))
                    )
                Text(label)
                    .font(.caption)
                    .foregroundStyle(DSColors.text)
            }
            Text(value)
                .font(DSFont.monospacedDigits(34, weight: .bold))
                .foregroundStyle(DSColors.text)
                .lineLimit(1)
                .minimumScaleFactor(0.6)
            if let caption {
                Text(caption)
                    .font(.caption2)
                    .foregroundStyle(DSColors.textSecondary)
                    .lineLimit(1)
            }
        }
        .frame(maxWidth: .infinity, alignment: .leading)
        .padding(DSSpacing.md)
        .background(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .fill(DSColors.surface)
        )
        .overlay(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
    }

    /// Current-pace label in profile units ("5:24 /km",
    /// "8:41 /mi"). Callers hide the tile when pace is nil
    /// (standing still), so this never formats an infinity.
    static func paceLabel(secPerKm: Double, distanceUnit: String) -> String {
        let perUnit = distanceUnit.lowercased() == "mi" ? secPerKm * 1.609_344 : secPerKm
        let total = max(0, Int(perUnit.rounded()))
        let suffix = distanceUnit.lowercased() == "mi" ? "/mi" : "/km"
        return String(format: "%d:%02d %@", total / 60, total % 60, suffix)
    }

    /// Elapsed label: M:SS under an hour (e.g. 90 → "1:30"),
    /// H:MM:SS beyond (e.g. 3720 → "1:02:00").
    static func elapsedLabel(_ seconds: TimeInterval) -> String {
        let total = max(0, Int(seconds))
        if total >= 3_600 {
            return String(format: "%d:%02d:%02d", total / 3_600, (total % 3_600) / 60, total % 60)
        }
        return String(format: "%d:%02d", total / 60, total % 60)
    }

    // MARK: - Header

    private func headerCard(_ workout: WorkoutWithItemsDTO) -> some View {
        let progress = player.blockProgress()
        let percent = progress.total > 0 ? progress.done * 100 / progress.total : 0
        return VStack(alignment: .leading, spacing: DSSpacing.xs) {
            HStack {
                Text(WorkoutDates.display(workout.scheduledDate))
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(DSColors.text)
                Spacer()
                // Workout status as a solid primary badge (accent
                // fill + on-color text, mirroring the strength
                // type pill) — the block badges stay secondary so
                // the workout state owns the brand colour.
                Text(workout.status.displayName)
                    .font(.caption.weight(.semibold))
                    .padding(.horizontal, DSSpacing.sm)
                    .padding(.vertical, DSSpacing.xxs + 2)
                    .background(Capsule().fill(DSColors.accent))
                    .foregroundStyle(DSColors.onPrimary)
            }
            // Plan description (e.g. session goal) — collapsed by
            // default, hidden when the workout has none.
            if !workout.description.isEmpty {
                descriptionDisclosure(workout.description)
            }
            if progress.total > 0 {
                ProgressView(value: Double(progress.done), total: Double(progress.total))
                // Bottom line: completion left, block counts
                // right-aligned against it.
                HStack {
                    Text("\(percent)% complete")
                    Spacer()
                    Text(workout.progressLabel)
                }
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(DSSpacing.md)
        .background(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .fill(DSColors.surface)
        )
        .overlay(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
    }

    // MARK: - Blocks

    private func blockCard(_ block: WorkoutBlockDetailDTO) -> some View {
        let isCollapsed = collapsedBlockIDs.contains(block.id)
        return VStack(alignment: .leading, spacing: DSSpacing.sm) {
            // Row 1: collapse toggle (chevron + name) and the
            // status menu.
            HStack(alignment: .top, spacing: DSSpacing.sm) {
                Button {
                    withAnimation {
                        if isCollapsed {
                            collapsedBlockIDs.remove(block.id)
                        } else {
                            collapsedBlockIDs.insert(block.id)
                        }
                    }
                } label: {
                    HStack(alignment: .top, spacing: DSSpacing.xs) {
                        Image(systemName: "chevron.right")
                            .font(.footnote.weight(.semibold))
                            .foregroundStyle(DSColors.textSecondary)
                            .rotationEffect(.degrees(isCollapsed ? 0 : 90))
                            .padding(.top, 4)
                        Text(block.blockName)
                            .font(.headline)
                            .foregroundStyle(DSColors.text)
                    }
                }
                .buttonStyle(.plain)
                .accessibilityLabel(isCollapsed ? "Expand \(block.blockName)" : "Collapse \(block.blockName)")
                Spacer()
                // Trailing controls centre-align against each
                // other — the 32pt timer hit-target is taller
                // than the status pill — while the row as a
                // whole stays top-anchored to the block name.
                HStack(alignment: .center, spacing: DSSpacing.xs) {
                    blockTimerMenu(block)
                    blockStatusMenu(block)
                }
            }
            // Row 2: block type left, logged counts right. Lives in
            // the card's full-width stack (not inside the toggle
            // button) — a Spacer inside a Button label collapses,
            // so right-alignment only works out here.
            HStack {
                Text(block.blockType.displayName)
                Spacer()
                Text(loggedLabel(for: block))
            }
            .font(.subheadline)
            .foregroundStyle(DSColors.textSecondary)
            // Row 3: time structure under the type (timed types
            // only) — its own line, so no separator ever dangles
            // after it or wraps awkwardly on narrow screens.
            if !block.timingSummary.isEmpty {
                Text(block.timingSummary)
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
            }
            if !isCollapsed {
                Divider().background(DSColors.separator)
                // Plan description from the block catalogue (e.g.
                // coaching cues) — same collapsible treatment as
                // the workout description, hidden when empty.
                if !block.blockDescription.isEmpty {
                    descriptionDisclosure(block.blockDescription)
                }
                if block.items.isEmpty {
                    Text("This block's exercises are unavailable — you can still mark the block skipped.")
                        .font(.footnote)
                        .foregroundStyle(DSColors.textSecondary)
                }
                ForEach(block.items) { item in
                    itemView(item)
                    if item.id != block.items.last?.id {
                        Divider().background(DSColors.separator)
                    }
                }
            }
            // The block's single call to action — the status chip
            // menu alone wasn't discoverable, and a separate Log
            // button proved duplicative. Tapping first autosubmits
            // every item's valid rows, then performs the same
            // status-only Done write as the menu's Done row (a
            // no-op when already done). Manual check-off stays the
            // source of truth.
            //
            // Done blocks keep the button while they hold valid
            // unsubmitted drafts: reopening a finished workout
            // leaves its blocks done (only the workout status
            // flips), so without this, sets typed into a done
            // block could never be posted. Compact secondary
            // chrome — the full-size "Finish workout" button
            // below stays the screen's only filled primary
            // action, so the two can never be confused.
            if block.status != .done || player.hasValidDrafts(in: block) {
                Button {
                    Task { await markBlockDone(block) }
                } label: {
                    if markingBlockIDs.contains(block.id) || player.isLoggingAny(in: block) {
                        ProgressView()
                            .tint(DSColors.accent)
                            .frame(maxWidth: .infinity)
                    } else {
                        Text(block.status == .done ? "Log additional sets" : "Mark as Done")
                            .frame(maxWidth: .infinity)
                    }
                }
                .buttonStyle(.dsSecondaryCompact)
                .disabled(markingBlockIDs.contains(block.id) || player.isLoggingAny(in: block))
            }
        }
        .padding(DSSpacing.md)
        .background(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .fill(DSColors.surface)
        )
        .overlay(
            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                .stroke(DSColors.separator, lineWidth: 0.5)
        )
    }

    /// Autosubmits the block's valid drafts, then marks it done
    /// via the shared store (status-only write) and refreshes the
    /// player's local statuses so the header and hints catch up
    /// without refetching items. A failed submit aborts before
    /// the status write — nothing is marked done unlogged, and
    /// the error stays on screen for retry.
    private func markBlockDone(_ block: WorkoutBlockDetailDTO) async {
        markingBlockIDs.insert(block.id)
        defer { markingBlockIDs.remove(block.id) }
        guard await player.logAllValid(in: block) else { return }
        await workoutStore.setBlockStatus(
            workoutID: workoutID,
            workoutBlockID: block.id,
            status: .done
        )
        await player.refreshBlockStatuses(from: workoutStore)
        // Fold the finished block away on success (a failed
        // submit or status write above leaves it open).
        if workoutStore.errorMessage == nil {
            withAnimation {
                collapsedBlockIDs.insert(block.id)
            }
        }
    }

    /// Collapsed-by-default "Description" disclosure, shared by
    /// the workout header and the block bodies (same label,
    /// styling, and behaviour in both places). The content text
    /// is pinned leading — without the explicit frame it drifts
    /// to the centre of the card width.
    private func descriptionDisclosure(_ text: String) -> some View {
        DisclosureGroup("Description") {
            Text(text)
                .font(.body)
                .foregroundStyle(DSColors.text)
                .frame(maxWidth: .infinity, alignment: .leading)
                .multilineTextAlignment(.leading)
        }
        .font(.footnote.weight(.semibold))
        .foregroundStyle(DSColors.textSecondary)
        .tint(DSColors.accent)
    }

    /// "X/Y logged" for the header's right-aligned counts slot.
    /// The type and time structure render as their own rows, so
    /// this carries counts only.
    private func loggedLabel(for block: WorkoutBlockDetailDTO) -> String {
        let logged = block.items.filter { player.isItemDone(itemID: $0.id) }.count
        return "\(logged)/\(block.items.count) logged"
    }

    /// Whole- or one-decimal-minute label for an AMRAP cap in
    /// seconds (e.g. 600 → "10", 90 → "1.5"), so the plan
    /// timer row reads "AMRAP 10mins (plan)".
    private func amrapCapLabel(_ capSeconds: Int) -> String {
        let mins = Double(capSeconds) / 60
        if mins == mins.rounded() {
            return String(Int(mins))
        }
        var text = String(format: "%.1f", mins)
        if text.hasSuffix(".0") {
            text = String(text.dropLast(2))
        }
        return text
    }

    /// Per-block timer picker. A single quiet icon — the
    /// manual choice of rest / EMOM / AMRAP lives one tap
    /// away without costing the card any vertical space, and
    /// it stays visible when the block is collapsed. The menu
    /// stays short on purpose: the block's own programmed
    /// timer first (when it has one), then the three standard
    /// rests. "Custom…" covers everything else. Starts land
    /// in the shared `timerStore`, replacing any running
    /// timer.
    private func blockTimerMenu(_ block: WorkoutBlockDetailDTO) -> some View {
        let timerIsOurs = timerStore.isActive && timerStore.blockID == block.id
        // The plan row only appears for genuinely distinct
        // configs — EMOM rounds, an AMRAP cap, or a rest that
        // isn't one of the three standards below (avoids a
        // duplicate row when the plan rest IS 30/60/90).
        return Menu {
            if block.blockType == .emom, block.rounds > 0, block.intervalSeconds > 0 {
                Button("\(emomSummary(minutes: block.rounds, intervalSeconds: block.intervalSeconds)) (plan)") {
                    timerStore.startEMOM(
                        rounds: block.rounds,
                        intervalSeconds: block.intervalSeconds,
                        blockID: block.id,
                        blockName: block.blockName
                    )
                }
            }
            if block.blockType == .amrap, block.timeCapSeconds > 0 {
                Button("AMRAP \(amrapCapLabel(block.timeCapSeconds))mins (plan)") {
                    timerStore.startAMRAP(
                        capSeconds: block.timeCapSeconds,
                        blockID: block.id,
                        blockName: block.blockName
                    )
                }
            }
            if block.restSeconds > 0, ![30, 60, 90].contains(block.restSeconds) {
                Button("Rest \(block.restSeconds)s (plan)") {
                    timerStore.startRest(
                        seconds: block.restSeconds,
                        blockID: block.id,
                        blockName: block.blockName
                    )
                }
            }
            Button("Rest 30s") {
                timerStore.startRest(seconds: 30, blockID: block.id, blockName: block.blockName)
            }
            Button("Rest 60s") {
                timerStore.startRest(seconds: 60, blockID: block.id, blockName: block.blockName)
            }
            Button("Rest 90s") {
                timerStore.startRest(seconds: 90, blockID: block.id, blockName: block.blockName)
            }
            Divider()
            Button("Custom…") {
                timerSetupBlock = block
                showingTimerSheet = true
            }
            if timerStore.isActive {
                Button("Open timer") {
                    timerSetupBlock = nil
                    showingTimerSheet = true
                }
            }
        } label: {
            Image(systemName: Icons.timer)
                .font(.body)
                .foregroundStyle(timerIsOurs ? DSColors.accent : DSColors.textSecondary)
                .frame(width: 32, height: 32)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Start timer for \(block.blockName)")
        .accessibilityHint("Rest, EMOM, or AMRAP timer for this block")
    }

    private func blockStatusMenu(_ block: WorkoutBlockDetailDTO) -> some View {
        Menu {
            ForEach(WorkoutBlockStatusDTO.allCases, id: \.self) { status in
                Button {
                    Task {
                        await workoutStore.setBlockStatus(
                            workoutID: workoutID,
                            workoutBlockID: block.id,
                            status: status
                        )
                        await player.refreshBlockStatuses(from: workoutStore)
                        // Keep collapse in sync with check-offs:
                        // finished blocks fold away, reopened ones
                        // unfold. The chevron still overrides after.
                        if workoutStore.errorMessage == nil {
                            withAnimation {
                                if status == .done || status == .skipped {
                                    collapsedBlockIDs.insert(block.id)
                                } else {
                                    collapsedBlockIDs.remove(block.id)
                                }
                            }
                        }
                    }
                } label: {
                    Label(
                        status.displayName,
                        systemImage: block.status == status ? "checkmark" : "circle"
                    )
                }
            }
        } label: {
            // Secondary badge chrome (inverted fill + on-color
            // text, mirroring the cardio type pill) — the status
            // reads from the label, not from brand colour.
            Text(block.status.displayName)
                .font(.caption.weight(.semibold))
                .padding(.horizontal, DSSpacing.sm)
                .padding(.vertical, DSSpacing.xxs + 2)
                .background(Capsule().fill(DSColors.secondary))
                .foregroundStyle(DSColors.onSecondary)
        }
        .buttonStyle(.plain)
        .accessibilityLabel("Mark \(block.blockName) \(block.status.displayName)")
    }

    // MARK: - Items

    private func itemView(_ item: BlockItemDTO) -> some View {
        let skipped = player.skippedItemIDs.contains(item.id)
        let logged = player.loggedCount(itemID: item.id)
        // No type badge: strength vs cardio is already evident
        // from the editors below (set rows vs session row), and
        // the block subtitle carries the block type.
        return VStack(alignment: .leading, spacing: DSSpacing.xs) {
            // Title row with Skip docked to it — the toggle reads
            // as part of the exercise header rather than floating
            // a row below the counts.
            HStack(alignment: .top, spacing: DSSpacing.xs) {
                VStack(alignment: .leading, spacing: 2) {
                    // The exercise name opens its reference
                    // detail sheet (details + stats + chart +
                    // full history, read-only). Name plus
                    // chevron marks it tappable; the target
                    // text below stays static.
                    Button {
                        openExerciseDetail(for: item)
                    } label: {
                        HStack(spacing: 2) {
                            Text(item.exerciseName)
                                .font(.body.weight(.semibold))
                                .foregroundStyle(DSColors.text)
                                .lineLimit(2)
                                .multilineTextAlignment(.leading)
                            Image(systemName: "chevron.right")
                                .font(.caption.weight(.semibold))
                                .foregroundStyle(DSColors.textSecondary)
                        }
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Show details and history for \(item.exerciseName)")
                    .accessibilityHint("Opens the exercise details and full history")
                    if !item.targetText.isEmpty {
                        Text(item.targetText)
                            .font(.subheadline)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
                Spacer()
                Button(skipped ? "Unskip" : "Skip") {
                    player.toggleSkip(itemID: item.id)
                }
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
                .disabled(player.isLogging(itemID: item.id))
            }
            // The logged count opens this exercise's sets for
            // the workout (history sheet with edit + delete).
            // Accent + chevron mark it tappable. Rendered only
            // when something is logged, so the row takes no
            // space otherwise.
            if logged > 0 {
                Button {
                    historyItem = item
                } label: {
                    HStack(spacing: 2) {
                        Text(logged == 1 ? "1 logged" : "\(logged) logged")
                        Image(systemName: "chevron.right")
                    }
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(DSColors.accent)
                }
                .buttonStyle(.plain)
                .accessibilityLabel("Show logged sets for \(item.exerciseName)")
                .accessibilityHint("Opens the set history for this workout")
            }
            if !skipped {
                editorRows(item)
                // Collapsible notes ride along with the item's
                // next log call (Mark as Done autosubmits).
                // Hidden while skipped — a skipped exercise logs
                // nothing, so there is nothing to attach notes
                // to. The text is kept in the store, so
                // unskipping restores it.
                notesDisclosure(item)
            } else {
                Text("Skipped — no sets will be logged for this exercise.")
                    .font(.footnote)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(.vertical, DSSpacing.xs)
    }

    /// Collapsible per-exercise notes, mirroring `NewSetView`'s
    /// notes section. Collapsed by default so it costs no screen
    /// room; every keystroke persists to the on-device snapshot.
    private func notesDisclosure(_ item: BlockItemDTO) -> some View {
        DisclosureGroup("Notes") {
            TextField(
                "Optional — e.g. felt easy, left knee niggle",
                text: notesBinding(for: item.id),
                axis: .vertical
            )
            .font(.body)
            .foregroundStyle(DSColors.text)
        }
        .font(.footnote.weight(.semibold))
        .foregroundStyle(DSColors.textSecondary)
        .tint(DSColors.accent)
    }

    private func notesBinding(for itemID: String) -> Binding<String> {
        Binding(
            get: { player.notes[itemID] ?? "" },
            set: { player.setNotes($0, for: itemID) }
        )
    }

    /// Opens the exercise reference sheet for a planned item.
    /// Presents a fallback built from the item immediately
    /// (id + name + type are enough for the history/chart
    /// endpoints), then upgrades it to the rich catalogue
    /// match in the background when the lazy catalogue load
    /// lands. Catalogue failure simply leaves the fallback —
    /// the history view surfaces its own load errors.
    private func openExerciseDetail(for item: BlockItemDTO) {
        // One sheet at a time: a logged-sets sheet never
        // underlaps the detail sheet or vice versa.
        historyItem = nil
        if let cached = exerciseCatalog?[item.exerciseID] {
            exerciseDetail = cached
            return
        }
        exerciseDetail = ExerciseDTO(
            id: item.exerciseID,
            name: item.exerciseName,
            description: "",
            videoURL: "",
            imgURL: "",
            imageURL: "",
            type: item.exerciseType
        )
        Task {
            do {
                if exerciseCatalog == nil {
                    let exercises = try await env.api.listExercises()
                    exerciseCatalog = Dictionary(
                        uniqueKeysWithValues: exercises.map { ($0.id, $0) }
                    )
                }
                if let match = exerciseCatalog?[item.exerciseID] {
                    exerciseDetail = match
                }
            } catch let error as APIError {
                if case .unauthorized = error { return }
                // Non-fatal: the fallback stays and the
                // history view reports its own errors.
            } catch {
                // Non-fatal — see above.
            }
        }
    }

    private func editorRows(_ item: BlockItemDTO) -> some View {
        // Editor writes go through `setDrafts(for:)` so every
        // keystroke persists to the on-device snapshot.
        let rows = Binding(
            get: { player.drafts[item.id] ?? [] },
            set: { player.setDrafts($0, for: item.id) }
        )
        let firstRow = Binding(
            get: { player.drafts[item.id]?.first ?? SetDraft(distanceUnit: distanceUnit) },
            set: { player.setDrafts([$0], for: item.id) }
        )
        return VStack(spacing: DSSpacing.xs) {
            if item.isCardio {
                // Cardio is a one-shot session, not repeated sets:
                // exactly one fixed row, no add/remove affordances
                // (mirrors `NewSetView`).
                SetRowEditor(
                    set: firstRow,
                    weightUnit: weightUnit,
                    distanceUnit: distanceUnit,
                    isCardioMode: true
                )
            } else {
                ForEach(rows) { $row in
                    HStack(alignment: .top, spacing: DSSpacing.xs) {
                        SetRowEditor(
                            set: $row,
                            weightUnit: weightUnit,
                            distanceUnit: distanceUnit,
                            isCardioMode: false
                        )
                        Button {
                            player.removeDraftRow(itemID: item.id, id: row.id)
                        } label: {
                            Image(systemName: "minus.circle")
                                .foregroundStyle(DSColors.textSecondary)
                        }
                        .buttonStyle(.plain)
                        .padding(.top, DSSpacing.sm)
                        .accessibilityLabel("Remove set row")
                    }
                }
                // Same affordance as `NewSetView.setsSection`: a
                // Label with the plus.circle icon and no custom
                // font, so the row reads identically in both
                // places. The plain style + accent keeps it
                // legible on the card surface (a Form row gets
                // that tint for free). Pinned leading to match the
                // exercise-entry form alignment.
                Button {
                    player.addDraftRow(itemID: item.id)
                } label: {
                    Label("Add set", systemImage: "plus.circle")
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                .buttonStyle(.plain)
                .foregroundStyle(DSColors.accent)
            }
        }
    }

    // MARK: - Finish

    private var finishSection: some View {
        Button {
            showingFinishConfirm = true
        } label: {
            if isFinishing {
                ProgressView()
                    .frame(maxWidth: .infinity)
                    .frame(height: 48)
            } else {
                Text("Finish workout")
                    .frame(maxWidth: .infinity)
            }
        }
        .buttonStyle(.dsPrimary)
        .disabled(isFinishing)
        // Extra breathing room above the workout-level action:
        // the last block's outlined "Mark as Done" sits just
        // above, and the added 8pt (24pt total with the stack
        // spacing) keeps the two from reading as one group.
        .padding(.top, DSSpacing.sm)
    }

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }

    /// Marks the workout completed via the status-only endpoint
    /// (plan and block check-offs untouched), clears the on-device
    /// drafts, and dismisses. Partial completion is allowed —
    /// pending blocks stay pending. Ends the Health session first
    /// (best-effort: a Health failure never blocks the Hylete save).
    private func finish() async {
        guard !isFinishing else { return }
        isFinishing = true
        defer { isFinishing = false }
        await healthStore.endAndSave()
        await workoutStore.setStatus(id: workoutID, status: .completed)
        if workoutStore.errorMessage == nil {
            player.clearSnapshot()
            dismiss()
        }
    }
}

/// One planned exercise's logged sets for this workout, newest
/// first, with swipe-to-edit and delete. Opened from the
/// player's tappable "X logged" row. Entries come from the
/// player store's retained buckets (resume fetch + everything
/// logged this session), so no extra fetch is needed and newly
/// logged sets appear immediately.
///
/// Editing reuses `EditSetView` (it performs the PUT; the save
/// callback splices the server-confirmed row into the bucket).
/// Deletes are pessimistic — the row leaves only on a confirmed
/// API delete, so counts can never desync. Read-only callers
/// (the detail view) render `HistorySetRow`s directly instead.
private struct WorkoutLoggedSetsSheet: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var player: WorkoutPlayerStore
    let item: BlockItemDTO

    @State private var editingEntry: ExerciseEntryDTO?
    @State private var entryPendingDelete: ExerciseEntryDTO?
    @State private var showingDeleteConfirm: Bool = false

    private var entries: [ExerciseEntryDTO] {
        player.loggedEntries(itemID: item.id)
    }

    var body: some View {
        NavigationStack {
            List {
                ForEach(entries) { entry in
                    HistorySetRow(
                        entry: entry,
                        weightUnit: weightUnit,
                        distanceUnit: distanceUnit,
                        showsNotes: true
                    )
                    .opacity(player.isDeleting(entryID: entry.id) ? 0.5 : 1)
                    .swipeActions(edge: .leading, allowsFullSwipe: true) {
                        Button {
                            editingEntry = entry
                        } label: {
                            Label("Edit", systemImage: Icons.edit)
                                .labelStyle(.iconOnly)
                        }
                        .tint(Color(.systemGray2))
                    }
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
            .navigationTitle(item.exerciseName)
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
            .sheet(item: $editingEntry) { entry in
                EditSetView(exerciseEntry: entry) { updated in
                    player.updateLoggedEntry(updated)
                }
                .environmentObject(env)
                .environmentObject(authStore)
            }
            .alert(
                item.isCardio ? "Delete this session?" : "Delete this set?",
                isPresented: $showingDeleteConfirm,
                presenting: entryPendingDelete
            ) { entry in
                Button("Delete", role: .destructive) {
                    Task { await player.deleteLoggedEntry(entry) }
                }
                Button("Cancel", role: .cancel) {}
            } message: { _ in
                Text(item.isCardio
                    ? "This will permanently remove the session from this workout's history."
                    : "This will permanently remove the set from this workout's history.")
            }
            // The last set deleted empties the sheet's reason to
            // exist — close it so the player (whose "X logged"
            // row is now gone) is what the user sees.
            .onChange(of: entries) { _, newEntries in
                if newEntries.isEmpty {
                    dismiss()
                }
            }
        }
    }

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }
}

/// Global Apple Health recording pill. Rendered by
/// `MainTabView` on every tab while a session is live —
/// including after the player was closed — so a recording
/// is never invisible (and never silently lost). Chrome
/// mirrors `PlayerTimerPill` (surface card, bottom divider,
/// full-width tap target, quick pause/resume + expand).
struct HealthRecordingPill: View {
    @ObservedObject var store: PlayerHealthStore
    var onExpand: () -> Void

    private var activityName: String {
        let type = store.activityTypeMirror ?? store.selectedType
        return WorkoutHealthActivityMapper.displayName(for: type)
    }

    var body: some View {
        HStack(spacing: DSSpacing.sm) {
            Circle()
                .fill(Color.red)
                .frame(width: 10, height: 10)
            VStack(alignment: .leading, spacing: 2) {
                Text("Recording workout")
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                TimelineView(.periodic(from: .now, by: 1)) { _ in
                    Text("\(activityName) · \(WorkoutPlayerView.elapsedLabel(store.liveStats.elapsedSeconds))")
                        .font(.headline.monospacedDigit())
                        .foregroundStyle(DSColors.text)
                        .contentTransition(.numericText())
                }
            }
            Spacer()
            Button {
                if store.recorderState == .active {
                    store.pause()
                } else {
                    store.resume()
                }
            } label: {
                Image(systemName: store.recorderState == .active ? "pause.fill" : "play.fill")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(DSColors.accent)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .accessibilityLabel(store.recorderState == .active ? "Pause recording" : "Resume recording")
            Button {
                onExpand()
            } label: {
                Image(systemName: "chevron.up")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Open recording details")
        }
        .padding(.horizontal, DSSpacing.md)
        .padding(.vertical, DSSpacing.sm)
        .background(DSColors.surface)
        .overlay(Divider().background(DSColors.separator), alignment: .bottom)
        .contentShape(Rectangle())
        .onTapGesture { onExpand() }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("Recording \(activityName)")
        .accessibilityHint("Opens the recording details")
    }
}

/// Mini sheet for the global recording pill: live status,
/// pause/resume, End & Save, Discard, and a jump back into
/// the recorded workout's player. Entry-point only — no set
/// logging lives here, so player session state is untouched.
struct HealthRecordingSheet: View {
    @Environment(\.dismiss) private var dismiss
    @EnvironmentObject private var authStore: AuthStore

    @ObservedObject var store: PlayerHealthStore
    var onOpenWorkout: () -> Void

    @State private var showingDiscardConfirm = false

    private var activityName: String {
        let type = store.activityTypeMirror ?? store.selectedType
        return WorkoutHealthActivityMapper.displayName(for: type)
    }

    private var distanceUnit: String {
        authStore.currentUser?.distanceUnit ?? "km"
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: 0) {
                List {
                    Section("Status") {
                        statusRow("Activity", activityName)
                        statusRow("State", store.recorderState == .paused ? "Paused" : "Recording")
                        TimelineView(.periodic(from: .now, by: 1)) { _ in
                            statusRow("Elapsed", WorkoutPlayerView.elapsedLabel(store.liveStats.elapsedSeconds))
                        }
                        if let hr = store.liveStats.heartRateBpm {
                            statusRow("Heart rate", "\(Int(hr)) bpm")
                        }
                        if let kcal = store.liveStats.activeEnergyKcal {
                            statusRow("Calories", String(format: "%.0f kcal", kcal))
                        }
                        if let meters = store.liveStats.distanceMeters {
                            statusRow("Distance", formattedDistance(meters))
                        }
                    }
                }
                // Actions live outside the List: a grouped
                // section card would wrap and clip them (its
                // own corner radius sliced the secondary
                // outlines). Out here they render exactly as
                // drawn — and stay on screen without scrolling.
                VStack(spacing: DSSpacing.sm) {
                    // Controls share one row, both secondary —
                    // neither competes with the full-width
                    // End & Save below.
                    HStack(spacing: DSSpacing.sm) {
                        Button(store.recorderState == .paused ? "Resume" : "Pause") {
                            if store.recorderState == .paused {
                                store.resume()
                            } else {
                                store.pause()
                            }
                        }
                        .buttonStyle(.dsSecondary(cornerRadius: DSSpacing.cornerRadiusSmall))
                        Button("Open workout") {
                            onOpenWorkout()
                        }
                        .buttonStyle(.dsSecondary(cornerRadius: DSSpacing.cornerRadiusSmall))
                    }
                    Button {
                        Task {
                            await store.endAndSave()
                            dismiss()
                        }
                    } label: {
                        if store.isSaving {
                            ProgressView()
                                .frame(maxWidth: .infinity)
                        } else {
                            Text("End & Save")
                                .frame(maxWidth: .infinity)
                        }
                    }
                    .buttonStyle(.dsPrimary(cornerRadius: DSSpacing.cornerRadiusSmall))
                    .disabled(store.isSaving)
                }
                .padding(DSSpacing.md)
                .background(DSColors.background)
            }
            .navigationTitle("Recording")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                // Discard lives in the header opposite Done —
                // destructive role keeps it out of the action
                // rows and impossible to tap by accident.
                ToolbarItem(placement: .topBarLeading) {
                    Button("Discard", role: .destructive) {
                        showingDiscardConfirm = true
                    }
                    .disabled(store.isSaving)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
            // Centered confirmation, matching the Delete
            // workout/block pattern elsewhere in the app
            // (a confirmationDialog would dock to the bottom
            // of the screen instead).
            .alert("Discard this recording?", isPresented: $showingDiscardConfirm) {
                Button("Discard", role: .destructive) {
                    store.discard()
                    dismiss()
                }
                Button("Keep recording", role: .cancel) {}
            } message: {
                Text("The Apple Health workout is abandoned without saving. Logged sets are unaffected.")
            }
        }
    }

    private func statusRow(_ label: String, _ value: String) -> some View {
        HStack {
            Text(label)
                .foregroundStyle(DSColors.textSecondary)
            Spacer()
            Text(value)
                .monospacedDigit()
                .foregroundStyle(DSColors.text)
        }
        .font(.body)
    }

    private func formattedDistance(_ meters: Double) -> String {
        if distanceUnit.lowercased() == "mi" {
            return String(format: "%.2f mi", meters / 1_609.344)
        }
        return String(format: "%.2f km", meters / 1_000)
    }
}

#Preview {
    WorkoutPlayerView(
        workoutID: "preview",
        workoutStore: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )),
        player: WorkoutPlayerStore(
            workoutID: "preview",
            api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )
        ),
        healthStore: PlayerHealthStore()
    )
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
