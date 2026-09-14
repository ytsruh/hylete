import SwiftUI

/// The Workouts list (Beta). A plain `List` of the user's scheduled
/// workouts, newest scheduled date first. Tapping a row pushes the
/// detail view; trailing swipe deletes, leading swipe duplicates.
///
/// Stack-less content: the More hub's `NavigationStack` provides the
/// single stack — never wrap this view. Sheets (the editor) keep
/// their own stacks.
///
/// All networking and state lives in `WorkoutStore`; this view is
/// purely presentational.
struct WorkoutsListView: View {
    @EnvironmentObject private var env: AppEnvironment
    @ObservedObject var store: WorkoutStore
    @ObservedObject var blockStore: BlockStore

    @State private var showingNewWorkout: Bool = false
    @State private var deletingWorkout: WorkoutSummaryDTO?
    @State private var showingDeleteAlert: Bool = false
    @State private var duplicatingWorkout: WorkoutDTO?
    /// Drives the programmatic push to a freshly-duplicated
    /// workout's detail. Set by the duplicate sheet's callback
    /// (after the sheet dismisses, so the list is top and the push
    /// is clean). `navigationDestination(item:)` needs iOS 17+,
    /// which is the app's deployment target.
    @State private var navigatingWorkout: WorkoutDTO?
    @State private var search: String = ""
    @State private var statusFilter: WorkoutStatusFilter = .all

    /// Client-side name + status filter over the loaded summaries.
    /// Trimmed, case-insensitive contains match — same as the
    /// exercise catalogue filter.
    private var filtered: [WorkoutSummaryDTO] {
        filterWorkouts(store.summaries, search: search, statusFilter: statusFilter)
    }

    var body: some View {
        content
            .navigationTitle("Workouts")
            .navigationDestination(item: $navigatingWorkout) { workout in
                WorkoutDetailView(store: store, blockStore: blockStore, workoutID: workout.id)
            }
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showingNewWorkout = true
                    } label: {
                        Image(systemName: Icons.addSet)
                    }
                    .accessibilityLabel("Add workout")
                }
            }
            .sheet(isPresented: $showingNewWorkout) {
                WorkoutEditorView(mode: .create, store: store, blockStore: blockStore)
                    .environmentObject(env)
            }
            .sheet(item: $duplicatingWorkout) { workout in
                WorkoutDuplicateSheet(workout: workout, store: store) { created in
                    // Sheet is dismissed first (inside the sheet),
                    // so the list is top and this push is clean.
                    navigatingWorkout = created
                }
                .environmentObject(env)
            }
            .alert("Delete this workout?", isPresented: $showingDeleteAlert) {
                Button("Delete", role: .destructive) {
                    if let workout = deletingWorkout {
                        Task { await store.delete(id: workout.id) }
                    }
                }
                Button("Cancel", role: .cancel) {
                    deletingWorkout = nil
                }
            } message: {
                Text("This permanently removes the workout and its plan. Logged sets are unaffected.")
            }
            .task { await store.load() }
            .refreshable { await store.load() }
    }

    @ViewBuilder
    private var content: some View {
        if store.isLoading && store.summaries.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = store.errorMessage, store.summaries.isEmpty {
            errorState(error)
        } else if store.summaries.isEmpty {
            emptyState
        } else {
            loadedList
        }
    }

    private var loadedList: some View {
        VStack(spacing: 0) {
            Text("Planned training days, built from your blocks")
                .font(.title3)
                .foregroundStyle(DSColors.textSecondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal)
                .padding(.top, DSSpacing.sm)
                .padding(.bottom, DSSpacing.xs)
            InlineSearchField(text: $search, prompt: "Search workouts")
                .padding(.horizontal)
                .padding(.bottom, DSSpacing.xs)
            WorkoutStatusFilterView(selection: $statusFilter)
                .padding(.bottom, DSSpacing.xs)
            if filtered.isEmpty {
                noMatches
            } else {
                List {
                    Section {
                        ForEach(filtered) { workout in
                    NavigationLink {
                        WorkoutDetailView(store: store, blockStore: blockStore, workoutID: workout.id)
                    } label: {
                        workoutRow(for: workout)
                    }
                    .swipeActions(edge: .leading, allowsFullSwipe: false) {
                        Button {
                            Task { duplicatingWorkout = await store.detail(id: workout.id) }
                        } label: {
                            Label("Duplicate", systemImage: "plus.square.on.square")
                        }
                        .tint(DSColors.accent)
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                        Button(role: .destructive) {
                            deletingWorkout = workout
                            showingDeleteAlert = true
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
            }
                }
                .listStyle(.automatic)
            }
        }
    }

    /// Shown when the list itself is non-empty but the current
    /// search/status combination hides every row.
    private var noMatches: some View {
        VStack(spacing: DSSpacing.md) {
            Spacer()
            Text("No workouts match your search.")
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Spacer()
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    /// Three-line row: name, "Mon, Sep 14 · Planned" subtitle, and
    /// "2/3 blocks" progress.
    private func workoutRow(for workout: WorkoutSummaryDTO) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(workout.name)
                .font(.body)
                .foregroundStyle(DSColors.text)
            Text("\(WorkoutDates.display(workout.scheduledDate)) · \(workout.status.displayName)")
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
            Text(workout.progressLabel)
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            ZStack {
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surfaceElevated)
                Image(systemName: "calendar.badge.clock")
                    .font(.system(size: 24))
                    .foregroundStyle(DSColors.text)
            }
            .frame(width: 48, height: 48)

            Text("No workouts yet")
                .font(.title3.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text("Plan a training day from your blocks, assign it a date, then check blocks off as you train.")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Text("Tap + to plan your first workout.")
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func errorState(_ message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Try again") {
                Task { await store.load() }
            }
            .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

/// Duplicate sheet: a one-off copy on a chosen date, or a repeat
/// (weekly on chosen weekdays, or every N days) that expands into an
/// explicit date list sent to the server-side batch endpoint (name
/// kept verbatim, block order copied, statuses reset to pending).
/// Separate from the editor because duplication never edits
/// structure — just dates.
struct WorkoutDuplicateSheet: View {
    /// Server cap: at most 50 copies per batch. The steppers clamp
    /// so the request can never exceed it; the footer says so too.
    private static let batchMax = 50

    enum Mode: String, CaseIterable {
        case once
        case weekly
        case interval

        var displayName: String {
            switch self {
            case .once: return "Once"
            case .weekly: return "Weekly"
            case .interval: return "Every N days"
            }
        }
    }

    @Environment(\.dismiss) private var dismiss

    let workout: WorkoutDTO
    @ObservedObject var store: WorkoutStore
    /// Fired with the created workout after a successful one-off
    /// duplicate (never on failure, never for batches) so the
    /// presenter can navigate — e.g. the list pushes the new
    /// detail, the detail view pops back to the list. The sheet
    /// dismisses itself before firing.
    let onDuplicated: ((WorkoutDTO) -> Void)?
    /// Fired after a successful batch duplicate so a presenting
    /// detail view can pop back to the list (the list itself just
    /// shows the merged rows). The sheet dismisses itself before
    /// firing.
    let onBatchDuplicated: (() -> Void)?

    init(
        workout: WorkoutDTO,
        store: WorkoutStore,
        onDuplicated: ((WorkoutDTO) -> Void)? = nil,
        onBatchDuplicated: (() -> Void)? = nil
    ) {
        self.workout = workout
        self._store = ObservedObject(wrappedValue: store)
        self.onDuplicated = onDuplicated
        self.onBatchDuplicated = onBatchDuplicated
    }

    @State private var mode: Mode = .once
    @State private var date: Date = Date()
    @State private var weekdays: Set<Int> = []
    @State private var weeks: Int = 6
    @State private var everyDays: Int = 2
    @State private var occurrences: Int = 8
    @State private var didSeed: Bool = false

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    /// The explicit date list the save will send. Single-element
    /// for Once; expanded recurrence otherwise.
    private var plannedDates: [String] {
        switch mode {
        case .once:
            return [WorkoutDates.dayString(from: date)]
        case .weekly:
            return WorkoutDates.datesForWeekly(starting: date, weekdays: weekdays, weeks: weeks)
        case .interval:
            return WorkoutDates.datesForInterval(starting: date, everyDays: everyDays, occurrences: occurrences)
        }
    }

    private var canSave: Bool {
        guard !isSaving else { return false }
        guard !plannedDates.isEmpty else { return false }
        return plannedDates.count <= Self.batchMax
    }

    private var saveLabel: String {
        if mode == .once { return "Duplicate" }
        return "Create \(plannedDates.count)"
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    Picker("Repeat", selection: $mode) {
                        ForEach(Mode.allCases, id: \.self) { mode in
                            Text(mode.displayName).tag(mode)
                        }
                    }
                    .pickerStyle(.segmented)
                }
                modeSection
                previewSection
                if let errorMessage {
                    Section {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            }
            .navigationTitle("Duplicate Workout")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                        .disabled(isSaving)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await save() }
                    } label: {
                        if isSaving {
                            ProgressView()
                        } else {
                            Text(saveLabel).bold()
                        }
                    }
                    .disabled(!canSave)
                }
            }
            .task { seedIfNeeded() }
        }
    }

    @ViewBuilder
    private var modeSection: some View {
        switch mode {
        case .once:
            Section {
                DatePicker("New date", selection: $date, displayedComponents: .date)
            } header: {
                Text("Duplicate \(workout.name)")
                } footer: {
                    Text("Saves a copy on the new date with all blocks pending.")
                }
        case .weekly:
            Section {
                DatePicker("Start", selection: $date, displayedComponents: .date)
                weekdayPicker
                Stepper("For \(weeks) week\(weeks == 1 ? "" : "s")", value: $weeks, in: 1...12)
            } header: {
                Text("Repeat \(workout.name) weekly")
            } footer: {
                Text("Repeats keep the workout's name — the date tells them apart. Batches are capped at \(Self.batchMax) workouts per request.")
            }
        case .interval:
            Section {
                DatePicker("Start", selection: $date, displayedComponents: .date)
                Stepper("Every \(everyDays) day\(everyDays == 1 ? "" : "s")", value: $everyDays, in: 1...30)
                Stepper("\(occurrences) times", value: $occurrences, in: 2...50)
            } header: {
                Text("Repeat \(workout.name)")
            } footer: {
                Text("Repeats keep the workout's name — the date tells them apart. Batches are capped at \(Self.batchMax) workouts per request.")
            }
        }
    }

    /// Weekday multi-select chips in locale order. At least one day
    /// must stay selected (tapping the last one is a no-op) so the
    /// preview can never go empty from here.
    private var weekdayPicker: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text("Days")
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
            LazyVGrid(columns: Array(repeating: GridItem(.flexible()), count: 7), spacing: DSSpacing.xs) {
                ForEach(WorkoutDates.orderedWeekdays(), id: \.number) { day in
                    Button {
                        if weekdays.contains(day.number) {
                            if weekdays.count > 1 {
                                weekdays.remove(day.number)
                            }
                        } else {
                            weekdays.insert(day.number)
                        }
                    } label: {
                        Text(day.symbol)
                            .font(.caption.weight(.semibold))
                            .frame(maxWidth: .infinity)
                            .padding(.vertical, DSSpacing.xs)
                            .background(
                                RoundedRectangle(cornerRadius: DSSpacing.xs, style: .continuous)
                                    .fill(weekdays.contains(day.number) ? DSColors.accent : DSColors.surfaceElevated)
                            )
                            .foregroundStyle(weekdays.contains(day.number) ? DSColors.onPrimary : DSColors.text)
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("\(day.symbol), \(weekdays.contains(day.number) ? "selected" : "not selected")")
                }
            }
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    @ViewBuilder
    private var previewSection: some View {
        if mode != .once {
            Section {
                if plannedDates.isEmpty {
                    Text("Pick at least one day.")
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                } else {
                    Text(previewText)
                        .font(.subheadline)
                        .foregroundStyle(DSColors.text)
                    if plannedDates.count > Self.batchMax {
                        Text("That selection makes \(plannedDates.count) workouts — the limit is \(Self.batchMax) per batch. Shorten the repeat or split it in two.")
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            } header: {
                Text("Preview")
            }
        }
    }

    private var previewText: String {
        guard let first = plannedDates.first, let last = plannedDates.last else { return "" }
        let count = plannedDates.count == 1 ? "1 workout" : "\(plannedDates.count) workouts"
        if first == last {
            return "\(count) · \(WorkoutDates.display(first))"
        }
        return "\(count) · \(WorkoutDates.display(first)) → \(WorkoutDates.display(last))"
    }

    /// Seeds the start date with today and the weekday set with the
    /// source workout's own weekday, so "every Monday for 6 weeks"
    /// is one tap away when duplicating a Monday workout.
    private func seedIfNeeded() {
        guard !didSeed else { return }
        didSeed = true
        let anchor = WorkoutDates.date(from: workout.scheduledDate) ?? Date()
        if mode == .once {
            date = Date()
        }
        let weekday = Calendar.current.component(.weekday, from: anchor)
        weekdays = [weekday]
    }

    private func save() async {
        errorMessage = nil
        let dates = plannedDates
        guard !dates.isEmpty, dates.count <= Self.batchMax else {
            errorMessage = "Pick dates within the \(Self.batchMax)-workout batch limit."
            return
        }
        isSaving = true
        defer { isSaving = false }
        if mode == .once, let single = dates.first, dates.count == 1 {
            guard let created = await store.duplicate(id: workout.id, scheduledDate: single) else {
                errorMessage = store.errorMessage ?? "Could not duplicate the workout."
                return
            }
            dismiss()
            onDuplicated?(created)
        } else {
            guard await store.duplicateBatch(id: workout.id, dates: dates) != nil else {
                errorMessage = store.errorMessage ?? "Could not duplicate the workouts."
                return
            }
            dismiss()
            onBatchDuplicated?()
        }
    }
}

#Preview {
    NavigationStack {
        WorkoutsListView(
            store: WorkoutStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )),
            blockStore: BlockStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            ))
        )
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
