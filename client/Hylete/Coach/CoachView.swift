import SwiftUI

/// The Coach tab: weekly reviews as cards, newest first, with
/// past reviews on the same screen. Tapping a card pushes the
/// shared detail; swiping a card dismisses it.
///
/// Always presented from inside `BetaFeature { ... }` — the call
/// site guarantees beta opt-in, while the store's `isOptedOut`
/// tracks the *server-side* Coach toggle (a separate mechanism).
public struct CoachView: View {
    @ObservedObject var store: CoachStore
    @State private var showingSettings = false

    public init(store: CoachStore) {
        self.store = store
    }

    public var body: some View {
        // The stack lives here (not at the tab site) so there is
        // exactly one in the hierarchy — every other tab
        // (Goals, Weight, Health) owns its NavigationStack the
        // same way. A second, outer stack would stack back
        // buttons on the detail screen.
        NavigationStack {
            Group {
                if store.preferences == nil {
                    // First load hasn't completed: never flash the
                    // opted-out screen (nil prefs read as opted-out).
                    ProgressView("Loading Coach…")
            } else if store.isOptedOut {
                optedOutView
            } else if store.visibleReports.isEmpty, store.dismissedReports.isEmpty {
                // Truly empty: nothing live, nothing archived.
                // (Any dismissed rows take the card list below,
                // so the archive stays reachable.)
                if store.isLoading {
                    ProgressView("Loading your reviews…")
                } else {
                    noReportView
                }
            } else {
                cardList
            }
            }
            .navigationTitle("Coach")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                // Gear, not a row: settings (toggle + aim) live
                // here so the card list is pure content. The
                // destination shares the tab's store, so a save
                // in the editor refreshes this screen on return.
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showingSettings = true
                    } label: {
                        Image(systemName: "gearshape")
                    }
                    .accessibilityLabel("Coach settings")
                }
            }
            .sheet(isPresented: $showingSettings) {
                NavigationStack {
                    CoachAimEditorView(store: store)
                }
                .presentationDetents([.large])
            }
            .alert("Coach", isPresented: errorBinding) {
                Button("OK", role: .cancel) {}
            } message: {
                Text(store.errorMessage ?? "")
            }
            .task {
                await store.load()
            }
        }
    }

    // MARK: - Card list

    private var cardList: some View {
        List {
            Section {
                if store.visibleReports.isEmpty {
                    // Everything is dismissed: say so and point at
                    // the archive below, instead of stranding the
                    // user on the "no reviews" empty state.
                    Text("You're all caught up — every review is dismissed.")
                } else {
                    ForEach(store.visibleReports) { report in
                        NavigationLink {
                            CoachReportView(report: report)
                        } label: {
                            CoachCardView(report: report)
                        }
                        .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                            Button(role: .destructive) {
                                Task { await store.dismiss(id: report.id) }
                            } label: {
                                Label("Dismiss", systemImage: "trash")
                            }
                        }
                    }
                }
            } header: {
                Text("Weekly Reviews")
            }

            Section {
                DisclosureGroup("What are these reviews?") {
                    VStack(alignment: .leading, spacing: DSSpacing.xs) {
                        Text("Every Monday morning Coach looks at your last full week of training and writes up what improved, what's stalling, and 3 specific things to do next week. The numbers are computed from your logged data; the AI only writes the words. Quiet weeks (fewer than 3 sessions) get no review instead of guesses.")
                        Text("How to read a review:")
                            .font(.subheadline.weight(.semibold))
                            .padding(.top, DSSpacing.xs)
                        Text("Volume is the total work per exercise — sets × reps × weight — compared against your recent average. Up means you did more work, not necessarily heavier work.")
                        Text("Frequency is how many days you trained that week versus your usual rhythm.")
                        Text("Bodyweight is the change in your median weigh-in versus recent weeks, shown in your preferred unit.")
                        Text("Adherence is sessions trained as a percentage of your usual weekly average — 100% means a normal week for you.")
                    }
                    .font(.subheadline)
                    .foregroundStyle(DSColors.textSecondary)
                }
            }

            // Always visible (even at 0) so the archive is
            // discoverable — a row that only appears after your
            // first dismiss reads as missing, not empty.
            Section {
                NavigationLink {
                    CoachArchiveView(store: store)
                } label: {
                    HStack {
                        Text("Archive")
                        Spacer()
                        Text("\(store.dismissedReports.count)")
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
            } footer: {
                Text("Dismissed reviews live here. Swipe one to put it back.")
            }
        }
        // Pull-to-refresh lives on the list itself, not the
        // outer Group: Group-level refresh propagates to pushed
        // detail screens, where a torn-down refresh surfaced a
        // bare "cancelled" alert.
        .refreshable {
            await store.load()
        }
    }

    // MARK: - States

    private var optedOutView: some View {
        List {
            Section {
                Text("Coach writes you a weekly review every Monday — what improved, what's stalling, and 3 specific things to do next week.")
                NavigationLink {
                    CoachAimEditorView(store: store)
                } label: {
                    Text("Turn on Coach")
                        .font(.body.weight(.semibold))
                }
            } header: {
                Text("Weekly review")
            } footer: {
                Text("Turning it on lets the server analyse your training with an AI model. You can switch it off any time.")
            }
        }
    }

    private var noReportView: some View {
        List {
            Section {
                Text("No reviews yet — your first one arrives Monday morning.")
            } header: {
                Text("Weekly review")
            } footer: {
                Text("Coach writes every Monday. Needs at least 3 sessions in the week, otherwise it stays quiet instead of guessing.")
            }
        }
    }

    private var errorBinding: Binding<Bool> {
        Binding(
            get: { store.errorMessage != nil },
            set: { if !$0 { store.errorMessage = nil } }
        )
    }
}
