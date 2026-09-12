import Foundation
import SwiftUI

/// View-model for the Workouts list. Owns the dated workouts for
/// the signed-in user and exposes the mutations that mirror the
/// server's `/api/v1/workouts/*` endpoints (`create`,
/// `duplicate`, `bulkCreate`, `update`, `complete`, `reopen`,
/// `cancel`, `delete`).
///
/// The store keeps headers for the list plus a detail cache for
/// opened workouts (tree + logged entries). Status changes are
/// optimistic so rows move sections in the same animation frame
/// and roll back only if the server rejects the change —
/// mirroring `GoalStore.markComplete(id:)`.
///
/// Range reads for the dashboard calendar go through
/// `loadRange(from:to:)` and are caller-owned (returned, not
/// cached) so the calendar's lazy paging never disturbs the
/// list's full ordering.
@MainActor
public final class WorkoutStore: ObservableObject {

    // MARK: - Published state

    /// Every workout header for the user: planned first
    /// (scheduled ascending, unscheduled last), then by
    /// recency (server ordering). Views split this into
    /// `plannedWorkouts` / `pastWorkouts`.
    @Published public private(set) var workouts: [WorkoutDTO] = []

    /// Detail cache by workout id (tree + logged entries),
    /// populated by `loadDetail(id:)`. Replaced wholesale on
    /// every fetch so progress lines always reflect the
    /// server state.
    @Published public private(set) var details: [String: WorkoutDetailDTO] = [:]

    /// Set during the initial load. Distinct from
    /// `workouts.isEmpty` so the empty-state UI doesn't flash
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
    /// `AuthStore` — same pattern `GoalStore.replaceAPI(_:)`
    /// uses.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Derived state

    /// Planned workouts, preserving server ordering. Drives
    /// the Today/Upcoming sections.
    public var plannedWorkouts: [WorkoutDTO] {
        workouts.filter { $0.isPlanned }
    }

    /// Completed + cancelled workouts, preserving server
    /// ordering. Drives the collapsible history section.
    public var pastWorkouts: [WorkoutDTO] {
        workouts.filter { !$0.isPlanned }
    }

    /// Returns the header with the matching id, or `nil`
    /// when no such workout is cached.
    public func workout(for id: String) -> WorkoutDTO? {
        workouts.first { $0.id == id }
    }

    /// Returns the cached detail for the workout, or `nil`
    /// when it hasn't been fetched yet.
    public func detail(for id: String) -> WorkoutDetailDTO? {
        details[id]
    }

    // MARK: - Loading

    /// Fetches every workout header from the server. Called on
    /// first appearance and from pull-to-refresh.
    public func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            workouts = try await api.listWorkouts()
        } catch APIError.unauthorized {
            workouts = []
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your workouts."
        }
    }

    /// Fetches workouts whose scheduled window overlaps the
    /// range, for the dashboard calendar. Caller-owned: the
    /// result is returned, not merged into `workouts`, so
    /// lazy paging never disturbs the list ordering. Returns
    /// an empty array on failure (`errorMessage` carries the
    /// reason).
    public func loadRange(from: Date, to: Date) async -> [WorkoutDTO] {
        errorMessage = nil
        do {
            return try await api.listWorkouts(from: from, to: to)
        } catch APIError.unauthorized {
            return []
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return []
        } catch {
            errorMessage = "Could not load your workouts."
            return []
        }
    }

    /// Fetches one workout's full detail (tree + logged
    /// entries) into the cache. Pass `force: true` to refetch
    /// a cached row — e.g. after logging sets into it via
    /// `noteLoggedEntries(id:)`. Returns the detail, or nil
    /// on failure.
    @discardableResult
    public func loadDetail(id: String, force: Bool = false) async -> WorkoutDetailDTO? {
        if !force, let cached = details[id] {
            return cached
        }
        errorMessage = nil
        do {
            let detail = try await api.getWorkout(id: id)
            details[id] = detail
            return detail
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not load the workout."
            return nil
        }
    }

    /// Refreshes a workout's detail after sets were logged
    /// into it (e.g. on `NewSetView` dismiss) so progress
    /// lines update without a full list refetch.
    public func noteLoggedEntries(workoutID: String) async {
        await loadDetail(id: workoutID, force: true)
    }

    // MARK: - Mutations

    /// Creates a dated workout with inline blocks and inserts
    /// the server-confirmed header in server
    /// position (planned rows sort by scheduled start; the
    /// simplest correct merge is a full reload — creation is
    /// rare and the list is small). Caches the returned
    /// detail. Returns the detail so the caller can open it
    /// without a follow-up GET.
    @discardableResult
    public func create(_ request: CreateWorkoutRequest) async -> WorkoutDetailDTO? {
        errorMessage = nil
        do {
            let detail = try await api.createWorkout(request)
            details[detail.id] = detail
            await load()
            return detail
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not save the workout."
            return nil
        }
    }

    /// Copies a workout (including its tree) — a blank name
    /// becomes "Copy of <source>" server-side. Reloads the list
    /// on success so the copy lands in server order, and caches
    /// the returned detail. Returns the detail so the caller
    /// can open it without a follow-up GET.
    @discardableResult
    public func duplicate(id: String, request: DuplicateWorkoutRequest = DuplicateWorkoutRequest()) async -> WorkoutDetailDTO? {
        errorMessage = nil
        do {
            let detail = try await api.duplicateWorkout(id: id, request: request)
            details[detail.id] = detail
            await load()
            return detail
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not duplicate the workout."
            return nil
        }
    }

    /// Snapshots one source workout into one dated copy per
    /// instance, atomically server-side. Reloads the list on
    /// success so the new copies land in server order.
    /// Returns the created headers in request order.
    @discardableResult
    public func bulkCreate(_ request: BulkCreateWorkoutsRequest) async -> [WorkoutDTO]? {
        errorMessage = nil
        do {
            let created = try await api.bulkCreateWorkouts(request)
            await load()
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not plan the workouts."
            return nil
        }
    }

    /// Updates a workout's header. Replaces the row in place
    /// (preserves its position) so the row doesn't jump
    /// around mid-edit, and refreshes the cached detail's
    /// header fields.
    public func update(id: String, request: UpdateWorkoutRequest) async {
        errorMessage = nil
        do {
            let detail = try await api.updateWorkout(id: id, request: request)
            replaceHeader(WorkoutDTO(
                id: detail.id, name: detail.name, notes: detail.notes,
                status: detail.status,
                scheduledStart: detail.scheduledStart, scheduledEnd: detail.scheduledEnd,
                completedAt: detail.completedAt,
                createdAt: detail.createdAt, updatedAt: detail.updatedAt
            ))
            details[id] = detail
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the workout."
        }
    }

    /// Marks a workout complete. Optimistic: the row moves to
    /// the history section in the same animation frame and
    /// rolls back on failure.
    public func complete(id: String) async {
        await setStatus(id: id, optimistic: "completed", apply: api.completeWorkout)
    }

    /// Moves a workout back to planned. Optimistic; rolls back
    /// on failure.
    public func reopen(id: String) async {
        await setStatus(id: id, optimistic: "planned", apply: api.reopenWorkout)
    }

    /// Marks a workout cancelled. Optimistic; rolls back on
    /// failure.
    public func cancel(id: String) async {
        await setStatus(id: id, optimistic: "cancelled", apply: api.cancelWorkout)
    }

    /// Hard-deletes a workout. The tree cascades server-side;
    /// logged exercise entries survive with their links
    /// cleared. Drops the cached detail too. On failure the
    /// list is re-fetched so a falsely-removed row reappears.
    public func delete(id: String) async {
        errorMessage = nil
        workouts.removeAll { $0.id == id }
        details.removeValue(forKey: id)
        do {
            try await api.deleteWorkout(id: id)
        } catch let error as APIError {
            errorMessage = error.errorDescription
            await load()
        } catch {
            errorMessage = "Could not delete the workout."
            await load()
        }
    }

    // MARK: - Mutation helpers

    /// Runs an optimistic status transition: stamps the local
    /// row (and cached detail) with the optimistic status,
    /// confirms with the server, then reconciles with the
    /// server-confirmed detail — rolling back to the original
    /// row on failure.
    private func setStatus(
        id: String,
        optimistic: String,
        apply: (String) async throws -> WorkoutDetailDTO
    ) async {
        guard let original = workout(for: id) else { return }
        errorMessage = nil
        replaceHeader(withStatus(of: original, status: optimistic))
        if var detail = details[id] {
            detail = WorkoutDetailDTO(
                id: detail.id, name: detail.name, notes: detail.notes,
                status: optimistic,
                scheduledStart: detail.scheduledStart, scheduledEnd: detail.scheduledEnd,
                completedAt: optimistic == "completed" ? Date() : nil,
                blocks: detail.blocks, loggedEntries: detail.loggedEntries,
                adHocEntries: detail.adHocEntries,
                createdAt: detail.createdAt, updatedAt: detail.updatedAt
            )
            details[id] = detail
        }
        do {
            let server = try await apply(id)
            replaceHeader(WorkoutDTO(
                id: server.id, name: server.name, notes: server.notes,
                status: server.status,
                scheduledStart: server.scheduledStart, scheduledEnd: server.scheduledEnd,
                completedAt: server.completedAt,
                createdAt: server.createdAt, updatedAt: server.updatedAt
            ))
            details[id] = server
        } catch let error as APIError {
            replaceHeader(original)
            errorMessage = error.errorDescription
        } catch {
            replaceHeader(original)
            errorMessage = "Could not update the workout."
        }
    }

    /// Replaces the header row with the matching id, preserving
    /// its position. No-op when the id isn't cached.
    private func replaceHeader(_ workout: WorkoutDTO) {
        guard let index = workouts.firstIndex(where: { $0.id == workout.id }) else {
            return
        }
        workouts[index] = workout
    }

    /// Returns the header with its status swapped. Used by the
    /// optimistic-transition path so the row moves sections
    /// the moment the user taps the button.
    private func withStatus(of workout: WorkoutDTO, status: String) -> WorkoutDTO {
        WorkoutDTO(
            id: workout.id, name: workout.name, notes: workout.notes,
            status: status,
            scheduledStart: workout.scheduledStart, scheduledEnd: workout.scheduledEnd,
            completedAt: status == "completed" ? Date() : nil,
            createdAt: workout.createdAt, updatedAt: workout.updatedAt
        )
    }
}
