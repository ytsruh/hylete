import SwiftUI

/// The registration screen. Presented as a sheet from
/// `LoginView` so the user can back out without losing the
/// login form's state.
struct RegisterView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    @State private var name: String = ""
    @State private var email: String = ""
    @State private var password: String = ""
    @State private var confirmPassword: String = ""
    @State private var isSubmitting: Bool = false
    @State private var errorMessage: String?

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: DSSpacing.lg) {
                    VStack(alignment: .leading, spacing: DSSpacing.xs) {
                        Text("Create an account")
                            .font(.title.bold())
                            .foregroundStyle(DSColors.text)
                        Text("Track your sets, build a streak.")
                            .font(.body)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    .padding(.top, DSSpacing.lg)

                    VStack(alignment: .leading, spacing: DSSpacing.md) {
                        field("Name", $name, contentType: .name)
                        field("Email", $email, contentType: .emailAddress, keyboard: .emailAddress)
                        secureField("Password", $password, contentType: .newPassword)
                        secureField("Confirm password", $confirmPassword, contentType: .newPassword)
                    }

                    if let errorMessage {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }

                    Button {
                        Task { await submit() }
                    } label: {
                        if isSubmitting {
                            ProgressView().tint(.white)
                        } else {
                            Text("Create account")
                        }
                    }
                    .buttonStyle(.dsPrimary)
                    .disabled(isSubmitting || !canSubmit)
                }
                .padding(DSSpacing.lg)
            }
            .background(DSColors.background)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                }
            }
        }
    }

    private var canSubmit: Bool {
        !name.trimmingCharacters(in: .whitespaces).isEmpty &&
        !email.trimmingCharacters(in: .whitespaces).isEmpty &&
        password.count >= 6 &&
        password == confirmPassword
    }

    @ViewBuilder
    private func field(
        _ title: String,
        _ text: Binding<String>,
        contentType: UITextContentType? = nil,
        keyboard: UIKeyboardType = .default
    ) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xxs) {
            Text(title)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(DSColors.textSecondary)
            TextField("", text: text)
                .keyboardType(keyboard)
                .autocapitalization(contentType == .emailAddress ? .none : .sentences)
                .autocorrectionDisabled(contentType == .emailAddress)
                .textContentType(contentType)
                .padding(DSSpacing.sm)
                .background(DSColors.surface)
                .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
        }
    }

    @ViewBuilder
    private func secureField(
        _ title: String,
        _ text: Binding<String>,
        contentType: UITextContentType?
    ) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xxs) {
            Text(title)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(DSColors.textSecondary)
            SecureField("", text: text)
                .textContentType(contentType)
                .padding(DSSpacing.sm)
                .background(DSColors.surface)
                .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
        }
    }

    private func submit() async {
        errorMessage = nil
        guard password == confirmPassword else {
            errorMessage = "Passwords do not match."
            return
        }
        isSubmitting = true
        defer { isSubmitting = false }
        do {
            let response = try await env.api.register(
                name: name.trimmingCharacters(in: .whitespacesAndNewlines),
                email: email.trimmingCharacters(in: .whitespacesAndNewlines),
                password: password
            )
            env.authStore.acceptSession(token: response.token, user: response.user)
            dismiss()
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Something went wrong. Please try again."
        }
    }
}

#Preview {
    RegisterView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
