import SwiftUI

/// Beta-gated "Coach" settings section for `ProfileView`.
///
/// Settings only — reports live on the Coach tab. Shows the
/// server-side toggle state (On/Off) plus whether an aim is set;
/// tapping pushes the edit action up to the parent (`onEdit`).
///
/// The store is owned by the parent (`ProfileView`) and the
/// editor sheet lives on the parent's NavigationStack host, NOT
/// here. A sheet attached to this Section tears down whenever
/// the row re-renders (e.g. when `load()` publishes `isLoading`
/// then `preferences`), which surfaced as the sheet popping up
/// and instantly closing with no data. Same rule as `ProfileView`'s
/// own sheets (see the comment on its `.task`/`.sheet` block).
///
/// Lives inside `BetaFeature { ... }` at the call site so it
/// never renders for non-beta users.
public struct CoachSettingsSectionView: View {
    @ObservedObject var store: CoachStore
    var onEdit: () -> Void

    public init(store: CoachStore, onEdit: @escaping () -> Void) {
        self.store = store
        self.onEdit = onEdit
    }

    public var body: some View {
        Section {
            Button {
                onEdit()
            } label: {
                HStack {
                    Image(systemName: "sparkles")
                        .foregroundStyle(DSColors.accent)
                    Text("Enable")
                        .foregroundStyle(DSColors.text)
                    Spacer()
                    Text(summaryText)
                        .foregroundStyle(DSColors.textSecondary)
                    Image(systemName: "chevron.right")
                        .font(.footnote.weight(.semibold))
                        .foregroundStyle(.tertiary)
                }
            }
        } header: {
            Text("Coach")
        } footer: {
            Text("Toggle & refine a Weekly AI review of your training")
        }
    }

    private var summaryText: String {
        guard let prefs = store.preferences else {
            return "Loading…"
        }
        return prefs.optIn ? "On" : "Off"
    }
}
