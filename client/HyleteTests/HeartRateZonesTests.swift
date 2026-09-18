import XCTest
@testable import Hylete

/// Tests for the estimated HR-zone math. Pure functions — no
/// HealthKit, safe on Simulator.
final class HeartRateZonesTests: XCTestCase {
    private var fixedNow: Date {
        var comps = DateComponents()
        comps.year = 2026
        comps.month = 9
        comps.day = 17
        comps.calendar = Calendar(identifier: .gregorian)
        return comps.date!
    }

    func testTanakaMax() {
        XCTAssertEqual(HeartRateZones.maxHeartRate(ageYears: 30), 187, accuracy: 0.001)
        XCTAssertEqual(HeartRateZones.maxHeartRate(ageYears: 0), 208, accuracy: 0.001)
    }

    func testAgeFromDOB() {
        // 30th birthday was 2026-03-04 → 30 whole years.
        XCTAssertEqual(
            HeartRateZones.ageYears(dateOfBirth: "1996-03-04", now: fixedNow),
            30
        )
        // Birthday tomorrow → still 29.
        XCTAssertEqual(
            HeartRateZones.ageYears(dateOfBirth: "1996-09-18", now: fixedNow),
            29
        )
    }

    func testBadDOBIsNil() {
        XCTAssertNil(HeartRateZones.ageYears(dateOfBirth: nil, now: fixedNow))
        XCTAssertNil(HeartRateZones.ageYears(dateOfBirth: "", now: fixedNow))
        XCTAssertNil(HeartRateZones.ageYears(dateOfBirth: "not-a-date", now: fixedNow))
        XCTAssertNil(HeartRateZones.maxHeartRate(dateOfBirth: nil, now: fixedNow))
    }

    func testZoneBoundaries() {
        let max: Double = 200
        XCTAssertEqual(HeartRateZones.zone(bpm: 100, maxHeartRate: max), 1)
        XCTAssertEqual(HeartRateZones.zone(bpm: 119.9, maxHeartRate: max), 1)
        XCTAssertEqual(HeartRateZones.zone(bpm: 120, maxHeartRate: max), 2)
        XCTAssertEqual(HeartRateZones.zone(bpm: 140, maxHeartRate: max), 3)
        XCTAssertEqual(HeartRateZones.zone(bpm: 160, maxHeartRate: max), 4)
        XCTAssertEqual(HeartRateZones.zone(bpm: 180, maxHeartRate: max), 5)
        XCTAssertEqual(HeartRateZones.zone(bpm: 220, maxHeartRate: max), 5)
    }

    func testZoneNilWithoutReadingOrMax() {
        XCTAssertNil(HeartRateZones.zone(bpm: nil, maxHeartRate: 190))
        XCTAssertNil(HeartRateZones.zone(bpm: 150, maxHeartRate: nil))
        XCTAssertNil(HeartRateZones.zone(bpm: 0, maxHeartRate: 190))
        XCTAssertNil(HeartRateZones.zone(bpm: -5, maxHeartRate: 190))
    }

    func testZoneNames() {
        XCTAssertEqual(HeartRateZones.name(for: 1), "Recovery")
        XCTAssertEqual(HeartRateZones.name(for: 3), "Aerobic")
        XCTAssertEqual(HeartRateZones.name(for: 5), "Maximum")
        XCTAssertEqual(HeartRateZones.name(for: 99), "Maximum")
    }
}
