import SwiftUI

/// Beta-gated "Apple Health" row for `ProfileView`.
///
/// Lives inside `BetaFeature { ... }` at the call site so it
/// never renders for non-beta users. Shows the coarse link
/// status and drives the system permission sheet (read-only);
/// a denied state deep-links to Settings because iOS won't
/// re-show the sheet once denied.
public struct HealthConnectRow: View {
    @State private var status: HealthAuthStatus
    @State private var isWorking = false
    /// Last connect failure, shown under the row so a failed
    /// tap never looks dead (previously `try?` swallowed the
    /// error and the button silently stayed on "Connect").
    @State private var errorMessage: String?

    private let provider: HealthDataProvider

    public init(provider: HealthDataProvider = LiveHealthStore()) {
        self.provider = provider
        _status = State(initialValue: provider.authorizationStatus())
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            HStack {
                Image(systemName: "heart.text.square")
                    .foregroundStyle(DSColors.accent)
                Text("Apple Health")
                Spacer()
                switch status {
                case .granted:
                    Text("Connected")
                        .foregroundStyle(DSColors.textSecondary)
                case .denied:
                    Button("Open Settings") { openSettings() }
                case .unavailable:
                    Text("Unavailable")
                        .foregroundStyle(DSColors.textSecondary)
                case .notRequested:
                    Button(isWorking ? "Connecting…" : "Connect") {
                        Task {
                            isWorking = true
                            errorMessage = nil
                            do {
                                try await provider.requestAuthorization()
                            } catch {
                                errorMessage = HealthViewModel.connectFailureMessage(for: error)
                            }
                            status = provider.authorizationStatus()
                            isWorking = false
                        }
                    }
                    .disabled(isWorking)
                }
            }
            if let errorMessage {
                Text(errorMessage)
                    .font(.caption)
                    .foregroundStyle(DSColors.textSecondary)
            }
        }
        .onAppear { status = provider.authorizationStatus() }
    }

    private func openSettings() {
        if let url = URL(string: UIApplication.openSettingsURLString) {
            UIApplication.shared.open(url)
        }
    }
}

#Preview("Not requested") {
    List {
        HealthConnectRow(provider: MockHealthStore(status: .notRequested))
        HealthConnectRow(provider: MockHealthStore(status: .granted))
        HealthConnectRow(provider: MockHealthStore(status: .denied))
    }
}
