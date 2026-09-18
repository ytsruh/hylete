import Foundation

/// Opt-out toggle for auto-starting Health tracking when the
/// Workout Player opens. Defaults ON (first launch and existing
/// installs with no stored value); Profile flips it off.
///
/// UserDefaults-local and iOS-only, like `healthSyncEnabled` —
/// there is intentionally no server component. `bool(forKey:)`
/// cannot express a true default, so the getter reads the raw
/// object and falls back to true when the key was never written.
public enum HealthAutoStart {
    /// UserDefaults key for the opt-out. Shared with the Profile
    /// toggle, which is the single writer — single source of
    /// truth, no manual mirroring.
    public static let enabledKey = "healthAutoStartEnabled"

    public static var isEnabled: Bool {
        UserDefaults.standard.object(forKey: enabledKey) as? Bool ?? true
    }

    public static func setEnabled(_ enabled: Bool) {
        UserDefaults.standard.set(enabled, forKey: enabledKey)
    }
}
