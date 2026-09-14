import Foundation
import SwiftUI

/// View-model for the Workout Player (Beta). Owns one in-progress
/// workout session: the planned blocks/items (from
/// `GET /api/v1/workouts/:id?include=items`), the user's per-exercise
/// drafts and notes, which items were skipped, and how many sets
/// were server-confirmed per item.
///
/// Drafts are local-only and persisted on-device (keyed by workout
/// id) so backgrounding or killing the app mid-workout loses nothing
/// typed. Only valid sets are POSTed — each POST carries
/// `workout_id`/`block_id`/`workout_block_id` so the server can
/// attribute the sets and flip a planned workout to in_progress.
/// Invalid/empty rows stay on screen for further editing and never
/// block saving their valid siblings.
///
/// Skipped items are local-only too: a skipped exercise logs zero
/// sets (e.g. no equipment, injury) but still counts toward block
/// and workout completion. Skipped blocks may hold partial logged
/// sets underneath.
@MainActor
public final class WorkoutPlayerStore: ObservableObject {

    // MARK: - Published state

    /// Planned blocks with items, in position order. Refreshed by
    /// `load()`; drafts (keyed by stable catalogue item id) survive
    /// refreshes.
    @Published public private(set) var workout: WorkoutWithItemsDTO?

    /// Half-typed rows per planned item, keyed by
    /// `BlockItemDTO.id`. Every item starts with one empty draft.
    /// Internal (not public): `SetDraft` is module-internal, and
    /// `@testable` tests can still reach this.
    @Published var drafts: [String: [SetDraft]] = [:]

    /// Item ids the user marked skipped (zero sets expected).
    /// Local-only: the server only tracks per-block status.
    @Published public var skippedItemIDs: Set<String> = []

    /// Free-text notes per planned item, keyed by
    /// `BlockItemDTO.id`. Sent with the item's next log call
    /// (notes are per log call server-side, shared across the
    /// sets in that call — same semantics as `NewSetView`).
    /// Persisted in the on-device snapshot like drafts.
    @Published public var notes: [String: String] = [:]

    /// Server-confirmed logged-set counts per item id. Seeded from
    /// the resume endpoint on load, incremented on every 201.
    @Published public private(set) var loggedCounts: [String: Int] = [:]

    /// Most-recent load/log error, if any. Cleared at the start of
    /// every operation.
    @Published public var errorMessage: String?

    /// `true` while the initial fetch is in flight.
    @Published public private(set) var isLoading: Bool = false

    /// Item ids currently being logged (drives per-row spinners).
    @Published public private(set) var loggingItemIDs: Set<String> = []

    // MARK: - Dependencies

    private var api: APIClient
    private let workoutID: String
    private let defaults: UserDefaults
    private let weightUnit: String
    private let distanceUnit: String

    public init(
        workoutID: String,
        api: APIClient,
        weightUnit: String = "kg",
        distanceUnit: String = "km",
        defaults: UserDefaults = .standard
    ) {
        self.workoutID = workoutID
        self.api = api
        self.weightUnit = weightUnit
        self.distanceUnit = distanceUnit
        self.defaults = defaults
    }

    /// Replaces the backing `APIClient`. Used when the player is
    /// constructed before the live API exists. Mirrors
    /// `WorkoutStore.replaceAPI(_:)`.
    public func replaceAPI(_ api: APIClient) {
        self.api = api
    }

    // MARK: - Loading

    /// Fetches the workout with items plus its already-logged sets
    /// (resume). Drafts and skips restore from the on-device
    /// snapshot first so a kill mid-workout loses nothing; the
    /// server is the source of truth for logged counts only.
    public func load() async {
        restore()
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            async let workoutTask = api.getWorkoutWithItems(id: workoutID)
            async let entriesTask = api.listWorkoutExerciseEntries(workoutID: workoutID)
            let (fetched, entries) = try await (workoutTask, entriesTask)
            workout = fetched
            loggedCounts = Self.countsByItem(in: fetched, entries: entries)
            ensureDrafts(for: fetched)
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load the workout."
        }
    }

    // MARK: - Drafts

    /// Replaces one item's draft rows (editor writes go through
    /// here so every keystroke persists to the on-device snapshot).
    /// Internal: `SetDraft` is module-internal.
    func setDrafts(_ rows: [SetDraft], for itemID: String) {
        drafts[itemID] = rows
        persist()
    }

    /// Refreshes the local block statuses from the shared workout
    /// store after a check-off, so the player header and hints
    /// reflect the server-confirmed state without refetching items.
    public func refreshBlockStatuses(from store: WorkoutStore) {
        guard let current = workout else { return }
        guard let fresh = store.details[current.id] else { return }
        // Duplicate-tolerant lookup: last value wins instead of
        // trapping on a server-side data oddity (same pattern as
        // the dashboard's exercise lookup).
        var statuses: [String: WorkoutBlockStatusDTO] = [:]
        for block in fresh.blocks {
            statuses[block.id] = block.status
        }
        // `blocks` is immutable, so rebuild the array with the
        // refreshed statuses rather than mutating in place. The
        // time config rides along unchanged — a status refresh
        // must never drop it.
        let merged = current.blocks.map { block -> WorkoutBlockDetailDTO in
            guard let status = statuses[block.id] else { return block }
            return WorkoutBlockDetailDTO(
                id: block.id,
                blockID: block.blockID,
                blockName: block.blockName,
                blockDescription: block.blockDescription,
                blockType: block.blockType,
                position: block.position,
                status: status,
                itemCount: block.itemCount,
                items: block.items,
                rounds: block.rounds,
                restSeconds: block.restSeconds,
                timeCapSeconds: block.timeCapSeconds,
                intervalSeconds: block.intervalSeconds
            )
        }
        workout = WorkoutWithItemsDTO(
            id: current.id,
            name: current.name,
            description: current.description,
            scheduledDate: current.scheduledDate,
            status: current.status,
            blocks: merged,
            createdAt: current.createdAt,
            updatedAt: current.updatedAt
        )
    }

    /// Flips the local copy planned to in_progress after the first
    /// linked sets land (mirrors the server rule, no request).
    public func noteLinkedSetsSaved() {
        guard var current = workout, current.status == .planned else { return }
        current = WorkoutWithItemsDTO(
            id: current.id,
            name: current.name,
            description: current.description,
            scheduledDate: current.scheduledDate,
            status: .inProgress,
            blocks: current.blocks,
            createdAt: current.createdAt,
            updatedAt: current.updatedAt
        )
        workout = current
    }

    /// Appends an empty row to the item's draft list.
    public func addDraftRow(itemID: String) {
        var rows = drafts[itemID] ?? []
        rows.append(SetDraft(distanceUnit: distanceUnit))
        drafts[itemID] = rows
        persist()
    }

    /// Removes one draft row, always leaving at least one row ready
    /// to fill (mirrors `NewSetView`'s delete behavior).
    public func removeDraftRow(itemID: String, id: UUID) {
        var rows = drafts[itemID] ?? []
        rows.removeAll { $0.id == id }
        if rows.isEmpty {
            rows.append(SetDraft(distanceUnit: distanceUnit))
        }
        drafts[itemID] = rows
        persist()
    }

    /// Replaces one item's notes. Trims whitespace and caps at 500
    /// characters to match the server's `notes` validation, then
    /// persists to the on-device snapshot.
    public func setNotes(_ text: String, for itemID: String) {
        let trimmed = text.trimmingCharacters(in: .whitespacesAndNewlines)
        if trimmed.isEmpty {
            notes.removeValue(forKey: itemID)
        } else {
            notes[itemID] = String(trimmed.prefix(500))
        }
        persist()
    }

    /// Marks an item skipped (zero sets expected, e.g. no equipment
    /// or injury) or un-skips it. Skipped items still count toward
    /// block completion; any already-logged sets underneath stay.
    public func toggleSkip(itemID: String) {
        if skippedItemIDs.contains(itemID) {
            skippedItemIDs.remove(itemID)
        } else {
            skippedItemIDs.insert(itemID)
        }
        persist()
    }

    // MARK: - Logging

    /// POSTs the item's valid drafts as linked exercise entries.
    /// Invalid/empty rows are kept for editing; valid rows clear on
    /// success and the server-confirmed count grows. The first
    /// linked set flips a planned workout to in_progress
    /// server-side — "Start" itself performs no request.
    public func log(item: BlockItemDTO, in block: WorkoutBlockDetailDTO) async {
        let rows = drafts[item.id] ?? []
        let isCardio = item.isCardio
        let valid = rows.filter { $0.isValid(isCardioMode: isCardio) }
        guard !valid.isEmpty else { return }
        errorMessage = nil
        loggingItemIDs.insert(item.id)
        defer { loggingItemIDs.remove(item.id) }
        do {
            let created = try await api.createExerciseEntries(
                CreateExerciseEntriesRequest(
                    exerciseID: item.exerciseID,
                    notes: notes[item.id] ?? "",
                    createdAt: nil,
                    sets: valid.map { $0.setInput(workoutID: workoutID, blockID: block.blockID, workoutBlockID: block.id) }
                )
            )
            loggedCounts[item.id, default: 0] += created.count
            // Keep only the rows that still need attention.
            let remaining = rows.filter { !$0.isValid(isCardioMode: isCardio) }
            drafts[item.id] = remaining.isEmpty ? [SetDraft(distanceUnit: distanceUnit)] : remaining
            noteLinkedSetsSaved()
            persist()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your sets."
        }
    }

    /// POSTs every non-skipped item's valid drafts in the block,
    /// reusing `log(item:in:)` per item. Backs "Mark as Done"'s
    /// autosubmit: the Done handler calls this first and only
    /// marks the block done on `true`.
    ///
    /// Returns `false` on the first failure — `errorMessage` is
    /// already set by the failing `log` call and the remaining
    /// items are left untouched so the user can fix and retry.
    /// Items with no valid drafts (empty rows or skipped items)
    /// are passed over, and `true` is returned when there was
    /// nothing to submit, preserving the skip-only Done path.
    public func logAllValid(in block: WorkoutBlockDetailDTO) async -> Bool {
        for item in block.items where !skippedItemIDs.contains(item.id) {
            let rows = drafts[item.id] ?? []
            guard rows.contains(where: { $0.isValid(isCardioMode: item.isCardio) }) else { continue }
            await log(item: item, in: block)
            if errorMessage != nil { return false }
        }
        return true
    }

    /// `true` while any of the block's items has a log request in
    /// flight. Keeps "Mark as Done" disabled through the whole
    /// autosubmit so a tap can't race the final submit.
    public func isLoggingAny(in block: WorkoutBlockDetailDTO) -> Bool {
        block.items.contains { loggingItemIDs.contains($0.id) }
    }

    /// `true` when any non-skipped item holds a valid unsubmitted
    /// draft. Done blocks keep their editors (extra sets can be
    /// logged after a reopen), so this re-exposes a submit button
    /// there — without it, sets typed into a done block could
    /// never be posted.
    public func hasValidDrafts(in block: WorkoutBlockDetailDTO) -> Bool {
        block.items.contains { item in
            !skippedItemIDs.contains(item.id)
                && (drafts[item.id] ?? []).contains { $0.isValid(isCardioMode: item.isCardio) }
        }
    }

    /// `true` while the item's log request is in flight.
    public func isLogging(itemID: String) -> Bool {
        loggingItemIDs.contains(itemID)
    }

    /// Drops the on-device snapshot (called on Finish or discard).
    /// Server-confirmed sets are unaffected — they are real
    /// exercise entries, not drafts.
    public func clearSnapshot() {
        defaults.removeObject(forKey: Self.snapshotKey(for: workoutID))
    }

    // MARK: - Progress (hybrid)

    /// Server-confirmed logged count for one item.
    public func loggedCount(itemID: String) -> Int {
        loggedCounts[itemID, default: 0]
    }

    /// An item is done when skipped or when at least one set was
    /// server-confirmed. Manual block check-offs stay the source of
    /// truth; this only drives the "ready to mark done" hint.
    public func isItemDone(itemID: String) -> Bool {
        skippedItemIDs.contains(itemID) || loggedCount(itemID: itemID) > 0
    }

    /// A block is ready to mark done when every planned item is
    /// done (logged or skipped). Empty blocks never count.
    public func isBlockReady(_ block: WorkoutBlockDetailDTO) -> Bool {
        guard !block.items.isEmpty else { return false }
        return block.items.allSatisfy { isItemDone(itemID: $0.id) }
    }

    /// Done/total blocks across the workout, for the header
    /// progress bar and "% complete" label. Done and skipped both
    /// count — a skip is a finished decision, matching how skips
    /// count toward readiness everywhere else. Block statuses are
    /// the check-off source of truth, so they (not per-item logged
    /// counts) drive the headline number.
    public func blockProgress() -> (done: Int, total: Int) {
        guard let workout else { return (0, 0) }
        return (Self.completedBlockCount(in: workout.blocks), workout.blocks.count)
    }

    /// Blocks finished by check-off: done or skipped. A skip is a
    /// finished decision, matching how skips count toward
    /// readiness everywhere else. Static so the math is directly
    /// unit-testable.
    public static func completedBlockCount(in blocks: [WorkoutBlockDetailDTO]) -> Int {
        blocks.filter { $0.status == .done || $0.status == .skipped }.count
    }

    // MARK: - Resume matching

    /// Buckets resume entries onto planned items by stable ids:
    /// the entry's `blockID` must equal the parent block's catalogue
    /// `blockID` and its `exerciseID` the item's exercise. The
    /// fragile `workoutBlockID` join pointer is deliberately
    /// ignored — workout edits regenerate join ids while
    /// `blockID` survives.
    public static func countsByItem(
        in workout: WorkoutWithItemsDTO,
        entries: [ExerciseEntryDTO]
    ) -> [String: Int] {
        var out: [String: Int] = [:]
        for block in workout.blocks {
            for item in block.items {
                let n = entries.filter {
                    $0.blockID == block.blockID && $0.exerciseID == item.exerciseID
                }.count
                if n > 0 {
                    out[item.id] = n
                }
            }
        }
        return out
    }

    // MARK: - Persistence

    private static func snapshotKey(for workoutID: String) -> String {
        "hylete.workout-player.\(workoutID)"
    }

    /// On-device snapshot: half-typed rows, skips, and notes.
    /// `SetDraft` is `Codable`, so the whole map round-trips as
    /// JSON. `notes` defaults to empty so snapshots written
    /// before notes existed still decode.
    private struct Snapshot: Codable {
        var drafts: [String: [SetDraft]]
        var skipped: [String]
        var notes: [String: String] = [:]
    }

    private func persist() {
        let snapshot = Snapshot(drafts: drafts, skipped: Array(skippedItemIDs), notes: notes)
        if let data = try? APIClient.jsonEncoder.encode(snapshot) {
            defaults.set(data, forKey: Self.snapshotKey(for: workoutID))
        }
    }

    private func restore() {
        guard let data = defaults.data(forKey: Self.snapshotKey(for: workoutID)),
              let snapshot = try? APIClient.jsonDecoder.decode(Snapshot.self, from: data)
        else { return }
        drafts = snapshot.drafts
        skippedItemIDs = Set(snapshot.skipped)
        notes = snapshot.notes
    }

    private func ensureDrafts(for workout: WorkoutWithItemsDTO) {
        for block in workout.blocks {
            for item in block.items {
                if drafts[item.id] == nil || drafts[item.id]?.isEmpty == true {
                    drafts[item.id] = [SetDraft(distanceUnit: distanceUnit)]
                }
            }
        }
    }
}

// MARK: - SetDraft mapping

extension SetDraft {
    /// Maps one draft row onto the API payload, stamping the player
    /// linkage. Callers filter with `isValid(isCardioMode:)` first —
    /// force-unwrap-free: cardio branches require parsed duration +
    /// distance, strength branches require reps + weight.
    func setInput(workoutID: String, blockID: String, workoutBlockID: String) -> CreateSetInput {
        if let seconds = durationSecondsValue, seconds > 0,
           let meters = distanceMetersValue, meters > 0 {
            return CreateSetInput(
                reps: 0,
                weight: 0,
                restTime: 0,
                durationSeconds: Int(seconds),
                distanceMeters: meters,
                avgHeartRate: avgHeartRate ?? 0,
                caloriesBurned: caloriesValue ?? 0,
                workoutID: workoutID,
                blockID: blockID,
                workoutBlockID: workoutBlockID
            )
        }
        return CreateSetInput(
            reps: reps ?? 0,
            weight: weightValue ?? 0,
            restTime: restSeconds ?? 0,
            workoutID: workoutID,
            blockID: blockID,
            workoutBlockID: workoutBlockID
        )
    }
}
