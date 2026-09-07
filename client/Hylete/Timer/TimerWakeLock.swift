import SwiftUI
import UIKit

/// Wraps `UIApplication.shared.isIdleTimerDisabled` so the
/// timer sub-views can keep the screen awake while running
/// without poking at `UIKit` directly. Best-effort: iOS
/// still applies its own background-idle rules once the app
/// leaves the foreground, which matches the web's
/// `wake-lock.js` behavior (the wake lock is a hint, not a
/// guarantee).
@MainActor
enum TimerWakeLock {
    /// Disables the screen idle timer. Idempotent — calling
    /// twice in a row has no extra effect.
    static func acquire() {
        UIApplication.shared.isIdleTimerDisabled = true
    }

    /// Re-enables the screen idle timer. Safe to call when
    /// already released (e.g. from `onDisappear` after a
    /// clean stop) and from any state-machine path so a
    /// forgotten `release()` upstream can't leave the screen
    /// on indefinitely.
    static func release() {
        UIApplication.shared.isIdleTimerDisabled = false
    }
}
