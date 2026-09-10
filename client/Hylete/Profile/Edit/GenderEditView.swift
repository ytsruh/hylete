import SwiftUI

/// Editor for the user's gender. Renders the fixed server-side set
/// plus a "Not set" row that clears the value (the server stores ""
/// for unset). Save is always enabled because the picker always
/// holds a valid value.
struct GenderEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var gender: String = ""
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    /// Picker options: "" (Not set) + the fixed server set, in the
    /// same order as `ValidGenders` on the server.
    private static let options: [String] = ["", "male", "female", "non-binary", "prefer-not-to-say"]

    private static func label(for value: String) -> String {
        switch value {
        case "": return "Not set"
        case "male": return "Male"
        case "female": return "Female"
        case "non-binary": return "Non-binary"
        case "prefer-not-to-say": return "Prefer not to say"
        default: return value
        }
    }

    private var canSave: Bool {
        !isSaving && Self.options.contains(gender)
    }

    var body: some View {
        Form {
            Section {
                Picker("Gender", selection: $gender) {
                    ForEach(Self.options, id: \.self) { value in
                        Text(Self.label(for: value)).tag(value)
                    }
                }
                .pickerStyle(.inline)
            } header: {
                Text("Gender")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Gender")
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
            if gender.isEmpty, Self.options.contains(user.gender) {
                gender = user.gender
            }
        }
    }

    private func save() async {
        errorMessage = nil
        guard Self.options.contains(gender) else {
            errorMessage = "Pick a valid option."
            return
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
                gender: gender,
                dateOfBirth: user.dateOfBirth
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your gender."
        }
    }
}

#Preview {
    NavigationStack {
        GenderEditView(user: UserDTO(
            id: "u1",
            name: "Alice",
            email: "alice@example.com",
            isAdmin: false,
            weightUnit: "kg",
            distanceUnit: "km",
            targetWeight: 75.0,
            heightCm: nil,
            gender: "female",
            dateOfBirth: nil
        ))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
