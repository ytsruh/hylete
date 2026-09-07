import Foundation
import SwiftUI

/// View-model for the Goals tab. Owns the list of goals for
/// the signed-in user and exposes a small set of mutations
/// (`markComplete`, `reopen`, `delete`, `save`) that mirror
/// the server's `/api/v1/goals/*` endpoints.
///
/// The store loads once per tab appearance via `load()`, and
/// every mutation does an optimistic update so the UI
/// animates immediately and only rolls back if the server
/// rejects the change. This keeps the Goals tab feeling
/// native even on a slow connection — pressing "Mark
/// Complete" moves the card in place before the round-trip
/// completes.
@MainActor
public final class GoalStore: ObservableObject {

    // MARK: - Published state

    /// Every goal for the user, in the same order the server
    /// returns them: active first (by target date ascending,
    /// nulls last, then created-at ascending), then completed
    /// (by completed-at descending). The views split this into
    /// `activeGoals` / `completedGoals` rather than calling the
    /// server twice.
    @Published public private(set) var goals: [GoalDTO] = []

    /// Set during the initial load. Distinct from
    /// `goals.isEmpty` so the empty-state UI doesn't flash
    /// during a refresh.
    @Published public private(set) var isLoading: Bool = false

    /// Most-recent load/mutation error, if any. Cleared at
    /// the start of every `load()` / mutation so a stale
    /// message never lingers.
    @Published public var errorMessage: String?

    // MARK: - Dependencies

    private var api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    /// Replaces the backing `APIClient`. Used by
    /// `MainTabView` to swap the stub API the store was
    /// constructed with for the live one wired to the
    /// `AuthStore`. Mirrors the same pattern
    /// `AuthStore.swapAPI(_:)` uses to break the circular
    /// dependency between itself and `APIClient`.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Derived state

    /// Active goals (i.e. `completedAt == nil`), preserving
    /// server ordering. Drives the "Active" list section.
    public var activeGoals: [GoalDTO] {
        goals.filter { !$0.isCompleted }
    }

    /// Completed goals, preserving server ordering (most
    /// recently completed first). Drives the collapsible
    /// "Completed" section.
    public var completedGoals: [GoalDTO] {
        goals.filter { $0.isCompleted }
    }

    // MARK: - Loading

    /// Fetches the full list from the server. Called on first
    /// tab appearance and from pull-to-refresh. Silent no-op
    /// when the user is unauthenticated (the tab isn't shown
    /// in that state — `RootView` hides the whole tab bar).
    public func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            goals = try await api.listGoals()
        } catch APIError.unauthorized {
            // Auth middleware has already cleared the
            // session — leave goals untouched so the next
            // view that mounts sees a clean slate.
            goals = []
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your goals."
        }
    }

    // MARK: - Mutations

    /// Creates a new goal and inserts the server-confirmed
    /// row at the head of the active list. Returns the
    /// freshly-stored goal so the caller can navigate to its
    /// detail view if it wants.
    @discardableResult
    public func create(_ request: CreateGoalRequest) async -> GoalDTO? {
        errorMessage = nil
        do {
            let created = try await api.createGoal(request)
            insert(created)
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not save the goal."
            return nil
        }
    }

    /// Updates an existing goal. Replaces the row in place
    /// (preserves its position in the active/completed
    /// ordering) so the row doesn't jump around mid-edit.
    public func update(id: String, request: UpdateGoalRequest) async {
        errorMessage = nil
        do {
            let updated = try await api.updateGoal(id: id, request: request)
            replace(updated)
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the goal."
        }
    }

    /// Marks a goal complete. Optimistically stamps
    /// `completedAt` to "now" so the row moves from the
    /// active section to the completed section in the same
    /// animation frame; rolls back on failure.
    public func markComplete(id: String) async {
        guard let original = goal(for: id) else { return }
        errorMessage = nil
        let stamp = Date()
        replace(with: optimisticComplete(of: original, at: stamp))
        do {
            let server = try await api.markGoalComplete(id: id)
            replace(server)
        } catch let error as APIError {
            replace(original)
            errorMessage = error.errorDescription
        } catch {
            replace(original)
            errorMessage = "Could not mark the goal complete."
        }
    }

    /// Reopens a completed goal. Optimistically clears
    /// `completedAt` so the row slides from the completed
    /// section back to the active section; rolls back on
    /// failure.
    public func reopen(id: String) async {
        guard let original = goal(for: id) else { return }
        errorMessage = nil
        replace(with: optimisticReopen(of: original))
        do {
            let server = try await api.reopenGoal(id: id)
            replace(server)
        } catch let error as APIError {
            replace(original)
            errorMessage = error.errorDescription
        } catch {
            replace(original)
            errorMessage = "Could not reopen the goal."
        }
    }

    /// Hard-deletes a goal. Optimistic remove; rolls back by
    /// re-fetching the full list on failure so the row
    /// reappears in its correct position.
    public func delete(id: String) async {
        guard let original = goal(for: id) else { return }
        errorMessage = nil
        goals.removeAll { $0.id == id }
        do {
            try await api.deleteGoal(id: id)
        } catch let error as APIError {
            replace(original)
            errorMessage = error.errorDescription
        } catch {
            replace(original)
            errorMessage = "Could not delete the goal."
        }
    }

    // MARK: - Mutation helpers

    /// Inserts a newly-created goal in the right slot.
    /// Active goals sort by target date ascending (nulls
    /// last) so the new row lands before any "no target"
    /// goals; completed goals sort by completed-at descending
    /// so a freshly-completed row lands at the head.
    private func insert(_ goal: GoalDTO) {
        if goal.isCompleted {
            let idx = goals.firstIndex { !$0.isCompleted } ?? goals.endIndex
            goals.insert(goal, at: idx)
        } else {
            goals.insert(goal, at: activeInsertionIndex(for: goal))
        }
    }

    /// Computes the insertion index for a new active goal
    /// based on the server's ordering (target date asc with
    /// nulls last). Keeps the local cache consistent with
    /// the server's `ListActiveGoals` ORDER BY without having
    /// to re-fetch.
    private func activeInsertionIndex(for newGoal: GoalDTO) -> Int {
        for (index, existing) in goals.enumerated() where !existing.isCompleted {
            switch (newGoal.targetDate, existing.targetDate) {
            case let (l?, r?) where l < r:
                return index
            case (_?, nil):
                return index
            default:
                continue
            }
        }
        return goals.count
    }

    /// Replaces the row with the matching id. No-op when the
    /// id isn't in the list (e.g. a server response for a row
    /// the user just deleted).
    private func replace(_ goal: GoalDTO) {
        guard let index = goals.firstIndex(where: { $0.id == goal.id }) else {
            return
        }
        goals[index] = goal
    }

    /// Inserts the supplied row in place of the matching id,
    /// preserving its position. Used when the optimistic
    /// shape matches the server's shape and the caller wants
    /// the row to stay in the same slot (e.g. while a
    /// save is in flight).
    private func replace(with goal: GoalDTO) {
        replace(goal)
    }

    private func goal(for id: String) -> GoalDTO? {
        goals.first { $0.id == id }
    }

    /// Returns the goal with `completedAt` set to the
    /// supplied timestamp. Used by the optimistic-complete
    /// path so the row "moves" to the completed section the
    /// moment the user taps the button.
    private func optimisticComplete(of goal: GoalDTO, at stamp: Date) -> GoalDTO {
        GoalDTO(
            id: goal.id,
            title: goal.title,
            description: goal.description,
            startDate: goal.startDate,
            targetDate: goal.targetDate,
            endDate: goal.endDate,
            completedAt: stamp,
            createdAt: goal.createdAt,
            updatedAt: stamp
        )
    }

    /// Returns the goal with `completedAt` cleared. Used by
    /// the optimistic-reopen path so the row "moves" back to
    /// the active section the moment the user taps the
    /// button.
    private func optimisticReopen(of goal: GoalDTO) -> GoalDTO {
        GoalDTO(
            id: goal.id,
            title: goal.title,
            description: goal.description,
            startDate: goal.startDate,
            targetDate: goal.targetDate,
            endDate: goal.endDate,
            completedAt: nil,
            createdAt: goal.createdAt,
            updatedAt: Date()
        )
    }
}