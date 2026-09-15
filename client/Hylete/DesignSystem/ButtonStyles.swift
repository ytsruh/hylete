import SwiftUI

/// Filled primary button — used for the most important
/// action on a screen (login, save, submit).
public struct DSPrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(.white)
            .frame(maxWidth: .infinity)
            .frame(height: 48)
            .background(isEnabled ? DSColors.accent : DSColors.accent.opacity(0.4))
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
            .opacity(configuration.isPressed ? 0.85 : 1)
            .animation(.easeOut(duration: 0.1), value: configuration.isPressed)
    }
}

/// Bordered secondary button — used for cancel / alternate
/// actions. Same size as the primary button so the two can
/// stack without visual imbalance.
public struct DSSecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.headline)
            .foregroundStyle(isEnabled ? DSColors.accent : DSColors.accent.opacity(0.4))
            .frame(maxWidth: .infinity)
            .frame(height: 48)
            .background(DSColors.surface)
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .stroke(DSColors.accent, lineWidth: 1)
            )
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
            .opacity(configuration.isPressed ? 0.85 : 1)
            .animation(.easeOut(duration: 0.1), value: configuration.isPressed)
    }
}

/// Compact filled primary button — same solid chrome as
/// `DSPrimaryButtonStyle` but one size down (40pt, subheadline).
/// For repeated per-card actions that must read below the
/// screen's full-size `dsPrimary` button (e.g. the player block
/// check-offs under "Finish workout").
public struct DSCompactPrimaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(.white)
            .frame(maxWidth: .infinity)
            .frame(height: 40)
            .background(isEnabled ? DSColors.accent : DSColors.accent.opacity(0.4))
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
            .opacity(configuration.isPressed ? 0.85 : 1)
            .animation(.easeOut(duration: 0.1), value: configuration.isPressed)
    }
}

/// Compact bordered secondary button — same outline chrome as
/// `DSSecondaryButtonStyle` but one size down (40pt, subheadline).
/// For repeated per-card secondary actions alongside a compact
/// or full-size primary button.
public struct DSCompactSecondaryButtonStyle: ButtonStyle {
    @Environment(\.isEnabled) private var isEnabled

    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(isEnabled ? DSColors.accent : DSColors.accent.opacity(0.4))
            .frame(maxWidth: .infinity)
            .frame(height: 40)
            .background(DSColors.surface)
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .stroke(DSColors.accent, lineWidth: 1)
            )
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
            .opacity(configuration.isPressed ? 0.85 : 1)
            .animation(.easeOut(duration: 0.1), value: configuration.isPressed)
    }
}

/// Plain text button used inside cards and list rows.
public struct DSTextButtonStyle: ButtonStyle {
    public init() {}

    public func makeBody(configuration: Configuration) -> some View {
        configuration.label
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(DSColors.accent)
            .opacity(configuration.isPressed ? 0.6 : 1)
    }
}

public extension ButtonStyle where Self == DSPrimaryButtonStyle {
    static var dsPrimary: DSPrimaryButtonStyle { DSPrimaryButtonStyle() }
}

public extension ButtonStyle where Self == DSSecondaryButtonStyle {
    static var dsSecondary: DSSecondaryButtonStyle { DSSecondaryButtonStyle() }
}

public extension ButtonStyle where Self == DSCompactSecondaryButtonStyle {
    static var dsSecondaryCompact: DSCompactSecondaryButtonStyle { DSCompactSecondaryButtonStyle() }
}

public extension ButtonStyle where Self == DSCompactPrimaryButtonStyle {
    static var dsPrimaryCompact: DSCompactPrimaryButtonStyle { DSCompactPrimaryButtonStyle() }
}

public extension ButtonStyle where Self == DSTextButtonStyle {
    static var dsText: DSTextButtonStyle { DSTextButtonStyle() }
}
