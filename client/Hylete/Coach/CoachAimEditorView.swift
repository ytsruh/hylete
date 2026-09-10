import SwiftUI

/// Editor sheet for the Coach consent state: the server-side
/// opt-in toggle plus the free-text training aim.
///
/// The toggle IS the consent — flipping it on lets the server
/// analyse training with an AI model; flipping it off stops all
/// generation (the cron skips opted-out users, the API 403s).
/// The aim is capped at `CoachPreferencesDTO.maxGoalTextLength`
/// chars (mirroring the server's 400) with a live counter; Save
/// stays disabled while over the limit or while saving.
public struct CoachAimEditorView: View {
    @ObservedObject var store: CoachStore
    @Environment(\.dismiss) private var dismiss

    @State private var optIn: Bool
    @State private var goalText: String
    @State private var isSaving = false

    public init(store: CoachStore) {
        self.store = store
        _optIn = State(initialValue: store.preferences?.optIn ?? false)
        _goalText = State(initialValue: store.preferences?.goalText ?? "")
    }

    public var body: some View {
        Form {
            Section {
                Toggle("Enable", isOn: $optIn)
            } footer: {
                Text("Weekly AI review of your training, every Monday. Off means no analysis and no data sent to the model.")
            }

            Section {
                TextEditor(text: $goalText)
                    .frame(minHeight: 120)
                    .accessibilityLabel("What are you trying to achieve?")
                HStack {
                    Spacer()
                    Text("\(goalText.count)/\(CoachPreferencesDTO.maxGoalTextLength)")
                        .font(.caption)
                        .foregroundStyle(overLimit ? Color.red : DSColors.textSecondary)
                }
            } header: {
                Text("Your aim")
            } footer: {
                Text("In your own words, e.g. \"Bench 100 kg by December while keeping bodyweight stable\". Empty is fine — Coach falls back to general progression.")
            }

            if let error = store.errorMessage {
                Section {
                    Text(error)
                        .foregroundStyle(Color.red)
                }
            }
        }
        .navigationTitle("Coach settings")
        .navigationBarTitleDisplayMode(.inline)
        .toolbar {
            ToolbarItem(placement: .confirmationAction) {
                Button(isSaving ? "Saving…" : "Save") {
                    Task {
                        isSaving = true
                        await store.savePreferences(optIn: optIn, goalText: goalText)
                        isSaving = false
                        if store.errorMessage == nil {
                            dismiss()
                        }
                    }
                }
                .disabled(overLimit || isSaving)
            }
            ToolbarItem(placement: .cancellationAction) {
                Button("Cancel") { dismiss() }
            }
        }
    }

    private var overLimit: Bool {
        goalText.trimmingCharacters(in: .whitespacesAndNewlines).count
            > CoachPreferencesDTO.maxGoalTextLength
    }
}
