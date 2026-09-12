import SwiftUI

/// Plan-ahead sheet: snapshots one workout into N dated copies
/// in a single atomic bulk call ("every Monday × 7"). Launched
/// from a workout row's Plan action or the detail view's "Plan
/// again" button; the source workout is fixed for the sheet's
/// lifetime.
///
/// Rows are editable datetime ranges (compact pickers with
/// delete). The repeat section fills rows with the next N
/// occurrences of a weekday, preserving the first row's
/// time-of-day and duration. Submit is disabled outside 1–31
/// rows (the server's bulk cap); any 400 shows inline with no
/// partial list to reconcile (the call is all-or-nothing).
struct PlanAheadSheet: View {
    @Environment(\.dismiss) private var dismiss

    let workoutID: String
    let workoutName: String
    @ObservedObject var workoutStore: WorkoutStore

    @State private var instances: [PlanInstanceDraft] = []
    @State private var repeatWeekday: Int = 2
    @State private var repeatCount: Int = 7

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    private var canSave: Bool {
        guard !isSaving else { return false }
        guard (1...31).contains(instances.count) else { return false }
        return true
    }

    var body: some View {
        NavigationStack {
            Form {
                sourceSection
                repeatSection
                datesSection
                if let errorMessage {
                    Section {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            }
            .navigationTitle("Plan ahead")
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
                            Text("Plan \(instances.count)").bold()
                        }
                    }
                    .disabled(!canSave)
                }
            }
            .onAppear { seedIfNeeded() }
        }
    }

    // MARK: - Sections

    private var sourceSection: some View {
        Section {
            HStack {
                Image(systemName: Icons.workouts)
                    .foregroundStyle(DSColors.accent)
                Text(workoutName)
                    .foregroundStyle(DSColors.text)
                    .lineLimit(1)
            }
        } header: {
            Text("Workout")
        } footer: {
            Text("Each date becomes its own workout — a snapshot copy. Later edits to this workout won't touch them.")
        }
    }

    private var repeatSection: some View {
        Section {
            HStack {
                Text("Every")
                    .foregroundStyle(DSColors.text)
                Spacer()
                Menu {
                    ForEach(1...7, id: \.self) { weekday in
                        Button(weekdayName(weekday)) { repeatWeekday = weekday }
                    }
                } label: {
                    HStack {
                        Text(weekdayName(repeatWeekday))
                            .foregroundStyle(DSColors.text)
                        Image(systemName: "chevron.up.chevron.down")
                            .font(.caption)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
                .accessibilityLabel("Repeat weekday")
            }
            Stepper("Weeks: \(repeatCount)", value: $repeatCount, in: 1...31)
            Button {
                fillFromRepeat()
            } label: {
                Label("Fill dates", systemImage: "calendar.badge.plus")
            }
        } header: {
            Text("Repeat")
        } footer: {
            Text("Fills the dates below with the next occurrences, keeping the first row's time and length.")
        }
    }

    private var datesSection: some View {
        Section {
            ForEach($instances) { $instance in
                VStack(spacing: DSSpacing.xs) {
                    DatePicker(
                        "Starts",
                        selection: $instance.start,
                        displayedComponents: [.date, .hourAndMinute]
                    )
                    DatePicker(
                        "Ends",
                        selection: $instance.end,
                        displayedComponents: [.date, .hourAndMinute]
                    )
                }
                .datePickerStyle(.compact)
                .padding(.vertical, DSSpacing.xs)
            }
            .onDelete { indexSet in
                instances.remove(atOffsets: indexSet)
            }
            Button {
                withAnimation {
                    instances.append(PlanInstanceDraft(
                        start: defaultStart(after: instances.last?.start),
                        end: defaultStart(after: instances.last?.start).addingTimeInterval(3600)
                    ))
                }
            } label: {
                Label("Add date", systemImage: "plus.circle")
            }
        } header: {
            Text("Dates · \(instances.count)")
        } footer: {
            if instances.count > 31 {
                Text("The server plans at most 31 at once — remove some or split into two batches.")
            }
        }
    }

    // MARK: - Data

    /// Seeds a single starter row (tomorrow 18:00–19:00 local)
    /// once, so the form is submittable immediately and
    /// re-renders never wipe the user's rows.
    private func seedIfNeeded() {
        guard instances.isEmpty else { return }
        let start = defaultStart(after: nil)
        instances = [PlanInstanceDraft(start: start, end: start.addingTimeInterval(3600))]
    }

    /// Default start for a fresh row: tomorrow at 18:00 local,
    /// or a week after the previous row (keeps manually added
    /// rows spaced).
    private func defaultStart(after previous: Date?) -> Date {
        let calendar = Calendar.current
        if let previous {
            return calendar.date(byAdding: .day, value: 7, to: previous) ?? previous
        }
        let tomorrow = calendar.date(byAdding: .day, value: 1, to: Date()) ?? Date()
        return calendar.date(bySettingHour: 18, minute: 0, second: 0, of: tomorrow) ?? tomorrow
    }

    private func weekdayName(_ weekday: Int) -> String {
        // Sunday-first full names; `weekday` follows the
        // Calendar.component convention (1 = Sunday).
        let symbols = Calendar.current.weekdaySymbols
        guard (1...7).contains(weekday) else { return "" }
        return symbols[weekday - 1]
    }

    /// Replaces the rows with the next `repeatCount`
    /// occurrences of `repeatWeekday`, preserving the first
    /// row's time-of-day and duration (or the 18:00/1h
    /// default when there are no rows yet).
    private func fillFromRepeat() {
        let anchor = instances.first
        let filled = nextWeekdayDates(
            weekday: repeatWeekday,
            count: repeatCount,
            from: Date(),
            hour: anchor.map { Calendar.current.component(.hour, from: $0.start) } ?? 18,
            minute: anchor.map { Calendar.current.component(.minute, from: $0.start) } ?? 0,
            duration: anchor.map { $0.end.timeIntervalSince($0.start) } ?? 3600,
            calendar: Calendar.current
        )
        withAnimation {
            instances = filled.map { PlanInstanceDraft(start: $0.start, end: $0.end) }
        }
    }

    // MARK: - Save

    private func save() async {
        errorMessage = nil
        guard canSave else { return }
        isSaving = true
        defer { isSaving = false }

        let created = await workoutStore.bulkCreate(BulkCreateWorkoutsRequest(
            workoutID: workoutID,
            instances: instances.map {
                BulkWorkoutInstanceRequest(scheduledStart: $0.start, scheduledEnd: $0.end)
            }
        ))
        if created == nil {
            errorMessage = workoutStore.errorMessage ?? "Could not plan the workouts."
            return
        }
        dismiss()
    }
}

// MARK: - Drafts

/// One editable datetime range in the sheet.
struct PlanInstanceDraft: Identifiable, Equatable {
    let id = UUID()
    var start: Date
    var end: Date
}

// MARK: - Date math

/// The next `count` occurrences of `weekday` (Calendar.component
/// convention: 1 = Sunday) at the given time-of-day, each `duration`
/// long. Today is included when its occurrence is still ahead —
/// otherwise the series starts next week — then continues weekly.
/// Pure (explicit calendar + `from`) so it can be unit-tested
/// without depending on the device clock or locale.
func nextWeekdayDates(
    weekday: Int,
    count: Int,
    from: Date,
    hour: Int,
    minute: Int,
    duration: TimeInterval,
    calendar: Calendar = .current
) -> [(start: Date, end: Date)] {
    guard (1...7).contains(weekday), count > 0 else { return [] }
    let dayStart = calendar.startOfDay(for: from)
    let todayWeekday = calendar.component(.weekday, from: dayStart)
    var delta = (weekday - todayWeekday + 7) % 7
    // Same weekday as today: take today when its time hasn't
    // passed, otherwise roll a full week forward.
    if delta == 0,
       let todayAt = calendar.date(bySettingHour: hour, minute: minute, second: 0, of: dayStart),
       todayAt <= from {
        delta = 7
    }
    var out: [(start: Date, end: Date)] = []
    for week in 0..<count {
        guard let day = calendar.date(byAdding: .day, value: delta + week * 7, to: dayStart),
              let start = calendar.date(bySettingHour: hour, minute: minute, second: 0, of: day) else {
            continue
        }
        out.append((start: start, end: start.addingTimeInterval(duration)))
    }
    return out
}

#Preview {
    PlanAheadSheet(
        workoutID: "w-1",
        workoutName: "Lower A",
        workoutStore: WorkoutStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        ))
    )
}
