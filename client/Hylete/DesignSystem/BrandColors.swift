import SwiftUI

/// Index of every brand colour used by the app, in one file for easy auditing.
///
/// **The values themselves live in the asset catalog** at
/// `client/Hylete/Assets.xcassets/DS/<token>.colorset/Contents.json`.
/// Each colorset declares a light (universal) variant and a dark
/// (`luminosity: dark`) variant. Swift code consumes them via
/// `Color("<token>", bundle: .main)` (see `Colors.swift`) —
/// Xcode strips the `DS/` folder prefix at compile time, so the
/// runtime name is the colorset name only. Three colorsets carry
/// a `ds-` prefix (`ds-primary`, `ds-secondary`, `ds-accent`) to
/// avoid Xcode generated-symbol conflicts; the web tokens in the
/// table below keep their bare names.
///
/// This file is the source-of-truth comment block: when the web
/// brand changes, update the OKLCH values in
/// `styles/input.css` first, re-run the conversion in
/// `scripts/oklch-to-srgb.py` (or eyeball it from a converter), and
/// then drop the new sRGB triples into the matching
/// `Contents.json` files. Update the table below to match.
///
/// Conversion is OKLCH → linear sRGB (Björn Ottosson's matrices),
/// then linear → gamma-corrected sRGB. Out-of-gamut colours are
/// chroma-reduced via binary search so they fit in [0, 1]^3.
///
/// Donut chart brand: `#F54900` lives at `DS/chart-1` and the
/// ramp at `DS/chart-2` … `DS/chart-5`.

public enum BrandColors {

    // MARK: - Light mode (`:root` in styles/input.css)

    public static let light: [String: String] = [
        "background":                 "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "foreground":                 "oklch(0.2686 0.0000 0.0000)  #252525",
        "card":                       "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "card-foreground":            "oklch(0.2686 0.0000 0.0000)  #252525",
        "popover":                    "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "popover-foreground":         "oklch(0.2686 0.0000 0.0000)  #252525",
        "primary":                    "oklch(0.6460 0.2220 41.1160) #F44900  (brand orange)",
        "primary-foreground":         "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "secondary":                  "oklch(0.9670 0.0029 264.5419) #F3F4F6",
        "secondary-foreground":       "oklch(0.4461 0.0263 256.8018) #4B5462",
        "muted":                      "oklch(0.9846 0.0017 247.8389) #F8F9FA",
        "muted-foreground":           "oklch(0.5510 0.0234 264.3637) #6A7180",
        "accent":                     "oklch(0.9869 0.0214 95.2774)  #FEFBEB",
        "accent-foreground":          "oklch(0.4732 0.1247 46.2007)  #92400E",
        "destructive":                "oklch(0.6368 0.2078 25.3313)  #EE4444",
        "destructive-foreground":     "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "border":                     "oklch(0.9276 0.0058 264.5313) #E5E7EA",
        "input":                      "oklch(0.9276 0.0058 264.5313) #E5E7EA",
        "ring":                       "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "chart-1":                    "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "chart-2":                    "oklch(0.6658 0.1574 58.3183)  #D87606",
        "chart-3":                    "oklch(0.5553 0.1455 48.9975)  #B45309",
        "chart-4":                    "oklch(0.4732 0.1247 46.2007)  #92400E",
        "chart-5":                    "oklch(0.4137 0.1054 45.9038)  #78340E",
        "sidebar":                    "oklch(0.9846 0.0017 247.8389) #F8F9FA",
        "sidebar-foreground":         "oklch(0.2686 0.0000 0.0000)  #252525",
        "sidebar-primary":            "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "sidebar-primary-foreground": "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "sidebar-accent":             "oklch(0.9869 0.0214 95.2774)  #FEFBEB",
        "sidebar-accent-foreground":  "oklch(0.4732 0.1247 46.2007)  #92400E",
        "sidebar-border":             "oklch(0.9276 0.0058 264.5313) #E5E7EA",
        "sidebar-ring":               "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "success":                    "(no web token)               #22C55E (Tailwind green-500)",
    ]

    // MARK: - Dark mode (`.dark` in styles/input.css)

    public static let dark: [String: String] = [
        "background":                 "oklch(0.2046 0.0000 0.0000)  #161616",
        "foreground":                 "oklch(0.9219 0.0000 0.0000)  #E4E4E4",
        "card":                       "oklch(0.2686 0.0000 0.0000)  #252525",
        "card-foreground":            "oklch(0.9219 0.0000 0.0000)  #E4E4E4",
        "popover":                    "oklch(0.2686 0.0000 0.0000)  #252525",
        "popover-foreground":         "oklch(0.9219 0.0000 0.0000)  #E4E4E4",
        "primary":                    "oklch(0.6460 0.2220 41.1160) #F44900  (brand orange)",
        "primary-foreground":         "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "secondary":                  "oklch(0.2686 0.0000 0.0000)  #252525",
        "secondary-foreground":       "oklch(0.9219 0.0000 0.0000)  #E4E4E4",
        "muted":                      "oklch(0.2393 0.0000 0.0000)  #1F1F1F",
        "muted-foreground":           "oklch(0.7155 0.0000 0.0000)  #A3A3A3",
        "accent":                     "oklch(0.4732 0.1247 46.2007)  #92400E",
        "accent-foreground":          "oklch(0.9243 0.1151 95.7459)  #FDE68A",
        "destructive":                "oklch(0.6368 0.2078 25.3313)  #EE4444",
        "destructive-foreground":     "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "border":                     "oklch(0.3715 0.0000 0.0000)  #404040",
        "input":                      "oklch(0.3715 0.0000 0.0000)  #404040",
        "ring":                       "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "chart-1":                    "oklch(0.8369 0.1644 84.4286)  #FBBF24",
        "chart-2":                    "oklch(0.6658 0.1574 58.3183)  #D87606",
        "chart-3":                    "oklch(0.4732 0.1247 46.2007)  #92400E",
        "chart-4":                    "oklch(0.5553 0.1455 48.9975)  #B45309",
        "chart-5":                    "oklch(0.4732 0.1247 46.2007)  #92400E",
        "sidebar":                    "oklch(0.1684 0.0000 0.0000)  #0E0E0E",
        "sidebar-foreground":         "oklch(0.9219 0.0000 0.0000)  #E4E4E4",
        "sidebar-primary":            "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "sidebar-primary-foreground": "oklch(1.0000 0.0000 0.0000)  #FEFEFE",
        "sidebar-accent":             "oklch(0.4732 0.1247 46.2007)  #92400E",
        "sidebar-accent-foreground":  "oklch(0.9243 0.1151 95.7459)  #FDE68A",
        "sidebar-border":             "oklch(0.3715 0.0000 0.0000)  #404040",
        "sidebar-ring":               "oklch(0.7686 0.1647 70.0804)  #F59D0A",
        "success":                    "(no web token)               #22C55E (Tailwind green-500)",
    ]

    /// The brand orange in plain sRGB, for use in code that needs
    /// a literal `Color` value (e.g. previews, debug overlays).
    /// Mirrors `--primary` from the web (light + dark are identical).
    public static let brandOrange = Color(red: 0.961, green: 0.286, blue: 0.0)
}
