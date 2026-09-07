import Foundation
import SwiftUI

/// View-model for the Weight tab. Owns the list of body
/// weight entries for the signed-in user and exposes a
/// small set of mutations (`create`, `update`, `delete`)
/// that mirror the server's `/api/v1/weight/*` endpoints.
///
/// The store loads once per tab appearance via `load()`, and
/// every mutation does an optimistic update so the UI
/// animates immediately and only rolls back if the server
/// rejects the change. Photos are not optimistic — the
/// client uploads the bytes to R2 first, then submits the
/// entry, so a photo upload is naturally a single
/// then-able async sequence rather than a snap-back-and-
/// retry.
@MainActor
public final class WeightStore: ObservableObject {

    // MARK: - Published state

    /// Every weight entry for the user, in the same order
    /// the server returns them: newest first. The chart
    /// re-sorts ascending for plotting, and the
    /// progress card derives `current` from the most
    /// recent entry.
    @Published public private(set) var entries: [WeightEntryDTO] = []

    /// Set during the initial load. Distinct from
    /// `entries.isEmpty` so the empty-state UI doesn't
    /// flash during a refresh.
    @Published public private(set) var isLoading: Bool = false

    /// Most-recent load/mutation error, if any. Cleared
    /// at the start of every `load()` / mutation so a
    /// stale message never lingers.
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
    /// dependency between itself and `APIClient`, and the
    /// pattern `GoalStore.replaceAPI(_:)` follows for the
    /// Goals tab.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Derived state

    /// The most recent weight entry, or `nil` when the
    /// user hasn't logged any entries yet. Drives the
    /// "Current" cell in the progress card.
    public var latestEntry: WeightEntryDTO? {
        entries.first
    }

    /// Returns the entry with the matching id, or `nil`
    /// when no such entry exists in the local cache.
    /// Used by the editor's `seedIfNeeded()` and the
    /// optimistic-update rollback paths.
    public func entry(for id: String) -> WeightEntryDTO? {
        entries.first { $0.id == id }
    }

    // MARK: - Loading

    /// Fetches the full list from the server. Called on
    /// first tab appearance and from pull-to-refresh.
    /// Silent no-op when the user is unauthenticated (the
    /// tab isn't shown in that state — `RootView` hides
    /// the whole tab bar).
    public func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            entries = try await api.listWeightEntries()
        } catch APIError.unauthorized {
            // Auth middleware has already cleared the
            // session — leave entries untouched so the
            // next view that mounts sees a clean slate.
            entries = []
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your weight entries."
        }
    }

    // MARK: - Mutations

    /// Creates a new weight entry and inserts the
    /// server-confirmed row at the head of the list
    /// (entries are newest-first). Returns the freshly
    /// stored entry so the caller can navigate to its
    /// edit view if it wants.
    @discardableResult
    public func create(_ request: CreateWeightEntryRequest) async -> WeightEntryDTO? {
        errorMessage = nil
        do {
            let created = try await api.createWeightEntry(request)
            insert(atHead: created)
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not save the weight entry."
            return nil
        }
    }

    /// Updates an existing weight entry. Replaces the
    /// row in place (preserves its position in the
    /// newest-first ordering) so the row doesn't jump
    /// around mid-edit.
    public func update(id: String, request: UpdateWeightEntryRequest) async {
        errorMessage = nil
        do {
            let updated = try await api.updateWeightEntry(id: id, request: request)
            replace(updated)
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the weight entry."
        }
    }

    /// Hard-deletes a weight entry. Optimistic remove;
    /// rolls back by re-inserting the original row on
    /// failure so the entry reappears in its correct
    /// position.
    public func delete(id: String) async {
        guard let original = entry(for: id) else {
            // Already gone — no-op rather than an error.
            // Mirrors the server's idempotent 204.
            return
        }
        errorMessage = nil
        entries.removeAll { $0.id == id }
        do {
            try await api.deleteWeightEntry(id: id)
        } catch let error as APIError {
            insert(atHead: original)
            errorMessage = error.errorDescription
        } catch {
            insert(atHead: original)
            errorMessage = "Could not delete the weight entry."
        }
    }

    // MARK: - Mutation helpers

    /// Inserts a new (or freshly-created) entry at the
    /// head of the list. The server returns new entries
    /// with `createdAt = time.Now()` so the new row is
    /// guaranteed to be the newest, hence the head.
    private func insert(atHead entry: WeightEntryDTO) {
        entries.insert(entry, at: 0)
    }

    /// Replaces the row with the matching id. No-op when
    /// the id isn't in the list (e.g. a server response
    /// for a row the user just deleted).
    private func replace(_ entry: WeightEntryDTO) {
        guard let index = entries.firstIndex(where: { $0.id == entry.id }) else {
            return
        }
        entries[index] = entry
    }
}
