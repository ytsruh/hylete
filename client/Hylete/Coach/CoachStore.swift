import Foundation
import SwiftUI

/// View-model for the beta-gated Coach tab. Owns the consent
/// state (`CoachPreferencesDTO`) and the card list (all stored
/// weekly reports, newest first, dismissed ones filtered out)
/// for the signed-in user.
///
/// Two UI states the views branch on (rather than probing error
/// strings): `hasNoReportYet` (opted in, list empty — the cron
/// hasn't produced a row, e.g. fresh opt-in) and `isOptedOut`
/// (server 403 — toggle off). Both derive from the typed
/// `APIError.server(status:)` case so copy stays exact.
@MainActor
public final class CoachStore: ObservableObject {

    // MARK: - Published state

    /// Server-side consent state. Nil until the first `load()`
    /// completes; the views render a loading row meanwhile.
    @Published public private(set) var preferences: CoachPreferencesDTO?

    /// Stored weekly reports, newest first, INCLUDING dismissed
    /// ones (dismiss stamps rather than deletes). Views render
    /// `visibleReports` / `dismissedReports`, never this raw list.
    @Published public private(set) var reports: [CoachReportDTO] = []

    /// Set during any load/build. Distinct from empty-list so
    /// the empty state doesn't flash mid-refresh.
    @Published public private(set) var isLoading: Bool = false

    /// True when the user is opted in but the list is empty —
    /// the cron hasn't written a row (or no training data yet).
    /// The view offers "Generate now".
    @Published public private(set) var hasNoReportYet: Bool = false
    /// Most-recent error, if any. Cleared at the start of every
    /// action so stale messages never linger.
    @Published public var errorMessage: String?

    // MARK: - Dependencies

    private var api: APIClient

    public init(api: APIClient) {
        self.api = api
    }

    /// Replaces the backing `APIClient`. Same stub-then-swap
    /// pattern `GoalStore.replaceAPI` uses (see `MainTabView`);
    /// the tab constructs with a stub and swaps on appear.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Derived state

    /// True when the server-side toggle is on. Nil preferences
    /// read as opted-out so the tab never flashes report UI
    /// before the first load.
    public var isOptedOut: Bool {
        preferences?.optIn != true
    }

    /// Live cards, newest first. Dismissed reports are excluded
    /// here and surface in the archive instead.
    public var visibleReports: [CoachReportDTO] {
        reports.filter { !$0.isDismissed }.sorted { $0.periodStart > $1.periodStart }
    }

    /// Dismissed reports, newest first. Dismiss stamps rather
    /// than deletes, so the archive is just this filter over
    /// the same server list.
    public var dismissedReports: [CoachReportDTO] {
        reports.filter { $0.isDismissed }.sorted { $0.periodStart > $1.periodStart }
    }

    // MARK: - Loading

    /// Loads consent state + the full card list (newest first).
    /// Called on tab appearance and pull-to-refresh.
    public func load() async {
        errorMessage = nil
        hasNoReportYet = false
        isLoading = true
        defer { isLoading = false }
        do {
            preferences = try await api.getCoachPreferences()
        } catch {
            fail(error, "Could not load Coach settings.")
            return
        }
        guard preferences?.optIn == true else { return }
        do {
            reports = try await api.listCoachReports()
            hasNoReportYet = visibleReports.isEmpty && dismissedReports.isEmpty
        } catch {
            fail(error, "Could not load your reviews.")
        }
    }

    // MARK: - Preferences

    /// Saves the opt-in toggle + aim. On success the stored
    /// state is published; opting out clears the card list
    /// (the API would 403 the next load anyway).
    public func savePreferences(optIn: Bool, goalText: String) async {
        errorMessage = nil
        let trimmed = goalText.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.count <= CoachPreferencesDTO.maxGoalTextLength else {
            errorMessage = "Your aim must be \(CoachPreferencesDTO.maxGoalTextLength) characters or less."
            return
        }
        do {
            preferences = try await api.updateCoachPreferences(
                UpdateCoachPreferencesRequest(optIn: optIn, goalText: trimmed)
            )
            if optIn {
                hasNoReportYet = visibleReports.isEmpty && dismissedReports.isEmpty
            } else {
                reports = []
                hasNoReportYet = false
            }
        } catch {
            fail(error, "Could not save Coach settings.")
        }
    }

    // MARK: - Dismiss

    /// Dismisses a card (swipe action). Stamps it immediately so
    /// it slides into the archive, then tells the server; on
    /// failure the list reloads so the card reappears rather
    /// than lying about its state.
    public func dismiss(id: String) async {
        errorMessage = nil
        stamp(id: id, dismissedAt: Date())
        do {
            try await api.dismissCoachReport(id: id)
            hasNoReportYet = visibleReports.isEmpty && dismissedReports.isEmpty && preferences?.optIn == true
        } catch {
            fail(error, "Could not dismiss the review.")
            await load()
        }
    }

    /// Restores an archived card (swipe action in the archive).
    /// Clears the stamp immediately so it rejoins the card list,
    /// then tells the server; on failure the list reloads.
    public func restore(id: String) async {
        errorMessage = nil
        stamp(id: id, dismissedAt: nil)
        do {
            try await api.restoreCoachReport(id: id)
            hasNoReportYet = false
        } catch {
            fail(error, "Could not restore the review.")
            await load()
        }
    }

    // MARK: - Helpers

    /// Records a failure unless it is a cancellation (torn-down
    /// refresh, navigation mid-request, app backgrounding).
    /// Cancellation surfaces as `URLError.cancelled` through
    /// `APIError.transport`, whose description is the bare word
    /// "cancelled" — banner-worthy nowhere, so it stays silent.
    private func fail(_ error: Error, _ fallback: String) {
        if error is CancellationError { return }
        if case APIError.transport(let urlError) = error,
           urlError.code == .cancelled {
            return
        }
        errorMessage = (error as? APIError)?.errorDescription ?? fallback
    }

    // MARK: - Helpers

    /// Replaces the row's `dismissedAt` in place (nil = live).
    /// No-op for unknown ids.
    private func stamp(id: String, dismissedAt: Date?) {
        guard let index = reports.firstIndex(where: { $0.id == id }) else { return }
        let old = reports[index]
        reports[index] = CoachReportDTO(
            id: old.id,
            type: old.type,
            periodStart: old.periodStart,
            periodEnd: old.periodEnd,
            promptVersion: old.promptVersion,
            model: old.model,
            payload: old.payload,
            dismissedAt: dismissedAt,
            createdAt: old.createdAt
        )
    }
}
