import SwiftUI

/// Type scale and font face for the app. The web uses Inter as
/// the primary face (see `styles/input.css` → `--font-sans`).
/// iOS doesn't ship Inter, so we load the static 18pt weights
/// from `Fonts/Inter/static/Inter_18pt-*.ttf`. Inter ships
/// three optical sizes (18pt, 24pt, 28pt); the 18pt variants
/// are the "Inter for UI" recommendation, optimised for
/// small text on screens.
///
/// The PostScript names below include the `18pt` segment
/// (e.g. `Inter18pt-Regular`) — that's how Inter namespaces
/// its optical sizes in the font's own metadata, and the
/// `Font.custom(_:)` lookup matches the PostScript name, not
/// the file name. The matching files are listed in
/// `Info.plist`'s `UIAppFonts` array so iOS registers them
/// on launch.
///
/// If a font isn't registered (e.g. before the user adds the
/// `.ttf` files), every accessor silently falls back to the
/// system font (SF Pro) — the app still renders.

public enum DSFont {

    // MARK: - Font file names (PostScript names expected by Core Text)

    private static let regular  = "Inter18pt-Regular"
    private static let medium   = "Inter18pt-Medium"
    private static let semibold = "Inter18pt-SemiBold"
    private static let bold     = "Inter18pt-Bold"

    // MARK: - Brand faces

    /// Inter Regular — body copy, default UI text.
    public static let sans: Font = .custom(regular, size: 17, relativeTo: .body)

    /// Source Serif 4 — used for marketing-ish headings on the
    /// web. Not bundled on iOS; falls back to the system serif
    /// (New York) which has a similar feel.
    public static let serif: Font = .system(.body, design: .serif)

    /// JetBrains Mono — for monospaced numerics (the dashboard's
    /// `reps × weight` readout, etc.). Falls back to SF Mono.
    public static let mono: Font = .system(.body, design: .monospaced)

    // MARK: - Type scale (matches Apple's text styles, Inter weighted)

    public static let largeTitle: Font = .custom(bold,     size: 34, relativeTo: .largeTitle)
    public static let title:      Font = .custom(bold,     size: 28, relativeTo: .title)
    public static let title2:     Font = .custom(semibold, size: 22, relativeTo: .title2)
    public static let title3:     Font = .custom(semibold, size: 20, relativeTo: .title3)
    public static let headline:   Font = .custom(semibold, size: 17, relativeTo: .headline)
    public static let body:       Font = .custom(regular,  size: 17, relativeTo: .body)
    public static let callout:    Font = .custom(regular,  size: 16, relativeTo: .callout)
    public static let subheadline: Font = .custom(medium,  size: 15, relativeTo: .subheadline)
    public static let footnote:   Font = .custom(regular,  size: 13, relativeTo: .footnote)
    public static let caption1:   Font = .custom(regular,  size: 12, relativeTo: .caption)
    public static let caption2:   Font = .custom(regular,  size: 11, relativeTo: .caption2)

    // MARK: - Numeric emphasis

    /// Monospaced SemiBold for weight / reps readouts —
    /// prevents numbers from jumping width as the value
    /// changes. The web uses Inter for the same purpose.
    public static func monospacedDigits(_ size: CGFloat, weight: Font.Weight = .semibold) -> Font {
        .system(size: size, weight: weight, design: .monospaced)
    }
}
