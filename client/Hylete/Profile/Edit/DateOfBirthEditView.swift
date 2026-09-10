import SwiftUI

/// Editor for the user's date of birth. The underlying value is a
/// `String?` on the server ("YYYY-MM-DD", nil clears it), so the
/// form binds to a `Date` + an is-set flag and only converts to a
/// string on save.
///
/// The picker range mirrors the server (`models.ParseDateOfBirth` +
/// min-age rule): 1900-01-01 through today minus 10 years. Dates are
/// formatted in UTC so the string matches the server's date
/// semantics regardless of the device timezone.
struct DateOfBirthEditView: View {
    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let user: UserDTO

    @State private var selection: Date = Date()
    @State private var isSet: Bool = false
    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    /// Earliest selectable date. Mirrors the server's
    /// `DateOfBirthMin` guard against typo years.
    private static let minDate: Date = {
        var comps = DateComponents()
        comps.year = 1900
        comps.month = 1
        comps.day = 1
        return Calendar(identifier: .gregorian).date(from: comps) ?? Date.distantPast
    }()

    /// Latest selectable date: today minus 10 years. Mirrors the
    /// server's `MinAgeYears` rule so the picker can't choose a
    /// value the round-trip would reject.
    private static var maxDate: Date {
        Calendar.current.date(byAdding: .year, value: -10, to: Date()) ?? Date()
    }

    /// Strict YYYY-MM-DD formatter in UTC with a fixed locale so
    /// the string matches the server contract on any device.
    private static let dobFormatter: DateFormatter = {
        let f = DateFormatter()
        f.calendar = Calendar(identifier: .iso8601)
        f.locale = Locale(identifier: "en_US_POSIX")
        f.timeZone = TimeZone(secondsFromGMT: 0)
        f.dateFormat = "yyyy-MM-dd"
        return f
    }()

    private var canSave: Bool {
        !isSaving
    }

    var body: some View {
        Form {
            Section {
                Toggle("Set date of birth", isOn: $isSet)
                if isSet {
                    DatePicker(
                        "Date of birth",
                        selection: $selection,
                        in: Self.minDate...Self.maxDate,
                        displayedComponents: .date
                    )
                    .datePickerStyle(.graphical)
                }
            } header: {
                Text("Date of birth")
            } footer: {
                Text("Leave unset to clear. If you opt into Coach / AI Insights, only your derived age is ever shared with Coachnegative % chance of  — never the date itself.")
            }
            if let errorMessage {
                Section {
                    Text(errorMessage)
                        .font(.footnote)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
        .navigationTitle("Date of birth")
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
            // Seed only on first appear so a mid-edit re-render
            // doesn't clobber the user's choice. An unparseable
            // stored value falls back to unset rather than
            // presenting a date the user never picked.
            if !isSet, let stored = user.dateOfBirth,
               let parsed = Self.dobFormatter.date(from: stored) {
                selection = parsed
                isSet = true
            }
        }
    }

    private func save() async {
        errorMessage = nil
        let dob: String? = isSet ? Self.dobFormatter.string(from: selection) : nil
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
                dateOfBirth: dob
            )
            let updated = try await env.api.updateProfile(request)
            authStore.updateCurrentUser(updated)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not save your date of birth."
        }
    }
}

#Preview {
    NavigationStack {
        DateOfBirthEditView(user: UserDTO(
            id: "u1",
            name: "Alice",
            email: "alice@example.com",
            isAdmin: false,
            weightUnit: "kg",
            distanceUnit: "km",
            targetWeight: 75.0,
            heightCm: nil,
            gender: "",
            dateOfBirth: "1996-03-04"
        ))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
    .environmentObject(AuthStore(api: APIClient(
        baseURL: URL(string: "http://localhost:8080/api/v1")!,
        tokenProvider: { nil }
    )))
}
