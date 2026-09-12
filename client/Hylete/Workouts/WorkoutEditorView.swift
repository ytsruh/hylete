import SwiftUI

/// Create / edit sheet for a workout. Sections: details
/// (title, description), the ordered block list (each row = a
/// block picked from the Blocks library; the same block may
/// repeat), and the schedule (explicit planned days plus a
/// repeat helper that expands weekdays x weeks into days).
///
/// Save POSTs (create, unscheduled first, then plans the
/// chosen days) or PUTs (edit, block links fully replaced,
/// then reconciles the schedule to the chosen date set) via
/// the shared `WorkoutStore`. Validation mirrors the server
/// so Save stays disabled until the body is valid.
struct WorkoutEditorView: View {
    enum Mode {
        case create
        case edit(WorkoutDTO)
    }

    @Environment(\.dismiss) private var dismiss

    let mode: Mode
    /// Shared with `WorkoutsListView` / `WorkoutDetailView`
    /// so the list and detail pick up the new / edited row in
    /// place.
    @ObservedObject var store: WorkoutStore
    /// Supplies the block library for the block picker.
    @ObservedObject var blockStore: BlockStore

    init(mode: Mode, store: WorkoutStore, blockStore: BlockStore) {
        self.mode = mode
        self._store = ObservedObject(wrappedValue: store)
        self._blockStore = ObservedObject(wrappedValue: blockStore)
    }

    /// One block in the form. `id` is a local UUID (server
    /// link ids arrive only after save); position is the
    /// array order.
    private struct DraftBlock: Identifiable, Equatable {
        let id: UUID
        var block: BlockSummaryDTO
    }

    @State private var title: String = ""
    @State private var description: String = ""
    @State private var drafts: [DraftBlock] = []
    /// Desired planned days as "YYYY-MM-DD" strings. Seeded
    /// from the workout's assignments in edit mode; empty for
    /// create.
    @State private var dates: [String] = []

    @State private var showingBlockPicker: Bool = false
    @State private var showingDatePicker: Bool = false
    @State private var singleDate: Date = Date()
    @State private var repeatStart: Date = Date()
    @State private var repeatWeekdays: Set<Int> = []
    @State private var repeatWeeks: Int = 4

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?
    @State private var didSeed: Bool = false

    private var isEditing: Bool {
        if case .edit = mode { return true }
        return false
    }

    /// The workout the form is seeded from (edit mode only).
    private var sourceWorkout: WorkoutDTO? {
        switch mode {
        case .create:
            return nil
        case .edit(let workout):
            return workout
        }
    }

    private var trimmedTitle: String {
        title.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    /// Mirrors the server rules (`validateWorkoutFields` +
    /// `validateWorkoutDates`): title 1-100, description
    /// <=1000, 1-20 blocks. Dates are validated as they are
    /// added, so the set is valid by construction.
    private var canSave: Bool {
        guard !isSaving else { return false }
        guard trimmedTitle.count >= 1, trimmedTitle.count <= 100 else { return false }
        guard description.count <= 1000 else { return false }
        guard drafts.count >= 1, drafts.count <= 20 else { return false }
        return true
    }

    var body: some View {
        NavigationStack {
            Form {
                detailsSection
                blocksSection
                scheduleSection
                repeatSection
                if let errorMessage {
                    Section {
                        Text(errorMessage)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            }
            .navigationTitle(isEditing ? "Edit workout" : "New workout")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { dismiss() }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Save") { Task { await save() } }
                        .disabled(!canSave)
                }
            }
            .sheet(isPresented: $showingBlockPicker) {
                blockPickerSheet
            }
            .sheet(isPresented: $showingDatePicker) {
                singleDateSheet
            }
            .task {
                guard !didSeed else { return }
                didSeed = true
                if let source = sourceWorkout {
                    title = source.title
                    description = source.description
                    // Seed the ordered blocks from the detail.
                    // The summary shape is enough for the rows;
                    // ids resolve again server-side on save.
                    drafts = source.blocks.map { link in
                        DraftBlock(
                            id: UUID(),
                            block: BlockSummaryDTO(
                                id: link.blockID,
                                name: link.blockName,
                                description: "",
                                type: BlockTypeDTO(rawValue: link.blockType) ?? .standard,
                                rounds: 0, restSeconds: 0,
                                timeCapSeconds: 0, intervalSeconds: 0,
                                itemCount: 0,
                                createdAt: Date(), updatedAt: Date()
                            )
                        )
                    }
                    dates = source.assignments.map(\.scheduledDate).sorted()
                }
                await blockStore.load()
            }
        }
    }

    // MARK: - Sections

    private var detailsSection: some View {
        Section("Details") {
            TextField("Title", text: $title)
            TextField("Description (optional)", text: $description, axis: .vertical)
        }
    }

    private var blocksSection: some View {
        Section {
            ForEach(drafts) { draft in
                VStack(alignment: .leading, spacing: 2) {
                    Text(draft.block.name)
                        .font(.body)
                        .foregroundStyle(DSColors.text)
                    Text(draft.block.type.displayName)
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
            .onDelete { drafts.remove(atOffsets: $0) }
            .onMove { drafts.move(fromOffsets: $0, toOffset: $1) }
            Button {
                showingBlockPicker = true
            } label: {
                Label("Add block", systemImage: "plus")
            }
        } header: {
            Text(drafts.count == 1 ? "1 block" : "\(drafts.count) blocks")
        }
    }

    private var scheduleSection: some View {
        Section {
            ForEach(dates, id: \.self) { date in
                Text(friendlyDate(date))
            }
            .onDelete { dates.remove(atOffsets: $0) }
            Button {
                singleDate = Date()
                showingDatePicker = true
            } label: {
                Label("Add a day", systemImage: "calendar.badge.plus")
            }
        } header: {
            Text(dates.count == 1 ? "1 planned day" : "\(dates.count) planned days")
        } footer: {
            Text("One-off days, or use Repeat below to expand weeks of training.")
        }
    }

    /// Repeat helper: pick weekdays + a number of weeks and
    /// expand them into individual days client-side (the
    /// server stores one row per day and never sees a
    /// recurrence rule). Days already planned are skipped.
    private var repeatSection: some View {
        Section {
            DatePicker("Starts", selection: $repeatStart, displayedComponents: .date)
            weekdayPicker
            Stepper("Repeat for \(repeatWeeks) \(repeatWeeks == 1 ? "week" : "weeks")",
                    value: $repeatWeeks, in: 1...12)
            Button {
                addExpandedRepeat()
            } label: {
                Label(repeatPreviewLabel, systemImage: "repeat")
            }
            .disabled(expandedRepeatDates().isEmpty)
        } header: {
            Text("Repeat")
        }
    }

    /// Weekday toggles in calendar display order (Sun-first
    /// for en-US, Mon-first for en-GB, ...). Stored as
    /// `Calendar` weekday numbers (1 = Sunday .. 7).
    private var weekdayPicker: some View {
        let calendar = Calendar.current
        let first = calendar.firstWeekday // 1-based
        let ordered = (0..<7).map { ((first - 1 + $0) % 7) + 1 }
        return HStack {
            ForEach(ordered, id: \.self) { weekday in
                let label = calendar.veryShortWeekdaySymbols[(weekday - 1) % 7]
                Button {
                    if repeatWeekdays.contains(weekday) {
                        repeatWeekdays.remove(weekday)
                    } else {
                        repeatWeekdays.insert(weekday)
                    }
                } label: {
                    Text(label)
                        .font(.subheadline.weight(.semibold))
                        .frame(width: 32, height: 32)
                        .background(
                            repeatWeekdays.contains(weekday)
                                ? DSColors.accent
                                : DSColors.surfaceElevated
                        )
                        .foregroundStyle(
                            repeatWeekdays.contains(weekday) ? .white : DSColors.text
                        )
                        .clipShape(Circle())
                }
                .buttonStyle(.plain)
            }
        }
    }

    // MARK: - Repeat expansion

    /// Expands the repeat helper into "YYYY-MM-DD" days:
    /// every selected weekday over `repeatWeeks` weeks from
    /// `repeatStart`, minus days already in `dates`, capped
    /// at the server's 100-dates-per-request limit.
    private func expandedRepeatDates() -> [String] {
        guard !repeatWeekdays.isEmpty else { return [] }
        let calendar = Calendar.current
        let start = CalendarMath.startOfDay(repeatStart)
        let existing = Set(dates)
        var out: [String] = []
        for offset in 0..<(repeatWeeks * 7) {
            guard let day = calendar.date(byAdding: .day, value: offset, to: start) else { continue }
            let weekday = calendar.component(.weekday, from: day)
            guard repeatWeekdays.contains(weekday) else { continue }
            let key = WorkoutAssignmentDTO.dayFormatter.string(from: day)
            if !existing.contains(key), !out.contains(key) {
                out.append(key)
            }
            if out.count >= 100 { break }
        }
        return out.sorted()
    }

    private var repeatPreviewLabel: String {
        let fresh = expandedRepeatDates()
        if repeatWeekdays.isEmpty {
            return "Pick weekdays to preview"
        }
        return fresh.isEmpty ? "No new days" : "Add \(fresh.count) \(fresh.count == 1 ? "day" : "days")"
    }

    private func addExpandedRepeat() {
        let fresh = expandedRepeatDates()
        guard !fresh.isEmpty else { return }
        let merged = Set(dates).union(fresh)
        dates = merged.sorted().suffix(365).sorted()
    }

    // MARK: - Sheets

    private var blockPickerSheet: some View {
        NavigationStack {
            List {
                if blockStore.blocks.isEmpty {
                    Text("No blocks yet — create one under More > Blocks first.")
                        .foregroundStyle(DSColors.textSecondary)
                } else {
                    ForEach(blockStore.blocks) { block in
                        Button {
                            drafts.append(DraftBlock(id: UUID(), block: block))
                            showingBlockPicker = false
                        } label: {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(block.name)
                                    .font(.body)
                                    .foregroundStyle(DSColors.text)
                                Text("\(block.type.displayName) · \(block.itemCount) \(block.itemCount == 1 ? "exercise" : "exercises")")
                                    .font(.subheadline)
                                    .foregroundStyle(DSColors.textSecondary)
                            }
                        }
                    }
                }
            }
            .navigationTitle("Pick a block")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Close") { showingBlockPicker = false }
                }
            }
        }
    }

    private var singleDateSheet: some View {
        NavigationStack {
            Form {
                DatePicker("Day", selection: $singleDate, displayedComponents: .date)
            }
            .navigationTitle("Add a day")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("Cancel") { showingDatePicker = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("Add") {
                        let key = WorkoutAssignmentDTO.dayFormatter.string(from: singleDate)
                        if !dates.contains(key) {
                            dates.append(key)
                            dates.sort()
                        }
                        showingDatePicker = false
                    }
                }
            }
        }
        .presentationDetents([.medium])
    }

    // MARK: - Save

    private func save() async {
        errorMessage = nil
        isSaving = true
        defer { isSaving = false }
        let blockIDs = drafts.map(\.block.id)
        switch mode {
        case .create:
            guard let created = await store.create(CreateWorkoutRequest(
                title: trimmedTitle, description: description, blockIDs: blockIDs
            )) else {
                errorMessage = store.errorMessage ?? "Could not save the workout."
                return
            }
            if !dates.isEmpty {
                await store.addAssignments(workoutID: created.id, dates: dates)
                if store.errorMessage != nil {
                    errorMessage = store.errorMessage
                    return
                }
            }
            dismiss()
        case .edit(let workout):
            await store.update(id: workout.id, request: UpdateWorkoutRequest(
                title: trimmedTitle, description: description, blockIDs: blockIDs
            ))
            guard store.errorMessage == nil else {
                errorMessage = store.errorMessage
                return
            }
            await store.syncSchedule(workoutID: workout.id, desiredDates: dates)
            guard store.errorMessage == nil else {
                errorMessage = store.errorMessage
                return
            }
            dismiss()
        }
    }

    private func friendlyDate(_ yyyyMMdd: String) -> String {
        guard let date = WorkoutAssignmentDTO.dayFormatter.date(from: yyyyMMdd) else {
            return yyyyMMdd
        }
        return date.formatted(date: .abbreviated, time: .omitted)
    }
}

#Preview {
    WorkoutEditorView(
        mode: .create,
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
