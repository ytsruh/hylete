import Foundation
import SwiftUI

/// View-model for the Workouts list (Beta). Owns the user's scheduled
/// workouts and exposes CRUD mutations mirroring the server's
/// `/api/v1/workouts/*` endpoints.
///
/// The list holds summaries (no blocks); full detail loads on demand
/// into `details`. A date-bucketed `workoutsByDay` cache backs the
/// dashboard calendar's week paging (`ensureWeekLoaded`), keyed by
/// local start-of-day derived from each summary's `scheduledDate`
/// (YYYY-MM-DD). Every mutation refreshes the affected rows from the
/// server response so list ordering stays truthful without a full
/// reload.
@MainActor
public final class WorkoutStore: ObservableObject {

    // MARK: - Published state

    /// Every workout summary for the user, newest scheduled date
    /// first (the server's order). Drives the list rows.
    @Published public private(set) var summaries: [WorkoutSummaryDTO] = []

    /// Fully-loaded workouts by id. Populated by `detail(id:)`
    /// and by create/update responses.
    @Published public private(set) var details: [String: WorkoutDTO] = [:]

    /// Session cache of fetched workout summaries bucketed by local
    /// start-of-day. Weeks are merged in as the calendar pages
    /// around; buckets are overwritten wholesale when their week is
    /// refetched.
    @Published public private(set) var workoutsByDay: [Date: [WorkoutSummaryDTO]] = [:]

    /// Set during the initial load. Distinct from
    /// `summaries.isEmpty` so the empty-state UI doesn't flash
    /// during a refresh.
    @Published public private(set) var isLoading: Bool = false

    /// Most-recent load/mutation error, if any. Cleared at the
    /// start of every operation so a stale message never
    /// lingers.
    /// The 409 from deleting a referenced block surfaces here
    /// verbatim — the server message already tells the user which
    /// workouts to unlink first.
    @Published public var errorMessage: String?

    // MARK: - Dependencies

    private var api: APIClient

    /// Weeks already present in `workoutsByDay`; guards against
    /// refetching a page the calendar has already visited.
    private var loadedWeeks: Set<Date> = []

    /// Week starts currently being fetched.
    private var loadingWeeks: Set<Date> = []

    /// `YYYY-MM-DD` formatter for `scheduledDate` bucketing and
    /// range queries. Device-local calendar so bucket days match
    /// what the calendar strip shows.
    private static let dayFormatter: DateFormatter = {
        let f = DateFormatter()
        f.dateFormat = "yyyy-MM-dd"
        f.locale = Locale(identifier: "en_US_POSIX")
        return f
    }()

    public init(api: APIClient) {
        self.api = api
    }

    /// Replaces the backing `APIClient`. Used by `MainTabView` to
    /// swap the stub API the store was constructed with for the live
    /// one wired to the `AuthStore`. Mirrors
    /// `BlockStore.replaceAPI(_:)`.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Loading

    /// Fetches the workout summaries from the server. Called on
    /// first list appearance and from pull-to-refresh. Silent no-op
    /// when unauthenticated.
    public func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            summaries = try await api.listWorkouts()
        } catch APIError.unauthorized {
            summaries = []
            details = [:]
            workoutsByDay = [:]
            loadedWeeks.removeAll()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your workouts."
        }
    }

    /// Returns the cached detail for a workout, fetching it when
    /// absent (or when `refresh` is true, e.g. after returning from
    /// the editor). Returns nil on failure and surfaces the error
    /// in `errorMessage`.
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

    /// Fetches the calendar week containing `date` unless it's
    /// already cached. Results are bucketed by local start-of-day
    /// from each summary's `scheduledDate`. Failures leave the week
    /// unmarked so a later visit retries.
    public func ensureWeekLoaded(for date: Date) async {
        let weekStart = CalendarMath.startOfWeek(for: date)
        guard !loadedWeeks.contains(weekStart), !loadingWeeks.contains(weekStart) else { return }
        loadingWeeks.insert(weekStart)
        defer { loadingWeeks.remove(weekStart) }

        let days = CalendarMath.days(inWeekOf: weekStart)
        guard let first = days.first, let last = days.last else { return }
        let from = Self.dayFormatter.string(from: first)
        let to = Self.dayFormatter.string(from: last)
        do {
            let fetched = try await api.listWorkouts(from: from, to: to)
            var byDay = workoutsByDay
            for day in days {
                byDay[day] = []
            }
            for summary in fetched {
                if let day = Self.dayFormatter.date(from: summary.scheduledDate) {
                    byDay[CalendarMath.startOfDay(day), default: []].append(summary)
                }
            }
            workoutsByDay = byDay
            loadedWeeks.insert(weekStart)
        } catch APIError.unauthorized {
            return
        } catch {
            // Transient failure — leave uncached to retry later.
        }
    }

    /// Drops every cached calendar week. Called after mutations that
    /// can move a workout across days (create/update/duplicate with
    /// a new date, delete) so the next calendar visit refetches.
    /// Also called by the dashboard's pull-to-refresh so a manual
    /// refresh always refetches the visible week.
    public func invalidateCalendarCache() {
        loadedWeeks.removeAll()
    }

    /// Reports whether the week containing `date` is currently being
    /// fetched. Drives the calendar day section's inline spinner.
    public func isLoadingWeek(for date: Date) -> Bool {
        loadingWeeks.contains(CalendarMath.startOfWeek(for: date))
    }

    // MARK: - Mutations

    /// Creates a workout. Prepends the server-confirmed summary and
    /// caches the full detail. Returns the created workout so the
    /// caller can navigate to it.
    @discardableResult
    public func create(_ request: CreateWorkoutRequest) async -> WorkoutDTO? {
        errorMessage = nil
        do {
            let created = try await api.createWorkout(request)
            details[created.id] = created
            summaries.insert(summary(of: created), at: 0)
            invalidateCalendarCache()
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not save the workout."
            return nil
        }
    }

    /// Updates a workout (blocks fully replaced, statuses reset to
    /// pending). Refreshes the cached detail, the summary row, and
    /// the calendar cache.
    public func update(id: String, request: UpdateWorkoutRequest) async {
        errorMessage = nil
        do {
            let updated = try await api.updateWorkout(id: id, request: request)
            details[id] = updated
            if let index = summaries.firstIndex(where: { $0.id == id }) {
                summaries[index] = summary(of: updated)
            }
            invalidateCalendarCache()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the workout."
        }
    }

    /// Changes only a workout's status (plan and block check-offs
    /// untouched). Backs the player Finish button and the detail
    /// status picker — both must preserve block progress, which
    /// the full-replacement `update` resets. Refreshes the cached
    /// detail, the summary row, and the calendar cache.
    public func setStatus(id: String, status: WorkoutStatusDTO) async {
        errorMessage = nil
        do {
            let updated = try await api.setWorkoutStatus(id: id, status: status)
            details[id] = updated
            if let index = summaries.firstIndex(where: { $0.id == id }) {
                summaries[index] = summary(of: updated)
            }
            invalidateCalendarCache()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the workout."
        }
    }

    /// Changes only a workout's Apple Health activity type (plan
    /// and block check-offs untouched). Backs the editor type row
    /// and the detail type picker — both must preserve block
    /// progress, which the full-replacement `update` resets.
    /// Refreshes the cached detail, the summary row, and the
    /// calendar cache.
    public func setHealthActivityType(id: String, healthActivityType: String) async {
        errorMessage = nil
        do {
            let updated = try await api.setWorkoutHealthActivityType(id: id, healthActivityType: healthActivityType)
            details[id] = updated
            if let index = summaries.firstIndex(where: { $0.id == id }) {
                summaries[index] = summary(of: updated)
            }
            invalidateCalendarCache()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the workout."
        }
    }

    /// Hard-deletes a workout. Optimistic remove; rolls back by
    /// re-inserting the summary on failure.
    public func delete(id: String) async {
        guard let index = summaries.firstIndex(where: { $0.id == id }) else { return }
        errorMessage = nil
        let removed = summaries.remove(at: index)
        details.removeValue(forKey: id)
        do {
            try await api.deleteWorkout(id: id)
            invalidateCalendarCache()
        } catch let error as APIError {
            summaries.insert(removed, at: min(index, summaries.count))
            errorMessage = error.errorDescription
        } catch {
            summaries.insert(removed, at: min(index, summaries.count))
            errorMessage = "Could not delete the workout."
        }
    }

    /// Marks one block in a workout pending/done/skipped. Refreshes
    /// the cached detail and the summary's done count in place.
    public func setBlockStatus(workoutID: String, workoutBlockID: String, status: WorkoutBlockStatusDTO) async {
        errorMessage = nil
        do {
            let updated = try await api.setWorkoutBlockStatus(workoutID: workoutID, workoutBlockID: workoutBlockID, status: status)
            details[workoutID] = updated
            if let index = summaries.firstIndex(where: { $0.id == workoutID }) {
                summaries[index] = summary(of: updated)
            }
            invalidateCalendarCache()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the block."
        }
    }

    /// Copies a workout onto a new scheduled date (exact name,
    /// block statuses reset to pending server-side).
    /// Prepends the new summary and caches the detail. Returns the
    /// duplicate so the caller can navigate to it.
    @discardableResult
    public func duplicate(id: String, scheduledDate: String) async -> WorkoutDTO? {
        errorMessage = nil
        do {
            let created = try await api.duplicateWorkout(id: id, scheduledDate: scheduledDate)
            details[created.id] = created
            summaries.insert(summary(of: created), at: 0)
            invalidateCalendarCache()
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not duplicate the workout."
            return nil
        }
    }

    /// Copies a workout onto every listed date in one atomic batch
    /// (a recurring schedule expanded by the caller). Merges the
    /// created workouts into the summaries (scheduled-date order)
    /// and caches their details. Returns the created workouts so
    /// the caller can report the count.
    @discardableResult
    public func duplicateBatch(id: String, dates: [String]) async -> [WorkoutDTO]? {
        errorMessage = nil
        do {
            let created = try await api.duplicateWorkouts(id: id, dates: dates)
            for workout in created {
                details[workout.id] = workout
                summaries.append(summary(of: workout))
            }
            summaries.sort { $0.scheduledDate > $1.scheduledDate }
            invalidateCalendarCache()
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not duplicate the workouts."
            return nil
        }
    }

    // MARK: - Helpers

    /// Derives a list summary from a full workout response so
    /// create/update/status mutations can refresh the list without
    /// refetching.
    private func summary(of workout: WorkoutDTO) -> WorkoutSummaryDTO {
        WorkoutSummaryDTO(
            id: workout.id,
            name: workout.name,
            description: workout.description,
            scheduledDate: workout.scheduledDate,
            status: workout.status,
            healthActivityType: workout.healthActivityType,
            blockCount: workout.blocks.count,
            doneCount: workout.blocks.filter { $0.status == .done }.count,
            createdAt: workout.createdAt,
            updatedAt: workout.updatedAt
        )
    }
}
