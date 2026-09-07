import Foundation

/// Errors surfaced by `APIClient` to the SwiftUI layer. Every
/// case maps to a user-actionable situation so the views can
/// show a meaningful alert (network down, bad credentials,
/// server error, etc.) without having to inspect the raw
/// `URLError` or HTTP status.
public enum APIError: Error, LocalizedError {
    /// The response URL or path was malformed. Should be
    /// unreachable in production — typically indicates a bug
    /// in the client (e.g. a path with a stray space).
    case invalidURL
    /// A transport-level failure (no network, DNS error,
    /// timeout, TLS error). The associated `URLError` carries
    /// the platform-specific reason.
    case transport(URLError)
    /// The server returned a 401. The client's `onUnauthorized`
    /// callback has already been invoked, so the user has
    /// typically been bounced to the login screen by the
    /// time this error is observed.
    case unauthorized
    /// The server returned a 4xx or 5xx other than 401. The
    /// `message` is the human-readable `error` field from the
    /// server's `APIError` body, or a generic fallback if the
    /// body was missing or unparseable.
    case server(status: Int, message: String)
    /// The response body could not be decoded into the
    /// expected DTO. Usually means the server and client
    /// are out of sync (a field was renamed, a date format
    /// changed, etc.).
    case decoding(Error)

    public var errorDescription: String? {
        switch self {
        case .invalidURL:
            return "The request URL was invalid."
        case .transport(let err):
            return err.localizedDescription
        case .unauthorized:
            return "Your session has expired. Please log in again."
        case .server(_, let message):
            return message
        case .decoding:
            return "The server returned an unexpected response."
        }
    }
}

/// Matches the server's `APIError` body shape. Kept private
/// to the networking layer — view code should only ever
/// handle `APIError`, not the raw JSON shape.
struct APIErrorBody: Decodable {
    let error: String
}
