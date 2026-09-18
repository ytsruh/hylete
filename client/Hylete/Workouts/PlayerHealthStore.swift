import Foundation
import HealthKit

/// Session owner for live Apple Health recording. One instance
/// lives at app scope (owned by `MainTabView`) so a recording
/// survives closing the Workout Player; players attach to it
/// per workout. Sits alongside `WorkoutPlayerStore` (which
/// owns Hylete exercise entries) so a Health denial or failure
/// can never block set logging.
///
/// Flow: idle → `attach()` claims a workout (adopts type/DOB,
/// caller auto-starts) → active ⇄ paused → `endAndSave()` from
/// Finish or the global pill, or `discard()` from the pill.
/// Closing the player ends nothing. Denial is not an error —
/// the player skips Health saving silently with an inline note
/// (`healthSkipped`).
@MainActor
public final class PlayerHealthStore: ObservableObject {
    /// Recorder state mirror for SwiftUI. Read from the
    /// recorder on every tick (see `tickTimer`).
    @Published public private(set) var recorderState: WorkoutHealthRecorderState = .idle
    /// Live numbers mirror for the swipeable Live page.
    @Published public private(set) var liveStats = LiveWorkoutStats()
    /// Activity type picked for this session (inferred default,
    /// user-overridable before Start).
    @Published public var selectedType: HKWorkoutActivityType
    /// `true` while the system permission sheet is up.
    @Published public private(set) var isRequestingAuth = false
    /// `true` while `endAndSave` is in flight (Finish button
    /// spinner coordination).
    @Published public private(set) var isSaving = false
    /// Non-blocking failure message. Hylete Finish still
    /// succeeds when this is set.
    @Published public var errorMessage: String?
    /// Set when write access was denied so the Live page can
    /// explain why nothing is recording (not an error).
    @Published public private(set) var healthSkipped = false

    private var workoutID: String
    private let recorder: WorkoutHealthRecording
    private let provider: HealthDataProvider
    private var tickTimer: Timer?

    /// `recorder` / `provider` default to the live HealthKit
    /// implementations via nil-coalescing inside the body: default
    /// arguments are evaluated in a non-isolated context, so
    /// constructing the `@MainActor` recorder there is rejected
    /// by the compiler (`#ActorIsolatedCall`).
    ///
    /// `workoutID` defaults empty: the app-scoped instance is
    /// created before any workout is known and re-targeted via
    /// `attach()` on every player open.
    public init(
        workoutID: String = "",
        items: [BlockItemDTO] = [],
        recorder: WorkoutHealthRecording? = nil,
        provider: HealthDataProvider? = nil
    ) {
        self.workoutID = workoutID
        self.recorder = recorder ?? LiveWorkoutHealthRecorder()
        self.provider = provider ?? LiveHealthStore()
        self.selectedType = WorkoutHealthActivityMapper.infer(items: items)
    }

    /// Result of a player open claiming this shared store.
    public enum HealthAttachment: Equatable {
        /// Idle store claimed for this workout (caller adopts
        /// nothing further — type/DOB adoption ran inside).
        case claimed
        /// Already recording this workout — mirror only, never
        /// re-start or clobber the running type.
        case reattached
        /// Recording a *different* workout — caller must not
        /// start, adopt, or mirror; show the busy banner.
        case busy
    }

    /// The workout this store is currently claimed by ("" when
    /// never claimed).
    public var claimedWorkoutID: String { workoutID }

    /// Claims (or reattaches to) this shared store for a player
    /// open. On `.claimed` the type/DOB adoption runs here so
    /// every opener shares one precedence path; the caller
    /// only decides whether to auto-start. A finished/failed
    /// prior session resets first; an active/paused session for
    /// another workout returns `.busy` untouched.
    public func attach(
        workoutID: String,
        items: [BlockItemDTO],
        serverKey: String?,
        dateOfBirth: String?
    ) -> HealthAttachment {
        switch recorderState {
        case .idle:
            beginClaim(workoutID: workoutID, items: items, serverKey: serverKey, dateOfBirth: dateOfBirth)
            return .claimed
        case .ended, .failed:
            discard()
            beginClaim(workoutID: workoutID, items: items, serverKey: serverKey, dateOfBirth: dateOfBirth)
            return .claimed
        case .active, .paused:
            return self.workoutID == workoutID ? .reattached : .busy
        }
    }

    /// Targets the idle store at a workout and runs the full
    /// adoption chain (server type → inference → profile DOB).
    /// Precondition: recorder is `.idle` (see `attach`).
    private func beginClaim(
        workoutID: String,
        items: [BlockItemDTO],
        serverKey: String?,
        dateOfBirth: String?
    ) {
        self.workoutID = workoutID
        serverTypeAdopted = false
        selectedType = .other
        errorMessage = nil
        healthSkipped = false
        adoptServerType(serverKey)
        adoptInferredDefault(items: items)
        adoptProfile(dateOfBirth: dateOfBirth)
    }

    /// Whether the route switch will arm GPS on Start (outdoor
    /// distance types only — indoor sessions never prompt).
    public var armsRouteOnStart: Bool {
        WorkoutHealthActivityMapper.isOutdoorRouteEligible(selectedType)
    }

    /// Whether a server type was adopted (beats inference).
    private var serverTypeAdopted = false

    /// Adopts the server-backed workout type. Precedence: server
    /// value wins over block-mix inference; nil/unknown keys leave
    /// the picker for inference. Only applies before Start.
    public func adoptServerType(_ key: String?) {
        guard recorderState == .idle else { return }
        guard let type = WorkoutHealthActivityMapper.activityType(forKey: key) else { return }
        selectedType = type
        serverTypeAdopted = true
    }

    /// Adopts the inferred default once items load. Only applies
    /// while no session has started, no server type was adopted,
    /// and the picker still holds the initial `.other`
    /// placeholder, so an explicit value is never clobbered.
    public func adoptInferredDefault(items: [BlockItemDTO]) {
        guard recorderState == .idle, !serverTypeAdopted,
              selectedType == .other, !items.isEmpty else { return }
        selectedType = WorkoutHealthActivityMapper.infer(items: items)
    }

    /// `true` once a session has been started (active, paused,
    /// or ended). Drives "Start" vs controls rendering.
    public var hasSession: Bool {
        recorderState == .active || recorderState == .paused || recorderState == .ended
    }

    /// `true` while a session is live (active or paused).
    /// Drives the global recording pill — idle, ended, and
    /// failed show nothing.
    public var isRecording: Bool {
        recorderState == .active || recorderState == .paused
    }

    /// Explicit Start tap. Requests write auth whenever it isn't
    /// granted yet — the per-type status API misreads on
    /// Simulator (granted reading as denied), so gating the
    /// sheet on `.notRequested` alone strands users with no
    /// prompt and no feedback. Requesting when truly denied is
    /// a harmless no-op; the re-check below then marks
    /// `healthSkipped` with a visible note. Every path out of
    /// here leaves visible state (active session, skip note, or
    /// error) — Start must never appear to do nothing.
    public func start() async {
        guard recorderState == .idle, !isRequestingAuth else { return }
        errorMessage = nil
        healthSkipped = false
        guard provider.isAvailable() else {
            errorMessage = "Apple Health isn't available on this device."
            return
        }
        if provider.writeAuthorizationStatus() != .granted {
            isRequestingAuth = true
            defer { isRequestingAuth = false }
            do {
                try await provider.requestWriteAuthorization()
            } catch {
                errorMessage = "Apple Health couldn't be reached. Your sets are unaffected."
                return
            }
        }
        guard provider.writeAuthorizationStatus() == .granted else {
            healthSkipped = true
            return
        }
        await recorder.start(
            activityType: selectedType,
            enablesRoute: WorkoutHealthActivityMapper.isOutdoorRouteEligible(selectedType)
        )
        refreshMirror()
        switch recorder.state {
        case .failed(let message):
            errorMessage = message
        case .active, .paused:
            startTick()
        case .idle, .ended:
            errorMessage = "Apple Health couldn't start the workout. Your sets are unaffected."
        }
    }

    /// Pauses / resumes the running session.
    public func pause() {
        recorder.pause()
        refreshMirror()
    }

    public func resume() {
        recorder.resume()
        refreshMirror()
    }

    /// Ends the session and persists one `HKWorkout` (idempotent
    /// via recorder). Called from player Finish. Returns the
    /// recorder result; Hylete Finish proceeds regardless.
    @discardableResult
    public func endAndSave() async -> Bool {
        guard recorderState == .active || recorderState == .paused else { return true }
        isSaving = true
        defer { isSaving = false }
        let saved = await recorder.endAndSave(hyleteWorkoutID: workoutID)
        refreshMirror()
        if case .failed(let message) = recorder.state {
            errorMessage = message
        }
        stopTick()
        return saved
    }

    /// Abandons without saving. Reached only from the global
    /// pill's explicit Discard (closing the player ends
    /// nothing — the recording keeps running).
    public func discard() {
        recorder.discard()
        stopTick()
        refreshMirror()
    }

    // MARK: - Mirroring

    /// Copies recorder state/stats into published slots. The
    /// recorder protocol isn't observable, so a 1s tick forwards
    /// live numbers while active (cheap, main-thread only).
    private func refreshMirror() {
        recorderState = recorder.state
        liveStats = recorder.stats
        activityTypeMirror = recorder.activityType
    }

    /// Tanaka max-HR estimate from the profile DOB. Nil when the
    /// DOB is missing/unparseable — the zone tile hides rather
    /// than guessing an age.
    @Published public private(set) var maxHeartRate: Double?

    /// Adopts the profile DOB for zone estimation. Only applies
    /// while no session has started, mirroring
    /// `adoptInferredDefault` (a mid-workout profile edit must
    /// not shift zones under the athlete).
    public func adoptProfile(dateOfBirth: String?) {
        guard recorderState == .idle else { return }
        maxHeartRate = HeartRateZones.maxHeartRate(dateOfBirth: dateOfBirth)
    }

    /// Recorder's activity type once started (nil before Start).
    @Published public private(set) var activityTypeMirror: HKWorkoutActivityType?

    private func startTick() {
        stopTick()
        tickTimer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            guard let self else { return }
            Task { @MainActor in self.refreshMirror() }
        }
    }

    private func stopTick() {
        tickTimer?.invalidate()
        tickTimer = nil
    }
}
