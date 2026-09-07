import Foundation
import SwiftUI

/// Encapsulates the two-step photo upload flow used by the
/// weight editor:
///
///   1. Ask the server for a presigned R2 PUT URL via
///      `APIClient.requestWeightPhotoUploadURL(...)`.
///   2. PUT the file bytes directly to that URL (the
///      server is not in the loop).
///
/// Uploaded bytes bypass the Go server entirely (the
/// presigned URL carries the auth), so the server only
/// sees the create / update request that names the new
/// `photoKey`. This mirrors the web's flow in
/// `internal/views/weight/photo_upload.templ`.
///
/// Errors are wrapped as `APIError`-compatible values so
/// the calling view can surface them inline through the
/// same `errorMessage` channel it already uses for the
/// create / update round-trip.
struct WeightPhotoUploader {

    /// The API client used to ask the server for the
    /// presigned PUT URL. The PUT itself goes through
    /// `URLSession.shared` so the call site doesn't need
    /// to know about R2.
    private let api: APIClient

    /// Standalone session used for the presigned PUT.
    /// `URLSession.shared` so the upload inherits the
    /// system's proxy / TLS settings — matches the web
    /// flow's `fetch(...)` browser-context.
    private let session: URLSession

    init(api: APIClient, session: URLSession = .shared) {
        self.api = api
        self.session = session
    }

    /// Uploads the supplied bytes to R2 and returns the
    /// server-side storage key that the iOS view then
    /// submits back to the create / update form.
    ///
    /// The caller is responsible for any client-side
    /// resizing. We deliberately do not compress the
    /// bytes here — `PhotosPicker` already returns the
    /// user's preferred format (typically JPEG) at a
    /// reasonable size, and an extra re-encode would
    /// degrade the photo without saving meaningful
    /// bandwidth at this scale.
    ///
    /// `filename` is informational; the server doesn't
    /// use it to derive the storage key (the key is a
    /// server-generated UUID) but it does appear in the
    /// R2 object metadata for debugging.
    /// `contentType` is sent both in the presigned URL
    /// and on the PUT itself so R2 stores the right
    /// MIME type.
    func upload(data: Data, filename: String, contentType: String) async throws -> String {
        let presigned = try await api.requestWeightPhotoUploadURL(
            filename: filename,
            contentType: contentType
        )

        guard let url = URL(string: presigned.url) else {
            throw APIError.invalidURL
        }

        var request = URLRequest(url: url)
        request.httpMethod = "PUT"
        request.setValue(contentType, forHTTPHeaderField: "Content-Type")
        request.httpBody = data

        let response: URLResponse
        do {
            (_, response) = try await session.data(for: request)
        } catch let urlError as URLError {
            throw APIError.transport(urlError)
        } catch {
            throw APIError.transport(URLError(.unknown))
        }

        guard let http = response as? HTTPURLResponse else {
            throw APIError.transport(URLError(.badServerResponse))
        }
        guard (200..<300).contains(http.statusCode) else {
            throw APIError.server(status: http.statusCode, message: "Photo upload failed (\(http.statusCode))")
        }

        return presigned.key
    }
}
