import Foundation

/// The single dependency container for the running app.
/// Created in `HyleteApp` once at launch and injected into
/// every view via `@EnvironmentObject`.
@MainActor
public final class AppEnvironment: ObservableObject {
    public let api: APIClient
    public let authStore: AuthStore

    public init(api: APIClient, authStore: AuthStore) {
        self.api = api
        self.authStore = authStore
    }

    /// Builds the live app container: one `APIClient` and
    /// one `AuthStore`, wired so that 401s from any request
    /// sign the user out and a successful launch with a
    /// stored token hydrates `currentUser` from `GET /me`.
    public static func live(baseURL override: URL? = nil) -> AppEnvironment {
        // The authStore needs a reference to the api (for
        // `me()` during session restore) and the api needs
        // a reference to the authStore (to read the token
        // and bounce on 401). We resolve the cycle by
        // creating the authStore first with a stub API,
        // then creating the real API wired to the store,
        // then swapping the stub for the real one on the
        // store.
        let resolvedURL: URL = override ?? resolveBaseURL()
        let authStore = AuthStore(api: APIClient(
            baseURL: resolvedURL,
            tokenProvider: { nil },
            onUnauthorized: {}
        ))
        let api = APIClient(
            baseURL: resolvedURL,
            tokenProvider: { [weak authStore] in authStore?.currentToken() },
            onUnauthorized: { [weak authStore] in
                Task { @MainActor in authStore?.signOut() }
            }
        )
        authStore.swapAPI(api)
        return AppEnvironment(api: api, authStore: authStore)
    }

    /// Resolves the API base URL from `Info.plist`. The
    /// default `http://localhost:8080/api/v1` matches the
    /// `make dev` server on the developer's machine when
    /// the simulator runs on the same Mac.
    private static func resolveBaseURL() -> URL {
        if let raw = Bundle.main.object(forInfoDictionaryKey: "HYLETE_API_BASE_URL") as? String,
           !raw.isEmpty,
           let url = URL(string: raw) {
            return url
        }
        return URL(string: "http://localhost:8080/api/v1")!
    }
}
