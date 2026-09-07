import SwiftUI

/// `BetaFeature { SomeView() }` renders `SomeView` only when the
/// user has opted into beta features via the toggle in Profile.
///
/// Gated content is *hidden*, not just hidden-then-shown: opt-in
/// is what makes it appear. Default behaviour is to hide, so
/// wrapping a feature in `BetaFeature` is a deliberate signal
/// that it's unfinished, may change without notice, and could
/// be removed entirely.
///
/// Opt-in is persisted per-device in `UserDefaults` under the
/// key `"betaFeaturesEnabled"` (a `Bool`). This is iOS-only —
/// the web app does not read or write the same key, and there
/// is intentionally no server-side component. If a beta feature
/// ever needs to be gated server-side (e.g. for an API that
/// hasn't shipped yet), that is a separate mechanism and should
/// be composed alongside, not instead of, this client switch.
///
/// Usage:
///
///     BetaFeature {
///         ExperimentalWorkoutTemplatesButton()
///     }
///
/// Future evolution: if per-feature toggles are wanted, upgrade
/// the storage from `Bool` to `Set<String>` of feature names
/// (`BetaFeature("workoutTemplates") { ... }`) and surface them
/// as individual rows in Profile. Today, one master switch keeps
/// the UI minimal and the intent clear.
public struct BetaFeature<Content: View>: View {

    /// Backing storage for the master opt-in. Mirrored by
    /// `ProfileView`'s `Toggle` so flipping the switch reactively
    /// reveals or hides any wrapped content in the same run loop.
    @AppStorage("betaFeaturesEnabled") private var isEnabled: Bool = false

    @ViewBuilder private let content: () -> Content

    public init(@ViewBuilder content: @escaping () -> Content) {
        self.content = content
    }

    public var body: some View {
        if isEnabled {
            content()
        } else {
            EmptyView()
        }
    }
}

/// Non-view access to the beta opt-in, for the rare call site
/// that needs to branch in a model / view-model rather than a
/// SwiftUI body. Prefer wrapping UI in `BetaFeature { ... }`
/// when possible so the gate stays declarative.
public enum BetaFeatureFlag {

    /// The same `UserDefaults` key `BetaFeature` reads.
    public static let storageKey = "betaFeaturesEnabled"

    /// `true` when the user has opted into beta features.
    /// Defaults to `false` for first-launch users and existing
    /// users whose device has never seen the key.
    public static var isEnabled: Bool {
        UserDefaults.standard.bool(forKey: storageKey)
    }

    /// Programmatic setter. Prefer driving changes through the
    /// Profile `Toggle` (which writes through `@AppStorage`)
    /// so the UI stays the source of truth.
    public static func setEnabled(_ enabled: Bool) {
        UserDefaults.standard.set(enabled, forKey: storageKey)
    }
}

#Preview("Opted in") {
    VStack(spacing: DSSpacing.md) {
        Text("Visible because the master switch is on.")
        BetaFeature {
            Text("Wrapped feature content")
                .padding()
                .background(DSColors.accent.opacity(0.15))
        }
    }
    .padding()
    .onAppear { BetaFeatureFlag.setEnabled(true) }
}

#Preview("Opted out") {
    VStack(spacing: DSSpacing.md) {
        Text("Hidden because the master switch is off.")
        BetaFeature {
            Text("Wrapped feature content — should NOT appear")
                .padding()
                .background(DSColors.accent.opacity(0.15))
        }
    }
    .padding()
    .onAppear { BetaFeatureFlag.setEnabled(false) }
}
