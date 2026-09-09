import SwiftUI

/// The "Profile" tab. Shows the signed-in user's basic info
/// and a sign-out button. Tapping the name, weight unit,
/// target weight, or weight-reminders row opens a per-field
/// editor as a sheet that PUTs the change and refreshes
/// `authStore.currentUser` on success so the rest of the app
/// sees the new value.
struct ProfileView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    @State private var showingSignOutConfirm: Bool = false

    /// Two-step state machine behind the toolbar feedback button:
    /// `showingFeedbackSheet` presents the form, then `onSuccess`
    /// on `FeedbackView` flips the sheet back down and raises
    /// `showingFeedbackThanks` so a native alert confirms the
    /// submission. Keeping the two pieces of state separate lets
    /// the alert survive after the sheet's dismissal animation
    /// finishes.
    @State private var showingFeedbackSheet: Bool = false
    @State private var showingFeedbackThanks: Bool = false

    /// Bound directly to `@AppStorage` so the change is picked
    /// up by `RootView`'s `applyThemeMode()` modifier
    /// immediately. Persists across app restarts; mirrored
    /// against the web app's `themeMode` `localStorage` key.
    @AppStorage("themeMode") private var themeModeRaw: String = ThemeMode.system.rawValue

    /// Master opt-in for beta features. Stored in `UserDefaults`
    /// under `"betaFeaturesEnabled"`; read reactively by
    /// `BetaFeature` so flipping this toggle reveals or hides
    /// any gated content in the same run loop. iOS-only — no
    /// server-side or web-app counterpart today.
    @AppStorage("betaFeaturesEnabled") private var betaFeaturesEnabled: Bool = false

    private var themeMode: Binding<ThemeMode> {
        Binding(
            get: { ThemeMode(rawValue: themeModeRaw) ?? .system },
            set: { themeModeRaw = $0.rawValue }
        )
    }

    /// The user's weight-reminder schedule, fetched once for the
    /// summary row and passed into the editor sheet as its initial
    /// state — the sheet itself never fetches. Nil until the first
    /// load completes; a failed load sets `remindersFailed` instead
    /// of a misleading default.
    @State private var reminderPrefs: ReminderPreferencesDTO?
    @State private var remindersFailed: Bool = false

    /// Guards the fetch below so overlapping callers (appear +
    /// row-tap) collapse into one request. Cleared *before* the
    /// fetched schedule is published or the sheet is presented,
    /// so no state write lands mid-presentation animation.
    @State private var remindersLoading: Bool = false

    /// Remembers a row tap that arrived while the schedule was still
    /// loading, so the sheet opens right after the data arrives —
    /// only after `remindersLoading` has been cleared, never while
    /// a write is still pending.
    @State private var remindersPendingOpen: Bool = false

    /// Presents the reminders editor sheet.
    @State private var showingRemindersSheet: Bool = false

    /// Present the per-field profile editors as sheets (same
    /// overlay style as weight reminders) so the sheet root
    /// shows only Cancel/Save with no push back button.
    @State private var showingNameSheet: Bool = false
    @State private var showingUnitSheet: Bool = false
    @State private var showingTargetSheet: Bool = false

    var body: some View {
        NavigationStack {
            List {
                if let user = authStore.currentUser {
                    headerSection(user: user)
                    accountSection(user: user)
                    preferencesSection(user: user)
                    BetaFeature {
                        Section {
                            HealthConnectRow()
                            HealthSyncRow(api: env.api)
                        } header: {
                            Text("Connected Accounts")
                        } footer: {
                            Text("Sync uploads the last 90 days of activity, heart, body, and sleep metrics to Hylete, then keeps each day caught up. Read-only — nothing is ever written back to Apple Health.")
                        }
                    }
                    appearanceSection
                    betaSection
                }

                Section {
                    Button(role: .destructive) {
                        showingSignOutConfirm = true
                    } label: {
                        HStack {
                            Spacer()
                            Text("Sign out")
                                .font(.body.weight(.semibold))
                            Spacer()
                        }
                    }
                }
            }
            .navigationTitle("Profile")
            .toolbar {
                /// Envelope icon at the top-right opens the
                /// feedback sheet. The web app surfaces the
                /// same form behind a sidebar link; here the
                /// toolbar button is the discoverable affordance
                /// without cluttering the Profile sections.
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showingFeedbackSheet = true
                    } label: {
                        Image(systemName: "envelope")
                    }
                    .accessibilityLabel("Send feedback")
                }
            }
            .sheet(isPresented: $showingFeedbackSheet) {
                NavigationStack {
                    FeedbackView(onSuccess: {
                        showingFeedbackSheet = false
                        showingFeedbackThanks = true
                    })
                }
                .presentationDetents([.large])
            }
            .alert(
                "Thanks for your feedback",
                isPresented: $showingFeedbackThanks
            ) {
                Button("OK", role: .cancel) {}
            } message: {
                Text("We'll read every submission.")
            }
            .alert(
                "Sign out of Hylete?",
                isPresented: $showingSignOutConfirm
            ) {
                Button("Sign out", role: .destructive) {
                    env.authStore.signOut()
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("You'll need to log in again.")
            }
            // Task + sheet live on the stable NavigationStack host,
            // not on the Preferences Section inside the List. A sheet
            // attached deep in the list tears down whenever the row
            // label (Loading… -> summary) or its Section identity
            // re-renders mid-presentation — the first-open
            // instant-close.
            .task {
                if reminderPrefs == nil && !remindersLoading {
                    await reloadReminders()
                }
            }
            .sheet(isPresented: $showingRemindersSheet) {
                if let prefs = reminderPrefs {
                    NavigationStack {
                        ReminderEditView(prefs: prefs, onSaved: { updated in
                            reminderPrefs = updated
                        })
                    }
                    .presentationDetents([.large])
                }
            }
            .sheet(isPresented: $showingNameSheet) {
                if let user = authStore.currentUser {
                    NavigationStack {
                        NameEditView(user: user)
                    }
                    .presentationDetents([.large])
                }
            }
            .sheet(isPresented: $showingUnitSheet) {
                if let user = authStore.currentUser {
                    NavigationStack {
                        WeightUnitEditView(user: user)
                    }
                    .presentationDetents([.large])
                }
            }
            .sheet(isPresented: $showingTargetSheet) {
                if let user = authStore.currentUser {
                    NavigationStack {
                        TargetWeightEditView(user: user)
                    }
                    .presentationDetents([.large])
                }
            }
        }
    }

    // MARK: - Sections

    private func headerSection(user: UserDTO) -> some View {
        Section {
            HStack(spacing: DSSpacing.md) {
                ZStack {
                    Circle()
                        .fill(DSColors.accent.opacity(0.15))
                        .frame(width: 56, height: 56)
                    Text(initials(for: user.name))
                        .font(.title2.weight(.semibold))
                        .foregroundStyle(DSColors.accent)
                }
                VStack(alignment: .leading, spacing: DSSpacing.xxs) {
                    Text(user.name)
                        .font(.headline)
                    Text(user.email)
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
            .padding(.vertical, DSSpacing.xs)
        }
    }

    private func accountSection(user: UserDTO) -> some View {
        Section("Account") {
            Button {
                showingNameSheet = true
            } label: {
                HStack {
                    Text("Name")
                        .foregroundStyle(DSColors.text)
                    Spacer()
                    Text(user.name)
                        .foregroundStyle(DSColors.textSecondary)
                    Image(systemName: "chevron.right")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.tertiary)
                }
            }
        }
    }

    private func preferencesSection(user: UserDTO) -> some View {
        Section("Preferences") {
            Button {
                showingUnitSheet = true
            } label: {
                HStack {
                    Text("Weight unit")
                        .foregroundStyle(DSColors.text)
                    Spacer()
                    Text(user.weightUnit)
                        .foregroundStyle(DSColors.textSecondary)
                    Image(systemName: "chevron.right")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.tertiary)
                }
            }
            Button {
                showingTargetSheet = true
            } label: {
                HStack {
                    Text("Target weight")
                        .foregroundStyle(DSColors.text)
                    Spacer()
                    if let target = user.targetWeight {
                        Text(String(format: "%.1f %@", target, user.weightUnit))
                            .foregroundStyle(DSColors.textSecondary)
                    } else {
                        Text("Not set")
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    Image(systemName: "chevron.right")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.tertiary)
                }
            }
            // Opens the editor sheet when the schedule is loaded.
            // Tapped while loading (or after a failure), it records
            // the intent and ensures a fetch is running — the sheet
            // then opens from the fetch completion (see
            // reloadReminders), so a failed fetch never strands the
            // row on a placeholder.
            Button {
                if reminderPrefs != nil {
                    showingRemindersSheet = true
                } else {
                    remindersPendingOpen = true
                    Task { await reloadReminders() }
                }
            } label: {
                HStack {
                    Text("Weight reminders")
                        .foregroundStyle(DSColors.text)
                    Spacer()
                    Text(reminderSummaryText)
                        .foregroundStyle(DSColors.textSecondary)
                    Image(systemName: "chevron.right")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.tertiary)
                }
            }
        }
    }

    /// Summary text for the weight-reminders row.
    private var reminderSummaryText: String {
        if let prefs = reminderPrefs {
            return prefs.summary
        }
        return remindersFailed ? "Couldn't load" : "Loading…"
    }

    /// Loads the reminder schedule once for the summary row (and as
    /// the editor sheet's initial state). Overlapping calls collapse
    /// into one (see remindersLoading). `remindersLoading` is cleared
    /// before `reminderPrefs` is published or a remembered tap-open
    /// presents the sheet, so no trailing write lands during the
    /// presentation animation. Failures flag for retry rather than
    /// inventing a default the sheet would then present as fact.
    @MainActor
    private func reloadReminders() async {
        guard !remindersLoading else { return }
        remindersLoading = true
        remindersFailed = false
        do {
            let fetched = try await env.api.getReminderPreferences()
            // Clear loading first: the presenter (see body) must be
            // idle before data + sheet-presentation mutate it.
            remindersLoading = false
            reminderPrefs = fetched
            if remindersPendingOpen {
                remindersPendingOpen = false
                showingRemindersSheet = true
            }
        } catch {
            remindersLoading = false
            remindersFailed = true
            remindersPendingOpen = false
        }
    }

    /// System / Light / Dark picker. Uses a 3-way segmented
    /// control so all three options are visible without a
    /// tap, matching the web's inline toggle.
    private var appearanceSection: some View {
        Section {
            Picker("Appearance", selection: themeMode) {
                ForEach(ThemeMode.allCases) { mode in
                    Label(mode.displayName, systemImage: mode.systemImage)
                        .tag(mode)
                }
            }
            .pickerStyle(.segmented)
        } header: {
            Text("Appearance")
        } footer: {
            Text("Match the device, or override with Light or Dark.")
        }
    }

    /// Master switch for beta features. Bound directly to
    /// `@AppStorage("betaFeaturesEnabled")` so the change
    /// writes through to `UserDefaults` immediately and the
    /// `BetaFeature` wrapper reflects it without any glue
    /// code. No "Save" toolbar — the `Toggle` is the source
    /// of truth, matching how the appearance picker just works.
    private var betaSection: some View {
        Section {
            Toggle("Beta features", isOn: $betaFeaturesEnabled)
        } header: {
            Text("Beta")
        } footer: {
            Text("Try features before they're released. They may change or be removed without notice.")
        }
    }

    // MARK: - Helpers

    private func initials(for name: String) -> String {
        let parts = name
            .split(separator: " ", omittingEmptySubsequences: true)
            .prefix(2)
        let chars = parts.compactMap { $0.first.map(String.init) }
        return chars.joined().uppercased()
    }
}

#Preview {
    ProfileView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
        .environmentObject(AuthStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
}
