import SwiftUI

/// Editor for the user's age in years. The underlying value is an
/// `Int?` on the server (an empty field clears the age), so the
/// form binds to a `String` and only converts to a number on save.
struct AgeEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var ageText: String = ""
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    @FocusState private var ageFocused: Bool

    private var canSave: Bool {
        guard !isSaving else { return false }
        let trimmed = ageText.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { return true }
        guard let value = Int(trimmed) else { return false }
        return value >= 10 && value <= 120
    }

    var body: some View {
        Form {
            Section {
                HStack {
                    TextField("30", text: $ageText)
                        .keyboardType(.numberPad)
                        .focused($ageFocused)
                    Text("years")
                        .foregroundStyle(DSColors.textSecondary)
                }
            } header: {
                Text("Age")
            } footer: {
                Text("Leave blank to clear.")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Age")
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
            if ageText.isEmpty, let age = user.age {
                ageText = String(age)
            }
            ageFocused = true
        }
    }

    private func save() async {
        errorMessage = nil
        let trimmed = ageText.trimmingCharacters(in: .whitespaces)
        var age: Int? = nil
        if !trimmed.isEmpty {
            guard let value = Int(trimmed) else {
                errorMessage = "Age must be a whole number."
                return
            }
            guard value >= 10, value <= 120 else {
                errorMessage = "Age must be between 10 and 120."
                return
            }
            age = value
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateMeRequest(
                name: user.name,
                targetWeight: user.targetWeight,
                weightUnit: user.weightUnit,
                distanceUnit: user.distanceUnit,
                heightCm: user.heightCm,
                gender: user.gender,
                age: age
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your age."
        }
    }
}

#Preview {
    NavigationStack {
        AgeEditView(user: UserDTO(
            id: "u1",
            name: "Alice",
            email: "alice@example.com",
            isAdmin: false,
            weightUnit: "kg",
            distanceUnit: "km",
            targetWeight: 75.0,
            heightCm: nil,
            gender: "",
            age: 30
        ))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
