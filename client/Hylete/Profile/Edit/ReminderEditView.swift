import SwiftUI

/// Sheet editor for the user's weight-reminder schedule. Mirrors the
/// reminder section of the web app's `/profile` form (master switch +
/// frequency off/daily/weekly/biweekly + day-of-week for
/// weekly/biweekly + hour-only time in UTC) and saves through
/// `PUT /api/v1/me/reminders`.
///
/// Presented as a sheet from the Profile tab (with its own
/// `NavigationStack` at the presentation site, like `FeedbackView`).
/// The schedule to edit is passed in — this view never fetches, so
/// there is no loading state and nothing here can overwrite an
/// in-progress edit. The only network call is Save.
struct ReminderEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    /// Called with the server-confirmed schedule after a successful
    /// save so the presenting view can refresh its summary row
    /// without a follow-up GET.
    var onSaved: ((ReminderPreferencesDTO) -> Void)?

    @State private var enabled: Bool
    @State private var frequency: String
    @State private var dayOfWeek: Int
    @State private var hour: Int
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    /// The schedule cadences. No "off" — the master toggle above
    /// owns on/off, so a redundant off option would only confuse.
    /// A legacy "off" from the server or web form maps to weekly
    /// on entry (see init).
    private static let frequencies: [String] = ["daily", "weekly", "biweekly"]

    /// Seeds local editing state from the passed-in schedule, so the
    /// sheet opens instantly on already-loaded data and every keystroke
    /// below only touches local state until Save.
    init(prefs: ReminderPreferencesDTO, onSaved: ((ReminderPreferencesDTO) -> Void)? = nil) {
        _enabled = State(initialValue: prefs.enabled)
        _frequency = State(initialValue: Self.frequencies.contains(prefs.frequency) ? prefs.frequency : "weekly")
        _dayOfWeek = State(initialValue: prefs.dayOfWeek ?? 0)
        _hour = State(initialValue: prefs.hour)
        self.onSaved = onSaved
    }

    private var needsDayOfWeek: Bool {
        frequency == "weekly" || frequency == "biweekly"
    }

    private var canSave: Bool {
        !isSaving
            && Self.frequencies.contains(frequency)
            && (!needsDayOfWeek || (0...6).contains(dayOfWeek))
            && (0...23).contains(hour)
    }

    private var timeString: String {
        String(format: "%02d:00", hour)
    }

    var body: some View {
        Form {
            Section {
                Toggle("Send me weight reminders", isOn: $enabled)
            } footer: {
                Text("Get an email nudge to log your weight on a schedule that works for you.")
            }
            Section {
                // Every control here selects inline — segmented,
                // stepper, toggle — with no popover or drill-down
                // push involved. Overlay presentations from this
                // sheet have been observed to reset the form to its
                // initial values, so nothing here is allowed to
                // present anything. The whole schedule section dims
                // when the master toggle is off, but keeps its
                // values so re-enabling restores the schedule.
                Picker("Frequency", selection: $frequency) {
                    ForEach(Self.frequencies, id: \.self) { value in
                        Text(frequencyLabel(value)).tag(value)
                    }
                }
                .pickerStyle(.segmented)
                if needsDayOfWeek {
                    Picker("Day of week", selection: $dayOfWeek) {
                        ForEach(0..<7, id: \.self) { day in
                            Text(ReminderPreferencesDTO.weekdayLabels[day]).tag(day)
                        }
                    }
                    .pickerStyle(.segmented)
                }
                Stepper(value: $hour, in: 0...23) {
                    HStack {
                        Text("Time of day")
                        Spacer()
                        Text("\(timeString) UTC")
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
            } footer: {
                Text("Times are UTC, not local. 09:00 UTC arrives at 10:00 during UK summer time (BST) and 09:00 in winter (GMT) — pick an hour earlier if you want a 9am local nudge.")
            }
            .disabled(!enabled)
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Weight reminders")
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
                        Text("Save").bold()
                    }
                }
                .disabled(!canSave)
            }
        }
    }

    private func save() async {
        errorMessage = nil
        guard Self.frequencies.contains(frequency) else {
            errorMessage = "Pick a supported frequency."
            return
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateReminderPreferencesRequest(
                enabled: enabled,
                frequency: frequency,
                dayOfWeek: needsDayOfWeek ? dayOfWeek : nil,
                time: timeString
            )
            let updated = try await env.api.updateReminderPreferences(request)
            onSaved?(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription ?? "Could not save your reminder settings."
        } catch {
            errorMessage = "Could not save your reminder settings."
        }
    }

    private func frequencyLabel(_ value: String) -> String {
        switch value {
        case "daily": return "Daily"
        case "weekly": return "Weekly"
        case "biweekly": return "Bi-weekly"
        default: return value
        }
    }
}

#Preview {
    NavigationStack {
        ReminderEditView(prefs: ReminderPreferencesDTO(
            enabled: true,
            frequency: "weekly",
            dayOfWeek: 0,
            time: "09:00"
        ))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
