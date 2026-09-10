import SwiftUI

/// The Coach archive: dismissed reviews, newest first. Dismiss
/// stamps rather than deletes, so this is just the store's
/// `dismissedReports` filter over the same server list — no
/// extra fetch, no extra endpoint.
///
/// Cards render with the shared `CoachCardView` and push the
/// shared detail. Swiping a card restores it to the main list
/// (leading-edge swipe, full-swipe enabled).
public struct CoachArchiveView: View {
    @ObservedObject var store: CoachStore

    public init(store: CoachStore) {
        self.store = store
    }

    public var body: some View {
        Group {
            if store.dismissedReports.isEmpty {
                List {
                    Section {
                        Text("Nothing here yet — dismissed reviews will appear here, and you can swipe any of them to put it back.")
                    }
                }
            } else {
                List(store.dismissedReports) { report in
                    NavigationLink {
                        CoachReportView(report: report)
                    } label: {
                        CoachCardView(report: report)
                    }
                    .swipeActions(edge: .leading, allowsFullSwipe: true) {
                        Button {
                            Task { await store.restore(id: report.id) }
                        } label: {
                            Label("Restore", systemImage: "arrow.uturn.left")
                        }
                        .tint(DSColors.accent)
                    }
                }
                .refreshable {
                    await store.load()
                }
            }
        }
        .navigationTitle("Archive")
        .navigationBarTitleDisplayMode(.inline)
        .refreshable {
            await store.load()
        }
        .alert("Coach", isPresented: errorBinding) {
            Button("OK", role: .cancel) {}
        } message: {
            Text(store.errorMessage ?? "")
        }
    }

    private var errorBinding: Binding<Bool> {
        Binding(
            get: { store.errorMessage != nil },
            set: { if !$0 { store.errorMessage = nil } }
        )
    }
}
