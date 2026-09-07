import SwiftUI

/// Semantic colour tokens. The values are looked up from the
/// asset catalog at `client/Hylete/Assets.xcassets/DS/<token>.colorset/`,
/// which declares both a light and a dark variant. SwiftUI
/// automatically picks the right one based on the current
/// `ColorScheme`, so every consumer of `DSColors.*` gets correct
/// light/dark rendering for free.
///
/// **Note on the asset name**: even though the colorsets live in
/// a `DS/` folder group in the asset catalog, the *runtime* asset
/// name is the colorset name only — Xcode strips the folder
/// prefix when it compiles the catalog. So the lookup is
/// `Color("background", bundle: .main)`, not `Color("DS/background", …)`.
/// Use a wrong prefix and the lookup silently fails: the resulting
/// `Color` is `.clear`, the view background isn't painted, and
/// (most visibly) any white-text-on-accent button becomes
/// white-on-white.
///
/// Three tokens — `ds-primary`, `ds-secondary`, `ds-accent` — carry
/// a `ds-` prefix: their bare names collide with symbols Xcode
/// auto-generates from the asset catalog (`Color.primary` /
/// `Color.secondary` are SwiftUI's system label colours), which
/// otherwise emits `#warning`s in `GeneratedAssetSymbols.swift`.
///
/// The mapping (semantic → web CSS variable) is documented in
/// `BrandColors.swift`. The actual `Color` literals live in the
/// `.colorset/Contents.json` files — change those to rebrand,
/// not this file.
public enum DSColors {

    // MARK: - Surfaces

    /// Page background (e.g. login, list backgrounds).
    /// Maps to web `--background` / `Color(.systemBackground)`.
    public static let background = Color("background", bundle: .main)
    /// Card / list-row surface. Maps to web `--card`.
    public static let surface = Color("card", bundle: .main)
    /// Elevated surface for nested cards, pills, popovers.
    /// Maps to web `--popover`.
    public static let surfaceElevated = Color("popover", bundle: .main)
    /// Hairline divider. Maps to web `--border`.
    public static let separator = Color("border", bundle: .main)

    // MARK: - Secondary (inverted chip fill)

    /// High-contrast "inverted" fill for secondary chips and badges
    /// (e.g. the cardio exercise-type pill). Deliberately NOT a muted
    /// surface tint like the web's original `--secondary`: the pairing
    /// takes the theme's foreground luminance so chips read against
    /// both surfaces — dark fill in light mode, light fill in dark
    /// mode. Maps to web `--secondary` (see styles/input.css).
    /// Asset name is prefixed `ds-` — see the header note.
    public static let secondary = Color("ds-secondary", bundle: .main)
    /// Text/icon colour on a `secondary` surface.
    /// Maps to web `--secondary-foreground`.
    public static let onSecondary = Color("secondary-foreground", bundle: .main)

    // MARK: - Text

    /// Primary text. Maps to web `--foreground` / `Color(.label)`.
    public static let text = Color("foreground", bundle: .main)
    /// Secondary / caption text. Maps to web `--muted-foreground`.
    public static let textSecondary = Color("muted-foreground", bundle: .main)
    /// Text on a `primary` coloured surface (e.g. button label).
    /// Maps to web `--primary-foreground`.
    public static let onPrimary = Color("primary-foreground", bundle: .main)
    /// Text on a `card` surface. Maps to web `--card-foreground`.
    public static let onCard = Color("card-foreground", bundle: .main)

    // MARK: - Accent / Brand

    /// The single brand colour — used for primary buttons, the
    /// active tab tint, links, and the profile initials halo.
    /// Maps to web `--primary` (#F44900 brand orange).
    /// Asset name is prefixed `ds-` — see the header note.
    public static let accent = Color("ds-primary", bundle: .main)
    /// Soft accent background (e.g. a tinted row in a list).
    /// Maps to web `--accent`. Asset name is prefixed `ds-` —
    /// see the header note.
    public static let accentSubtle = Color("ds-accent", bundle: .main)
    /// Text on the soft accent surface. Maps to web `--accent-foreground`.
    public static let onAccentSubtle = Color("accent-foreground", bundle: .main)
    /// Focus ring. Maps to web `--ring`.
    public static let focusRing = Color("ring", bundle: .main)

    // MARK: - Status

    /// Destructive actions (delete, sign out). Maps to web `--destructive`.
    public static let destructive = Color("destructive", bundle: .main)
    /// Text on a destructive surface. Maps to web `--destructive-foreground`.
    public static let onDestructive = Color("destructive-foreground", bundle: .main)
    /// Success state (toasts, confirmation icons). Not in the web
    /// palette — uses Tailwind's green-500 (#22C55E).
    public static let success = Color("success", bundle: .main)

    // MARK: - Chart palette

    /// Ordered dark → light ramp, matching the donut chart's
    /// default brand palette (see `internal/views/components/
    /// donut_chart.templ`).
    public static let chart1 = Color("chart-1", bundle: .main)
    public static let chart2 = Color("chart-2", bundle: .main)
    public static let chart3 = Color("chart-3", bundle: .main)
    public static let chart4 = Color("chart-4", bundle: .main)
    public static let chart5 = Color("chart-5", bundle: .main)

    // MARK: - System colour fallback

    /// The system-managed accent (`Assets.xcassets/AccentColor`).
    /// Most code should use `DSColors.accent` instead; this is
    /// here for system components that read `Color.accentColor`.
    public static let systemAccent: Color = .accentColor
}
