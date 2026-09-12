import Foundation

/// HTTP client for the Hylete server's `/api/v1` namespace.
/// One method per endpoint, all `async throws`, all returning
/// a strongly-typed DTO. No third-party networking
/// dependency — just `URLSession`.
///
/// Auth is handled by an injected `tokenProvider` closure
/// (typically `AuthStore.currentToken`). On 401 the
/// `onUnauthorized` callback fires so the caller can clear
/// the session and bounce the user to the login screen
/// before the calling view's error handler runs.
public final class APIClient: @unchecked Sendable {
    private let baseURL: URL
    private let session: URLSession
    private let tokenProvider: @Sendable () -> String?
    private let onUnauthorized: @Sendable () -> Void

    /// Shared JSON encoder/decoder pair configured for the
    /// server's wire format. Dates are RFC 3339 with
    /// fractional seconds (matching Go's `time.Time` JSON
    /// output) with a no-fractional-seconds fallback for
    /// older server versions.
    public static let jsonEncoder: JSONEncoder = {
        let enc = JSONEncoder()
        enc.dateEncodingStrategy = .custom { date, encoder in
            var container = encoder.singleValueContainer()
            try container.encode(APIClient.dateFormatter.string(from: date))
        }
        return enc
    }()

    public static let jsonDecoder: JSONDecoder = {
        let dec = JSONDecoder()
        dec.dateDecodingStrategy = .custom { decoder in
            let container = try decoder.singleValueContainer()
            let str = try container.decode(String.self)
            // Try the two on-the-wire variants we expect from
            // the server, in order of specificity.
            for options in APIClient.dateFormatOptions {
                APIClient.dateFormatter.formatOptions = options
                if let date = APIClient.dateFormatter.date(from: str) {
                    return date
                }
            }
            throw DecodingError.dataCorruptedError(
                in: container,
                debugDescription: "Could not parse date string: \(str)"
            )
        }
        return dec
    }()

    private static let dateFormatOptions: [ISO8601DateFormatter.Options] = [
        [.withInternetDateTime, .withFractionalSeconds],
        [.withInternetDateTime],
    ]

    private static let dateFormatter: ISO8601DateFormatter = {
        let f = ISO8601DateFormatter()
        return f
    }()

    /// RFC 3339 formatter for query-string dates (the
    /// `from`/`to` params). Default GMT output keeps every
    /// produced string "+"-free so no percent-encoding of the
    /// offset is required.
    private static let queryDateFormatter = ISO8601DateFormatter()

    public init(
        baseURL: URL,
        session: URLSession = .shared,
        tokenProvider: @escaping @Sendable () -> String?,
        onUnauthorized: @escaping @Sendable () -> Void = {}
    ) {
        // Force a trailing slash so relative paths like
        // "auth/login" resolve to ".../api/v1/auth/login",
        // not ".../api/auth/login". Without the trailing
        // slash, Swift's URL resolver treats the last
        // segment of baseURL as a file and replaces it.
        self.baseURL = baseURL.absoluteString.hasSuffix("/")
            ? baseURL
            : baseURL.appendingPathComponent("")
        self.session = session
        self.tokenProvider = tokenProvider
        self.onUnauthorized = onUnauthorized
    }

    // MARK: - Auth

    public func login(email: String, password: String) async throws -> AuthResponse {
        try await send(
            "POST",
            "auth/login",
            body: LoginRequest(email: email, password: password),
            requiresAuth: false
        )
    }

    public func register(name: String, email: String, password: String) async throws -> AuthResponse {
        try await send(
            "POST",
            "auth/register",
            body: RegisterRequest(name: name, email: email, password: password),
            requiresAuth: false
        )
    }

    /// Requests a password-reset email. The link opens the existing
    /// web reset form; the iOS app deliberately does not handle the
    /// reset token or duplicate that form.
    public func requestPasswordReset(email: String) async throws {
        let _: PasswordResetResponse = try await send(
            "POST",
            "auth/password-reset/request",
            body: PasswordResetRequest(email: email),
            requiresAuth: false
        )
    }

    public func logout() async throws {
        try await sendVoid("POST", "auth/logout", requiresAuth: false)
    }

    public func me() async throws -> UserDTO {
        try await send("GET", "me")
    }

    /// Updates the authenticated user's profile. Mirrors the
    /// user-editable subset of the web app's `/profile` form
    /// (name, target weight, weight unit). Returns the updated
    /// user so the caller can refresh `AuthStore.currentUser`
    /// without a follow-up `me()` round trip.
    ///
    /// Reminder preferences are NOT writable through this endpoint —
    /// they live on `getReminderPreferences` /
    /// `updateReminderPreferences` below so a `PUT /me` that doesn't
    /// know about reminders can never clobber them.
    public func updateProfile(_ request: UpdateMeRequest) async throws -> UserDTO {
        try await send("PUT", "me", body: request)
    }

    // MARK: - Weight reminders

    /// Fetches the authenticated user's weight-reminder schedule
    /// (`GET /api/v1/me/reminders`). A row that has never been touched
    /// reads as reminders-off with the 09:00 default time, so the
    /// editor always has a usable first-paint state.
    public func getReminderPreferences() async throws -> ReminderPreferencesDTO {
        try await send("GET", "me/reminders")
    }

    /// Replaces the authenticated user's weight-reminder schedule
    /// (`PUT /api/v1/me/reminders`). Mirrors the web `/profile` form's
    /// reminder section: `frequency` is one of off/daily/weekly/
    /// biweekly, `dayOfWeek` is 0–6 (Sunday=0) for weekly/biweekly and
    /// nil otherwise, `time` is `"HH:00"` in 24h UTC (hour-only by
    /// design). Returns the stored schedule so the caller can render
    /// the exact server state without a follow-up GET.
    public func updateReminderPreferences(_ request: UpdateReminderPreferencesRequest) async throws -> ReminderPreferencesDTO {
        try await send("PUT", "me/reminders", body: request)
    }

    // MARK: - Exercises

    public func listExercises() async throws -> [ExerciseDTO] {
        try await send("GET", "exercises")
    }

    // MARK: - Exercise entries (sets)

    public func listExerciseEntries(days: Int = 7) async throws -> [ExerciseEntryDTO] {
        try await send("GET", "exercise-entries?days=\(days)")
    }

    /// Lists exercise entries within an explicit range
    /// (`GET /api/v1/exercise-entries?from=<RFC3339>&to=<RFC3339>`,
    /// inclusive on both ends). The week calendar uses this so day
    /// boundaries are computed client-side in the user's local
    /// timezone: callers pass absolute instants and the server never
    /// needs to know the user's offset. Timestamps are formatted in
    /// UTC ("Z"), which is the same instant — this also sidesteps
    /// the classic "+ becomes a space" query-encoding pitfall, since
    /// a UTC RFC 3339 string contains no "+" sign.
    public func listExerciseEntries(from: Date, to: Date) async throws -> [ExerciseEntryDTO] {
        let path = "exercise-entries"
            + "?from=" + Self.queryDateFormatter.string(from: from)
            + "&to=" + Self.queryDateFormatter.string(from: to)
        return try await send("GET", path)
    }

    public func getExerciseEntry(id: String) async throws -> ExerciseEntryDTO {
        try await send("GET", "exercise-entries/\(id)")
    }

    public func createExerciseEntries(_ request: CreateExerciseEntriesRequest) async throws -> [ExerciseEntryDTO] {
        try await send("POST", "exercise-entries", body: request)
    }

    public func updateExerciseEntry(id: String, request: UpdateExerciseEntryRequest) async throws -> ExerciseEntryDTO {
        try await send("PUT", "exercise-entries/\(id)", body: request)
    }

    public func deleteExerciseEntry(id: String) async throws {
        try await sendVoid("DELETE", "exercise-entries/\(id)")
    }

    // MARK: - Per-exercise history & chart

    public func getExerciseHistory(id: String, page: Int = 1) async throws -> HistoryPageDTO {
        try await send("GET", "exercises/\(id)/history?page=\(page)")
    }

    public func getExerciseChartData(id: String) async throws -> [ExerciseEntryDTO] {
        try await send("GET", "exercises/\(id)/chart")
    }

    // MARK: - Workouts

    /// Lists every workout for the authenticated user, planned
    /// first. Used by the Workouts list.
    public func listWorkouts() async throws -> [WorkoutDTO] {
        try await send("GET", "workouts")
    }

    /// Lists workouts whose scheduled window overlaps the
    /// inclusive range (`GET
    /// /api/v1/workouts?from=<RFC3339>&to=<RFC3339>`). The
    /// dashboard calendar uses this so day boundaries are
    /// computed client-side in the user's local timezone —
    /// same pattern as `listExerciseEntries(from:to:)`.
    public func listWorkouts(from: Date, to: Date) async throws -> [WorkoutDTO] {
        let path = "workouts"
            + "?from=" + Self.queryDateFormatter.string(from: from)
            + "&to=" + Self.queryDateFormatter.string(from: to)
        return try await send("GET", path)
    }

    /// Creates a dated workout with inline blocks (may be empty
    /// for a bare shell). Returns the full detail so the caller
    /// can open it without a follow-up GET.
    public func createWorkout(_ request: CreateWorkoutRequest) async throws -> WorkoutDetailDTO {
        try await send("POST", "workouts", body: request)
    }

    /// Snapshots one source workout into one dated copy per
    /// instance, atomically (all or nothing). Powers plan-ahead
    /// flows ("every Monday x N"). Returns the created headers
    /// in request order.
    public func bulkCreateWorkouts(_ request: BulkCreateWorkoutsRequest) async throws -> [WorkoutDTO] {
        try await send("POST", "workouts/bulk", body: request)
    }

    /// Copies a workout (including its tree) — a blank name
    /// becomes "Copy of <source>" server-side, nil schedule
    /// bounds inherit the source's window. Returns the new
    /// detail.
    public func duplicateWorkout(id: String, request: DuplicateWorkoutRequest = DuplicateWorkoutRequest()) async throws -> WorkoutDetailDTO {
        try await send("POST", "workouts/\(id)/duplicate", body: request)
    }

    /// Fetches a workout with its tree and logged exercise
    /// entries (split into item-linked vs ad-hoc).
    public func getWorkout(id: String) async throws -> WorkoutDetailDTO {
        try await send("GET", "workouts/\(id)")
    }

    /// Updates a workout's header (name, notes, scheduled
    /// window). The tree is fixed at creation; status moves
    /// through the dedicated complete/reopen/cancel methods.
    public func updateWorkout(id: String, request: UpdateWorkoutRequest) async throws -> WorkoutDetailDTO {
        try await send("PUT", "workouts/\(id)", body: request)
    }

    /// Hard-deletes a workout. Its tree cascades; logged
    /// exercise entries survive with their links cleared.
    public func deleteWorkout(id: String) async throws {
        try await sendVoid("DELETE", "workouts/\(id)")
    }

    /// Marks a workout complete. The server owns
    /// `completed_at`. Idempotent.
    public func completeWorkout(id: String) async throws -> WorkoutDetailDTO {
        try await send("POST", "workouts/\(id)/complete")
    }

    /// Moves a workout back to planned (clears
    /// `completed_at`). Idempotent.
    public func reopenWorkout(id: String) async throws -> WorkoutDetailDTO {
        try await send("POST", "workouts/\(id)/reopen")
    }

    /// Marks a workout cancelled. Idempotent.
    public func cancelWorkout(id: String) async throws -> WorkoutDetailDTO {
        try await send("POST", "workouts/\(id)/cancel")
    }

    // MARK: - Goals

    /// Lists every goal for the authenticated user. The server
    /// returns active goals first (ordered by target date
    /// ascending, nulls last) followed by completed goals
    /// (most-recently-completed first). Mirrors the ordering
    /// the web view renders so the iOS list can reuse the
    /// same active/completed sections.
    public func listGoals() async throws -> [GoalDTO] {
        let response: GoalsResponse = try await send("GET", "goals")
        return response.goals
    }

    public func getGoal(id: String) async throws -> GoalDTO {
        try await send("GET", "goals/\(id)")
    }

    public func createGoal(_ request: CreateGoalRequest) async throws -> GoalDTO {
        try await send("POST", "goals", body: request)
    }

    public func updateGoal(id: String, request: UpdateGoalRequest) async throws -> GoalDTO {
        try await send("PUT", "goals/\(id)", body: request)
    }

    /// Marks a goal complete. The server sets `completed_at`
    /// to `time.Now()` — the client does not send a timestamp.
    /// Idempotent: completing an already-complete goal is a
    /// no-op that still returns the current row.
    public func markGoalComplete(id: String) async throws -> GoalDTO {
        try await send("POST", "goals/\(id)/complete")
    }

    /// Reopens a completed goal (clears `completed_at`).
    /// Idempotent on already-active goals.
    public func reopenGoal(id: String) async throws -> GoalDTO {
        try await send("POST", "goals/\(id)/reopen")
    }

    public func deleteGoal(id: String) async throws {
        try await sendVoid("DELETE", "goals/\(id)")
    }

    // MARK: - Feedback

    /// Submits user feedback to the server. Mirrors the web
    /// app's `/feedback` POST handler — both surface the same
    /// validation rules (`title` 5–100, `message` 10–1000,
    /// both trimmed) enforced by `FeedbackController.Submit`.
    /// The server stores the row scoped to the authenticated
    /// user; returns 204 No Content on success so the iOS
    /// view can dismiss and surface a "Thanks for your
    /// feedback" alert.
    public func submitFeedback(_ request: SubmitFeedbackRequest) async throws {
        try await sendVoid("POST", "feedback", body: request)
    }

    // MARK: - Weight

    /// Lists every weight entry for the authenticated user,
    /// newest first. The server returns the entries wrapped
    /// in a `WeightEntriesResponse` envelope so the iOS
    /// view treats the response as opaque and only reads
    /// `.entries`.
    public func listWeightEntries() async throws -> [WeightEntryDTO] {
        let response: WeightEntriesResponse = try await send("GET", "weight")
        return response.entries
    }

    /// Fetches two photo-bearing weight entries for the native
    /// comparison view. The server orders them chronologically
    /// and supplies the display-ready photo URLs. `angle` is
    /// one of front/side/back (defaults to front); both entries
    /// must have a photo in that slot or the server returns a
    /// 400 naming the angles they do share.
    public func compareWeightEntries(a: String, b: String, angle: String = "front") async throws -> WeightCompareResponse {
        try await send("GET", "weight/compare?a=\(a)&b=\(b)&angle=\(angle)")
    }

    /// Creates a new weight entry. `createdAt` is optional
    /// on the server — when the client sends `nil` the
    /// server stamps the entry with the current time. The
    /// returns the server-confirmed row so the iOS view can
    /// splice it into its local cache without a follow-up
    /// GET.
    public func createWeightEntry(_ request: CreateWeightEntryRequest) async throws -> WeightEntryDTO {
        try await send("POST", "weight", body: request)
    }

    /// Fetches a single weight entry by ID. Returns `nil`
    /// when the entry is missing or owned by another user
    /// (the server returns 404 for both cases). The iOS
    /// view treats the two outcomes uniformly so the user
    /// just sees "entry not found".
    public func getWeightEntry(id: String) async throws -> WeightEntryDTO {
        try await send("GET", "weight/\(id)")
    }

    /// Updates an existing weight entry. Photo handling is per
    /// angle slot: `remove*Photo = true` clears that slot, a
    /// non-empty `*PhotoKey` replaces the existing key, and
    /// otherwise the existing key is preserved. `createdAt` is
    /// optional — when omitted the server keeps the existing
    /// timestamp so the "edit a recent entry" flow doesn't
    /// accidentally reset it.
    public func updateWeightEntry(id: String, request: UpdateWeightEntryRequest) async throws -> WeightEntryDTO {
        try await send("PUT", "weight/\(id)", body: request)
    }

    /// Deletes a weight entry. The server returns 204 No
    /// Content on success and is idempotent for missing
    /// entries (matches the web's behaviour) so the iOS
    /// view can safely issue a delete without a prior
    /// existence check.
    public func deleteWeightEntry(id: String) async throws {
        try await sendVoid("DELETE", "weight/\(id)")
    }

    /// Asks the server for a presigned R2 PUT URL so the
    /// client can upload a photo file directly to R2 without
    /// proxying the bytes through the Go server. The
    /// returned `key` is what the iOS view submits back to
    /// the create or update form as the angle's photo key.
    /// `angle` optionally namespaces the storage key
    /// (front/side/back).
    public func requestWeightPhotoUploadURL(filename: String, contentType: String, angle: String? = nil) async throws -> WeightPhotoUploadResponse {
        try await send("POST", "weight/photo-upload", body: WeightPhotoUploadRequest(
            filename: filename,
            contentType: contentType,
            angle: angle
        ))
    }

    // MARK: - Health snapshots

    /// Uploads a batch of daily Health snapshots. The server
    /// upserts each against (user_id, snapshot_date), so
    /// re-uploads overwrite instead of duplicating — retries,
    /// backfills, and late corrections are safe. Returns the
    /// persisted rows in request order.
    public func upsertHealthSnapshots(_ snapshots: [HealthSnapshotPayload]) async throws -> [HealthSnapshotDTO] {
        struct Body: Encodable {
            let snapshots: [HealthSnapshotPayload]
        }
        let response: HealthSnapshotsResponse = try await send("POST", "health-snapshots", body: Body(snapshots: snapshots))
        return response.snapshots
    }

    /// Lists persisted snapshots for the last N device-local
    /// calendar dates, newest first. Used for future analysis
    /// surfaces; the sync flow itself tracks confirmation
    /// locally and does not need this.
    public func listHealthSnapshots(days: Int = 90) async throws -> [HealthSnapshotDTO] {
        let response: HealthSnapshotsResponse = try await send("GET", "health-snapshots?days=\(days)")
        return response.snapshots
    }

    // MARK: - Coach (AI features, beta-gated)

    /// Fetches the Coach consent state (`GET
    /// /api/v1/me/coach-preferences`). Works even when the
    /// user hasn't opted in — that is the point: the Profile
    /// section reads this to paint the toggle.
    public func getCoachPreferences() async throws -> CoachPreferencesDTO {
        try await send("GET", "me/coach-preferences")
    }

    /// Replaces the Coach consent state (`PUT
    /// /api/v1/me/coach-preferences`). `goalText` is trimmed
    /// client-side and capped at 1000 chars (mirroring the
    /// server's 400 rule) so the editor can disable Save
    /// before the round-trip. Returns the stored state.
    public func updateCoachPreferences(_ request: UpdateCoachPreferencesRequest) async throws -> CoachPreferencesDTO {
        try await send("PUT", "me/coach-preferences", body: request)
    }

    /// Fetches the newest stored weekly report (`GET
    /// /api/v1/coach/weekly:latest`). Throws a 404
    /// `APIError.server` when the cron hasn't produced one yet
    /// and 403 when the user hasn't opted in — the store maps
    /// both to dedicated UI states.
    public func getCoachLatest() async throws -> CoachReportDTO {
        try await send("GET", "coach/weekly:latest")
    }

    /// Lists stored weekly reports, newest first (`GET
    /// /api/v1/coach/weekly?limit=N`).
    public func listCoachReports(limit: Int = 12) async throws -> [CoachReportDTO] {
        let response: CoachReportsResponse = try await send("GET", "coach/weekly?limit=\(limit)")
        return response.reports
    }

    /// Dismisses a report (`POST
    /// /api/v1/coach/weekly/:id/dismiss`). Idempotent.
    public func dismissCoachReport(id: String) async throws {
        try await sendVoid("POST", "coach/weekly/\(id)/dismiss")
    }

    /// Restores a dismissed report to the card list (`POST
    /// /api/v1/coach/weekly/:id/restore`). Idempotent.
    public func restoreCoachReport(id: String) async throws {
        try await sendVoid("POST", "coach/weekly/\(id)/restore")
    }

    // MARK: - Request plumbing
    /// Generic request method for endpoints that return a
    /// JSON body. Throws `APIError` on transport, status,
    /// and decoding failures. The 401 path fires
    /// `onUnauthorized` so the caller doesn't have to.
    private func send<T: Decodable>(
        _ method: String,
        _ path: String,
        body: (any Encodable)? = nil,
        requiresAuth: Bool = true
    ) async throws -> T {
        let request = try makeRequest(method: method, path: path, body: body, requiresAuth: requiresAuth)
        let (data, response) = try await performRequest(request)
        try checkStatus(response: response, data: data)
        do {
            return try Self.jsonDecoder.decode(T.self, from: data)
        } catch {
            throw APIError.decoding(error)
        }
    }

    /// Variant of `send` for endpoints that return no body
    /// (204 No Content). Same status-code handling, no
    /// decoder step.
    private func sendVoid(
        _ method: String,
        _ path: String,
        body: (any Encodable)? = nil,
        requiresAuth: Bool = true
    ) async throws {
        let request = try makeRequest(method: method, path: path, body: body, requiresAuth: requiresAuth)
        let (data, response) = try await performRequest(request)
        try checkStatus(response: response, data: data)
    }

    private func makeRequest(
        method: String,
        path: String,
        body: (any Encodable)?,
        requiresAuth: Bool
    ) throws -> URLRequest {
        // Strip a leading slash so the join doesn't produce
        // a doubled "//" segment, e.g. "...v1//auth/login".
        let normalizedPath = path.hasPrefix("/") ? String(path.dropFirst()) : path
        let url: URL
        if let composed = URL(string: normalizedPath, relativeTo: baseURL)?.absoluteURL {
            url = composed
        } else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = method
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        if requiresAuth, let token = tokenProvider(), !token.isEmpty {
            request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        }
        if let body {
            do {
                request.httpBody = try Self.jsonEncoder.encode(AnyEncodable(body))
            } catch {
                throw APIError.decoding(error)
            }
        }
        return request
    }

    private func performRequest(_ request: URLRequest) async throws -> (Data, URLResponse) {
        do {
            return try await session.data(for: request)
        } catch let urlError as URLError {
            throw APIError.transport(urlError)
        } catch {
            throw APIError.transport(URLError(.unknown))
        }
    }

    private func checkStatus(response: URLResponse, data: Data) throws {
        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport(URLError(.badServerResponse))
        }
        switch http.statusCode {
        case 200..<300:
            return
        case 401:
            onUnauthorized()
            throw APIError.unauthorized
        default:
            let message: String
            if let body = try? Self.jsonDecoder.decode(APIErrorBody.self, from: data) {
                message = body.error
            } else {
                message = "HTTP \(http.statusCode)"
            }
            throw APIError.server(status: http.statusCode, message: message)
        }
    }
}

/// Type-erased `Encodable` so `send` can take an `any
/// Encodable` body without requiring every caller to use a
/// generic function. Cheaper than `JSONEncoder` doing the
/// lookup at runtime via `Any`, and `Encodable` constraints
/// on the static method already guarantee the wrapped value
/// encodes successfully.
private struct AnyEncodable: Encodable {
    private let _encode: (Encoder) throws -> Void

    init(_ wrapped: any Encodable) {
        self._encode = wrapped.encode
    }

    func encode(to encoder: Encoder) throws {
        try _encode(encoder)
    }
}

// `APIClient` is the production `HealthSnapshotAPI`: its
// `upsertHealthSnapshots` matches the protocol requirement, so
// only the conformance needs declaring. Tests inject a fake.
extension APIClient: HealthSnapshotAPI {}