import SwiftUI

/// The single entry point. Builds the live `AppEnvironment`
/// at launch and switches the root view between the auth
/// flow and the main tab view based on `authStore.currentUser`.
///
/// The `restoreSession()` call runs on first appearance so
/// the app can rehydrate the Keychain token and the cached
/// user before the user sees anything other than a brief
/// splash. While that's running, `RootView` shows a spinner
/// (see `isRestoring` on `AuthStore`).
@main
struct HyleteApp: App {
    @StateObject private var env = AppEnvironment.live()

    var body: some Scene {
        WindowGroup {
            RootView()
                .environmentObject(env)
                .environmentObject(env.authStore)
                .applyThemeMode()
                .task {
                    await env.authStore.restoreSession()
                }
        }
    }
}

/// The actual root view. Switches between the auth flow
/// (when no user is signed in) and the main tab view.
struct RootView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore

    var body: some View {
        Group {
            if authStore.isRestoring {
                SplashView()
            } else if authStore.currentUser != nil {
                MainTabView()
            } else {
                LoginView()
            }
        }
        .animation(.easeInOut(duration: 0.2), value: authStore.isRestoring)
        .animation(.easeInOut(duration: 0.2), value: authStore.currentUser?.id)
    }
}

/// Lightweight splash shown while `restoreSession` is
/// running. Avoids the flash-of-login-screen on cold start
/// for users who already have a valid token.
struct SplashView: View {
    var body: some View {
        ZStack {
            DSColors.background.ignoresSafeArea()
            ProgressView()
                .controlSize(.large)
        }
    }
}

/// The main app shell — five fixed tabs: dashboard,
/// exercises, weight, goals, and more. The count NEVER
/// exceeds five: iOS collapses tab 6+ into an auto-generated
/// system `More` list, whose own navigation controller stacked
/// on top of each tab's `NavigationStack` and produced the
/// double nav-bar + stray back-button bug. `Profile`, `Coach`,
/// and `Health` live as rows inside the `More` hub instead —
/// new destinations scale there, never as new tabs.
///
/// Each tab owns exactly one `NavigationStack`: `Dashboard`
/// brings its own (it needs a `path` binding), the other four
/// are wrapped here. Tab content itself is stack-less by
/// convention — never add a `NavigationStack` inside a pushed
/// destination. Sheets present outside the tab hierarchy, so
/// sheet editors keep their own stacks.
///
/// `GoalStore`, `WeightStore`, and `CoachStore` are constructed
/// once here (instead of inside their respective views) so they
/// outlive view rebuilds and can be shared with editors and
/// the More hub — every observer sees the same store, so a
/// save updates all rows immediately.
struct MainTabView: View {
    @EnvironmentObject private var env: AppEnvironment

    @StateObject private var goalStore: GoalStore
    @StateObject private var weightStore: WeightStore
    @StateObject private var coachStore: CoachStore
    @StateObject private var blockStore: BlockStore

    init() {
        // Construct with a stub API; swapped to the real
        // one in `.onAppear` once the environment is
        // available. SwiftUI's `@StateObject` ignores the
        // closure's environment on init, so the swap is
        // safer than reading `env.api` here.
        let stub = APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )
        _goalStore = StateObject(wrappedValue: GoalStore(api: stub))
        _weightStore = StateObject(wrappedValue: WeightStore(api: stub))
        _coachStore = StateObject(wrappedValue: CoachStore(api: stub))
        _blockStore = StateObject(wrappedValue: BlockStore(api: stub))
    }

    var body: some View {
        TabView {
            // Dashboard owns its stack (path-bound for
            // exercise-history pushes) — do NOT wrap it.
            DashboardView(distanceUnit: env.authStore.currentUser?.distanceUnit ?? "km")
                .tabItem { Label("Dashboard", systemImage: "house") }

            NavigationStack {
                ExerciseListView()
            }
            .tabItem { Label("Exercises", systemImage: "dumbbell") }

            NavigationStack {
                WeightListView(store: weightStore)
            }
            .tabItem { Label("Weight", systemImage: Icons.weight) }

            NavigationStack {
                GoalsListView(store: goalStore)
            }
            .tabItem { Label("Goals", systemImage: Icons.goals) }

            NavigationStack {
                MoreView(coachStore: coachStore, blockStore: blockStore)
            }
            .tabItem { Label("More", systemImage: Icons.more) }
        }
        .onAppear {
            // The stores need the real `APIClient` (which
            // reads the JWT from the auth store on each
            // request). Rebuilding them here is safe because
            // `onAppear` runs after the environment is
            // mounted, and `@StateObject` only honours the
            // initial wrapped value on the first body
            // evaluation.
            goalStore.replaceAPI(env.api)
            weightStore.replaceAPI(env.api)
            coachStore.replaceAPI(env.api)
            blockStore.replaceAPI(env.api)
        }
    }
}
