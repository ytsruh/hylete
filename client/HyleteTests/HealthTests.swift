import HealthKit
import XCTest
@testable import Hylete

/// Tests for the beta-gated Health vitals surface. Formatting
/// and view-model cases run against `MockHealthStore` — no
/// device needed. The VO2-unit regression test touches the
/// real `HKUnit` construction (which previously raised an
/// uncatchable NSException) but performs no HealthKit queries,
/// so it is safe to run on a simulator with no Health data.
final class HealthTests: XCTestCase {

    // MARK: - Formatting

    func testNilFormatsAsEmDash() {
        XCTAssertEqual(HealthMetric.steps.formatted(value: nil), "—")
        XCTAssertEqual(HealthMetric.weight.formatted(value: nil), "—")
    }

    func testWeightFollowsProfileUnit() {
        // Canonical provider unit is kilograms.
        XCTAssertEqual(
            HealthMetric.weight.formatted(value: 80, weightUnit: "kg"),
            "80.0 kg"
        )
        XCTAssertEqual(
            HealthMetric.weight.formatted(value: 80, weightUnit: "lb"),
            "176.4 lb"
        )
    }

    func testDistanceFollowsProfileUnit() {
        // Canonical provider unit is metres.
        XCTAssertEqual(
            HealthMetric.distance.formatted(value: 1_000, distanceUnit: "km"),
            "1.00 km"
        )
        XCTAssertEqual(
            HealthMetric.distance.formatted(value: 1_609.344, distanceUnit: "mi"),
            "1.00 mi"
        )
    }

    func testFixedUnitMetrics() {
        XCTAssertEqual(HealthMetric.heartRate.formatted(value: 72.4), "72 bpm")
        XCTAssertEqual(HealthMetric.heartRateVariability.formatted(value: 41.6), "42 ms")
        XCTAssertEqual(HealthMetric.bodyFatPercentage.formatted(value: 18.25), "18.2 %")
        XCTAssertEqual(HealthMetric.bmi.formatted(value: 24.96), "25.0")
        XCTAssertEqual(HealthMetric.activeEnergy.formatted(value: 512.7), "513 kcal")
        XCTAssertEqual(HealthMetric.sleep.formatted(value: 7.25), "7.2 h")
        XCTAssertEqual(HealthMetric.exerciseTime.formatted(value: 32.4), "32 min")
    }

    func testStepsLabel() {
        XCTAssertTrue(HealthMetric.steps.formatted(value: 8_432).hasSuffix("steps"))
    }

    // MARK: - Sample-age footnotes

    /// Latest-kind metric with a sample date → non-nil age
    /// footnote, so stale vitals read differently from fresh.
    @MainActor
    func testFootnoteForLatestMetricWithDate() async {
        let vm = HealthViewModel(provider: MockHealthStore(readings: [
            .weight: HealthSample(value: 80, date: Date().addingTimeInterval(-90 * 24 * 3_600)),
        ]))
        await vm.refresh()
        let footnote = vm.footnote(for: .weight)
        XCTAssertNotNil(footnote)
        XCTAssertFalse(footnote!.isEmpty)
    }

    /// Daily totals never get footnotes (they're "today so
    /// far"; the grid footer already shows fetch time), and a
    /// missing sample date renders nothing.
    @MainActor
    func testFootnoteNilForTotalsAndMissingDates() async {
        let vm = HealthViewModel(provider: MockHealthStore(readings: [
            .steps: HealthSample(value: 1_000, date: Date()),
            .heartRate: HealthSample(value: 70, date: nil),
        ]))
        await vm.refresh()
        XCTAssertNil(vm.footnote(for: .steps))
        XCTAssertNil(vm.footnote(for: .heartRate))
        XCTAssertNil(vm.footnote(for: .sleep)) // absent entirely
    }

    // MARK: - View model

    /// Granted + readings → loaded grid with formatted text.
    @MainActor
    func testRefreshLoadsReadings() async {
        let vm = HealthViewModel(
            provider: MockHealthStore(readings: [
                .steps: HealthSample(value: 1_000, date: Date()),
            ]),
            weightUnit: "kg",
            distanceUnit: "km"
        )
        await vm.refresh()
        XCTAssertEqual(vm.status, .loaded)
        XCTAssertTrue(vm.text(for: .steps).hasSuffix("steps"))
        XCTAssertEqual(vm.text(for: .heartRate), "—")
    }

    /// Denied + no data → denied state (Open Settings CTA).
    @MainActor
    func testDeniedWithNoDataMapsToDenied() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .denied))
        await vm.refresh()
        XCTAssertEqual(vm.status, .denied)
    }

    // MARK: - Samples are ground truth

    /// Returned samples always win over the advisory status
    /// signal: even a provider reporting `.denied` (stale or
    /// Simulator-flaky `authorizationStatus`) resolves to
    /// `.loaded` when queries actually return data. This is the
    /// regression test for the stuck-denied-screen loop.
    @MainActor
    func testReturnedDataOverridesDeniedStatus() async {
        let vm = HealthViewModel(provider: MockHealthStore(
            status: .denied,
            readings: [.steps: HealthSample(value: 1_000, date: Date())]
        ))
        await vm.refresh()
        XCTAssertEqual(vm.status, .loaded)
        XCTAssertTrue(vm.text(for: .steps).hasSuffix("steps"))
    }

    /// Granted but nothing logged in Health → loaded grid of
    /// "—" cards, never an error or denied screen.
    @MainActor
    func testGrantedWithNoDataMapsToLoaded() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .granted))
        await vm.refresh()
        XCTAssertEqual(vm.status, .loaded)
        XCTAssertEqual(vm.text(for: .steps), "—")
    }

    /// Never-requested + empty fetch → back to the connect
    /// prompt, not a dead-end denied screen.
    @MainActor
    func testNotRequestedWithNoDataMapsToNotRequested() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .notRequested))
        await vm.refresh()
        XCTAssertEqual(vm.status, .notRequested)
    }

    /// Unavailable device → unavailable state, no fetch.
    @MainActor
    func testUnavailableStaysUnavailable() async {
        let vm = HealthViewModel(provider: MockHealthStore(status: .unavailable))
        XCTAssertEqual(vm.status, .unavailable)
        await vm.refresh()
        XCTAssertEqual(vm.status, .unavailable)
    }

    // MARK: - History helpers

    private var utcCalendar: Calendar {
        var cal = Calendar(identifier: .gregorian)
        cal.timeZone = TimeZone(identifier: "UTC")!
        return cal
    }

    private func utcMidnight(_ year: Int, _ month: Int, _ day: Int) -> Date {
        utcCalendar.date(from: DateComponents(year: year, month: month, day: day))!
    }

    func testSyncDaysExcludesTodayOldestFirst() {
        let now = utcMidnight(2026, 9, 8).addingTimeInterval(12 * 3_600) // midday
        let days = syncDays(count: 3, calendar: utcCalendar, now: now)
        XCTAssertEqual(days.count, 3)
        XCTAssertEqual(days.map { snapshotDateString($0, calendar: utcCalendar) },
                       ["2026-09-05", "2026-09-06", "2026-09-07"])
        XCTAssertEqual(syncDays(count: 0, calendar: utcCalendar, now: now), [])
    }

    func testBucketLatestCarriesForward() {
        let d5 = utcMidnight(2026, 9, 5)
        let d6 = utcMidnight(2026, 9, 6)
        let d7 = utcMidnight(2026, 9, 7)
        // One weigh-in on the 5th; nothing after.
        let samples = [(date: d5.addingTimeInterval(8 * 3_600), value: 80.0)]
        let out = bucketLatest(samples: samples, days: [d5, d6, d7], calendar: utcCalendar)
        XCTAssertEqual(out[d5]?.value, 80.0)
        XCTAssertEqual(out[d6]?.value, 80.0, "carry-forward fills the gap")
        XCTAssertEqual(out[d7]?.value, 80.0)
        XCTAssertEqual(out[d7]?.measuredAt, samples[0].date, "carried values keep the original observation date")
        // Nothing before the first sample → no value.
        XCTAssertNil(bucketLatest(samples: samples, days: [utcMidnight(2026, 9, 4)], calendar: utcCalendar)[utcMidnight(2026, 9, 4)])
    }

    func testBucketLatestMidnightBoundary() {
        let d6 = utcMidnight(2026, 9, 6)
        let d7 = utcMidnight(2026, 9, 7)
        // Sample exactly at midnight belongs to the new day.
        let out = bucketLatest(samples: [(date: d7, value: 1)], days: [d6, d7], calendar: utcCalendar)
        XCTAssertNil(out[d6], "midnight sample is not the previous day's")
        XCTAssertEqual(out[d7]?.value, 1)
    }

    func testBucketSleepSplitsMidnight() {
        let d6 = utcMidnight(2026, 9, 6)
        let d7 = utcMidnight(2026, 9, 7)
        // 22:00 → 06:00 across midnight: 2h on the 6th, 6h on the 7th.
        let seg = (start: d6.addingTimeInterval(22 * 3_600), end: d7.addingTimeInterval(6 * 3_600))
        let out = bucketSleep(segments: [seg], days: [d6, d7], calendar: utcCalendar)
        XCTAssertEqual(out[d6] ?? -1, 2, accuracy: 0.001)
        XCTAssertEqual(out[d7] ?? -1, 6, accuracy: 0.001)
    }

    // MARK: - Snapshot payload

    func testPayloadMappingAndKeys() throws {
        let day = DailyHealthSnapshot(
            date: "2026-09-06",
            timeZone: "Europe/London",
            values: [.steps: 8432, .sleep: 7.5, .weight: 80.2],
            measuredAt: [.weight: Date(timeIntervalSince1970: 1_000_000)]
        )
        let payload = HealthSnapshotPayload(day: day)
        XCTAssertEqual(payload.steps, 8432)
        XCTAssertEqual(payload.sleepSeconds, 7.5 * 3_600, "sleep converts hours to seconds")
        XCTAssertEqual(payload.distanceMeters, 0, "absent metrics encode as 0")
        XCTAssertNil(payload.bmiMeasuredAt, "absent measured_at stays nil (server NULL)")

        let data = try JSONEncoder().encode(payload)
        let json = try JSONSerialization.jsonObject(with: data) as? [String: Any]
        XCTAssertNotNil(json?["snapshot_date"], "snake_case keys")
        XCTAssertNotNil(json?["hrv_ms"])
        XCTAssertNotNil(json?["weight_measured_at"])
        XCTAssertNil(json?["bmi_measured_at"], "nil optionals are omitted, not null")
    }

    // MARK: - Sync store

    /// Consent off → no HealthKit touched, no upload attempted.
    @MainActor
    func testSyncDoesNothingWhenDisabled() async {
        await withHealthSyncDefaults(disabled: true) {
            let api = MockSnapshotAPI()
            let store = HealthSyncStore(history: MockHealthStore(history: [DailyHealthSnapshot(date: "2026-09-06", timeZone: "UTC")]))
            await store.syncIfNeeded(api: api, windowDays: 5)
            XCTAssertTrue(api.uploaded.isEmpty)
            XCTAssertEqual(store.phase, .idle)
        }
    }

    /// Enabled → missing days upload in chunks and confirm.
    /// A follow-up run re-verifies the recent window (uploading
    /// its non-empty days again — upsert makes this safe) while
    /// older confirmed days stay skipped.
    @MainActor
    func testSyncUploadsChunksAndConfirms() async {
        await withHealthSyncDefaults(disabled: false) {
            let days = syncDays(count: 65)
            let history = days.map {
                DailyHealthSnapshot(date: snapshotDateString($0), timeZone: "UTC", values: [.steps: 100])
            }
            let api = MockSnapshotAPI()
            let store = HealthSyncStore(history: MockHealthStore(history: history))
            await store.syncIfNeeded(api: api, windowDays: 65)
            XCTAssertEqual(api.uploaded.count, 3, "65 days chunk into 30/30/5")
            XCTAssertEqual(api.uploaded.flatMap { $0 }.count, 65)
            XCTAssertEqual(store.phase, .idle)

            let second = MockSnapshotAPI()
            let store2 = HealthSyncStore(history: MockHealthStore(history: history))
            await store2.syncIfNeeded(api: second, windowDays: 65)
            let reuploaded = second.uploaded.flatMap { $0 }.map(\.snapshotDate)
            let recentKeys = Set(syncDays(count: 7, includingToday: true).map { snapshotDateString($0) })
            XCTAssertEqual(second.uploaded.count, 1, "re-verify window uploads in one chunk")
            XCTAssertEqual(
                Set(reuploaded), recentKeys.intersection(history.map(\.date)),
                "only the recent window re-uploads; older confirmed days stay skipped"
            )
        }
    }

    /// Today's non-empty snapshot uploads on the same day —
    /// no waiting for tomorrow.
    @MainActor
    func testSyncIncludesToday() async {
        await withHealthSyncDefaults(disabled: false) {
            let todayKey = snapshotDateString(syncDays(count: 1, includingToday: true)[0])
            let api = MockSnapshotAPI()
            let store = HealthSyncStore(history: MockHealthStore(history: [
                DailyHealthSnapshot(date: todayKey, timeZone: "UTC", values: [.heartRate: 72]),
            ]))
            await store.syncIfNeeded(api: api, windowDays: 5)
            XCTAssertEqual(api.uploaded.flatMap { $0 }.map(\.snapshotDate), [todayKey])
        }
    }

    func testSyncDaysIncludingToday() {
        let now = utcMidnight(2026, 9, 8).addingTimeInterval(12 * 3_600)
        let days = syncDays(count: 3, includingToday: true, calendar: utcCalendar, now: now)
        XCTAssertEqual(days.map { snapshotDateString($0, calendar: utcCalendar) },
                       ["2026-09-06", "2026-09-07", "2026-09-08"])
    }

    /// Upload failure → failed phase, dates stay unconfirmed
    /// for the next run. Uses yesterday (non-empty) so the day
    /// is actually attempted — an old empty day would seal
    /// without uploading and never reach the API.
    @MainActor
    func testSyncFailureLeavesDatesUnconfirmed() async {
        await withHealthSyncDefaults(disabled: false) {
            struct Boom: Error {}
            let api = MockSnapshotAPI()
            api.error = Boom()
            let yesterday = snapshotDateString(syncDays(count: 1)[0])
            let store = HealthSyncStore(history: MockHealthStore(history: [
                DailyHealthSnapshot(date: yesterday, timeZone: "UTC", values: [.steps: 100]),
            ]))
            await store.syncIfNeeded(api: api, windowDays: 5)
            guard case .failed = store.phase else {
                return XCTFail("expected .failed, got \(store.phase)")
            }
            XCTAssertFalse(HealthSyncStore.confirmedDates().contains(yesterday))
        }
    }

    // MARK: - Empty-day skip + 7-day sealing

    func testSnapshotIsEmpty() {
        XCTAssertTrue(DailyHealthSnapshot(date: "2026-09-06", timeZone: "UTC").isEmpty)
        XCTAssertTrue(
            DailyHealthSnapshot(date: "2026-09-06", timeZone: "UTC", values: [.steps: 0]).isEmpty,
            "explicit zeros are still empty"
        )
        XCTAssertFalse(
            DailyHealthSnapshot(date: "2026-09-06", timeZone: "UTC", values: [.steps: 12]).isEmpty
        )
        XCTAssertFalse(
            DailyHealthSnapshot(
                date: "2026-09-06", timeZone: "UTC",
                values: [.steps: 0], measuredAt: [.steps: Date()]
            ).isEmpty,
            "a measured zero is data, not emptiness"
        )
    }

    /// Mixed window: only non-empty days upload; recent
    /// empties stay unconfirmed (late data may still arrive),
    /// old empties seal as confirmed.
    @MainActor
    func testSyncSkipsEmptySealsOld() async {
        await withHealthSyncDefaults(disabled: false) {
            let days = syncDays(count: 10)
            let dates = days.map { snapshotDateString($0) }
            // Oldest day (10 back, empty) seals; newest non-empty
            // uploads; the middle empty recent day stays open.
            let history = [
                DailyHealthSnapshot(date: dates[0], timeZone: "UTC"),
                DailyHealthSnapshot(date: dates[4], timeZone: "UTC"),
                DailyHealthSnapshot(date: dates[9], timeZone: "UTC", values: [.steps: 500]),
            ]
            let api = MockSnapshotAPI()
            let store = HealthSyncStore(history: MockHealthStore(history: history))
            await store.syncIfNeeded(api: api, windowDays: 10)

            let uploadedDates = api.uploaded.flatMap { $0 }.map(\.snapshotDate)
            XCTAssertEqual(uploadedDates, [dates[9]], "only the non-empty day uploads")

            let confirmed = HealthSyncStore.confirmedDates()
            XCTAssertTrue(confirmed.contains(dates[0]), "10-day-old empty seals")
            XCTAssertTrue(confirmed.contains(dates[9]), "uploaded day confirms")
            XCTAssertFalse(confirmed.contains(dates[4]), "recent empty stays open for late data")
            XCTAssertEqual(store.phase, .idle)
        }
    }

    /// Every day empty → no POST at all (the server rejects
    /// empty batches) and idle phase.
    @MainActor
    func testSyncAllEmptyUploadsNothing() async {
        await withHealthSyncDefaults(disabled: false) {
            let days = syncDays(count: 3)
            let history = days.map {
                DailyHealthSnapshot(date: snapshotDateString($0), timeZone: "UTC")
            }
            let api = MockSnapshotAPI()
            let store = HealthSyncStore(history: MockHealthStore(history: history))
            await store.syncIfNeeded(api: api, windowDays: 3)
            XCTAssertTrue(api.uploaded.isEmpty, "no POST for an all-empty window")
            XCTAssertEqual(store.phase, .idle)
        }
    }

    /// Runs `body` with the sync UserDefaults keys isolated,
    /// restoring whatever was there afterwards so tests never
    /// leak consent or confirmation state into each other (or
    /// the developer's simulator).
    private func withHealthSyncDefaults(disabled: Bool, body: () async -> Void) async {
        let defaults = UserDefaults.standard
        let oldEnabled = defaults.object(forKey: HealthSyncStore.enabledKey)
        let oldConfirmed = defaults.object(forKey: HealthSyncStore.confirmedDatesKey)
        defaults.set(!disabled, forKey: HealthSyncStore.enabledKey)
        defaults.removeObject(forKey: HealthSyncStore.confirmedDatesKey)
        defer {
            if let oldEnabled { defaults.set(oldEnabled, forKey: HealthSyncStore.enabledKey) }
            else { defaults.removeObject(forKey: HealthSyncStore.enabledKey) }
            if let oldConfirmed { defaults.set(oldConfirmed, forKey: HealthSyncStore.confirmedDatesKey) }
            else { defaults.removeObject(forKey: HealthSyncStore.confirmedDatesKey) }
        }
        await body()
    }

    // MARK: - VO2 unit regression

    /// `HKUnit(from: "ml/kg/min")` raised an uncatchable
    /// NSException out of `+[HKUnit unitFromString:]` and
    /// crashed `fetchReadings()` (via the Health tab's
    /// "Try again" button). The store now builds the unit from
    /// typed constructors — merely touching it here guards the
    /// regression, since the old code crashed the test runner.
    func testVO2UnitConstructionAndFormatting() {
        let expected = HKUnit.literUnit(with: .milli)
            .unitDivided(by: .gramUnit(with: .kilo))
            .unitDivided(by: .minute())
        XCTAssertEqual(LiveHealthStore.vo2Unit, expected)
        XCTAssertEqual(HealthMetric.cardioFitness.formatted(value: 45.0), "45.0 VO₂")
    }

    // MARK: - Graceful connect failure

    /// A *thrown* `requestAuthorization` (unreachable store,
    /// restricted device, Simulator without Health) must land
    /// on `.error` with a graceful message: underlying detail
    /// kept for diagnosis, user reassured Hylete is unaffected.
    /// Denial never throws, so this path is distinct from the
    /// `.denied` / "Open Settings" state.
    @MainActor
    func testConnectFailureIsGracefulError() async {
        let vm = HealthViewModel(provider: ThrowingHealthStore())
        await vm.connect()
        guard case .error(let message) = vm.status else {
            return XCTFail("expected .error, got \(vm.status)")
        }
        XCTAssertTrue(message.contains("simulated failure"))
        XCTAssertTrue(message.contains("unaffected"))
    }

    // MARK: - Launch-time stale-read recovery

    /// Simulator can report not-determined after a grant. With
    /// the connect-completed flag set, init takes the
    /// fetch-first path (.loading) and a refresh resolves to
    /// the grid from returned samples — never a bogus connect
    /// prompt.
    @MainActor
    func testStaleStatusRecoversViaFetch() async {
        await withConnectFlag(true) {
            let vm = HealthViewModel(provider: MockHealthStore(
                status: .notRequested,
                readings: [.steps: HealthSample(value: 500, date: Date())]
            ))
            XCTAssertEqual(vm.status, .loading)
            await vm.refresh()
            XCTAssertEqual(vm.status, .loaded)
        }
    }

    /// Successful connect records the flag for future launches.
    @MainActor
    func testConnectSetsCompletedFlag() async {
        await withConnectFlag(false) {
            let vm = HealthViewModel(provider: MockHealthStore(status: .granted))
            await vm.connect()
            XCTAssertTrue(HealthViewModel.hasCompletedConnect)
            XCTAssertEqual(vm.status, .loaded)
        }
    }

    /// Never-asked users keep the immediate connect prompt (no
    /// wasted fetch, no spinner flash).
    @MainActor
    func testNeverAskedKeepsConnectPrompt() async {
        await withConnectFlag(false) {
            let vm = HealthViewModel(provider: MockHealthStore(status: .notRequested))
            XCTAssertEqual(vm.status, .notRequested)
        }
    }

    /// Revoked-after-connect: flag set, union denied, fetch
    /// empty → access-off screen after one refresh.
    @MainActor
    func testRevokedAfterConnectShowsDenied() async {
        await withConnectFlag(true) {
            let vm = HealthViewModel(provider: MockHealthStore(status: .denied))
            XCTAssertEqual(vm.status, .loading)
            await vm.refresh()
            XCTAssertEqual(vm.status, .denied)
        }
    }

    /// Runs `body` with the connect-completed flag isolated,
    /// restoring whatever was there afterwards.
    private func withConnectFlag(_ value: Bool, body: () async -> Void) async {
        let defaults = UserDefaults.standard
        let key = HealthViewModel.connectCompletedKey
        let old = defaults.object(forKey: key)
        defaults.set(value, forKey: key)
        defer {
            if let old { defaults.set(old, forKey: key) }
            else { defaults.removeObject(forKey: key) }
        }
        await body()
    }
}

/// Stub whose authorization always throws, simulating a
/// Simulator or restricted device where the Health store is
/// unreachable. Never touches HealthKit.
private struct ThrowingHealthStore: HealthDataProvider {
    func isAvailable() -> Bool { true }
    func authorizationStatus() -> HealthAuthStatus { .notRequested }
    func requestAuthorization() async throws {
        throw NSError(
            domain: "HealthTests",
            code: 1,
            userInfo: [NSLocalizedDescriptionKey: "simulated failure"]
        )
    }
    func fetchReadings() async -> [HealthMetric: HealthSample] { [:] }
}
