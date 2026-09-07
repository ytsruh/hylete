import SwiftUI

/// The user's appearance preference. Mirrors the web app's
/// `themeMode` `localStorage` value (which is one of `system`,
/// `light`, or `dark`); the iOS default is `system` so the
/// app follows the device until the user explicitly picks
/// otherwise in Profile → Appearance.
///
/// Persisted via `@AppStorage("themeMode")`; the raw value is
/// the enum's `rawValue` string so the storage is
/// human-readable in `defaults read com.hyleteapp.hylete`.
public enum ThemeMode: String, CaseIterable, Identifiable {

    /// Follow the device's current `ColorScheme`. The default.
    case system
    /// Force light, regardless of the device's setting.
    case light
    /// Force dark, regardless of the device's setting.
    case dark

    public var id: String { rawValue }

    public var displayName: String {
        switch self {
        case .system: return "System"
        case .light:  return "Light"
        case .dark:   return "Dark"
        }
    }

    public var systemImage: String {
        switch self {
        case .system: return "circle.lefthalf.filled"
        case .light:  return "sun.max"
        case .dark:   return "moon.fill"
        }
    }

    /// Maps the enum to a `ColorScheme?` that SwiftUI's
    /// `.preferredColorScheme(_:)` modifier accepts. `system`
    /// returns `nil` so the system setting wins.
    public var colorScheme: ColorScheme? {
        switch self {
        case .system: return nil
        case .light:  return .light
        case .dark:   return .dark
        }
    }
}

/// `View` extension that applies the user's persisted theme
/// choice as `.preferredColorScheme(_:)`. Use at the app root
/// so the override applies to every screen. Reads the same
/// `@AppStorage("themeMode")` key the Profile picker writes to.
public extension View {
    func applyThemeMode() -> some View {
        modifier(ThemeModeModifier())
    }
}

private struct ThemeModeModifier: ViewModifier {
    @AppStorage("themeMode") private var modeRaw: String = ThemeMode.system.rawValue

    func body(content: Content) -> some View {
        let mode = ThemeMode(rawValue: modeRaw) ?? .system
        content.preferredColorScheme(mode.colorScheme)
    }
}
