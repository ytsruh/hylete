import SwiftUI

/// Subtle text field style that uses the design system's
/// `surfaceElevated` colour as the background. The system
/// `.roundedBorder` style paints a stark white-on-black
/// (or black-on-white) field that's jarring in dark mode
/// and visually heavy in light mode. This style reads as
/// a soft, design-system-consistent input in both modes.
public struct DSTextFieldStyle: TextFieldStyle {
    public init() {}

    public func _body(configuration: TextField<Self._Label>) -> some View {
        configuration
            .padding(.horizontal, DSSpacing.sm)
            .padding(.vertical, DSSpacing.xs)
            .background(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
                    .fill(DSColors.surfaceElevated)
            )
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
                    .stroke(DSColors.separator, lineWidth: 0.5)
            )
    }
}

public extension TextFieldStyle where Self == DSTextFieldStyle {
    static var ds: DSTextFieldStyle { DSTextFieldStyle() }
}
