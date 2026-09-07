import SwiftUI

/// The "Today" tab. The layout mirrors the web dashboard:
/// action cards at the top, a "Last 7 Days" stats row, the
/// "Most Popular Exercises (7d)" donut, and a week calendar
/// whose selected day lists that day's sets. Tapping a set row
/// pushes the exercise's history view, matching the web's
/// eye-icon link.
///
/// The calendar replaces the old always-on "Recent Sets"
/// list: weeks are fetched lazily via the entry-range endpoint
/// as the user pages back and forth, cached per day for the
/// session (`entriesByDay`), and the donut keeps using the
/// fixed last-7-days snapshot fetched at load time.
struct DashboardView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    /// Last-7-days snapshot feeding the donut only; the
    /// calendar reads from `entriesByDay` instead so lazy
    /// paging doesn't change what the donut shows.
    @State private var recentEntries: [ExerciseEntryDTO] = []
    @State private var exerciseLookup: [String: ExerciseDTO] = [:]
    @State private var isLoading: Bool = false
    @State private var errorMessage: String?

    // MARK: Calendar state

    /// The day tapped in the week calendar (any instant;
    /// normalise with `startOfDay` before use).
    @State private var selectedDate: Date = Date()
    /// Session cache of fetched exercise entries bucketed by
    /// local start-of-day. Weeks are merged in as the user
    /// pages around; buckets are overwritten wholesale when
    /// their week is refetched.
    @State private var entriesByDay: [Date: [ExerciseEntryDTO]] = [:]
    /// Week starts currently being fetched (drives the inline
    /// spinner under the selected day).
    @State private var loadingWeeks: Set<Date> = []
    /// Week starts already present in `entriesByDay`; guards
    /// against refetching a page the user has already visited.
    @State private var loadedWeeks: Set<Date> = []
    /// Distinguishes the first appearance (where `.task
    /// { load() }` alone fetches) from every *reappearance*
    /// (pop-back from a history view, tab re-entry), which
    /// additionally triggers the `onAppear` cache-invalidation
    /// refresh because entries may have been edited or deleted
    /// elsewhere. Note that `.task` also restarts on
    /// reappearance per Apple's documented behaviour, so both
    /// paths fire on a reappear; the single-flight guard in
    /// `load()` coalesces them into one network pass.
    @State private var hasAppeared: Bool = false
    /// Bound navigation path so pop-back is detectable. The
    /// stack only ever pushes `ExerciseDTO` destinations, so a
    /// plain array suffices; an empty path after a non-empty one
    /// means the user returned from a pushed view.
    @State private var navigationPath: [ExerciseDTO] = []
    /// Serialises the reappear-refresh triggers (`onAppear` and
    /// path-change can both fire for one pop-back).
    @State private var isRefreshing: Bool = false

    @State private var showingNewSet: Bool = false
    @State private var showingTimer: Bool = false

    var body: some View {
        NavigationStack(path: $navigationPath) {
            content
                .navigationTitle("Dashboard")
                .navigationDestination(for: ExerciseDTO.self) { exercise in
                    ExerciseHistoryView(exercise: exercise)
                }
                // NOTE: attached to the content INSIDE the stack,
                // not to the NavigationStack itself — the outer
                // container never disappears/appears as children
                // are pushed and popped, so its onAppear would
                // fire exactly once per tab lifetime.
                .onAppear {
                    if hasAppeared {
                        refreshIfIdle()
                    }
                    hasAppeared = true
                }
                .sheet(isPresented: $showingNewSet, onDismiss: {
                    // `refresh()` is `@MainActor`; this unstructured
                    // task is the one place an unannotated call
                    // would otherwise compile-time error on us,
                    // so pin the body to the main actor explicitly
                    // rather than relying on inference.
                    Task { @MainActor in await refresh() }
                }) {
                    NewSetView()
                        .environmentObject(env)
                }
                .sheet(isPresented: $showingTimer) {
                    TimerView()
                }
        }
        .task { await load() }
        .refreshable { await refresh() }
        .onChange(of: navigationPath) { _, newPath in
            // Deterministic pop-back signal (covers devices/
            // OS versions where root onAppear semantics are
            // unreliable). Only an empty path — i.e. back at
            // the dashboard itself — needs a reload.
            if newPath.isEmpty && hasAppeared {
                refreshIfIdle()
            }
        }
        .onChange(of: selectedDate) { _, newSelection in
            // `ensureWeekLoaded` is `@MainActor`; pin the task
            // body to the main actor explicitly so the day
            // change handler can't quietly hop off-main and
            // trip SwiftUI's iOS 17 main-thread `@State`
            // assertion.
            Task { @MainActor in await ensureWeekLoaded(for: newSelection) }
        }
    }

    /// Runs `refresh()` unless one is already in flight, so the
    /// overlapping reappear signals coalesce into one reload.
    ///
    /// The task body is explicitly pinned to `@MainActor` —
    /// `refresh()` is `@MainActor` so the compiler would error
    /// here anyway, but the annotation makes the intent clear:
    /// this unstructured task must not hop to the global
    /// executor, which is exactly what was crashing the
    /// dashboard on reappear (the original bug).
    private func refreshIfIdle() {
        guard !isRefreshing else { return }
        isRefreshing = true
        Task { @MainActor in
            await refresh()
            isRefreshing = false
        }
    }

    // MARK: - Content states

    @ViewBuilder
    private var content: some View {
        if isLoading && recentEntries.isEmpty && entriesByDay.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let errorMessage, recentEntries.isEmpty && entriesByDay.isEmpty {
            errorState(errorMessage)
        } else {
            loadedScrollView
        }
    }

    /// Renders the dashboard's normal layout: action cards,
    /// donut, calendar. The donut section is always attempted
    /// and hides itself when the last-7-days snapshot has
    /// nothing to chart (original dashboard behaviour); the
    /// calendar's own per-day empty states handle sparse days.
    @ViewBuilder
    private var loadedScrollView: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: DSSpacing.md) {
                actionCards
                donutSection
                sectionHeader("Calendar")
                WeekCalendarView(
                    selection: $selectedDate,
                    isBusy: { day in
                        !(entriesByDay[CalendarMath.startOfDay(day)] ?? []).isEmpty
                    }
                ) { day in
                    SelectedDaySetList(
                        date: day,
                        entries: entriesByDay[day] ?? [],
                        isLoading: loadingWeeks.contains(CalendarMath.startOfWeek(for: day)),
                        weightUnit: weightUnit,
                        distanceUnit: authStore.currentUser?.distanceUnit ?? "km",
                        exerciseLookup: exerciseLookup
                    )
                }
            }
            .padding(DSSpacing.md)
        }
        .background(DSColors.background.ignoresSafeArea())
    }

    private var actionCards: some View {
        HStack(spacing: DSSpacing.xs) {
            ActionCard(label: "Add Set", systemImage: Icons.addSet) {
                showingNewSet = true
            }
            ActionCard(label: "Timer", systemImage: Icons.timer) {
                showingTimer = true
            }
        }
    }

    @ViewBuilder
    private var donutSection: some View {
        let buckets = popularExerciseBuckets(recentEntries)
        if !buckets.isEmpty {
            VStack(alignment: .leading, spacing: DSSpacing.xs) {
                sectionHeader("7 Day History")
                DashboardDonutChart(buckets: buckets)
                    .padding(DSSpacing.md)
                    .frame(maxWidth: .infinity)
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
    }

    private func sectionHeader(_ text: String) -> some View {
        Text(text)
            .font(.headline)
            .foregroundStyle(DSColors.text)
            .padding(.horizontal, DSSpacing.xs)
    }

    private func errorState(_ message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Try again") { Task { await load() } }
                .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Derived data

    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    // MARK: - Networking

    /// Initial load: the 7-day snapshot for the donut plus the
    /// full exercise catalogue (needed so every list row can
    /// navigate to its exercise's history view without a fetch
    /// per row), then the selected day's week for the calendar.
    /// All fetches share the same 401 handling, so an expired
    /// session signs the user out exactly once.
    ///
    /// `@MainActor` because this method mutates `@State`
    /// (`isLoading`, `errorMessage`, `recentEntries`,
    /// `exerciseLookup`, plus `entriesByDay`/`loadingWeeks`/
    /// `loadedWeeks` via `ensureWeekLoaded`). SwiftUI enforces
    /// main-thread `@State` access on iOS 17, and the
    /// reappear-refresh path in particular used to launch this
    /// from an unstructured `Task { ... }` which runs on the
    /// global executor — pinning the method to `@MainActor`
    /// turns that into a compile-time error at the call site.
    ///
    /// Single-flight: `.task` restarts on every reappear (per
    /// Apple's documented behaviour) AND the `onAppear` path
    /// triggers its own refresh, so two loads can be requested
    /// in the same transaction. A caller arriving while one is
    /// already in flight returns immediately; the in-flight
    /// pass refetches everything anyway, and its trailing
    /// `ensureWeekLoaded` picks up any week markers a
    /// concurrent `refresh()` cleared while it was still
    /// awaiting the network.
    @MainActor
    private func load() async {
        guard !isLoading else { return }
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            async let entriesTask = env.api.listExerciseEntries(days: 7)
            async let exercisesTask = env.api.listExercises()
            let (fetchedEntries, fetchedExercises) = try await (entriesTask, exercisesTask)
            recentEntries = fetchedEntries
            // Duplicate-tolerant lookup build. `Dictionary(
            // uniqueKeysWithValues:)` calls fatalError on a
            // repeated key, which would take the whole app down
            // over a server-side data oddity; last value wins
            // instead.
            var lookup: [String: ExerciseDTO] = [:]
            for exercise in fetchedExercises {
                lookup[exercise.id] = exercise
            }
            exerciseLookup = lookup
        } catch let error as APIError {
            if case .unauthorized = error { return }
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your sets."
        }
        await ensureWeekLoaded(for: selectedDate)
    }

    /// Post-change reload (pull-to-refresh, new-set sheet
    /// dismiss, reappearing from a history view where entries
    /// may have been edited or deleted). Drops *every* cached
    /// week — not just the visible one — because edits can move
    /// an entry across days/weeks and new sets can be back-dated,
    /// then re-pulls the donut snapshot; `load` refetches the
    /// selected week since its marker was just cleared. Any
    /// other week is fetched fresh the next time it's visited.
    ///
    /// `@MainActor` because this clears `loadedWeeks` before
    /// delegating to `load()`. See `load()` for the full
    /// rationale on pinning these methods to the main actor.
    @MainActor
    private func refresh() async {
        loadedWeeks.removeAll()
        await load()
    }

    /// Fetches the calendar week containing `date` unless it's
    /// already cached. Results are bucketed by local start-of-day
    /// into `entriesByDay`. The range is sent as absolute instants
    /// (local midnight → next-local-midnight minus one second) so
    /// day semantics are computed on-device, never server-side.
    ///
    /// Failures leave the week unmarked in `loadedWeeks` so a
    /// later visit retries; the spinner clears either way.
    ///
    /// `@MainActor` because this mutates `@State`
    /// (`loadingWeeks`, `entriesByDay`, `loadedWeeks`).
    /// See `load()` for the full rationale.
    @MainActor
    private func ensureWeekLoaded(for date: Date, force: Bool = false) async {
        let weekStart = CalendarMath.startOfWeek(for: date)
        if !force, loadedWeeks.contains(weekStart) || loadingWeeks.contains(weekStart) {
            return
        }
        let calendar = Calendar.current
        guard
            let weekEndExclusive = calendar.date(byAdding: .day, value: 7, to: weekStart),
            let weekEndInclusive = calendar.date(byAdding: .second, value: -1, to: weekEndExclusive)
        else { return }

        loadingWeeks.insert(weekStart)
        defer { loadingWeeks.remove(weekStart) }

        do {
            let entries = try await env.api.listExerciseEntries(
                from: weekStart,
                to: weekEndInclusive
            )
            // Defensive de-duplication by id: a duplicated row
            // (server anomaly, sync hiccup) would otherwise give
            // `SelectedDaySetList`'s ForEach duplicate identities,
            // which crashes SwiftUI. Keep the first occurrence.
            var seen = Set<String>()
            let uniqueEntries = entries.filter { seen.insert($0.id).inserted }
            var byDay = entriesByDay
            for day in CalendarMath.days(inWeekOf: weekStart) {
                byDay[day] = []
            }
            for entry in uniqueEntries {
                let dayKey = CalendarMath.startOfDay(entry.createdAt)
                byDay[dayKey, default: []].append(entry)
            }
            // Newest first within each day bucket.
            for key in byDay.keys where CalendarMath.days(inWeekOf: weekStart).contains(key) {
                byDay[key]?.sort { $0.createdAt > $1.createdAt }
            }
            entriesByDay = byDay
            loadedWeeks.insert(weekStart)
        } catch let error as APIError {
            if case .unauthorized = error { return }
        } catch {
            // Transient failure — leave uncached to retry later.
        }
    }
}

#Preview {
    DashboardView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
        .environmentObject(AuthStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
}
