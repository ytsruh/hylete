import Foundation

// MARK: - DTOs that match the Go server's /api/v1 JSON shapes.
//
// Every struct here is a direct mirror of a DTO in
// `internal/routes/api_dto.go`. The `CodingKeys` overrides
// map Swift's camelCase to the server's snake_case so the
// network layer doesn't have to do any field renaming.
//
// Dates are decoded with the same RFC 3339 (with fractional
// seconds) format `time.Time` produces on the server, via
// `APIClient.jsonDecoder`.

// MARK: Auth

public struct LoginRequest: Encodable {
    public let email: String
    public let password: String

    public init(email: String, password: String) {
        self.email = email
        self.password = password
    }
}

public struct RegisterRequest: Encodable {
    public let name: String
    public let email: String
    public let password: String

    public init(name: String, email: String, password: String) {
        self.name = name
        self.email = email
        self.password = password
    }
}

public struct PasswordResetRequest: Encodable {
    public let email: String

    public init(email: String) {
        self.email = email
    }
}

public struct PasswordResetResponse: Decodable {
    public let message: String
}

public struct AuthResponse: Decodable {
    public let token: String
    public let user: UserDTO
}

// MARK: User

public struct UserDTO: Codable, Equatable, Identifiable {
    public let id: String
    public let name: String
    public let email: String
    public let isAdmin: Bool
    public let weightUnit: String
    public let distanceUnit: String
    public let targetWeight: Double?

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case email
        case isAdmin = "is_admin"
        case weightUnit = "weight_unit"
        case distanceUnit = "distance_unit"
        case targetWeight = "target_weight"
    }

    public init(
        id: String,
        name: String,
        email: String,
        isAdmin: Bool,
        weightUnit: String,
        distanceUnit: String,
        targetWeight: Double?
    ) {
        self.id = id
        self.name = name
        self.email = email
        self.isAdmin = isAdmin
        self.weightUnit = weightUnit
        self.distanceUnit = distanceUnit
        self.targetWeight = targetWeight
    }
}

// MARK: Profile

/// JSON body for `PUT /api/v1/me`. Mirrors the user-editable
/// subset of the server's `UpdateMeRequest` (name, target
/// weight, weight unit, distance unit). Reminder preferences
/// are deliberately omitted — they live on the dedicated
/// `GET`/`PUT /api/v1/me/reminders` endpoints (see
/// `ReminderPreferencesDTO` below) so a `PUT /me` that doesn't
/// know about reminders can never clobber them.
///
/// `targetWeight` is an optional pointer so an explicit `nil`
/// clears the goal (matching the HTML form's empty-input
/// behavior). The iOS edit form binds the field to a `String`
/// and converts to a `Double?` so the user can leave it blank.
public struct UpdateMeRequest: Encodable, Equatable {
    public let name: String
    public let targetWeight: Double?
    public let weightUnit: String
    public let distanceUnit: String

    enum CodingKeys: String, CodingKey {
        case name
        case targetWeight = "target_weight"
        case weightUnit = "weight_unit"
        case distanceUnit = "distance_unit"
    }

    public init(
        name: String,
        targetWeight: Double?,
        weightUnit: String,
        distanceUnit: String
    ) {
        self.name = name
        self.targetWeight = targetWeight
        self.weightUnit = weightUnit
        self.distanceUnit = distanceUnit
    }
}

// MARK: Weight reminders

/// JSON shape for the user's weight-reminder schedule. Mirrors the
/// server's `ReminderPreferencesDTO` in
/// `internal/routes/api_dto.go` (served by `GET
/// /api/v1/me/reminders`, accepted by `PUT /api/v1/me/reminders`).
///
/// `frequency` is one of `"off" | "daily" | "weekly" | "biweekly"`;
/// `dayOfWeek` is 0–6 (Sunday=0) for weekly/biweekly and nil
/// otherwise (the server omits the key via `omitempty`, so
/// `decodeIfPresent` maps a missing key to nil rather than failing
/// the decode); `time` is `"HH:00"` in 24h UTC (hour-only by
/// design — the web form and the iOS editor both pick whole hours).
///
/// Reminders are email-only: `enabled` IS the opt-in, there is no
/// separate channel flag.
public struct ReminderPreferencesDTO: Codable, Equatable {
    public let enabled: Bool
    public let frequency: String
    public let dayOfWeek: Int?
    public let time: String

    enum CodingKeys: String, CodingKey {
        case enabled
        case frequency
        case dayOfWeek = "day_of_week"
        case time
    }

    public init(enabled: Bool, frequency: String, dayOfWeek: Int?, time: String) {
        self.enabled = enabled
        self.frequency = frequency
        self.dayOfWeek = dayOfWeek
        self.time = time
    }

    /// `true` when the schedule uses the day-of-week field
    /// (weekly/biweekly). The editor shows the day picker only then,
    /// mirroring the server's `ReminderFrequency.NeedsDayOfWeek` and
    /// the web form's show/hide script.
    public var needsDayOfWeek: Bool {
        frequency == "weekly" || frequency == "biweekly"
    }

    /// Short human summary for the Profile row, e.g. "Off",
    /// "Daily 09:00 UTC", "Weekly Sun 09:00 UTC". Falls back to the
    /// raw frequency string for unknown values so the row never
    /// renders blank.
    public var summary: String {
        guard enabled, frequency != "off" else { return "Off" }
        var parts: [String] = [frequency.capitalized]
        if needsDayOfWeek, let day = dayOfWeek, day >= 0, day < 7 {
            parts.append(Self.weekdayLabels[day])
        }
        parts.append("\(time) UTC")
        return parts.joined(separator: " ")
    }

    /// 0–6 (Sunday=0) short labels, matching the web form's
    /// `reminderDayLabels` order.
    public static let weekdayLabels = ["Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"]

    /// Hour of day (0–23) parsed from `time`, or 9 when malformed.
    /// Mirrors the server's `ReminderHour` fallback so a corrupt row
    /// never breaks the editor.
    public var hour: Int {
        let parts = time.split(separator: ":")
        guard parts.count == 2, let h = Int(parts[0]), (0...23).contains(h), parts[1] == "00" else { return 9 }
        return h
    }
}

/// JSON body for `PUT /api/v1/me/reminders`. Same shape as
/// `ReminderPreferencesDTO`; kept as a separate type so the request
/// stays `Encodable`-only and future response-only fields (e.g. a
/// computed next-fire timestamp) don't leak into what the editor
/// sends.
public struct UpdateReminderPreferencesRequest: Encodable, Equatable {
    public let enabled: Bool
    public let frequency: String
    public let dayOfWeek: Int?
    public let time: String

    enum CodingKeys: String, CodingKey {
        case enabled
        case frequency
        case dayOfWeek = "day_of_week"
        case time
    }

    public init(enabled: Bool, frequency: String, dayOfWeek: Int?, time: String) {
        self.enabled = enabled
        self.frequency = frequency
        self.dayOfWeek = dayOfWeek
        self.time = time
    }
}

// MARK: Exercises

/// Mirrors the server's `ExerciseDTO` in
/// `internal/routes/api_dto.go`. The server returns an
/// `image_url` field that is the fully-qualified public URL
/// (resolved via `utils.PublicURLFor(img_url)`) and is what
/// the iOS side should use for `AsyncImage`. `imgURL` is
/// kept around for backwards-compatibility (older server
/// builds will not populate `imageURL`, in which case the
/// views should fall back to the placeholder icon).
public struct ExerciseDTO: Codable, Equatable, Identifiable, Hashable {
    public let id: String
    public let name: String
    /// Comma-separated alternate names. Never displayed; only used for
    /// client-side search filtering (mirrors the web `data-aliases` attr).
    /// Defaults to "" when the key is absent (older server builds).
    public let aliases: String
    public let description: String
    public let videoURL: String
    public let imgURL: String
    public let imageURL: String
    /// Higher-resolution variant (2400x800) for the full-screen
    /// image viewer. Optional: older server builds omit the key
    /// entirely, and exercises without an original omit it via
    /// `omitempty`.
    public let imageURLOriginal: String?
    public let type: String

    enum CodingKeys: String, CodingKey {
        case id
        case name
        case aliases
        case description
        case videoURL = "video_url"
        case imgURL = "img_url"
        case imageURL = "image_url"
        case imageURLOriginal = "image_url_original"
        case type
    }

    public init(
        id: String,
        name: String,
        aliases: String = "",
        description: String,
        videoURL: String,
        imgURL: String,
        imageURL: String,
        imageURLOriginal: String? = nil,
        type: String
    ) {
        self.id = id
        self.name = name
        self.aliases = aliases
        self.description = description
        self.videoURL = videoURL
        self.imgURL = imgURL
        self.imageURL = imageURL
        self.imageURLOriginal = imageURLOriginal
        self.type = type
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        id = try container.decode(String.self, forKey: .id)
        name = try container.decode(String.self, forKey: .name)
        aliases = try container.decodeIfPresent(String.self, forKey: .aliases) ?? ""
        description = try container.decode(String.self, forKey: .description)
        videoURL = try container.decode(String.self, forKey: .videoURL)
        imgURL = try container.decode(String.self, forKey: .imgURL)
        imageURL = try container.decode(String.self, forKey: .imageURL)
        imageURLOriginal = try container.decodeIfPresent(String.self, forKey: .imageURLOriginal)
        type = try container.decode(String.self, forKey: .type)
    }

    /// `true` when the exercise has a renderable image. The
    /// server returns empty strings for missing media, so
    /// reading emptiness on either field is equivalent;
    /// `imageURL` is the field the views actually use.
    public var hasImage: Bool { !imageURL.isEmpty }

    /// The URL the full-screen image viewer should load: the
    /// higher-resolution original when the server provides one
    /// (sharper pinch-zoom), otherwise the display image.
    public var viewerImageURL: String {
        if let original = imageURLOriginal, !original.isEmpty {
            return original
        }
        return imageURL
    }

    /// `true` when the exercise has a video link to open.
    public var hasVideo: Bool { !videoURL.isEmpty }

    /// Pretty display name for the `type` string. Matches the
    /// web's `capitalize` badge styling (`strength` →
    /// `Strength`, `cardio` → `Cardio`, `other` → `Other`).
    /// Unknown values fall back to a capitalized version of
    /// the raw string so we never show a lowercase badge.
    public var typeDisplayName: String {
        switch type.lowercased() {
        case "strength": return "Strength"
        case "cardio":   return "Cardio"
        case "other":    return "Other"
        default:         return type.capitalized
        }
    }
}

private extension String {
    /// Locale-aware first-letter-uppercase. Avoids pulling in
    /// `Foundation.NSString.capitalizedString` differences and
    /// matches the web's `capitalize` CSS for ASCII.
    var capitalized: String {
        guard let first = first else { return self }
        return first.uppercased() + dropFirst()
    }
}

// MARK: Exercise entries (sets)

/// Mirrors the server's `ExerciseEntryDTO`. The metric pair that
/// carries meaning depends on `exerciseType`: strength entries use
/// reps/weight/restTime, cardio entries use durationSeconds/
/// distanceMeters (+ optional avgHeartRate/caloriesBurned). The
/// server always sends every field with 0 for the non-applicable
/// pair, and `paceSecPerKm`/`isCardio` derive the rest — callers
/// should branch on `isCardio` rather than probing for zeros.
public struct ExerciseEntryDTO: Codable, Equatable, Identifiable, Hashable {
    public let id: String
    public let exerciseID: String
    public let exerciseName: String
    /// `"strength" | "cardio" | "other"`. Optional so a cached entry
    /// from an older app build still decodes; treat nil as strength.
    public let exerciseType: String?
    public let reps: Int
    public let weight: Double
    public let notes: String
    public let restTime: Int
    public let durationSeconds: Int
    /// Stored in metres (canonical unit); convert for display only.
    public let distanceMeters: Double
    public let avgHeartRate: Int
    public let caloriesBurned: Double
    public let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case exerciseID = "exercise_id"
        case exerciseName = "exercise_name"
        case exerciseType = "exercise_type"
        case reps
        case weight
        case notes
        case restTime = "rest_time"
        case durationSeconds = "duration_seconds"
        case distanceMeters = "distance_meters"
        case avgHeartRate = "avg_heart_rate"
        case caloriesBurned = "calories_burned"
        case createdAt = "created_at"
    }

    public init(
        id: String,
        exerciseID: String,
        exerciseName: String,
        exerciseType: String? = nil,
        reps: Int,
        weight: Double,
        notes: String,
        restTime: Int,
        durationSeconds: Int = 0,
        distanceMeters: Double = 0,
        avgHeartRate: Int = 0,
        caloriesBurned: Double = 0,
        createdAt: Date
    ) {
        self.id = id
        self.exerciseID = exerciseID
        self.exerciseName = exerciseName
        self.exerciseType = exerciseType
        self.reps = reps
        self.weight = weight
        self.notes = notes
        self.restTime = restTime
        self.durationSeconds = durationSeconds
        self.distanceMeters = distanceMeters
        self.avgHeartRate = avgHeartRate
        self.caloriesBurned = caloriesBurned
        self.createdAt = createdAt
    }

    /// `true` when this entry belongs to a cardio exercise. Nil type
    /// (legacy cache) renders as strength.
    public var isCardio: Bool { exerciseType?.lowercased() == "cardio" }

    /// Derived pace in seconds per kilometre, or nil when either
    /// duration or distance is missing — there is no meaningful pace
    /// without both.
    public var paceSecPerKm: Double? {
        guard durationSeconds > 0, distanceMeters > 0 else { return nil }
        return Double(durationSeconds) / (distanceMeters / 1000.0)
    }
}

public struct CreateSetInput: Encodable, Equatable, Hashable {
    public let reps: Int
    public let weight: Double
    public let restTime: Int
    public var durationSeconds: Int
    public var distanceMeters: Double
    public var avgHeartRate: Int
    public var caloriesBurned: Double

    enum CodingKeys: String, CodingKey {
        case reps
        case weight
        case restTime = "rest_time"
        case durationSeconds = "duration_seconds"
        case distanceMeters = "distance_meters"
        case avgHeartRate = "avg_heart_rate"
        case caloriesBurned = "calories_burned"
    }

    public init(
        reps: Int,
        weight: Double,
        restTime: Int,
        durationSeconds: Int = 0,
        distanceMeters: Double = 0,
        avgHeartRate: Int = 0,
        caloriesBurned: Double = 0
    ) {
        self.reps = reps
        self.weight = weight
        self.restTime = restTime
        self.durationSeconds = durationSeconds
        self.distanceMeters = distanceMeters
        self.avgHeartRate = avgHeartRate
        self.caloriesBurned = caloriesBurned
    }
}

public struct CreateExerciseEntriesRequest: Encodable {
    public let exerciseID: String
    public let notes: String
    public let createdAt: Date?
    public let sets: [CreateSetInput]

    enum CodingKeys: String, CodingKey {
        case exerciseID = "exercise_id"
        case notes
        case createdAt = "created_at"
        case sets
    }

    public init(exerciseID: String, notes: String, createdAt: Date?, sets: [CreateSetInput]) {
        self.exerciseID = exerciseID
        self.notes = notes
        self.createdAt = createdAt
        self.sets = sets
    }
}

public struct UpdateExerciseEntryRequest: Encodable {
    public let exerciseID: String
    public let notes: String
    public let reps: Int
    public let weight: Double
    public let restTime: Int
    public var durationSeconds: Int
    public var distanceMeters: Double
    public var avgHeartRate: Int
    public var caloriesBurned: Double
    public let createdAt: Date?

    enum CodingKeys: String, CodingKey {
        case exerciseID = "exercise_id"
        case notes
        case reps
        case weight
        case restTime = "rest_time"
        case durationSeconds = "duration_seconds"
        case distanceMeters = "distance_meters"
        case avgHeartRate = "avg_heart_rate"
        case caloriesBurned = "calories_burned"
        case createdAt = "created_at"
    }

    public init(
        exerciseID: String,
        notes: String,
        reps: Int,
        weight: Double,
        restTime: Int,
        durationSeconds: Int = 0,
        distanceMeters: Double = 0,
        avgHeartRate: Int = 0,
        caloriesBurned: Double = 0,
        createdAt: Date?
    ) {
        self.exerciseID = exerciseID
        self.notes = notes
        self.reps = reps
        self.weight = weight
        self.restTime = restTime
        self.durationSeconds = durationSeconds
        self.distanceMeters = distanceMeters
        self.avgHeartRate = avgHeartRate
        self.caloriesBurned = caloriesBurned
        self.createdAt = createdAt
    }
}

// MARK: History & chart

public struct HistoryStatsDTO: Codable, Equatable {
    public let maxWeight: Double
    /// Fastest pace ever recorded for the exercise, in seconds per
    /// kilometre. 0 when no entry has both a duration and a distance.
    public let bestPaceSecPerKm: Double
    public let longestDistanceMeters: Double
    public let lastSet: ExerciseEntryDTO?

    enum CodingKeys: String, CodingKey {
        case maxWeight = "max_weight"
        case bestPaceSecPerKm = "best_pace_sec_per_km"
        case longestDistanceMeters = "longest_distance_meters"
        case lastSet = "last_set"
    }
}

public struct HistoryPageDTO: Codable, Equatable {
    /// The exercise's type ("strength" | "cardio" | "other") so the
    /// history view can pick which stat cards and chart to render.
    public let exerciseType: String?
    public let entries: [ExerciseEntryDTO]
    public let stats: HistoryStatsDTO
    public let page: Int
    public let hasPrev: Bool
    public let hasNext: Bool

    enum CodingKeys: String, CodingKey {
        case exerciseType = "exercise_type"
        case entries
        case stats
        case page
        case hasPrev = "has_prev"
        case hasNext = "has_next"
    }

    /// Convenience mirror of the server's type semantics: nil (legacy
    /// cache) behaves as strength.
    public var isCardio: Bool { exerciseType?.lowercased() == "cardio" }
}

// MARK: Goals

/// JSON shape for a single goal returned by
/// `/api/v1/goals/*`. Mirrors the server's `GoalDTO` in
/// `internal/routes/api_dto.go`. All four date fields are
/// optional `Date`s (decoded from the server's RFC 3339
/// timestamps) so a missing date is `nil` rather than
/// `1970-01-01`, which would break the UI's conditional
/// rendering of the date chips.
///
/// `completedAt` is the source of truth for whether the
/// goal is done — use `isCompleted` rather than checking
/// `completedAt != nil` directly at call sites.
public struct GoalDTO: Codable, Equatable, Identifiable, Hashable {
    public let id: String
    public let title: String
    public let description: String
    public let startDate: Date?
    public let targetDate: Date?
    public let endDate: Date?
    public let completedAt: Date?
    public let createdAt: Date
    public let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case title
        case description
        case startDate = "start_date"
        case targetDate = "target_date"
        case endDate = "end_date"
        case completedAt = "completed_at"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }

    public init(
        id: String,
        title: String,
        description: String,
        startDate: Date?,
        targetDate: Date?,
        endDate: Date?,
        completedAt: Date?,
        createdAt: Date,
        updatedAt: Date
    ) {
        self.id = id
        self.title = title
        self.description = description
        self.startDate = startDate
        self.targetDate = targetDate
        self.endDate = endDate
        self.completedAt = completedAt
        self.createdAt = createdAt
        self.updatedAt = updatedAt
    }

    /// Convenience flag — equivalent to `completedAt != nil`
    /// but reads better at call sites.
    public var isCompleted: Bool { completedAt != nil }
}

/// Response body for `GET /api/v1/goals`. Wrapping the slice
/// in a named struct (rather than returning `[GoalDTO]`
/// directly) lets the server add fields like pagination
/// metadata without breaking the iOS contract.
public struct GoalsResponse: Decodable, Equatable {
    public let goals: [GoalDTO]
}

/// JSON body for `POST /api/v1/goals`. Mirrors the server's
/// `CreateGoalRequest`. Title is required; description and
/// all three dates are optional. The server enforces the
/// same length limits as the HTML form (title 1–200,
/// description ≤2000).
public struct CreateGoalRequest: Encodable, Equatable {
    public let title: String
    public let description: String
    public let startDate: Date?
    public let targetDate: Date?
    public let endDate: Date?

    enum CodingKeys: String, CodingKey {
        case title
        case description
        case startDate = "start_date"
        case targetDate = "target_date"
        case endDate = "end_date"
    }

    public init(
        title: String,
        description: String,
        startDate: Date?,
        targetDate: Date?,
        endDate: Date?
    ) {
        self.title = title
        self.description = description
        self.startDate = startDate
        self.targetDate = targetDate
        self.endDate = endDate
    }
}

/// JSON body for `PUT /api/v1/goals/:id`. Mirrors
/// `CreateGoalRequest` — same fields, same validation.
/// `completedAt` is intentionally NOT editable here; status
/// changes go through `markGoalComplete(id:)` /
/// `reopenGoal(id:)` so the server owns the completion
/// timestamp.
public struct UpdateGoalRequest: Encodable, Equatable {
    public let title: String
    public let description: String
    public let startDate: Date?
    public let targetDate: Date?
    public let endDate: Date?

    enum CodingKeys: String, CodingKey {
        case title
        case description
        case startDate = "start_date"
        case targetDate = "target_date"
        case endDate = "end_date"
    }

    public init(
        title: String,
        description: String,
        startDate: Date?,
        targetDate: Date?,
        endDate: Date?
    ) {
        self.title = title
        self.description = description
        self.startDate = startDate
        self.targetDate = targetDate
        self.endDate = endDate
    }
}

// MARK: Feedback

/// JSON body for `POST /api/v1/feedback`. Mirrors the
/// web app's `/feedback` form: a short `title` (5–100 chars)
/// and a longer `message` (10–1000 chars). Both are trimmed
/// and validated by `FeedbackController.Submit` on the
/// server; the iOS form mirrors those rules locally so the
/// Save button is disabled until the body is valid. The
/// `user_id` is taken from the JWT — this DTO deliberately
/// has no field for it.
public struct SubmitFeedbackRequest: Encodable, Equatable {
    public let title: String
    public let message: String

    public init(title: String, message: String) {
        self.title = title
        self.message = message
    }
}

// MARK: Weight

/// JSON shape for a single body-weight entry on the
/// `/api/v1/weight/*` namespace. Mirrors the server's
/// `WeightEntryDTO` in `internal/routes/api_dto.go`. The
/// server resolves the raw R2 storage key into a fully
/// qualified `photoURL` so the iOS view can hand it
/// straight to `AsyncImage` without any extra config.
///
/// `hasPhoto` is the single source of truth for "is there
/// a renderable image?" — checking both `photoKey` and
/// `photoURL` for emptiness is unnecessary because the
/// server omits both fields when there is no photo
/// (the `omitempty` JSON tag). The custom decoder below
/// uses `decodeIfPresent` for those two fields so a
/// missing key is mapped to an empty string rather than
/// failing the decode.
///
/// `photoKey` is included so the iOS editor can re-submit
/// the existing key on update (the server's PUT handler
/// expects to be told the existing key explicitly when
/// replacing a photo). The iOS view is free to ignore it
/// for read-only display.
public struct WeightEntryDTO: Codable, Equatable, Identifiable, Hashable {
    public let id: String
    public let weight: Double
    public let notes: String
    public let photoKey: String
    public let photoURL: String
    public let hasPhoto: Bool
    public let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case weight
        case notes
        case photoKey = "photo_key"
        case photoURL = "photo_url"
        case hasPhoto = "has_photo"
        case createdAt = "created_at"
    }

    public init(
        id: String,
        weight: Double,
        notes: String,
        photoKey: String,
        photoURL: String,
        hasPhoto: Bool,
        createdAt: Date
    ) {
        self.id = id
        self.weight = weight
        self.notes = notes
        self.photoKey = photoKey
        self.photoURL = photoURL
        self.hasPhoto = hasPhoto
        self.createdAt = createdAt
    }

    /// Custom decoder. The server uses `omitempty` on
    /// `photo_key` and `photo_url`, so a photo-less
    /// entry omits both fields entirely from the JSON
    /// response. `decodeIfPresent` maps that to an
    /// empty string here so the DTO's non-optional
    /// invariant holds (`hasPhoto` is the single source
    /// of truth for "is there a photo?").
    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        self.id = try container.decode(String.self, forKey: .id)
        self.weight = try container.decode(Double.self, forKey: .weight)
        self.notes = try container.decode(String.self, forKey: .notes)
        self.photoKey = try container.decodeIfPresent(String.self, forKey: .photoKey) ?? ""
        self.photoURL = try container.decodeIfPresent(String.self, forKey: .photoURL) ?? ""
        self.hasPhoto = try container.decode(Bool.self, forKey: .hasPhoto)
        self.createdAt = try container.decode(Date.self, forKey: .createdAt)
    }

    /// Pretty-printed weight in the user's preferred unit.
    /// The unit is the caller's responsibility — the DTO
    /// carries the raw number so the iOS view can re-label
    /// it whenever the user flips their unit preference in
    /// Profile.
    public func formattedWeight(in unit: String) -> String {
        String(format: "%.1f %@", weight, unit)
    }
}

/// Response body for `GET /api/v1/weight`. Wrapping the
/// slice in a named struct (rather than returning
/// `[WeightEntryDTO]` directly) lets the server add fields
/// like pagination or summary stats without breaking the
/// iOS contract.
public struct WeightEntriesResponse: Decodable, Equatable {
    public let entries: [WeightEntryDTO]
}

/// Response body for `GET /api/v1/weight/compare`. The server
/// validates ownership and photo availability, then returns the
/// pair in chronological order so the client can render `before`
/// and `after` without duplicating that business rule.
public struct WeightCompareResponse: Decodable, Equatable {
    public let before: WeightEntryDTO
    public let after: WeightEntryDTO
    public let deltaText: String

    enum CodingKeys: String, CodingKey {
        case before
        case after
        case deltaText = "delta_text"
    }
}

/// JSON body for `POST /api/v1/weight`. Mirrors the server's
/// `CreateWeightEntryRequest`. `weight` is the only required
/// field; `notes` and `photoKey` are optional, and `createdAt`
/// is optional (defaults to time.Now() on the server when
/// omitted) so the iOS "log it now" button can send an empty
/// body field.
///
/// `createdAt` is a `Date?` rather than a `Date` so the JSON
/// encoder can omit the field entirely when the user wants
/// the server default. The server-side limit mirrors the
/// HTML form: weight 0–1000, notes ≤ 1000.
public struct CreateWeightEntryRequest: Encodable, Equatable {
    public let weight: Double
    public let notes: String
    public let photoKey: String
    public let createdAt: Date?

    enum CodingKeys: String, CodingKey {
        case weight
        case notes
        case photoKey = "photo_key"
        case createdAt = "created_at"
    }

    public init(
        weight: Double,
        notes: String,
        photoKey: String,
        createdAt: Date?
    ) {
        self.weight = weight
        self.notes = notes
        self.photoKey = photoKey
        self.createdAt = createdAt
    }
}

/// JSON body for `PUT /api/v1/weight/:id`. Mirrors the
/// server's `UpdateWeightEntryRequest`. The photo-handling
/// precedence matches the HTML form:
///
///   - `removePhoto = true` clears the photo (the server
///     ignores `photoKey` in this case)
///   - non-empty `photoKey` replaces the existing key
///   - otherwise the existing key is preserved
///
/// `createdAt` is optional and preserves the existing
/// timestamp when omitted so the "edit a recent entry"
/// flow doesn't accidentally reset the entry to time.Now().
public struct UpdateWeightEntryRequest: Encodable, Equatable {
    public let weight: Double
    public let notes: String
    public let photoKey: String
    public let removePhoto: Bool
    public let createdAt: Date?

    enum CodingKeys: String, CodingKey {
        case weight
        case notes
        case photoKey = "photo_key"
        case removePhoto = "remove_photo"
        case createdAt = "created_at"
    }

    public init(
        weight: Double,
        notes: String,
        photoKey: String,
        removePhoto: Bool,
        createdAt: Date?
    ) {
        self.weight = weight
        self.notes = notes
        self.photoKey = photoKey
        self.removePhoto = removePhoto
        self.createdAt = createdAt
    }
}

/// JSON body for `POST /api/v1/weight/photo-upload`. The
/// iOS client POSTs the user's filename + preferred content
/// type, receives a presigned R2 PUT URL and a server-side
/// storage key, then PUTs the file bytes directly to R2.
public struct WeightPhotoUploadRequest: Encodable, Equatable {
    public let filename: String
    public let contentType: String

    public init(filename: String, contentType: String) {
        self.filename = filename
        self.contentType = contentType
    }
}

/// JSON body returned by `POST /api/v1/weight/photo-upload`.
/// `url` is a presigned R2 PUT URL (valid for one hour) and
/// `key` is the server-side storage key the iOS view submits
/// back to the create or update form.
public struct WeightPhotoUploadResponse: Decodable, Equatable {
    public let url: String
    public let key: String
}

// MARK: Health snapshots

/// One persisted day of Apple Health vitals, mirroring the
/// server's `HealthSnapshotDTO` in
/// `internal/routes/api_dto.go`. Canonical units, identical to
/// the upload payload; `measuredAt` dates are nil when the
/// server holds no observation for that metric (carried or
/// never measured). Synthesized decoding suffices: the server
/// omits nil timestamps (`omitempty`), and Swift maps absent
/// optional keys to nil.
public struct HealthSnapshotDTO: Decodable, Equatable {
    public let id: String
    public let snapshotDate: String
    public let tz: String
    public let steps: Int
    public let distanceMeters: Double
    public let activeEnergyKcal: Double
    public let basalEnergyKcal: Double
    public let exerciseMinutes: Double
    public let sleepSeconds: Double
    public let weight: Double
    public let weightMeasuredAt: Date?
    public let bmi: Double
    public let bmiMeasuredAt: Date?
    public let bodyFatPercentage: Double
    public let bodyFatMeasuredAt: Date?
    public let leanBodyMass: Double
    public let leanMassMeasuredAt: Date?
    public let heartRate: Double
    public let heartRateMeasuredAt: Date?
    public let restingHeartRate: Double
    public let restingHRMeasuredAt: Date?
    public let walkingHeartRateAvg: Double
    public let walkingHRMeasuredAt: Date?
    public let hrvMs: Double
    public let hrvMeasuredAt: Date?
    public let cardioRecoveryBpm: Double
    public let cardioRecoveryMeasuredAt: Date?
    public let vo2Max: Double
    public let vo2MeasuredAt: Date?
    public let createdAt: Date
    public let updatedAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case snapshotDate = "snapshot_date"
        case tz
        case steps
        case distanceMeters = "distance_meters"
        case activeEnergyKcal = "active_energy_kcal"
        case basalEnergyKcal = "basal_energy_kcal"
        case exerciseMinutes = "exercise_minutes"
        case sleepSeconds = "sleep_seconds"
        case weight
        case weightMeasuredAt = "weight_measured_at"
        case bmi
        case bmiMeasuredAt = "bmi_measured_at"
        case bodyFatPercentage = "body_fat_percentage"
        case bodyFatMeasuredAt = "body_fat_measured_at"
        case leanBodyMass = "lean_body_mass"
        case leanMassMeasuredAt = "lean_mass_measured_at"
        case heartRate = "heart_rate"
        case heartRateMeasuredAt = "heart_rate_measured_at"
        case restingHeartRate = "resting_heart_rate"
        case restingHRMeasuredAt = "resting_hr_measured_at"
        case walkingHeartRateAvg = "walking_heart_rate_avg"
        case walkingHRMeasuredAt = "walking_hr_measured_at"
        case hrvMs = "hrv_ms"
        case hrvMeasuredAt = "hrv_measured_at"
        case cardioRecoveryBpm = "cardio_recovery_bpm"
        case cardioRecoveryMeasuredAt = "cardio_recovery_measured_at"
        case vo2Max = "vo2_max"
        case vo2MeasuredAt = "vo2_measured_at"
        case createdAt = "created_at"
        case updatedAt = "updated_at"
    }
}

/// Response body for `POST /api/v1/health-snapshots` and
/// `GET /api/v1/health-snapshots`. Wrapping the slice lets the
/// server add fields without breaking the iOS contract.
public struct HealthSnapshotsResponse: Decodable, Equatable {
    public let snapshots: [HealthSnapshotDTO]
}

// MARK: - Coach (AI features, beta-gated)

// DTOs mirror `internal/routes/api_coach.go`. "Coach" is the
// user-facing name; the server stores rows in `ai_reports` and
// the `type` field carries `weekly` (later `insight`/`monthly`).
// The report `payload` is the validated JSON produced from the
// versioned server prompt — the client renders it verbatim and
// never re-interprets it, so prompt evolution can't break old
// rows cached on device.

/// One stored weekly report (`GET /api/v1/coach/weekly:latest`,
/// history items). `payload` decodes straight into
/// `CoachReportPayload`; `dismissedAt` is nil until the user
/// swipes the card away.
public struct CoachReportDTO: Decodable, Equatable, Identifiable {
    public let id: String
    public let type: String
    public let periodStart: Date
    public let periodEnd: Date
    public let promptVersion: String
    public let model: String
    public let payload: CoachReportPayload
    public let dismissedAt: Date?
    public let createdAt: Date

    enum CodingKeys: String, CodingKey {
        case id
        case type
        case periodStart = "period_start"
        case periodEnd = "period_end"
        case promptVersion = "prompt_version"
        case model
        case payload
        case dismissedAt = "dismissed_at"
        case createdAt = "created_at"
    }

    public var isDismissed: Bool { dismissedAt != nil }
}

/// The validated weekly payload. Every array defaults to empty
/// when the server omits it, so a thinner future schema still
/// decodes; `summary` and `recommendations` are required (the
/// server rejects reports without them).
public struct CoachReportPayload: Decodable, Equatable {
    public let summary: String
    public let progressPerGoal: [CoachGoalProgress]
    public let prs: [String]
    public let stalling: [String]
    public let trends: CoachTrends
    public let adherence: String
    public let recoverySignals: [String]
    public let recommendations: [String]

    enum CodingKeys: String, CodingKey {
        case summary
        case progressPerGoal = "progress_per_goal"
        case prs
        case stalling
        case trends
        case adherence
        case recoverySignals = "recovery_signals"
        case recommendations
    }

    public init(from decoder: Decoder) throws {
        let container = try decoder.container(keyedBy: CodingKeys.self)
        summary = try container.decode(String.self, forKey: .summary)
        progressPerGoal = try container.decodeIfPresent([CoachGoalProgress].self, forKey: .progressPerGoal) ?? []
        prs = try container.decodeIfPresent([String].self, forKey: .prs) ?? []
        stalling = try container.decodeIfPresent([String].self, forKey: .stalling) ?? []
        trends = try container.decodeIfPresent(CoachTrends.self, forKey: .trends)
            ?? CoachTrends(volume: "n/a", frequency: "n/a", bodyweight: "n/a")
        adherence = try container.decodeIfPresent(String.self, forKey: .adherence) ?? ""
        recoverySignals = try container.decodeIfPresent([String].self, forKey: .recoverySignals) ?? []
        recommendations = try container.decode([String].self, forKey: .recommendations)
    }
}

/// One per-goal line in the weekly payload. Free text on both
/// sides — the server never parses goal titles, it only relays
/// them, so there is nothing to normalize here.
public struct CoachGoalProgress: Decodable, Equatable {
    public let goal: String
    public let status: String
}

/// Volume/frequency/bodyweight trend lines, pre-phrased by the
/// server prompt (e.g. "up 12% — mostly added sets").
public struct CoachTrends: Decodable, Equatable {
    public let volume: String
    public let frequency: String
    public let bodyweight: String
}

/// Response body for `GET /api/v1/coach/weekly?limit=N`.
/// Newest first.
public struct CoachReportsResponse: Decodable, Equatable {
    public let reports: [CoachReportDTO]
}

/// Server-side Coach consent state (`GET
/// /api/v1/me/coach-preferences`). `optIn` gates all workout data
/// leaving the server; `goalText` is the free-text training aim
/// ("" = no stated aim), capped at 1000 chars server-side.
public struct CoachPreferencesDTO: Codable, Equatable {
    public let optIn: Bool
    public let goalText: String

    enum CodingKeys: String, CodingKey {
        case optIn = "opt_in"
        case goalText = "goal_text"
    }

    public init(optIn: Bool, goalText: String) {
        self.optIn = optIn
        self.goalText = goalText
    }

    /// Client-side mirror of the server's 1000-char cap so the
    /// editor can disable Save before the round-trip rejects it.
    public static let maxGoalTextLength = 1000
}

/// JSON body for `PUT /api/v1/me/coach-preferences`. Same shape
/// as the response; kept separate so the request stays
/// `Encodable`-only.
public struct UpdateCoachPreferencesRequest: Encodable, Equatable {
    public let optIn: Bool
    public let goalText: String

    enum CodingKeys: String, CodingKey {
        case optIn = "opt_in"
        case goalText = "goal_text"
    }

    public init(optIn: Bool, goalText: String) {
        self.optIn = optIn
        self.goalText = goalText
    }
}
