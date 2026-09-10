import SwiftUI

/// Editor for the user's height in centimetres. The underlying
/// value is a `Double?` on the server (an empty field clears the
/// height), so the form binds to a `String` and only converts to
/// a number on save — the same pattern as `TargetWeightEditView`.
struct HeightEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var heightText: String = ""
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    @FocusState private var heightFocused: Bool

    private var parsedHeight: Double? {
        let trimmed = heightText.trimmingCharacters(in: .whitespaces)
        return trimmed.isEmpty ? nil : Double(trimmed)
    }

    private var canSave: Bool {
        guard !isSaving else { return false }
        let trimmed = heightText.trimmingCharacters(in: .whitespaces)
        if trimmed.isEmpty { return true }
        guard let value = Double(trimmed) else { return false }
        return value >= 0 && value <= 300
    }

    var body: some View {
        Form {
            Section {
                HStack {
                    TextField("180.0", text: $heightText)
                        .keyboardType(.decimalPad)
                        .focused($heightFocused)
                    Text("cm")
                        .foregroundStyle(DSColors.textSecondary)
                }
            } header: {
                Text("Height")
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
        .navigationTitle("Height")
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
            if heightText.isEmpty, let height = user.heightCm {
                heightText = String(format: "%.1f", height)
            }
            heightFocused = true
        }
    }

    private func save() async {
        errorMessage = nil
        let trimmed = heightText.trimmingCharacters(in: .whitespaces)
        var height: Double? = nil
        if !trimmed.isEmpty {
            guard let value = Double(trimmed) else {
                errorMessage = "Height must be a number."
                return
            }
            guard value >= 0, value <= 300 else {
                errorMessage = "Height must be between 0 and 300 cm."
                return
            }
            height = value
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateMeRequest(
                name: user.name,
                targetWeight: user.targetWeight,
                weightUnit: user.weightUnit,
                distanceUnit: user.distanceUnit,
                heightCm: height,
                gender: user.gender,
                dateOfBirth: user.dateOfBirth
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your height."
        }
    }
}

#Preview {
    NavigationStack {
        HeightEditView(user: UserDTO(
            id: "u1",
            name: "Alice",
            email: "alice@example.com",
            isAdmin: false,
            weightUnit: "kg",
            distanceUnit: "km",
            targetWeight: 75.0,
            heightCm: 170.0,
            gender: "",
            dateOfBirth: nil
        ))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
