import Foundation
import SwiftUI

/// View-model for the Workouts list (Beta). Owns the user's
/// planned workouts and exposes CRUD plus scheduling mutations
/// mirroring the server's `/api/v1/workouts/*` endpoints.
///
/// The list holds summaries (no blocks); full detail loads on
/// demand into `details` so the detail view can render without
/// a second spinner after coming back from the editor. Every
/// mutation refreshes the affected rows from the server
/// response so the list ordering (newest first) stays truthful
/// without a full reload.
///
/// Scheduling (planned calendar days) lives on the detail:
/// `addAssignments` / `deleteAssignment` mutate single days,
/// and `syncSchedule` reconciles the editor's desired date set
/// against the known assignments with minimal add/delete
/// calls. Repeats are expanded into individual days by the
/// editor before they reach this store — the server stores one
/// row per day and never sees a recurrence rule.
@MainActor
public final class WorkoutStore: ObservableObject {

    // MARK: - Published state

    /// Every workout summary for the user, newest first (the
    /// server's order). Drives the list rows.
    @Published public private(set) var workouts: [WorkoutSummaryDTO] = []

    /// Fully-loaded workouts by id. Populated by `detail(id:)`
    /// and by create/update/duplicate responses.
    @Published public private(set) var details: [String: WorkoutDTO] = [:]

    /// Set during the initial load. Distinct from
    /// `workouts.isEmpty` so the empty-state UI doesn't flash
    /// during a refresh.
    @Published public private(set) var isLoading: Bool = false

    /// Most-recent load/mutation error, if any. Cleared at
    /// the start of every operation so a stale message never
    /// lingers.
    @Published public var errorMessage: String?

    // MARK: - Dependencies

    private var api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    /// Replaces the backing `APIClient`. Used by
    /// `MainTabView` to swap the stub API the store was
    /// constructed with for the live one wired to the
    /// `AuthStore`. Mirrors `BlockStore.replaceAPI(_:)`.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Loading

    /// Fetches the workout summaries from the server. Called
    /// on first list appearance and from pull-to-refresh.
    /// Silent no-op when unauthenticated.
    public func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            workouts = try await api.listWorkouts()
        } catch APIError.unauthorized {
            workouts = []
            details = [:]
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your workouts."
        }
    }

    /// Returns the cached detail for a workout, fetching it
    /// when absent (or when `refresh` is true, e.g. after
    /// returning from the editor). Returns nil on failure
    /// and surfaces the error in `errorMessage`.
    @discardableResult
    public func detail(id: String, refresh: Bool = false) async -> WorkoutDTO? {
        if !refresh, let cached = details[id] {
            return cached
        }
        errorMessage = nil
        do {
            let workout = try await api.getWorkout(id: id)
            details[id] = workout
            return workout
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not load the workout."
            return nil
        }
    }

    // MARK: - Mutations

    /// Creates a workout (unscheduled). Prepends the
    /// server-confirmed summary (newest first matches the
    /// server order) and caches the full detail. Returns the
    /// created workout so the caller can navigate to it.
    @discardableResult
    public func create(_ request: CreateWorkoutRequest) async -> WorkoutDTO? {
        errorMessage = nil
        do {
            let created = try await api.createWorkout(request)
            details[created.id] = created
            workouts.insert(summary(of: created), at: 0)
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not save the workout."
            return nil
        }
    }

    /// Updates a workout (block links fully replaced;
    /// assignments untouched). Refreshes the cached detail
    /// and the summary row in place.
    public func update(id: String, request: UpdateWorkoutRequest) async {
        errorMessage = nil
        do {
            let updated = try await api.updateWorkout(id: id, request: request)
            details[id] = updated
            if let index = workouts.firstIndex(where: { $0.id == id }) {
                workouts[index] = summary(of: updated)
            }
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the workout."
        }
    }

    /// Duplicates a workout server-side ("<title> copy" with
    /// the same blocks). The schedule is copied only when
    /// `copySchedule` is true. Prepends the copy like
    /// `create` and returns it so the caller can navigate.
    @discardableResult
    public func duplicate(id: String, copySchedule: Bool = false) async -> WorkoutDTO? {
        errorMessage = nil
        do {
            let dup = try await api.duplicateWorkout(id: id, copySchedule: copySchedule)
            details[dup.id] = dup
            workouts.insert(summary(of: dup), at: 0)
            return dup
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not duplicate the workout."
            return nil
        }
    }

    /// Hard-deletes a workout with its block links and
    /// assignments. Optimistic remove; rolls back by
    /// re-inserting the summary on failure.
    public func delete(id: String) async {
        guard let index = workouts.firstIndex(where: { $0.id == id }) else { return }
        errorMessage = nil
        let removed = workouts.remove(at: index)
        details.removeValue(forKey: id)
        do {
            try await api.deleteWorkout(id: id)
        } catch let error as APIError {
            workouts.insert(removed, at: min(index, workouts.count))
            errorMessage = error.errorDescription
        } catch {
            workouts.insert(removed, at: min(index, workouts.count))
            errorMessage = "Could not delete the workout."
        }
    }

    // MARK: - Scheduling

    /// Plans the workout on explicit "YYYY-MM-DD" days.
    /// Refreshes the cached detail's assignments from the
    /// server response so the detail view updates in place.
    /// Returns the assignments actually created (duplicate
    /// days are skipped server-side).
    @discardableResult
    public func addAssignments(workoutID: String, dates: [String]) async -> [WorkoutAssignmentDTO] {
        errorMessage = nil
        do {
            let created = try await api.addWorkoutAssignments(workoutID: workoutID, dates: dates)
            if var cached = details[workoutID] {
                var merged = cached.assignments
                let known = Set(merged.map(\.scheduledDate))
                merged.append(contentsOf: created.filter { !known.contains($0.scheduledDate) })
                merged.sort { $0.scheduledDate < $1.scheduledDate }
                cached = WorkoutDTO(
                    id: cached.id, title: cached.title, description: cached.description,
                    blocks: cached.blocks, assignments: merged,
                    createdAt: cached.createdAt, updatedAt: cached.updatedAt
                )
                details[workoutID] = cached
            }
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return []
        } catch {
            errorMessage = "Could not schedule the workout."
            return []
        }
    }

    /// Removes a single planned day. Updates the cached
    /// detail optimistically, rolling back on failure.
    public func deleteAssignment(workoutID: String, assignmentID: String) async {
        errorMessage = nil
        let previous = details[workoutID]
        if var cached = previous {
            cached = WorkoutDTO(
                id: cached.id, title: cached.title, description: cached.description,
                blocks: cached.blocks,
                assignments: cached.assignments.filter { $0.id != assignmentID },
                createdAt: cached.createdAt, updatedAt: cached.updatedAt
            )
            details[workoutID] = cached
        }
        do {
            try await api.deleteWorkoutAssignment(id: assignmentID)
        } catch let error as APIError {
            if let previous { details[workoutID] = previous }
            errorMessage = error.errorDescription
        } catch {
            if let previous { details[workoutID] = previous }
            errorMessage = "Could not remove the planned day."
        }
    }

    /// Reconciles the workout's schedule to exactly
    /// `desiredDates` (the editor's date set): adds missing
    /// days, deletes removed ones. No-ops when nothing
    /// changed so an edit that only retitles the workout
    /// issues no schedule calls.
    public func syncSchedule(workoutID: String, desiredDates: [String]) async {
        let current = Set(details[workoutID]?.assignments.map(\.scheduledDate) ?? [])
        let desired = Set(desiredDates)
        let toAdd = desired.subtracting(current).sorted()
        if !toAdd.isEmpty {
            await addAssignments(workoutID: workoutID, dates: toAdd)
            guard errorMessage == nil else { return }
        }
        let currentByDate = Dictionary(
            (details[workoutID]?.assignments ?? []).map { ($0.scheduledDate, $0.id) },
            uniquingKeysWith: { first, _ in first }
        )
        for date in current.subtracting(desired).sorted() {
            guard let assignmentID = currentByDate[date] else { continue }
            await deleteAssignment(workoutID: workoutID, assignmentID: assignmentID)
            guard errorMessage == nil else { return }
        }
    }

    // MARK: - Helpers

    /// Derives a list summary from a full workout response so
    /// create/update/duplicate can refresh the list without
    /// refetching.
    private func summary(of workout: WorkoutDTO) -> WorkoutSummaryDTO {
        WorkoutSummaryDTO(
            id: workout.id,
            title: workout.title,
            description: workout.description,
            blockCount: workout.blocks.count,
            createdAt: workout.createdAt,
            updatedAt: workout.updatedAt
        )
    }
}
