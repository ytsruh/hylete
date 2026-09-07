import Foundation
import HealthKit

/// Live HealthKit implementation of `HealthDataProvider`.
///
/// Read-only: requests share (read) access only, never writes.
/// All queries are scoped to "today" (midnight → now) for
/// `.dailyTotal` metrics and most-recent-sample for `.latest`
/// metrics (sleep = total asleep in the last 24h). Failures
/// are per-metric and swallowed by `fetchReadings()` so the
/// grid degrades card-by-card.
///
/// Canonical units returned match `HealthMetric.formatted`
/// expectations (kg, metres, kcal, bpm, ms, ml/kg/min,
/// percent 0–100, hours, minutes, count).
public final class LiveHealthStore: HealthDataProvider {
    private let store = HKHealthStore()

    /// VO2 max unit (mL/kg/min) built from typed constructors.
    /// Never use `HKUnit(from:)` with a UCUM string here: an
    /// unparseable string raises an uncatchable NSException out
    /// of `+[HKUnit unitFromString:]` and kills the app (this
    /// crashed `fetchReadings()` on the cardio-fitness fetch).
    static let vo2Unit = HKUnit.literUnit(with: .milli)
        .unitDivided(by: .gramUnit(with: .kilo))
        .unitDivided(by: .minute())

    public init() {}

    // MARK: - HealthDataProvider

    public func isAvailable() -> Bool {
        HKHealthStore.isHealthDataAvailable()
    }

    /// Coarse authorization state across ALL requested types.
    /// `.granted` when any single type is authorized — one denied
    /// type must never veto a working link (partial grants are
    /// normal: users toggle categories individually). `.denied`
    /// only when nothing is authorized and at least one type was
    /// decided. Advisory only: `authorizationStatus(for:)` is
    /// unreliable on Simulator (it can report denied while
    /// Settings shows full access), so callers must treat
    /// actually-returned samples as ground truth — see
    /// `HealthViewModel.refresh()`.
    public func authorizationStatus() -> HealthAuthStatus {
        guard isAvailable() else { return .unavailable }
        let types = Self.readTypes.compactMap { $0 }
        guard !types.isEmpty else { return .unavailable }
        let statuses = types.map { store.authorizationStatus(for: $0) }
        if statuses.contains(.sharingAuthorized) { return .granted }
        if statuses.allSatisfy({ $0 == .notDetermined }) { return .notRequested }
        return .denied
    }

    public func requestAuthorization() async throws {
        let types = Set(Self.readTypes.compactMap { $0 })
        try await store.requestAuthorization(toShare: [], read: types)
    }

    public func fetchReadings() async -> [HealthMetric: HealthSample] {
        guard isAvailable() else { return [:] }
        var out: [HealthMetric: HealthSample] = [:]

        // Daily totals (midnight → now).
        if let s = await dailyTotal(.stepCount, unit: .count()) {
            out[.steps] = s
        }
        if let s = await dailyTotal(.distanceWalkingRunning, unit: .meter()) {
            out[.distance] = s
        }
        if let s = await dailyTotal(.activeEnergyBurned, unit: .kilocalorie()) {
            out[.activeEnergy] = s
        }
        if let s = await dailyTotal(.basalEnergyBurned, unit: .kilocalorie()) {
            out[.basalEnergy] = s
        }
        if let s = await dailyTotal(.appleExerciseTime, unit: .minute()) {
            out[.exerciseTime] = s
        }
        if let s = await sleepHours() {
            out[.sleep] = s
        }

        // Latest samples.
        if let s = await latest(.bodyMass, unit: .gramUnit(with: .kilo)) {
            out[.weight] = s
        }
        if let s = await latest(.bodyMassIndex, unit: .count()) {
            out[.bmi] = s
        }
        if let s = await latest(.bodyFatPercentage, unit: .percent()) {
            // HealthKit reports 0–1; cards show 0–100.
            out[.bodyFatPercentage] = HealthSample(value: s.value * 100, date: s.date)
        }
        if let s = await latest(.leanBodyMass, unit: .gramUnit(with: .kilo)) {
            out[.leanBodyMass] = s
        }
        let bpm = HKUnit.count().unitDivided(by: .minute())
        if let s = await latest(.heartRate, unit: bpm) { out[.heartRate] = s }
        if let s = await latest(.restingHeartRate, unit: bpm) { out[.restingHeartRate] = s }
        if let s = await latest(.walkingHeartRateAverage, unit: bpm) { out[.walkingHeartRateAverage] = s }
        if let s = await latest(.heartRateRecoveryOneMinute, unit: bpm) { out[.cardioRecovery] = s }
        if let s = await latest(.heartRateVariabilitySDNN, unit: HKUnit.secondUnit(with: .milli)) {
            out[.heartRateVariability] = s
        }
        if let s = await latest(.vo2Max, unit: Self.vo2Unit) {
            out[.cardioFitness] = s
        }
        return out
    }

    // MARK: - Read types

    /// Every quantity/category type the grid reads. Centralised
    /// so `requestAuthorization` and the fetchers can't drift.
    private static var readTypes: [HKSampleType?] {
        let quantityIDs: [HKQuantityTypeIdentifier] = [
            .stepCount, .distanceWalkingRunning,
            .activeEnergyBurned, .basalEnergyBurned,
            .appleExerciseTime, .bodyMass, .bodyMassIndex,
            .bodyFatPercentage, .leanBodyMass, .heartRate,
            .restingHeartRate, .walkingHeartRateAverage,
            .heartRateRecoveryOneMinute, .heartRateVariabilitySDNN,
            .vo2Max,
        ]
        var types: [HKSampleType?] = quantityIDs.map {
            HKQuantityType.quantityType(forIdentifier: $0)
        }
        types.append(HKCategoryType.categoryType(forIdentifier: .sleepAnalysis))
        return types
    }

    // MARK: - Queries

    /// Cumulative sum from local midnight to now.
    private func dailyTotal(
        _ identifier: HKQuantityTypeIdentifier,
        unit: HKUnit
    ) async -> HealthSample? {
        guard let type = HKQuantityType.quantityType(forIdentifier: identifier) else {
            return nil
        }
        let start = Calendar.current.startOfDay(for: Date())
        let predicate = HKQuery.predicateForSamples(
            withStart: start, end: Date(), options: .strictStartDate
        )
        do {
            let sum: Double? = try await withCheckedThrowingContinuation { cont in
                let q = HKStatisticsQuery(
                    quantityType: type,
                    quantitySamplePredicate: predicate,
                    options: .cumulativeSum
                ) { _, stats, error in
                    if let error { cont.resume(throwing: error); return }
                    cont.resume(returning: stats?.sumQuantity()?.doubleValue(for: unit))
                }
                self.store.execute(q)
            }
            guard let sum else { return nil }
            return HealthSample(value: sum, date: Date())
        } catch {
            return nil
        }
    }

    /// Most-recent sample value.
    private func latest(
        _ identifier: HKQuantityTypeIdentifier,
        unit: HKUnit
    ) async -> HealthSample? {
        guard let type = HKQuantityType.quantityType(forIdentifier: identifier) else {
            return nil
        }
        let sort = NSSortDescriptor(key: HKSampleSortIdentifierEndDate, ascending: false)
        do {
            return try await withCheckedThrowingContinuation { cont in
                let q = HKSampleQuery(
                    sampleType: type,
                    predicate: nil,
                    limit: 1,
                    sortDescriptors: [sort]
                ) { _, samples, error in
                    if let error { cont.resume(throwing: error); return }
                    guard let s = samples?.first as? HKQuantitySample else {
                        cont.resume(returning: nil); return
                    }
                    cont.resume(returning: HealthSample(
                        value: s.quantity.doubleValue(for: unit),
                        date: s.endDate
                    ))
                }
                self.store.execute(q)
            }
        } catch {
            return nil
        }
    }

    /// Total time asleep (core + deep + REM + unspecified) over
    /// the trailing 24h, in hours. In-bed / awake samples are
    /// excluded — the card answers "how much did I sleep?".
    private func sleepHours() async -> HealthSample? {
        guard let type = HKCategoryType.categoryType(forIdentifier: .sleepAnalysis) else {
            return nil
        }
        let start = Date().addingTimeInterval(-24 * 3_600)
        let predicate = HKQuery.predicateForSamples(withStart: start, end: Date(), options: [])
        let asleep: Set<HKCategoryValueSleepAnalysis> = [
            .asleepCore, .asleepDeep, .asleepREM, .asleepUnspecified,
        ]
        do {
            return try await withCheckedThrowingContinuation { cont in
                let sort = NSSortDescriptor(key: HKSampleSortIdentifierEndDate, ascending: false)
                let q = HKSampleQuery(
                    sampleType: type,
                    predicate: predicate,
                    limit: HKObjectQueryNoLimit,
                    sortDescriptors: [sort]
                ) { _, samples, error in
                    if let error { cont.resume(throwing: error); return }
                    let samples = (samples as? [HKCategorySample]) ?? []
                    var seconds = 0.0
                    var newest: Date?
                    for s in samples {
                        guard let v = HKCategoryValueSleepAnalysis(rawValue: s.value),
                              asleep.contains(v) else { continue }
                        seconds += s.endDate.timeIntervalSince(s.startDate)
                        if newest == nil { newest = s.endDate }
                    }
                    guard seconds > 0 else { cont.resume(returning: nil); return }
                    cont.resume(returning: HealthSample(value: seconds / 3_600, date: newest))
                }
                self.store.execute(q)
            }
        } catch {
            return nil
        }
    }
}

/// Deterministic stub for previews and unit tests. Never
/// touches HealthKit.
public final class MockHealthStore: HealthDataProvider {
    private let status: HealthAuthStatus
    private let readings: [HealthMetric: HealthSample]

    public init(
        status: HealthAuthStatus = .granted,
        readings: [HealthMetric: HealthSample] = [:]
    ) {
        self.status = status
        self.readings = readings
    }

    public func isAvailable() -> Bool { status != .unavailable }
    public func authorizationStatus() -> HealthAuthStatus { status }
    public func requestAuthorization() async throws {}
    public func fetchReadings() async -> [HealthMetric: HealthSample] { readings }
}
