import CoreLocation
import Foundation
import HealthKit

/// Lifecycle of one explicit Health workout recording.
///
/// `idle` before Start and after a discard; `active` while the
/// session clock runs; `paused` while paused; `ended` after a
/// clean save; `failed` carries a display-ready message (denial
/// is NOT a failure — the player skips saving silently and stays
/// usable).
public enum WorkoutHealthRecorderState: Equatable {
    case idle
    case active
    case paused
    case ended
    case failed(String)
}

/// Live numbers rendered on the player's swipeable Live page.
/// Canonical units matching `HealthMetric.formatted`
/// expectations (seconds, bpm, kcal, metres). All optional:
/// nil means "no sample yet", never zero-fill.
public struct LiveWorkoutStats: Equatable {
    public var elapsedSeconds: TimeInterval
    public var heartRateBpm: Double?
    public var activeEnergyKcal: Double?
    public var distanceMeters: Double?
    /// Trailing-window current pace, seconds per kilometre. Nil
    /// when there's too little movement to form a pace
    /// (standing still shows "—", never an infinite pace).
    public var currentPaceSecPerKm: Double?

    public init(
        elapsedSeconds: TimeInterval = 0,
        heartRateBpm: Double? = nil,
        activeEnergyKcal: Double? = nil,
        distanceMeters: Double? = nil,
        currentPaceSecPerKm: Double? = nil
    ) {
        self.elapsedSeconds = elapsedSeconds
        self.heartRateBpm = heartRateBpm
        self.activeEnergyKcal = activeEnergyKcal
        self.distanceMeters = distanceMeters
        self.currentPaceSecPerKm = currentPaceSecPerKm
    }
}

/// Interface the workout player consumes (per codebase
/// convention: consumers define what they need). The live
/// HealthKit implementation is `LiveWorkoutHealthRecorder`;
/// previews and tests inject `MockWorkoutHealthRecorder`.
@MainActor
public protocol WorkoutHealthRecording: AnyObject {
    /// Current lifecycle state.
    var state: WorkoutHealthRecorderState { get }
    /// Latest live numbers.
    var stats: LiveWorkoutStats { get }
    /// Activity type the session was started with, if any.
    var activityType: HKWorkoutActivityType? { get }

    /// Starts one explicit session. `enablesRoute` must already
    /// be gated on outdoor type + location consent — the
    /// recorder never prompts for location itself.
    func start(activityType: HKWorkoutActivityType, enablesRoute: Bool) async
    /// Pauses / resumes the running session.
    func pause()
    func resume()
    /// Ends tracking and persists one `HKWorkout` stamped with
    /// `hyleteWorkoutID` metadata. Idempotent: a second call
    /// after `.ended` is a no-op. Returns `true` on save.
    @discardableResult
    func endAndSave(hyleteWorkoutID: String) async -> Bool
    /// Abandons the session without saving (kill/Close path).
    /// Safe in any state.
    func discard()
}

// MARK: - Live implementation

/// Track-and-save HealthKit recorder, iOS 17-compatible.
///
/// Why not `HKWorkoutSession` + `HKLiveWorkoutBuilder`? On
/// iPhone those require iOS 26+ (`associatedWorkoutBuilder` is
/// `API_AVAILABLE(ios(26.0))`; the older session inits are
/// watchOS-only) while this app deploys to iOS 17. So the
/// recorder owns its clock, GPS trace (outdoor-only,
/// caller-gated), and live HR polling, then saves one
/// `HKWorkout` (+ route) via `HKHealthStore` on Finish.
///
/// Trade-off to know: without a system session the Watch never
/// enters workout mode, so live HR arrives at background
/// cadence (sparse) rather than second-by-second. Totals
/// (energy over the workout window) are still queried at save
/// time, so the saved workout is complete. When deployment
/// allows iOS 26+, swap the engine for `HKWorkoutSession` and
/// keep this protocol + the player's UX untouched.
@MainActor
public final class LiveWorkoutHealthRecorder: NSObject, WorkoutHealthRecording {
    /// Reverse-DNS metadata keys (HealthKit rejects custom keys
    /// without a prefix). The workout ID links one Hylete
    /// workout to at most one `HKWorkout` — live-forward only,
    /// so no history dedup fetch is needed.
    public static let hyleteWorkoutIDKey = "com.hyleteapp.workoutID"
    public static let hyleteAppVersionKey = "com.hyleteapp.version"

    public private(set) var state: WorkoutHealthRecorderState = .idle
    public private(set) var stats = LiveWorkoutStats()
    public private(set) var activityType: HKWorkoutActivityType?

    private let healthStore = HKHealthStore()
    private var locationManager: CLLocationManager?
    private var startDate: Date?
    private var pauseStartedAt: Date?
    private var pausedSeconds: TimeInterval = 0
    /// Completed pause intervals. Plain dates only — no
    /// HealthKit objects are built until save time, so
    /// pause/resume taps are pure state mutation and cannot
    /// throw (previously an `HKWorkoutEvent` built here took
    /// down the app on pause).
    private var pauseIntervals: [DateInterval] = []
    private var locations: [CLLocation] = []
    private var elapsedTimer: Timer?
    private var hrTimer: Timer?
    private var wantsRoute = false
    private var didFinish = false

    public override init() { super.init() }

    public func start(activityType: HKWorkoutActivityType, enablesRoute: Bool) async {
        guard state == .idle, HKHealthStore.isHealthDataAvailable() else { return }
        self.activityType = activityType
        self.wantsRoute = enablesRoute
        self.didFinish = false
        self.pauseIntervals = []
        self.locations = []
        self.pausedSeconds = 0
        self.pauseStartedAt = nil
        let now = Date()
        startDate = now
        state = .active
        startElapsedTimer()
        startHRPolling()
        if enablesRoute {
            startRoute()
        }
    }

    /// Pure state mutation by design: records when the pause
    /// began and flips state. The `HKWorkoutEvent`s are built
    /// once at save time from `pauseIntervals`, so this path
    /// performs no HealthKit calls and cannot throw.
    public func pause() {
        guard state == .active, pauseStartedAt == nil else { return }
        pauseStartedAt = Date()
        state = .paused
    }

    public func resume() {
        guard state == .paused, let pausedAt = pauseStartedAt else { return }
        let now = Date()
        pausedSeconds += now.timeIntervalSince(pausedAt)
        pauseIntervals.append(DateInterval(start: pausedAt, end: now))
        pauseStartedAt = nil
        state = .active
    }

    @discardableResult
    public func endAndSave(hyleteWorkoutID: String) async -> Bool {
        guard !didFinish else { return true }
        guard state == .active || state == .paused else { return false }
        didFinish = true
        stopTimers()
        stopRouteUpdates()
        // Close an open pause so elapsed math and events below
        // are exact.
        let end = Date()
        if let pausedAt = pauseStartedAt {
            pausedSeconds += end.timeIntervalSince(pausedAt)
            pauseIntervals.append(DateInterval(start: pausedAt, end: end))
            pauseStartedAt = nil
        }
        guard let start = startDate, let type = activityType else {
            state = .failed("Apple Health couldn't save the workout. Your sets are unaffected.")
            return false
        }
        do {
            let energy = await energyTotal(from: start, to: end)
            let distance = routeDistance()
            // One pause event per paused interval (the interval
            // IS the pause). Built here, once, at save time.
            let events = pauseIntervals.map {
                HKWorkoutEvent(type: .pause, dateInterval: $0, metadata: nil)
            }
            // HealthKit rejects a sync identifier without a
            // sync version, so both ride together (version 1 =
            // this metadata schema; bump if it ever changes).
            let metadata: [String: Any] = [
                Self.hyleteWorkoutIDKey: hyleteWorkoutID,
                HKMetadataKeySyncIdentifier: hyleteWorkoutID,
                HKMetadataKeySyncVersion: NSNumber(value: 1),
                HKMetadataKeyWorkoutBrandName: "Hylete",
                Self.hyleteAppVersionKey: Bundle.main.infoDictionary?["CFBundleShortVersionString"] as? String ?? "1",
            ]
            let workout = HKWorkout(
                activityType: type,
                start: start,
                end: end,
                workoutEvents: events.isEmpty ? nil : events,
                totalEnergyBurned: energy.map { HKQuantity(unit: .kilocalorie(), doubleValue: $0) },
                totalDistance: distance.map { HKQuantity(unit: .meter(), doubleValue: $0) },
                metadata: metadata
            )
            try await healthStore.save(workout)
            await saveRoute(for: workout)
            state = .ended
            return true
        } catch {
            state = .failed("Apple Health couldn't save the workout. Your sets are unaffected.")
            return false
        }
    }

    public func discard() {
        stopTimers()
        stopRouteUpdates()
        startDate = nil
        pauseStartedAt = nil
        pausedSeconds = 0
        pauseIntervals = []
        locations = []
        activityType = nil
        wantsRoute = false
        didFinish = false
        stats = LiveWorkoutStats()
        state = .idle
    }

    // MARK: - Elapsed clock

    private func elapsedNow() -> TimeInterval {
        guard let start = startDate else { return 0 }
        var elapsed = Date().timeIntervalSince(start) - pausedSeconds
        if let pausedAt = pauseStartedAt {
            elapsed -= Date().timeIntervalSince(pausedAt)
        }
        return max(0, elapsed)
    }

    private func startElapsedTimer() {
        stopElapsedTimer()
        elapsedTimer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { [weak self] _ in
            guard let self else { return }
            Task { @MainActor in
                self.stats.elapsedSeconds = self.elapsedNow()
                // Current pace only while moving (active): a
                // paused athlete standing still must read "—".
                if self.state == .active {
                    self.stats.currentPaceSecPerKm = Self.rollingPaceSecPerKm(
                        locations: self.locations,
                        now: Date()
                    )
                } else {
                    self.stats.currentPaceSecPerKm = nil
                }
            }
        }
    }

    /// Current pace from the trailing GPS window (default 30s):
    /// window distance ÷ window span, expressed as seconds per
    /// kilometre. Returns nil with fewer than two in-window
    /// fixes or under `minDistanceMeters` of movement, so
    /// standstills never produce an infinite pace. Pure
    /// function of its inputs for direct unit testing.
    static func rollingPaceSecPerKm(
        locations: [CLLocation],
        now: Date,
        windowSeconds: TimeInterval = 30,
        minDistanceMeters: Double = 10
    ) -> Double? {
        let from = now.addingTimeInterval(-windowSeconds)
        let windowed = locations.filter { $0.timestamp >= from && $0.timestamp <= now }
        guard windowed.count >= 2 else { return nil }
        var distance = 0.0
        for index in windowed.indices.dropFirst() {
            distance += windowed[index].distance(from: windowed[index - 1])
        }
        guard distance >= minDistanceMeters else { return nil }
        let span = windowed.last!.timestamp.timeIntervalSince(windowed.first!.timestamp)
        guard span > 0 else { return nil }
        return span / distance * 1_000
    }

    private func stopElapsedTimer() {
        elapsedTimer?.invalidate()
        elapsedTimer = nil
    }

    private func stopTimers() {
        stopElapsedTimer()
        hrTimer?.invalidate()
        hrTimer = nil
    }

    // MARK: - Live HR + energy polling (background cadence)

    private func startHRPolling() {
        hrTimer?.invalidate()
        pollHRAndEnergy()
        hrTimer = Timer.scheduledTimer(withTimeInterval: 15, repeats: true) { [weak self] _ in
            guard let self else { return }
            Task { @MainActor in self.pollHRAndEnergy() }
        }
    }

    /// Polls live numbers. Heart rate reads the latest sample
    /// *overall* (predicate nil), not just samples inside the
    /// workout window: without a system workout session the
    /// Watch never enters workout mode, so high-frequency HR
    /// only starts minutes in. Windowing the query meant the
    /// card read "—" until the first in-window sample landed
    /// even with a Watch on; latest-overall shows the user's
    /// current HR immediately and converges to workout HR as
    /// fresh samples arrive. Energy stays windowed — a
    /// day-total would misattribute the whole day to the workout.
    private func pollHRAndEnergy() {
        guard let start = startDate, state == .active || state == .paused else { return }
        let bpm = HKUnit.count().unitDivided(by: .minute())
        guard let hrType = HKQuantityType.quantityType(forIdentifier: .heartRate) else { return }
        let sort = NSSortDescriptor(key: HKSampleSortIdentifierEndDate, ascending: false)
        let query = HKSampleQuery(
            sampleType: hrType,
            predicate: nil,
            limit: 1,
            sortDescriptors: [sort]
        ) { [weak self] _, samples, _ in
            guard let self, let sample = samples?.first as? HKQuantitySample else { return }
            let value = sample.quantity.doubleValue(for: bpm)
            Task { @MainActor in self.stats.heartRateBpm = value }
        }
        healthStore.execute(query)
        Task { @MainActor in
            if let total = await self.energyTotal(from: start, to: Date()) {
                self.stats.activeEnergyKcal = total
            }
        }
    }

    /// Summed active energy over a window (nil when Health has
    /// no samples — never zero-fill the Live display).
    private func energyTotal(from start: Date, to end: Date) async -> Double? {
        guard let type = HKQuantityType.quantityType(forIdentifier: .activeEnergyBurned) else {
            return nil
        }
        let predicate = HKQuery.predicateForSamples(withStart: start, end: end, options: .strictStartDate)
        do {
            return try await withCheckedThrowingContinuation { cont in
                let query = HKStatisticsQuery(
                    quantityType: type,
                    quantitySamplePredicate: predicate,
                    options: .cumulativeSum
                ) { _, result, error in
                    if let error { cont.resume(throwing: error); return }
                    cont.resume(returning: result?.sumQuantity()?.doubleValue(for: .kilocalorie()))
                }
                self.healthStore.execute(query)
            }
        } catch {
            return nil
        }
    }

    // MARK: - Route (outdoor-only, caller-gated)

    private func startRoute() {
        let manager = CLLocationManager()
        manager.delegate = self
        manager.desiredAccuracy = kCLLocationAccuracyBest
        manager.allowsBackgroundLocationUpdates = true
        manager.pausesLocationUpdatesAutomatically = false
        manager.requestWhenInUseAuthorization()
        manager.startUpdatingLocation()
        locationManager = manager
    }

    private func stopRouteUpdates() {
        locationManager?.stopUpdatingLocation()
        locationManager = nil
    }

    private func routeDistance() -> Double? {
        guard wantsRoute, locations.count > 1 else { return nil }
        var total = 0.0
        for index in locations.indices.dropFirst() {
            total += locations[index].distance(from: locations[index - 1])
        }
        stats.distanceMeters = total
        return total > 0 ? total : nil
    }

    /// Persists the GPS trace against the saved workout. Best
    /// effort: a route failure never fails the workout save.
    private func saveRoute(for workout: HKWorkout) async {
        guard wantsRoute, locations.count > 1 else { return }
        let builder = HKWorkoutRouteBuilder(healthStore: healthStore, device: .local())
        do {
            try await builder.insertRouteData(locations)
            await withCheckedContinuation { cont in
                builder.finishRoute(with: workout, metadata: nil) { _, _ in
                    cont.resume()
                }
            }
        } catch {}
    }
}

// MARK: - CLLocationManagerDelegate (route only)

extension LiveWorkoutHealthRecorder: CLLocationManagerDelegate {
    nonisolated public func locationManager(
        _ manager: CLLocationManager,
        didUpdateLocations newLocations: [CLLocation]
    ) {
        let valid = newLocations.filter { $0.horizontalAccuracy >= 0 && $0.horizontalAccuracy <= 50 }
        guard !valid.isEmpty else { return }
        Task { @MainActor in
            guard self.wantsRoute, self.state == .active || self.state == .paused else { return }
            self.locations.append(contentsOf: valid)
            if self.locations.count > 1 {
                var total = 0.0
                for index in self.locations.indices.dropFirst() {
                    total += self.locations[index].distance(from: self.locations[index - 1])
                }
                self.stats.distanceMeters = total
            }
        }
    }
}

// MARK: - Mock

/// In-memory stub for previews and unit tests. Never touches
/// HealthKit or CoreLocation. Records intents so tests can
/// assert the player drove the session correctly.
@MainActor
public final class MockWorkoutHealthRecorder: WorkoutHealthRecording {
    public private(set) var state: WorkoutHealthRecorderState = .idle
    public var stats = LiveWorkoutStats()
    public private(set) var activityType: HKWorkoutActivityType?
    public private(set) var starts: [(type: HKWorkoutActivityType, route: Bool)] = []
    public private(set) var saves: [String] = []
    public private(set) var discards = 0
    public var saveResult = true
    /// When `true`, `start` records the intent but stays `.idle`
    /// (simulates HealthKit refusing to start, e.g. restricted
    /// device). Lets tests assert the player surfaces an error
    /// instead of silently doing nothing.
    public var refusesStart = false

    public init() {}

    public func start(activityType: HKWorkoutActivityType, enablesRoute: Bool) async {
        self.activityType = activityType
        starts.append((activityType, enablesRoute))
        if !refusesStart {
            state = .active
        }
    }

    public func pause() {
        if state == .active { state = .paused }
    }

    public func resume() {
        if state == .paused { state = .active }
    }

    @discardableResult
    public func endAndSave(hyleteWorkoutID: String) async -> Bool {
        saves.append(hyleteWorkoutID)
        if saveResult { state = .ended }
        return saveResult
    }

    public func discard() {
        discards += 1
        state = .idle
        stats = LiveWorkoutStats()
    }
}
