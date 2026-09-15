import Foundation
import SwiftUI

/// The single shared timer for the Workout Player.
///
/// The standalone Dashboard timer (`CountdownTimerView` /
/// `EMOMView`) keeps its state in local `@State`, which is the
/// right call for a modal sheet but the wrong call inside the
/// player: block cards collapse, scroll off-screen, and get
/// rebuilt, so any per-card timer state would die with the
/// row. This store owns the one active timer for the whole
/// player session instead — block header chips only *start*
/// timers here, and the sticky pill + expanded sheet only
/// *render* this store. Starting a new timer replaces the
/// running one; there is deliberately no multi-timer support.
///
/// Timing is anchored to an absolute `endDate` (same pattern
/// as the standalone timers) so backgrounding or a late
/// `TimelineView` tick can't skew the display: every render
/// recomputes from `Date()`. All mutating methods take an
/// explicit `now` (defaulting to `Date()`) so the state
/// machine is unit-testable without sleeping.
///
/// Feedback (`TimerFeedback`) is intentionally NOT fired here —
/// it plays system sounds/haptics, which unit tests must never
/// trigger. Callers (the pill's `.task`) fire it based on the
/// return values of `completeNow()` / `advanceRound(now:)`.
///
/// Wake-locking is also owned by the caller: the player holds
/// `TimerWakeLock` for its whole lifetime, so the store never
/// acquires or releases it.
@MainActor
public final class PlayerTimerStore: ObservableObject {

    /// Which countdown the store is running. Rest and AMRAP
    /// are both single countdowns (AMRAP counts down from the
    /// time cap — V1 has no count-up/stopwatch mode); EMOM
    /// chains `totalRounds` countdowns of `intervalSeconds`.
    public enum Kind: String, Equatable, Hashable, CaseIterable {
        case rest
        case emom
        case amrap

        /// User-facing label for the sheet's type picker.
        public var displayName: String {
            switch self {
            case .rest: return "Rest"
            case .emom: return "EMOM"
            case .amrap: return "AMRAP"
            }
        }
    }

    /// Observable timer events for visual feedback. The pill
    /// flashes on each event and auto-expands the sheet on
    /// completion; haptics/sounds stay with the caller (see
    /// header). Each event carries a fresh id so repeated
    /// boundaries re-trigger `.onChange` observers.
    public enum TimerEvent: Equatable {
        case roundBoundary(id: UUID)
        case sessionComplete(id: UUID)
    }

    /// `true` once a timer has been started and until `reset()`.
    /// Drives the sticky pill's visibility. Stays `true`
    /// through the complete state so the pill can show
    /// "Timer Complete!" with a dismiss/start-another action.
    @Published public private(set) var isActive: Bool = false

    /// The most recent event (see `TimerEvent`). Nil when idle
    /// or after `reset()`.
    @Published public private(set) var lastEvent: TimerEvent?

    /// What kind of timer is (or was) running.
    @Published public private(set) var kind: Kind = .rest

    /// The workout-block id that started the timer, for
    /// highlighting that block's timer chip while active.
    /// Nil for timers started from the sheet's custom form
    /// without a block context.
    @Published public private(set) var blockID: String?

    /// The block's display name, shown in the pill subtitle
    /// (e.g. "Rest · Push"). Falls back to "Workout" when the
    /// timer was started without a block context.
    @Published public private(set) var blockName: String = ""

    /// Absolute fire date while running; nil while paused or
    /// complete. The pill's sleeper task keys on this value.
    @Published public private(set) var endDate: Date?

    /// `true` while the countdown is ticking.
    @Published public private(set) var isRunning: Bool = false

    /// `true` once the countdown hits zero (or the final EMOM
    /// round ends) and until `reset()` or a new start.
    @Published public private(set) var isComplete: Bool = false

    /// EMOM only: total rounds in this session.
    @Published public private(set) var totalRounds: Int = 0

    /// EMOM only: the round currently counting down (1-based).
    @Published public private(set) var currentRound: Int = 1

    /// EMOM only: seconds per round. Unlike the standalone
    /// `EMOMView` (hardcoded 60s), the player honours each
    /// block's `intervalSeconds`.
    @Published public private(set) var intervalSeconds: Int = 60

    /// Seconds shown when not running: the full duration
    /// before the first tick, the frozen remainder while
    /// paused. Updated on every pause so resume re-arms from
    /// the exact remainder — including mid-round for EMOM
    /// (the standalone EMOM view restarts the whole round on
    /// resume; the player keeps the remainder, which is
    /// strictly less surprising mid-workout).
    @Published private(set) var pendingSeconds: Int = 60

    public init() {}

    // MARK: - Starts (each replaces any running timer)

    /// Starts a rest countdown of `seconds`.
    public func startRest(seconds: Int, blockID: String?, blockName: String, now: Date = Date()) {
        guard seconds > 0 else { return }
        kind = .rest
        self.blockID = blockID
        self.blockName = blockName
        totalRounds = 0
        currentRound = 1
        pendingSeconds = seconds
        arm(seconds: seconds, now: now)
    }

    /// Starts an EMOM of `rounds` rounds, `intervalSeconds`
    /// per round. Unlike the standalone view the interval is
    /// a parameter so blocks with non-60s intervals work.
    public func startEMOM(rounds: Int, intervalSeconds: Int, blockID: String?, blockName: String, now: Date = Date()) {
        guard rounds > 0, intervalSeconds > 0 else { return }
        kind = .emom
        self.blockID = blockID
        self.blockName = blockName
        totalRounds = rounds
        currentRound = 1
        self.intervalSeconds = intervalSeconds
        pendingSeconds = intervalSeconds
        arm(seconds: intervalSeconds, now: now)
    }

    /// Starts an AMRAP cap countdown of `capSeconds`. V1 is a
    /// countdown from the cap (matching the web); there is no
    /// count-up mode — round/rep counting is already covered
    /// by logged-set counts.
    public func startAMRAP(capSeconds: Int, blockID: String?, blockName: String, now: Date = Date()) {
        guard capSeconds > 0 else { return }
        kind = .amrap
        self.blockID = blockID
        self.blockName = blockName
        totalRounds = 0
        currentRound = 1
        pendingSeconds = capSeconds
        arm(seconds: capSeconds, now: now)
    }

    // MARK: - Controls

    /// Freezes the countdown, keeping the remainder in
    /// `pendingSeconds` for `resume(now:)`.
    public func pause(now: Date = Date()) {
        guard isRunning else { return }
        pendingSeconds = remainingSeconds(at: now)
        endDate = nil
        isRunning = false
    }

    /// Re-arms the countdown from the paused remainder.
    public func resume(now: Date = Date()) {
        guard isActive, !isRunning, !isComplete else { return }
        endDate = now.addingTimeInterval(TimeInterval(pendingSeconds))
        isRunning = true
    }

    /// Returns to idle. Hides the sticky pill.
    public func reset() {
        isActive = false
        endDate = nil
        isRunning = false
        isComplete = false
        blockID = nil
        blockName = ""
        totalRounds = 0
        currentRound = 1
        lastEvent = nil
    }

    /// Transitions to the complete state. Returns `true` so
    /// the caller can fire `TimerFeedback.sessionComplete()`
    /// (kept out of the store for testability — see header).
    /// Also publishes the completion event for the pill's
    /// flash + auto-expand.
    @discardableResult
    public func completeNow() -> Bool {
        guard isActive, !isComplete else { return false }
        isRunning = false
        endDate = nil
        isComplete = true
        lastEvent = .sessionComplete(id: UUID())
        return true
    }

    /// Advances an EMOM past the current round boundary.
    /// Returns `true` when a round boundary was crossed (caller
    /// fires `TimerFeedback.roundBoundary()`), `false` when
    /// there was nothing to advance. On the final round this
    /// transitions to complete instead (caller additionally
    /// fires `TimerFeedback.sessionComplete()`). Each crossing
    /// publishes the matching event for the pill's flash.
    @discardableResult
    public func advanceRound(now: Date = Date()) -> Bool {
        guard isActive, kind == .emom, isRunning, !isComplete else { return false }
        if currentRound < totalRounds {
            currentRound += 1
            pendingSeconds = intervalSeconds
            endDate = now.addingTimeInterval(TimeInterval(intervalSeconds))
            lastEvent = .roundBoundary(id: UUID())
            return true
        }
        completeNow()
        return true
    }

    // MARK: - Display

    /// Seconds remaining at `now`, clamped at zero. The frozen
    /// `pendingSeconds` while paused or idle.
    public func remainingSeconds(at now: Date = Date()) -> Int {
        guard let endDate else { return pendingSeconds }
        return max(0, Int(endDate.timeIntervalSince(now).rounded(.up)))
    }

    /// `M:SS` countdown string for the pill and sheet.
    public func displayString(at now: Date = Date()) -> String {
        Self.formatDuration(remainingSeconds(at: now))
    }

    /// Pill subtitle, e.g. "Rest · Push" or "Round 2/5 · Push".
    /// The block name falls back to "Workout" when the timer
    /// was started without a block context.
    public var subtitle: String {
        let context = blockName.isEmpty ? "Workout" : blockName
        switch kind {
        case .rest:
            return "Rest · \(context)"
        case .emom:
            return "Round \(currentRound)/\(totalRounds) · \(context)"
        case .amrap:
            return "AMRAP · \(context)"
        }
    }

    /// Callout text for an EMOM round boundary ("Round 2 —
    /// go!"). Static and pure so the pill's temporary callout
    /// is unit-testable without UI timing; the pill only ever
    /// feeds it rounds from `.roundBoundary` events, which
    /// only EMOM publishes.
    public static func boundaryCallout(round: Int) -> String {
        "Round \(round) — go!"
    }

    // MARK: - Private

    private func arm(seconds: Int, now: Date) {
        isActive = true
        isComplete = false
        lastEvent = nil
        endDate = now.addingTimeInterval(TimeInterval(seconds))
        isRunning = true
    }

    /// `M:SS` formatting shared by the pill and sheet.
    /// Duplicated (not shared) with the standalone timer views
    /// on purpose — those are untouched by the player work and
    /// this keeps the player diff self-contained.
    static func formatDuration(_ seconds: Int) -> String {
        let minutes = seconds / 60
        let rem = seconds % 60
        return String(format: "%d:%02d", minutes, rem)
    }
}
