import SwiftUI
import UIKit

/// The "Goals" tab. Two sections in a plain `List` with the
/// default iOS chrome (alternating row backgrounds — matches
/// the Profile tab's "Name" / "Weight unit" rows):
///   - **Active goals** — always expanded
///   - **Completed goals** — wrapped in a `DisclosureGroup`,
///     collapsed by default so finished goals don't dominate
///     the screen
///
/// Mirrors the web's `/goals` page
/// (`internal/views/goals/list.templ`) where completed lives
/// inside a `<details>` accordion.
///
/// All networking and state lives in `GoalStore`; this view
/// is purely presentational. The store is injected by the
/// parent (`MainTabView` constructs it once from
/// `AppEnvironment.api` so it can outlive view rebuilds).
struct GoalsListView: View {
    @EnvironmentObject private var env: AppEnvironment
    @ObservedObject var store: GoalStore

    @State private var showingNewGoal: Bool = false
    @State private var editingGoal: GoalDTO?
    @State private var completedExpanded: Bool = false
    @State private var completionCelebrationTrigger: Int = 0

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Goals")
                .toolbar { toolbarContent }
                .sheet(isPresented: $showingNewGoal) {
                    GoalEditorView(mode: .create, store: store)
                        .environmentObject(env)
                }
                .sheet(item: $editingGoal) { goal in
                    GoalEditorView(mode: .edit(goal), store: store)
                        .environmentObject(env)
                }
        }
        .overlay {
            GoalCompletionCelebration(trigger: completionCelebrationTrigger)
                .allowsHitTesting(false)
                .ignoresSafeArea()
        }
        .task { await store.load() }
        .refreshable { await store.load() }
    }

    // MARK: - Toolbar

    @ToolbarContentBuilder
    private var toolbarContent: some ToolbarContent {
        ToolbarItem(placement: .topBarTrailing) {
            Button {
                showingNewGoal = true
            } label: {
                Image(systemName: Icons.addSet)
            }
            .accessibilityLabel("Add goal")
        }
    }

    // MARK: - Content states

    @ViewBuilder
    private var content: some View {
        if store.isLoading && store.goals.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = store.errorMessage, store.goals.isEmpty {
            errorState(error)
        } else if store.goals.isEmpty {
            emptyState
        } else {
            loadedList
        }
    }

    /// Active section + collapsible completed section. Uses
    /// the default `List` style so iOS provides the row
    /// chrome — no custom backgrounds / separators / insets
    /// to fight the system.
    private var loadedList: some View {
        List {
            activeSection
            completedSection
        }
        .listStyle(.automatic)
    }

    @ViewBuilder
    private var activeSection: some View {
        if !store.activeGoals.isEmpty {
            Section {
                ForEach(store.activeGoals) { goal in
                    row(for: goal)
                }
            } header: {
                sectionHeader("Active", count: store.activeGoals.count)
            }
        }
    }

    /// Completed section uses `DisclosureGroup` so it starts
    /// collapsed by default (matches the web's `<details>`
    /// accordion in `list.templ:99-112`). The disclosure
    /// header carries the same "Title · count" pattern as the
    /// active header so they read consistently.
    @ViewBuilder
    private var completedSection: some View {
        if !store.completedGoals.isEmpty {
            DisclosureGroup(isExpanded: $completedExpanded) {
                ForEach(store.completedGoals) { goal in
                    row(for: goal)
                }
            } label: {
                sectionHeader("Completed", count: store.completedGoals.count)
            }
        }
    }

    /// Single row presentation shared by both sections. The
    /// row is wrapped in a `Button` so tapping it opens the
    /// editor, and gets `.swipeActions` for the gesture
    /// shortcuts — leading swipe toggles complete/reopen
    /// (full-swipe enabled, the iOS "fast triage" gesture
    /// from Mail / Reminders), trailing swipe opens the
    /// editor (no full swipe — destructive-ish edit, the
    /// user should tap-and-confirm).
    @ViewBuilder
    private func row(for goal: GoalDTO) -> some View {
        Button {
            editingGoal = goal
        } label: {
            GoalRow(goal: goal)
        }
        .buttonStyle(.plain)
        .swipeActions(edge: .leading, allowsFullSwipe: true) {
            if goal.isCompleted {
                Button {
                    reopenGoal(goal)
                } label: {
                    Label("Reopen", systemImage: "arrow.uturn.backward")
                }
                .tint(DSColors.accent)
            } else {
                Button {
                    completeGoal(goal)
                } label: {
                    Label("Complete", systemImage: "checkmark")
                }
                .tint(DSColors.accent)
            }
        }
        .swipeActions(edge: .trailing, allowsFullSwipe: false) {
            Button {
                editingGoal = goal
            } label: {
                Label("Edit", systemImage: "pencil")
            }
            // Neutral grey so Edit doesn't compete visually
            // with the primary (Complete/Reopen) action on
            // the leading edge. Matches the standard iOS
            // pattern (Mail's "Move", Notes' "Move to...").
            .tint(.gray)
        }
    }

    /// "Title · count" header used by both sections. The
    /// middle dot + small count text matches the web's
    /// `GoalsSectionHeader` (`list.templ:136-143`).
    private func sectionHeader(_ title: String, count: Int) -> some View {
        HStack(spacing: DSSpacing.xs) {
            Text(title)
                .font(.headline)
                .foregroundStyle(DSColors.text)
            Text("·")
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
            Text("\(count)")
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
        }
        .textCase(nil)
    }

    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            ZStack {
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surfaceElevated)
                Image(systemName: Icons.goals)
                    .font(.system(size: 24))
                    .foregroundStyle(DSColors.text)
            }
            .frame(width: 48, height: 48)

            Text("No goals yet")
                .font(.title3.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text("Track what you're working towards. Goals can have optional start, target, and end dates.")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Text("Tap + to add your first goal.")
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func errorState(_ message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Try again") {
                Task { await store.load() }
            }
            .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    // MARK: - Mutation handlers

    /// Marks a goal complete. The optimistic update happens
    /// inside `GoalStore.markComplete(id:)`; we additionally
    /// fire the success haptic here (alongside the row's own
    /// strikethrough animation) so the user gets tactile
    /// confirmation that the goal is done.
    private func completeGoal(_ goal: GoalDTO) {
        guard !goal.isCompleted else { return }
        completionCelebrationTrigger &+= 1
        let generator = UINotificationFeedbackGenerator()
        generator.prepare()
        generator.notificationOccurred(.success)
        Task { await store.markComplete(id: goal.id) }
    }

    /// Reopens a completed goal. Uses a lighter haptic (a
    /// "selection" tap) rather than the success chime so
    /// reopening feels less celebratory — matches the web's
    /// "Goal reopened" toast that has no confetti.
    private func reopenGoal(_ goal: GoalDTO) {
        guard goal.isCompleted else { return }
        let generator = UISelectionFeedbackGenerator()
        generator.prepare()
        generator.selectionChanged()
        Task { await store.reopen(id: goal.id) }
    }
}

#Preview {
    GoalsListView(store: GoalStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
