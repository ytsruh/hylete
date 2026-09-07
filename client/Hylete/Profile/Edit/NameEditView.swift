import SwiftUI

/// Editor for the user's display name. Mirrors the
/// `name` field on the server's `UpdateMeRequest`. Cancels
/// without touching the server; Save sends a full update
/// (carrying the current email, target weight, and weight
/// unit) so the iOS edit flow never accidentally clobbers
/// another field the user is mid-edit on in a different
/// screen.
struct NameEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    /// The user's current values. Captured at construction
    /// time so the merge in `save()` uses the values as
    /// they were when the editor opened — even if another
    /// flow concurrently updates `authStore.currentUser`
    /// mid-edit, the editor always sends a self-consistent
    /// payload.
    let user: UserDTO

    @State private var name: String = ""
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    @FocusState private var nameFocused: Bool

    private var canSave: Bool {
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        return trimmed.count >= 2 && trimmed.count <= 100 && !isSaving
    }

    var body: some View {
        Form {
            Section {
                TextField("Name", text: $name)
                    .textContentType(.name)
                    .textInputAutocapitalization(.words)
                    .autocorrectionDisabled()
                    .focused($nameFocused)
            } header: {
                Text("Display name")
            } footer: {
                Text("Shown across the app, including your dashboard and any sets you log.")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Name")
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
                        Text("Save").bold()
                    }
                }
                .disabled(!canSave)
            }
        }
        .onAppear {
            // Seed only on first appear so a SwiftUI
            // re-render mid-edit (e.g. theme switch) doesn't
            // blow away the user's typing.
            if name.isEmpty {
                name = user.name
                nameFocused = true
            }
        }
    }

    private func save() async {
        errorMessage = nil
        let trimmed = name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard trimmed.count >= 2, trimmed.count <= 100 else {
            errorMessage = "Name must be between 2 and 100 characters."
            return
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateMeRequest(
                name: trimmed,
                targetWeight: user.targetWeight,
                weightUnit: user.weightUnit,
                distanceUnit: user.distanceUnit
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your name."
        }
    }
}

#Preview {
    NavigationStack {
        NameEditView(user: UserDTO(
            id: "u1",
            name: "Alice",
            email: "alice@example.com",
            isAdmin: false,
            weightUnit: "kg",
            distanceUnit: "km",
            targetWeight: 75.0
        ))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
