import SwiftUI

/// Beta-gated "Auto-start tracking" row for `ProfileView`'s
/// Connected Accounts section.
///
/// Opt-out for auto-starting Health tracking when the Workout
/// Player opens. Defaults ON (first launch and existing installs
/// with no stored value read as true) — `@AppStorage` cannot
/// express a true default, so this row binds through
/// `HealthAutoStart` instead. Lives inside `BetaFeature { ... }`
/// at the call site like the connect/sync rows. Off restores the
/// manual Start button as the only trigger.
public struct HealthAutoStartRow: View {
    @State private var isEnabled: Bool = HealthAutoStart.isEnabled

    public init() {}

    public var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Toggle("Auto-start tracking", isOn: $isEnabled)
                .onChange(of: isEnabled) { _, on in
                    HealthAutoStart.setEnabled(on)
                }
            Text("Start recording when a workout opens.")
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
        }
    }
}

#Preview("On") {
    List {
        HealthAutoStartRow()
    }
}
