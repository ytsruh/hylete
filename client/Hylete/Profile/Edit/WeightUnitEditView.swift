import SwiftUI

/// Editor for the user's preferred weight unit. Renders as
/// a segmented picker so the choice is one tap. Save is
/// always enabled because the unit is always set to a
/// valid value (the picker has no "off" state).
struct WeightUnitEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var unit: String = "kg"
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    private static let supportedUnits: [String] = ["kg", "lbs"]

    private var canSave: Bool {
        !isSaving && Self.supportedUnits.contains(unit)
    }

    var body: some View {
        Form {
            Section {
                Picker("Unit", selection: $unit) {
                    ForEach(Self.supportedUnits, id: \.self) { value in
                        Text(value).tag(value)
                    }
                }
                .pickerStyle(.segmented)
            } header: {
                Text("Preferred unit")
            } footer: {
                Text("Used everywhere your weight is shown: dashboard, charts, exports.")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Weight unit")
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
            // Seed only on first appear so a mid-edit
            // re-render doesn't snap the picker back to
            // the stored value. Falls back to the current
            // value if the stored value is somehow not in
            // the supported set.
            if !Self.supportedUnits.contains(unit) {
                unit = Self.supportedUnits.contains(user.weightUnit) ? user.weightUnit : "kg"
            }
        }
    }

    private func save() async {
        errorMessage = nil
        guard Self.supportedUnits.contains(unit) else {
            errorMessage = "Pick a supported weight unit."
            return
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateMeRequest(
                name: user.name,
                targetWeight: user.targetWeight,
                weightUnit: unit,
                distanceUnit: user.distanceUnit
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your weight unit."
        }
    }
}

#Preview {
    NavigationStack {
        WeightUnitEditView(user: UserDTO(
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
