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
    /// Reminder preferences and push subscriptions are NOT
    /// writable through this endpoint — the iOS app surfaces
    /// no UI for them yet, so the server keeps the form-only
    /// ownership of those fields. When those surfaces land
    /// on iOS, add dedicated endpoints rather than expanding
    /// this one.
    public func updateProfile(_ request: UpdateMeRequest) async throws -> UserDTO {
        try await send("PUT", "me", body: request)
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
    /// and supplies the display-ready photo URLs.
    public func compareWeightEntries(a: String, b: String) async throws -> WeightCompareResponse {
        try await send("GET", "weight/compare?a=\(a)&b=\(b)")
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

    /// Updates an existing weight entry. The photo-handling
    /// precedence mirrors the HTML form: `removePhoto = true`
    /// clears the photo, a non-empty `photoKey` replaces the
    /// existing key, and otherwise the existing key is
    /// preserved. `createdAt` is optional — when omitted the
    /// server keeps the existing timestamp so the "edit a
    /// recent entry" flow doesn't accidentally reset it.
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
    /// the create or update form as `photoKey`.
    public func requestWeightPhotoUploadURL(filename: String, contentType: String) async throws -> WeightPhotoUploadResponse {
        try await send("POST", "weight/photo-upload", body: WeightPhotoUploadRequest(
            filename: filename,
            contentType: contentType
        ))
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
