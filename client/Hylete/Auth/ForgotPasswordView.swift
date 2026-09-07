import SwiftUI

/// Requests a password-reset email. The emailed link opens the existing
/// web reset form, keeping reset-token handling and password changes in one
/// place on the server.
struct ForgotPasswordView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss
    @Environment(\.colorScheme) private var colorScheme

    @State private var email: String = ""
    @State private var isSubmitting: Bool = false
    @State private var errorMessage: String?
    @State private var didSubmit: Bool = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: DSSpacing.lg) {
                    VStack(alignment: .leading, spacing: DSSpacing.xs) {
                        Text("Forgot password?")
                            .font(.title.bold())
                            .foregroundStyle(DSColors.text)
                        Text("Enter your email and we'll send you a link to reset your password on the Hylete website.")
                            .font(.body)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    .padding(.top, DSSpacing.lg)

                    if didSubmit {
                        Text("If an account exists for that email, a reset link has been sent. Check your inbox and spam folder.")
                            .font(.body)
                            .foregroundStyle(DSColors.text)
                    } else {
                        VStack(alignment: .leading, spacing: DSSpacing.xxs) {
                            Text("Email")
                                .font(.subheadline.weight(.medium))
                                .foregroundStyle(DSColors.text)
                            ZStack(alignment: .leading) {
                                if email.isEmpty {
                                    Text("you@example.com")
                                    .foregroundStyle(DSColors.text)
                                }
                                TextField("", text: $email)
                                    .keyboardType(.emailAddress)
                                    .autocapitalization(.none)
                                    .autocorrectionDisabled(true)
                                    .textContentType(.emailAddress)
                                    .foregroundStyle(DSColors.text)
                            }
                                .padding(DSSpacing.sm)
                                .background(DSColors.surface)
                                .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
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
                                Text("Send reset link")
                            }
                        }
                        .buttonStyle(.dsPrimary)
                        .disabled(isSubmitting || email.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
                    }
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

    private func submit() async {
        errorMessage = nil
        isSubmitting = true
        defer { isSubmitting = false }

        do {
            try await env.api.requestPasswordReset(
                email: email.trimmingCharacters(in: .whitespacesAndNewlines)
            )
            didSubmit = true
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Something went wrong. Please try again."
        }
    }
}

#Preview {
    ForgotPasswordView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
