import SwiftUI

/// The "Profile" tab. Shows the signed-in user's basic info
/// and a sign-out button. Tapping the name, weight unit, or
/// target weight row pushes a per-field editor that PUTs the
/// change to `/api/v1/me` and refreshes `authStore.currentUser`
/// on success so the rest of the app sees the new value.
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

    var body: some View {
        NavigationStack {
            List {
                if let user = authStore.currentUser {
                    headerSection(user: user)
                    accountSection(user: user)
                    preferencesSection(user: user)
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
            NavigationLink {
                NameEditView(user: user)
            } label: {
                HStack {
                    Text("Name")
                    Spacer()
                    Text(user.name)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
        }
    }

    private func preferencesSection(user: UserDTO) -> some View {
        Section("Preferences") {
            NavigationLink {
                WeightUnitEditView(user: user)
            } label: {
                HStack {
                    Text("Weight unit")
                    Spacer()
                    Text(user.weightUnit)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
            NavigationLink {
                TargetWeightEditView(user: user)
            } label: {
                HStack {
                    Text("Target weight")
                    Spacer()
                    if let target = user.targetWeight {
                        Text(String(format: "%.1f %@", target, user.weightUnit))
                            .foregroundStyle(DSColors.textSecondary)
                    } else {
                        Text("Not set")
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
            }
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
