import Foundation
import SwiftUI

/// View-model for the Blocks list (Beta). Owns the user's planned
/// blocks and exposes CRUD mutations mirroring the server's
/// `/api/v1/blocks/*` endpoints.
///
/// The list holds summaries (no items); full detail loads on
/// demand into `details` so the detail view can render without a
/// second spinner after coming back from the editor. Every
/// mutation refreshes the affected rows from the server response
/// so the list ordering (newest first) stays truthful without a
/// full reload.
@MainActor
public final class BlockStore: ObservableObject {

    // MARK: - Published state

    /// Every block summary for the user, newest first (the
    /// server's order). Drives the list rows.
    @Published public private(set) var blocks: [BlockSummaryDTO] = []

    /// Fully-loaded blocks by id. Populated by `detail(id:)`
    /// and by create/update responses.
    @Published public private(set) var details: [String: BlockDTO] = [:]

    /// Set during the initial load. Distinct from
    /// `blocks.isEmpty` so the empty-state UI doesn't flash
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
    /// `AuthStore`. Mirrors `GoalStore.replaceAPI(_:)`.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Loading

    /// Fetches the block summaries from the server. Called on
    /// first list appearance and from pull-to-refresh. Silent
    /// no-op when unauthenticated.
    public func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            blocks = try await api.listBlocks()
        } catch APIError.unauthorized {
            blocks = []
            details = [:]
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load your blocks."
        }
    }

    /// Returns the cached detail for a block, fetching it when
    /// absent (or when `refresh` is true, e.g. after returning
    /// from the editor). Returns nil on failure and surfaces
    /// the error in `errorMessage`.
    @discardableResult
    public func detail(id: String, refresh: Bool = false) async -> BlockDTO? {
        if !refresh, let cached = details[id] {
            return cached
        }
        errorMessage = nil
        do {
            let block = try await api.getBlock(id: id)
            details[id] = block
            return block
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not load the block."
            return nil
        }
    }

    // MARK: - Mutations

    /// Creates a block. Prepends the server-confirmed summary
    /// (newest first matches the server order) and caches the
    /// full detail. Returns the created block so the caller
    /// can navigate to it.
    @discardableResult
    public func create(_ request: CreateBlockRequest) async -> BlockDTO? {
        errorMessage = nil
        do {
            let created = try await api.createBlock(request)
            details[created.id] = created
            blocks.insert(summary(of: created), at: 0)
            return created
        } catch let error as APIError {
            errorMessage = error.errorDescription
            return nil
        } catch {
            errorMessage = "Could not save the block."
            return nil
        }
    }

    /// Updates a block (items fully replaced). Refreshes the
    /// cached detail and the summary row in place.
    public func update(id: String, request: UpdateBlockRequest) async {
        errorMessage = nil
        do {
            let updated = try await api.updateBlock(id: id, request: request)
            details[id] = updated
            if let index = blocks.firstIndex(where: { $0.id == id }) {
                blocks[index] = summary(of: updated)
            }
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not update the block."
        }
    }

    /// Hard-deletes a block. Optimistic remove; rolls back by
    /// re-inserting the summary on failure.
    public func delete(id: String) async {
        guard let index = blocks.firstIndex(where: { $0.id == id }) else { return }
        errorMessage = nil
        let removed = blocks.remove(at: index)
        details.removeValue(forKey: id)
        do {
            try await api.deleteBlock(id: id)
        } catch let error as APIError {
            blocks.insert(removed, at: min(index, blocks.count))
            errorMessage = error.errorDescription
        } catch {
            blocks.insert(removed, at: min(index, blocks.count))
            errorMessage = "Could not delete the block."
        }
    }

    // MARK: - Helpers

    /// Derives a list summary from a full block response so
    /// create/update can refresh the list without refetching.
    private func summary(of block: BlockDTO) -> BlockSummaryDTO {
        BlockSummaryDTO(
            id: block.id,
            name: block.name,
            description: block.description,
            type: block.type,
            rounds: block.rounds,
            restSeconds: block.restSeconds,
            timeCapSeconds: block.timeCapSeconds,
            intervalSeconds: block.intervalSeconds,
            itemCount: block.items.count,
            createdAt: block.createdAt,
            updatedAt: block.updatedAt
        )
    }
}
