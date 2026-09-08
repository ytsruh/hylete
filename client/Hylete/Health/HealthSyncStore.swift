import Foundation

/// Owns the Health-to-server sync: 90-day backfill at opt-in
/// plus opportunistic catch-up thereafter. Uploads are
/// idempotent server-side upserts, so runs are safe to repeat —
/// failures stay unconfirmed for the next run, and a closed app
/// just means delayed (never lost) uploads.
///
/// Two fetch sets per run: a rolling re-verify window (the last
/// 7 days including today, always re-fetched and re-upserted so
/// late Watch data and corrections land) plus the older
/// backfill window (unconfirmed days only; empties skipped,
/// old empties sealed). Confirmed dates outside the re-verify
/// window are skipped.
///
/// Consent lives in `UserDefaults` under `enabledKey` (driven
/// by the Profile toggle); nothing uploads while off.
@MainActor
public final class HealthSyncStore: ObservableObject {
    /// Sync progress for the Profile row.
    public enum Phase: Equatable {
        case idle
        case fetching
        case uploading(done: Int, total: Int)
        case failed(String)
    }

    /// UserDefaults key for the opt-in. Shared with the Profile
    /// toggle's `@AppStorage`, which writes through to the same
    /// store — single source of truth, no manual mirroring.
    public static let enabledKey = "healthSyncEnabled"
    /// Persisted set of confirmed YYYY-MM-DD dates.
    public static let confirmedDatesKey = "healthSyncConfirmedDates"
    /// Sync window: past N days for backfill (today is still
    /// accumulating, so backfill excludes it — but the rolling
    /// re-verify window below always includes today).
    public static let windowDays = 90
    /// The rolling re-verify window always re-fetched and
    /// re-upserted: the last 7 days including today. Catches
    /// late Watch backfills, user corrections in Apple Health,
    /// and today's accumulating totals.
    public static let reverifyDays = 7
    /// Upload chunk size: a full backfill takes three
    /// requests, each well under the server's batch cap, and
    /// progress reads honestly per chunk.
    public static let chunkSize = 30

    public static var isEnabled: Bool {
        UserDefaults.standard.bool(forKey: enabledKey)
    }

    /// Shared instance bound in the Health tab and Profile.
    /// Tests construct their own with mock providers.
    public static let shared = HealthSyncStore()

    @Published public private(set) var phase: Phase = .idle
    @Published public private(set) var lastSync: Date?

    private let history: HealthHistoryProvider

    public init(history: HealthHistoryProvider = LiveHealthStore()) {
        self.history = history
    }

    /// One-line status for the Profile row.
    public var statusLine: String {
        switch phase {
        case .idle:
            if let lastSync {
                return "Last synced \(lastSync.formatted(date: .abbreviated, time: .shortened))"
            }
            return Self.isEnabled ? "Up to date" : "Off"
        case .fetching:
            return "Reading Apple Health…"
        case .uploading(let done, let total):
            return "Uploading \(done) of \(total) days…"
        case .failed(let message):
            return message
        }
    }

    /// Runs one sync pass: always re-verifies the recent
    /// window (last 7 days including today — late data and
    /// corrections overwrite via upsert), plus any unconfirmed
    /// older days in the backfill window. Empty days are never
    /// uploaded; old empties seal, recent empties stay open.
    /// No-op while opted out. Cancellation resets to idle —
    /// confirmed chunks stay confirmed.
    public func syncIfNeeded(api: HealthSnapshotAPI, windowDays: Int? = nil) async {
        guard Self.isEnabled else { return }
        let window = windowDays ?? Self.windowDays
        guard history.isAvailable() else {
            phase = .failed("Apple Health isn't available on this device.")
            return
        }
        let calendar = Calendar.current
        // Rolling re-verify window first (oldest first), then
        // unconfirmed older backfill days. The two sets are
        // disjoint by construction: backfill excludes anything
        // in the re-verify window.
        let recentDays = syncDays(count: Self.reverifyDays, includingToday: true)
        let recentKeys = Set(recentDays.map { snapshotDateString($0) })
        let confirmed = Self.confirmedDates()
        let olderDays = syncDays(count: window).filter { day in
            let key = snapshotDateString(day)
            return !recentKeys.contains(key) && !confirmed.contains(key)
        }
        let fetchDays = (recentDays + olderDays).sorted()
        do {
            phase = .fetching
            try Task.checkCancellation()
            let snapshots = await history.fetchHistory(days: fetchDays)
            // YYYY-MM-DD strings sort chronologically, so the
            // 7-day sealing cutoff is a plain string compare.
            // (<= so a 7-day-old empty seals instead of lingering
            // one day outside the re-verify window forever.)
            let sealOnOrBefore = snapshotDateString(
                calendar.date(byAdding: .day, value: -Self.reverifyDays, to: Date()) ?? Date()
            )
            var payloads: [HealthSnapshotPayload] = []
            var sealed: [String] = []
            for snap in snapshots {
                if snap.isEmpty {
                    if snap.date <= sealOnOrBefore { sealed.append(snap.date) }
                    continue
                }
                payloads.append(HealthSnapshotPayload(day: snap))
            }
            if !sealed.isEmpty { Self.markConfirmed(sealed) }
            if !payloads.isEmpty {
                var done = 0
                for chunk in payloads.chunked(into: Self.chunkSize) {
                    try Task.checkCancellation()
                    phase = .uploading(done: done, total: payloads.count)
                    _ = try await api.upsertHealthSnapshots(Array(chunk))
                    done += chunk.count
                    Self.markConfirmed(chunk.map(\.snapshotDate))
                    phase = .uploading(done: done, total: payloads.count)
                }
            }
            lastSync = Date()
            phase = .idle
        } catch is CancellationError {
            phase = .idle
        } catch {
            phase = .failed("Couldn't reach the server. Will retry next time.")
        }
    }

    // MARK: - Confirmed-date bookkeeping

    static func confirmedDates() -> Set<String> {
        Set(UserDefaults.standard.stringArray(forKey: confirmedDatesKey) ?? [])
    }

    static func markConfirmed(_ dates: [String]) {
        var set = confirmedDates()
        for date in dates { set.insert(date) }
        // Bound the set: drop anything older than 120 days so
        // the array can't grow without limit.
        let cutoff = snapshotDateString(
            Calendar.current.date(byAdding: .day, value: -120, to: Date()) ?? Date()
        )
        set = set.filter { $0 >= cutoff }
        UserDefaults.standard.set(Array(set), forKey: confirmedDatesKey)
    }
}

private extension Array {
    /// Splits into consecutive chunks of at most `size`
    /// (last chunk may be smaller). Precondition: size > 0.
    func chunked(into size: Int) -> [[Element]] {
        stride(from: 0, to: count, by: size).map { start in
            Array(self[start..<Swift.min(start + size, count)])
        }
    }
}
