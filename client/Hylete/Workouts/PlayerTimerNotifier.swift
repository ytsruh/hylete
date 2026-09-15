import Foundation
import SwiftUI

/// Bridges `PlayerTimerStore` fire dates to fallback local
/// notifications. The player observes `timerStore.endDate`
/// and forwards every change here — one observation point
/// covers starts, EMOM round rolls, pauses, resumes, resets,
/// and completions, so no chip/sheet call site needs
/// notification code.
///
/// Permission is requested on the first schedule attempt
/// (first timer start), never at launch or player open. A
/// denied outcome degrades silently to foreground-only cues
/// — the timer itself never depends on the answer.
@MainActor
public final class PlayerTimerNotifier: ObservableObject {
    private let center: TimerNotificationCenter
    private var pendingID: String?
    /// Monotonic schedule generation. A rapid start→replace
    /// leaves the old schedule Task in flight; it must not
    /// plant a stale notification after the new timer's.
    private var generation = 0

    /// Last-known authorisation, driving the sheet's
    /// explainer caption. Refreshed on every schedule
    /// attempt and on sheet appear.
    @Published public private(set) var status: TimerNotificationAuthStatus = .notDetermined

    public init(center: TimerNotificationCenter = LiveTimerNotificationCenter()) {
        self.center = center
    }

    /// Call on every `timerStore.endDate` change. Nil (pause /
    /// reset / complete) cancels the pending fallback; a new
    /// date re-schedules it.
    public func timerFireDateChanged(_ date: Date?, store: PlayerTimerStore) {
        generation += 1
        cancelPending()
        guard let date else { return }
        let current = generation
        Task {
            await self.scheduleIfGranted(id: UUID().uuidString, at: date, store: store, generation: current)
        }
    }

    /// Re-reads the system authorisation into `status`.
    public func refreshStatus() async {
        status = await center.authorizationStatus()
    }

    /// Alert copy for a timer state. Static so tests can
    /// assert it without a notifier.
    public static func content(for store: PlayerTimerStore) -> (title: String, body: String) {
        let context = store.blockName.isEmpty ? "Workout" : store.blockName
        switch store.kind {
        case .rest:
            return ("Rest over", "\(context) — time to lift.")
        case .amrap:
            return ("Time!", "\(context) — cap reached.")
        case .emom:
            if store.currentRound >= store.totalRounds {
                return ("EMOM complete", "\(context) — all \(store.totalRounds) rounds done.")
            }
            return (
                "Round \(store.currentRound) done",
                "\(context) — round \(store.currentRound + 1) of \(store.totalRounds) is up."
            )
        }
    }

    private func scheduleIfGranted(id: String, at date: Date, store: PlayerTimerStore, generation: Int) async {
        guard date.timeIntervalSinceNow > 0 else { return }
        if await center.authorizationStatus() == .notDetermined {
            _ = try? await center.requestAuthorization()
        }
        status = await center.authorizationStatus()
        guard status == .granted, generation == self.generation else { return }
        pendingID = id
        let content = Self.content(for: store)
        try? await center.schedule(id: id, at: date, title: content.title, body: content.body)
    }

    private func cancelPending() {
        if let pendingID {
            center.cancel(id: pendingID)
        }
        pendingID = nil
    }
}
