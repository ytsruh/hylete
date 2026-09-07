import SwiftUI

/// Spacing scale. All paddings, gaps, and insets in the app
/// pull from here so the rhythm stays consistent and a
/// future re-tune is a one-file change.
public enum DSSpacing {
    public static let xxs: CGFloat = 4
    public static let xs: CGFloat = 8
    public static let sm: CGFloat = 12
    public static let md: CGFloat = 16
    public static let lg: CGFloat = 24
    public static let xl: CGFloat = 32
    public static let xxl: CGFloat = 48

    /// Standard corner radius for cards and buttons.
    public static let cornerRadius: CGFloat = 12
    /// Smaller radius used inside cards (chips, inline
    /// fields, etc.).
    public static let cornerRadiusSmall: CGFloat = 8
}
