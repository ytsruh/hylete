import SwiftUI

/// Editor for the user's preferred distance unit. Renders as
/// a segmented picker so the choice is one tap. Save is
/// always enabled because the unit is always set to a
/// valid value (the picker has no "off" state). Mirrors
/// `WeightUnitEditView` — the server stores distances in
/// metres and this preference only controls display.
struct DistanceUnitEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var unit: String = "km"
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    private static let supportedUnits: [String] = ["km", "mi"]

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
                Text("Used everywhere distance and pace are shown: history, charts, exports etc.")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Distance unit")
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
                unit = Self.supportedUnits.contains(user.distanceUnit) ? user.distanceUnit : "km"
            }
        }
    }

    private func save() async {
        errorMessage = nil
        guard Self.supportedUnits.contains(unit) else {
            errorMessage = "Pick a supported distance unit."
            return
        }
        isSaving = true
        defer { isSaving = false }
        do {
            let request = UpdateMeRequest(
                name: user.name,
                targetWeight: user.targetWeight,
                weightUnit: user.weightUnit,
                distanceUnit: unit,
                heightCm: user.heightCm,
                gender: user.gender,
                dateOfBirth: user.dateOfBirth
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your distance unit."
        }
    }
}

#Preview {
    NavigationStack {
        DistanceUnitEditView(user: UserDTO(
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
