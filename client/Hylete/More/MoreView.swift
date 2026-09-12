import SwiftUI

/// The "More" hub tab — the explicit replacement for the old
/// auto-generated system `More` tab.
///
/// The tab bar is hard-capped at five (`Dashboard`, `Exercises`,
/// `Weight`, `Goals`, `More`) so iOS never collapses overflow
/// into its own `More` list. The system one wrapped selections
/// in its own navigation controller, which stacked on top of
/// each tab's own `NavigationStack` and produced the double
/// nav-bar + back-button bug. Here `MainTabView` owns the single
/// `NavigationStack` around this view, and every destination
/// below is stack-less content — push, one bar, back works.
///
/// New destinations scale here, never as new tabs: add a row
/// (gated by `BetaFeature` while experimental). `Timer` stays
/// where it is (a `Dashboard` sheet) by deliberate scope cut —
/// promote it to a row here only if it outgrows the sheet.
struct MoreView: View {
    @EnvironmentObject private var env: AppEnvironment

    @ObservedObject var coachStore: CoachStore
    @ObservedObject var blockStore: BlockStore

    /// Mirrors the Profile beta toggle. Gates the Insights
    /// section itself (in addition to the `BetaFeature`
    /// wrapper on the destinations) so opted-out users see
    /// just their account instead of an empty section.
    @AppStorage("betaFeaturesEnabled") private var betaFeaturesEnabled: Bool = false

    var body: some View {
        List {
            Section("Account") {
                NavigationLink {
                    ProfileView()
                } label: {
                    profileRow
                }
                .accessibilityLabel("Open Profile")
            }

            if betaFeaturesEnabled {
                Section {
                    BetaFeature {
                        NavigationLink {
                            BlocksListView(store: blockStore)
                        } label: {
                            hubRow(
                                title: "Blocks",
                                subtitle: "Planned exercise groups",
                                systemImage: "rectangle.3.group"
                            )
                        }
                        .accessibilityLabel("Open Blocks")
                    }
                } header: {
                    Text("Workouts")
                } footer: {
                    Text("Beta features. Turn them off anytime in Profile.")
                }

                Section {
                    BetaFeature {
                        NavigationLink {
                            CoachView(store: coachStore)
                        } label: {
                            hubRow(
                                title: "Coach",
                                subtitle: "Weekly training reviews",
                                systemImage: "sparkles"
                            )
                        }
                        .accessibilityLabel("Open Coach")
                    }
                    BetaFeature {
                        NavigationLink {
                            HealthView(
                                weightUnit: env.authStore.currentUser?.weightUnit ?? "kg",
                                distanceUnit: env.authStore.currentUser?.distanceUnit ?? "km",
                                api: env.api
                            )
                        } label: {
                            hubRow(
                                title: "Health",
                                subtitle: "Apple Health vitals",
                                systemImage: "heart.text.square"
                            )
                        }
                        .accessibilityLabel("Open Health")
                    }
                } header: {
                    Text("Insights")
                } footer: {
                    Text("Beta features. Turn them off anytime in Profile.")
                }
            }
        }
        .navigationTitle("More")
    }

    /// Account card row: avatar + name/email, in the idiom of
    /// the iOS Settings account card. Falls back to a plain
    /// label when signed out (the tab is hidden in that state,
    /// but the fallback keeps body evaluation safe).
    @ViewBuilder
    private var profileRow: some View {
        if let user = env.authStore.currentUser {
            HStack(spacing: DSSpacing.md) {
                ZStack {
                    Circle()
                        .fill(DSColors.accent.opacity(0.15))
                        .frame(width: 44, height: 44)
                    Text(initials(for: user.name))
                        .font(.headline)
                        .foregroundStyle(DSColors.accent)
                }
                VStack(alignment: .leading, spacing: 2) {
                    Text(user.name)
                        .font(.body)
                        .foregroundStyle(DSColors.text)
                    Text(user.email)
                        .font(.subheadline)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
            .padding(.vertical, DSSpacing.xxs)
        } else {
            Label("Profile", systemImage: Icons.profile)
        }
    }

    /// Hub row with a two-line label so each destination is
    /// self-explanatory without opening it.
    private func hubRow(title: String, subtitle: String, systemImage: String) -> some View {
        HStack(spacing: DSSpacing.md) {
            Image(systemName: systemImage)
                .font(.title3)
                .foregroundStyle(DSColors.accent)
                .frame(width: 28)
            VStack(alignment: .leading, spacing: 2) {
                Text(title)
                    .font(.body)
                    .foregroundStyle(DSColors.text)
                Text(subtitle)
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    private func initials(for name: String) -> String {
        let parts = name
            .split(separator: " ", omittingEmptySubsequences: true)
            .prefix(2)
        let chars = parts.compactMap { $0.first.map(String.init) }
        return chars.joined().uppercased()
    }
}

#Preview {
    NavigationStack {
        MoreView(
            coachStore: CoachStore(api: APIClient(
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
