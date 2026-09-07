import SwiftUI

/// Sheet for submitting user feedback. Mirrors the web app's
/// `/feedback` form (see `internal/views/feedback/feedback.templ`):
/// a short `title` (5–100 chars) and a longer `message`
/// (10–1000 chars), both trimmed and validated by
/// `FeedbackController.Submit` on the server. The local
/// `canSave` check mirrors those rules so the Save button
/// stays disabled until the body is valid.
///
/// The view is intentionally read-only-after-success: the
/// network response just dismisses the sheet via the
/// `onSuccess` closure (the parent owns the dismissal so it
/// can also trigger the "Thanks for your feedback" alert).
/// On any API error the form stays open and the message is
/// rendered inline — same pattern `NameEditView` and
/// `GoalEditorView` use for per-field save failures.
struct FeedbackView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    /// Called by the form when the server confirms the submission.
    /// The parent (`ProfileView`) flips both the sheet state and
    /// the success-alert state so the user sees a native "Thanks
    /// for your feedback" alert after the sheet dismisses.
    let onSuccess: () -> Void

    @State private var title: String = ""
    @State private var message: String = ""
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    @FocusState private var titleFocused: Bool

    /// Validation mirrors the server rules exactly (title
    /// 5–100, message 10–1000, both trimmed) so the
    /// greyed-out Save button is a faithful preview of what
    /// the server will accept.
    private var canSave: Bool {
        guard !isSaving else { return false }
        let t = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let m = message.trimmingCharacters(in: .whitespacesAndNewlines)
        return t.count >= 5 && t.count <= 100 && m.count >= 10 && m.count <= 1000
    }

    var body: some View {
        Form {
            titleSection
            messageSection
            if let errorMessage {
                errorSection(errorMessage)
            }
        }
        .navigationTitle("Send Feedback")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .topBarLeading) {
                Button("Cancel") { dismiss() }
                    .disabled(isSaving)
            }
            ToolbarItem(placement: .topBarTrailing) {
                Button {
                    Task { await save() }
                } label: {
                    if isSaving {
                        ProgressView()
                    } else {
                        Text("Send").bold()
                    }
                }
                .disabled(!canSave)
            }
        }
        .onAppear {
            // Auto-focus the title so the user can start
            // typing immediately. The check guards against
            // a SwiftUI re-render mid-presentation snapping
            // focus back to the field after the user has
            // already started filling in the message.
            if !titleFocused {
                titleFocused = true
            }
        }
    }

    // MARK: - Sections

    private var titleSection: some View {
        Section {
            TextField("Brief summary", text: $title)
                .focused($titleFocused)
                .textInputAutocapitalization(.sentences)
        } header: {
            Text("Title")
        } footer: {
            Text("5–100 characters")
        }
    }

    /// Vertical-axis text field so the message box grows with
    /// the user's input (matches the web's `<textarea rows="6">`).
    /// `lineLimit(4...15)` keeps the field from collapsing to a
    /// single line on first appearance but also stops it from
    /// swallowing the whole screen at the 1000-character limit.
    private var messageSection: some View {
        Section {
            TextField("What's on your mind?", text: $message, axis: .vertical)
                .lineLimit(4...15)
        } header: {
            Text("Message")
        } footer: {
            Text("10–1000 characters")
        }
    }

    private func errorSection(_ message: String) -> some View {
        Section {
            Text(message)
                .font(.footnote)
                .foregroundStyle(DSColors.destructive)
        }
    }

    // MARK: - Save

    private func save() async {
        errorMessage = nil
        let trimmedTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
        let trimmedMessage = message.trimmingCharacters(in: .whitespacesAndNewlines)

        // Mirror the server's validation up front so the
        // user gets a local error immediately instead of
        // waiting for a round trip. The server runs the
        // exact same checks (controllers/feedback.go:33-46)
        // so a locally-accepted request is guaranteed to be
        // accepted over the wire too.
        guard trimmedTitle.count >= 5, trimmedTitle.count <= 100 else {
            errorMessage = "Title must be between 5 and 100 characters."
            return
        }
        guard trimmedMessage.count >= 10, trimmedMessage.count <= 1000 else {
            errorMessage = "Message must be between 10 and 1000 characters."
            return
        }

        isSaving = true
        defer { isSaving = false }
        do {
            try await env.api.submitFeedback(SubmitFeedbackRequest(
                title: trimmedTitle,
                message: trimmedMessage
            ))
            onSuccess()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not send your feedback."
        }
    }
}

#Preview {
    NavigationStack {
        FeedbackView(onSuccess: {})
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
