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

/// The main app shell — five tabs: dashboard, exercises,
/// weight, goals, and profile. The "new set" flow is a
/// sheet from the dashboard's toolbar so it's always one
/// tap away.
///
/// `GoalStore` and `WeightStore` are constructed once here
/// (instead of inside their respective list views) so they
/// outlive view rebuilds and can be shared with the
/// editor — both views observe the same store, so a save
/// in the editor updates the list immediately.
struct MainTabView: View {
    @EnvironmentObject private var env: AppEnvironment

    @StateObject private var goalStore: GoalStore
    @StateObject private var weightStore: WeightStore

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
    }

    var body: some View {
        TabView {
            DashboardView()
                .tabItem { Label("Dashboard", systemImage: "house") }

            ExerciseListView()
                .tabItem { Label("Exercises", systemImage: "dumbbell") }

            WeightListView(store: weightStore)
                .tabItem { Label("Weight", systemImage: Icons.weight) }

            GoalsListView(store: goalStore)
                .tabItem { Label("Goals", systemImage: Icons.goals) }

            ProfileView()
                .tabItem { Label("Profile", systemImage: "person.crop.circle") }
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
        }
    }
}
