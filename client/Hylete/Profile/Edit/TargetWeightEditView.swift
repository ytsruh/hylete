import SwiftUI

/// Editor for the user's target body-weight goal. The
/// underlying value is a `Double?` on the server (an empty
/// field clears the goal), so the form binds to a `String`
/// and only converts to a number on save. The "Clear"
/// button removes the goal entirely without the user
/// needing to backspace to zero.
struct TargetWeightEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var weightText: String = ""
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    @FocusState private var weightFocused: Bool

    /// Parsed weight, or nil if the field is empty.
    /// `Double("")` would be nil but we keep an explicit
    /// empty-check so the form is explicit about "blank
    /// means no goal" rather than relying on parser quirks.
    private var parsedWeight: Double? {
        let trimmed = weightText.trimmingCharacters(in: .whitespaces)
        return trimmed.isEmpty ? nil : Double(trimmed)
    }

    private var canSave: Bool {
        guard !isSaving else { return false }
        // Empty is always valid (clears the goal). Otherwise
        // the value must parse AND be in the server's
        // accepted range (0–1000).
        let trimmed = weightText.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { return true }
        guard let value = Double(trimmed) else { return false }
        return value >= 0 && value <= 1000
    }

    var body: some View {
        Form {
            Section {
                HStack {
                    TextField("85.0", text: $weightText)
                        .keyboardType(.decimalPad)
                        .focused($weightFocused)
                    Text(user.weightUnit)
                        .foregroundStyle(DSColors.textSecondary)
                }
                if !weightText.isEmpty {
                    Button(role: .destructive) {
                        weightText = ""
                    } label: {
                        Label("Clear target weight", systemImage: "xmark.circle")
                    }
                }
            } header: {
                Text("Target weight")
            } footer: {
                Text("Leave blank to clear your goal. Values are in your preferred unit (\(user.weightUnit)).")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Target weight")
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
            // Seed only once so a mid-edit re-render
            // (e.g. theme switch) doesn't clobber the
            // user's typing. The display format matches
            // the dashboard / web app: one decimal place.
            if weightText.isEmpty, let target = user.targetWeight {
                weightText = String(format: "%.1f", target)
            }
            weightFocused = true
        }
    }

    private func save() async {
        errorMessage = nil
        let trimmed = weightText.trimmingCharacters(in: .whitespaces)
        var target: Double? = nil
        if !trimmed.isEmpty {
            guard let value = Double(trimmed) else {
                errorMessage = "Target weight must be a number."
                return
            }
            guard value >= 0, value <= 1000 else {
                errorMessage = "Target weight must be between 0 and 1000."
                return
            }
            target = value
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateMeRequest(
                name: user.name,
                targetWeight: target,
                weightUnit: user.weightUnit,
                distanceUnit: user.distanceUnit
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your target weight."
        }
    }
}

#Preview {
    NavigationStack {
        TargetWeightEditView(user: UserDTO(
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
