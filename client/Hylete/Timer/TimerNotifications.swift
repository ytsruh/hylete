import Foundation
import UserNotifications

/// Authorisation state for timer fallback notifications.
/// Mirrors `HealthAuthStatus` semantics: unknown until
/// checked, settled after the system prompt.
public enum TimerNotificationAuthStatus: Equatable, Sendable {
    case notDetermined
    case granted
    case denied
}

/// Schedules and cancels the player timer's fallback local
/// notifications — the banner + vibration (+ sound unless
/// muted) that fires when iOS suspends the app mid-timer.
///
/// Local only: no push certificates, no server, no
/// entitlements, no `Info.plist` changes. While the app is
/// foreground iOS suppresses the banner by default, which is
/// exactly right — the pill flash + auto-expanded sheet own
/// the foreground case.
///
/// A protocol (like `HealthDataProvider`) so tests can drive
/// the scheduling logic with a mock instead of the system
/// notification center.
public protocol TimerNotificationCenter: Sendable {
    /// Current system authorisation. Cheap — callers may
    /// check before every schedule.
    func authorizationStatus() async -> TimerNotificationAuthStatus
    /// Shows the one-time system prompt. Returns the user's
    /// decision; throws on system error (treated as denied).
    func requestAuthorization() async throws -> Bool
    /// Schedules a one-shot time-sensitive alert. Replaces
    /// any pending request with the same id.
    func schedule(id: String, at date: Date, title: String, body: String) async throws
    /// Drops a pending request. Safe for unknown ids.
    func cancel(id: String)
}

/// Live `TimerNotificationCenter` backed by
/// `UNUserNotificationCenter`.
public final class LiveTimerNotificationCenter: TimerNotificationCenter, @unchecked Sendable {
    private let center: UNUserNotificationCenter

    public init(center: UNUserNotificationCenter = .current()) {
        self.center = center
    }

    public func authorizationStatus() async -> TimerNotificationAuthStatus {
        let settings = await center.notificationSettings()
        switch settings.authorizationStatus {
        case .authorized, .provisional, .ephemeral:
            return .granted
        case .denied:
            return .denied
        case .notDetermined:
            return .notDetermined
        @unknown default:
            return .notDetermined
        }
    }

    public func requestAuthorization() async throws -> Bool {
        try await center.requestAuthorization(options: [.alert, .sound, .badge])
    }

    public func schedule(id: String, at date: Date, title: String, body: String) async throws {
        // Time-sensitive so the alert breaks through Focus —
        // a rest timer you sleep through is useless. Needs no
        // entitlement (unlike critical alerts).
        let content = UNMutableNotificationContent()
        content.title = title
        content.body = body
        content.sound = .default
        content.interruptionLevel = .timeSensitive
        let interval = max(1, date.timeIntervalSinceNow)
        let trigger = UNTimeIntervalNotificationTrigger(timeInterval: interval, repeats: false)
        // `add` with an existing identifier replaces the old
        // request, so re-scheduling an id is self-cleaning.
        try await center.add(UNNotificationRequest(identifier: id, content: content, trigger: trigger))
    }

    public func cancel(id: String) {
        center.removePendingNotificationRequests(withIdentifiers: [id])
    }
}
