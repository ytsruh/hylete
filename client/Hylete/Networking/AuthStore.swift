import Foundation
import SwiftUI

/// Owns the JWT and the cached `UserDTO`. Lives at the app
/// root and is injected into every view via `@EnvironmentObject`.
///
/// All mutations happen on the main actor (Keychain access
/// is thread-safe but `@Published` requires main-actor writes
/// for SwiftUI to pick them up reliably). Reads from the
/// APIClient never go through this object — the client uses
/// its own `tokenProvider` closure.
@MainActor
public final class AuthStore: ObservableObject {
    /// Key under which the JWT is stored in the Keychain.
    /// Marked `nonisolated` so the (nonisolated) `currentToken()`
    /// and the API client's token-provider closure can read it
    /// from any thread — an immutable `String` constant needs no
    /// actor protection.
    public nonisolated static let tokenKey = "auth_token"

    /// The currently authenticated user, or nil when no
    /// session is active. SwiftUI views observe this via
    /// `@EnvironmentObject` to switch between the login
    /// flow and the main tab view.
    @Published public private(set) var currentUser: UserDTO?

    /// Set while `restoreSession()` is running on launch so
    /// the splash view can show a spinner instead of a
    /// flash-of-login-then-dashboard.
    @Published public private(set) var isRestoring: Bool = true

    private var api: APIClient
    private let userDefaultsKey = "hylete.cachedUser"

    public init(api: APIClient) {
        self.api = api
    }

    /// Used by `AppEnvironment.live()` to swap the API
    /// reference after both `AuthStore` and `APIClient`
    /// have been fully wired (the two have a circular
    /// dependency — the API needs a `tokenProvider` closure
    /// that reads from the store, and the store needs the
    /// API to call `me()` on launch). Production code paths
    /// never call this.
    public func swapAPI(_ api: APIClient) {
        self.api = api
    }

    /// Rehydrate the session from the Keychain on app launch.
    /// Reads the stored token, asks the server `GET /me` to
    /// confirm it's still valid, and populates `currentUser`
    /// on success. On any failure the token is discarded so
    /// the user lands on the login screen with a clean slate.
    public func restoreSession() async {
        defer { isRestoring = false }
        guard let token = try? Keychain.string(for: Self.tokenKey), !token.isEmpty else {
            currentUser = nil
            return
        }
        do {
            let me = try await api.me()
            currentUser = me
        } catch {
            // Stale or invalid token — clear it so the next
            // login starts from a clean slate.
            try? Keychain.remove(Self.tokenKey)
            currentUser = nil
        }
    }

    /// Persist the token + user after a successful login or
    /// register. The user object is also cached in
    /// UserDefaults so the UI can render the name
    /// immediately on the next launch, before the
    /// `GET /me` round-trip completes.
    public func acceptSession(token: String, user: UserDTO) {
        try? Keychain.set(token, for: Self.tokenKey)
        if let data = try? JSONEncoder().encode(user) {
            UserDefaults.standard.set(data, forKey: userDefaultsKey)
        }
        currentUser = user
    }

    /// Replace the cached user without touching the auth
    /// token. Used by the profile-edit flow after a
    /// successful `PUT /me`: the response carries the
    /// updated `UserDTO`, and the JWT itself is unchanged
    /// (the iOS client reads the token directly from the
    /// Keychain and never relies on server-issued cookies).
    /// The UserDefaults cache is refreshed so the next
    /// launch's name render is correct.
    public func updateCurrentUser(_ user: UserDTO) {
        if let data = try? JSONEncoder().encode(user) {
            UserDefaults.standard.set(data, forKey: userDefaultsKey)
        }
        currentUser = user
    }

    /// Forget the current session. The token is removed from
    /// the Keychain and the cached user is cleared. The
    /// server has nothing to do (JWTs are stateless) so no
    /// network call is needed.
    public func signOut() {
        try? Keychain.remove(Self.tokenKey)
        UserDefaults.standard.removeObject(forKey: userDefaultsKey)
        currentUser = nil
    }

    /// Used by `APIClient` to read the current token without
    /// the client having to know about the Keychain. Returns
    /// nil when no session is active.
    public nonisolated func currentToken() -> String? {
        try? Keychain.string(for: Self.tokenKey)
    }
}
