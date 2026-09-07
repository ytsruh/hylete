import SwiftUI

/// The login screen. Pushed as the root view when no
/// session is active; presented modally from the profile tab
/// after sign-out so the user can log in as a different
/// account without relaunching the app.
struct LoginView: View {
    @EnvironmentObject private var env: AppEnvironment

    @State private var email: String = ""
    @State private var password: String = ""
    @State private var isSubmitting: Bool = false
    @State private var errorMessage: String?

    @State private var showingRegister: Bool = false
    @State private var showingForgotPassword: Bool = false

    var body: some View {
        NavigationStack {
            ScrollView {
                VStack(alignment: .leading, spacing: DSSpacing.lg) {
                    header

                    VStack(alignment: .leading, spacing: DSSpacing.md) {
                        labeledField(title: "Email", text: $email, contentType: .emailAddress, keyboard: .emailAddress)
                        labeledField(title: "Password", text: $password, contentType: .password, isSecure: true)

                        Button("Forgot password?") {
                            showingForgotPassword = true
                        }
                        .font(.footnote)
                        .foregroundStyle(DSColors.accent)
                    }

                    if let errorMessage {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                            .multilineTextAlignment(.leading)
                    }

                    Button {
                        Task { await submit() }
                    } label: {
                        if isSubmitting {
                            ProgressView()
                                .tint(.white)
                        } else {
                            Text("Log in")
                        }
                    }
                    .buttonStyle(.dsPrimary)
                    .disabled(isSubmitting || !canSubmit)

                    Button("Create an account") {
                        showingRegister = true
                    }
                    .buttonStyle(.dsSecondary)
                }
                .padding(DSSpacing.lg)
            }
            .background(DSColors.background)
            .navigationBarHidden(true)
        }
        .sheet(isPresented: $showingRegister) {
            RegisterView()
                .environmentObject(env)
        }
        .sheet(isPresented: $showingForgotPassword) {
            ForgotPasswordView()
                .environmentObject(env)
        }
    }

    private var header: some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text("Hylete")
                .font(.largeTitle.bold())
                .foregroundStyle(DSColors.text)
            Text("Track your lifts.")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(.top, DSSpacing.xxl)
    }

    private var canSubmit: Bool {
        !email.trimmingCharacters(in: .whitespaces).isEmpty &&
        !password.isEmpty
    }

    @ViewBuilder
    private func labeledField(
        title: String,
        text: Binding<String>,
        contentType: UITextContentType? = nil,
        keyboard: UIKeyboardType = .default,
        isSecure: Bool = false
    ) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xxs) {
            Text(title)
                .font(.subheadline.weight(.medium))
                .foregroundStyle(DSColors.textSecondary)
            Group {
                if isSecure {
                    SecureField("", text: text)
                } else {
                    TextField("", text: text)
                        .keyboardType(keyboard)
                        .autocapitalization(.none)
                        .autocorrectionDisabled(true)
                }
            }
            .textContentType(contentType)
            .padding(DSSpacing.sm)
            .background(DSColors.surface)
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
        }
    }

    private func submit() async {
        errorMessage = nil
        isSubmitting = true
        defer { isSubmitting = false }
        do {
            let response = try await env.api.login(
                email: email.trimmingCharacters(in: .whitespacesAndNewlines),
                password: password
            )
            env.authStore.acceptSession(token: response.token, user: response.user)
        } catch let error as APIError {
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Something went wrong. Please try again."
        }
    }
}

#Preview {
    LoginView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
