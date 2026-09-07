import SwiftUI

/// The timer sheet. A segmented picker at the top toggles
/// between the countdown and EMOM modes; the chosen mode's
/// sub-view fills the rest of the sheet. Switching modes
/// destroys the inactive sub-view's `@State` (matching the
/// web's full-page tab navigation), so an in-flight timer
/// in the other mode is automatically abandoned and its
/// wake lock released via the sub-view's `onDisappear`.
struct TimerView: View {
    @Environment(\.dismiss) private var dismiss
    @State private var mode: TimerMode = .timer

    var body: some View {
        NavigationStack {
            VStack(spacing: DSSpacing.md) {
                Picker("Mode", selection: $mode) {
                    ForEach(TimerMode.allCases) { mode in
                        Text(mode.rawValue).tag(mode)
                    }
                }
                .pickerStyle(.segmented)
                .padding(.horizontal, DSSpacing.md)
                .padding(.top, DSSpacing.xs)

                Group {
                    switch mode {
                    case .timer:
                        CountdownTimerView()
                    case .emom:
                        EMOMView()
                    }
                }
                .frame(maxWidth: .infinity, maxHeight: .infinity)
            }
            .background(DSColors.background.ignoresSafeArea())
            .navigationTitle("Timer")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
        }
    }
}

#Preview {
    TimerView()
}
