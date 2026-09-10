import SwiftUI

/// Beta-gated "Sync Health to Hylete" row for `ProfileView`'s
/// Connected Accounts section.
///
/// This is *consent*, not the beta flag: flipping it on runs
/// the one-time 90-day backfill, then keeps every past day
/// caught up opportunistically. Flipping it off cancels any
/// in-flight run and stops future uploads (already-uploaded
/// days stay on the server until a future export/deletion
/// surface covers them). Lives inside `BetaFeature { ... }`
/// at the call site like the connect row.
public struct HealthSyncRow: View {
    @AppStorage(HealthSyncStore.enabledKey) private var isEnabled: Bool = false
    @ObservedObject private var syncStore = HealthSyncStore.shared
    @State private var runTask: Task<Void, Never>?

    private let api: HealthSnapshotAPI

    public init(api: HealthSnapshotAPI) {
        self.api = api
    }

    public var body: some View {
        VStack(alignment: .leading, spacing: 2) {
            Toggle("Sync Health", isOn: $isEnabled)
                .onChange(of: isEnabled) { _, on in
                    if on {
                        runTask = Task {
                            await syncStore.syncIfNeeded(api: api)
                        }
                    } else {
                        runTask?.cancel()
                        runTask = nil
                    }
                }
            Text(syncStore.statusLine)
                .font(.caption)
                .foregroundStyle(DSColors.textSecondary)
        }
    }
}

#Preview("Off") {
    List {
        HealthSyncRow(api: MockSnapshotAPI())
    }
}
