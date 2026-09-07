import SwiftUI

/// The EMOM (Every Minute On the Minute) mode of the timer
/// sheet. Mirrors the web's `internal/views/timers/emom.templ`:
/// preset round chips (3 / 5 / 7 / 10), a custom-rounds input
/// (1–15), a 60-second countdown per round (hardcoded to match
/// the web), and a "Round X/Y" header that auto-advances on
/// each round boundary.
///
/// The completion check is a `.task(id: endDate)` that sleeps
/// until the current round's end-date and then re-schedules
/// itself for the next round (or transitions to the complete
/// state on the final round). This keeps the state machine
/// self-driving without polling.
struct EMOMView: View {
    /// Hardcoded per the web — every EMOM round is 60
    /// seconds long, with no UI to change it.
    private static let roundDuration: TimeInterval = 60
    /// Preset round counts. Matches the web's chip row.
    private let presets: [Int] = [3, 5, 7, 10]
    /// Inclusive bounds for the custom round count. Matches
    /// the web's `<input min="1" max="15">`.
    private let minCustom = 1
    private let maxCustom = 15

    @State private var totalRounds: Int = 0
    @State private var currentRound: Int = 1
    @State private var endDate: Date?
    @State private var isRunning: Bool = false
    @State private var hasStarted: Bool = false
    @State private var isComplete: Bool = false
    @State private var customInput: String = ""
    @State private var customError: String?

    var body: some View {
        VStack(spacing: DSSpacing.lg) {
            if isComplete {
                Spacer()
                completeState
                Spacer()
                startAnotherButton
            } else {
                Spacer()
                roundHeader
                display
                Spacer()
                if hasStarted {
                    runningControls
                } else {
                    idleControls
                }
            }
        }
        .padding(DSSpacing.lg)
        .onDisappear { TimerWakeLock.release() }
        .task(id: endDate) {
            // Self-rescheduling task. When the current
            // round's end-date passes, either advance to
            // the next round (re-setting endDate, which
            // re-fires this task) or transition to the
            // complete state on the final round. The two
            // guards cover pause/reset (`Task.isCancelled`)
            // and a new session starting on top of a stale
            // wake-up (`endDate == target`).
            guard let target = endDate, isRunning else { return }
            let interval = target.timeIntervalSinceNow
            if interval > 0 {
                try? await Task.sleep(for: .seconds(interval))
            }
            guard !Task.isCancelled,
                  isRunning,
                  endDate == target else { return }
            advanceRound()
        }
    }

    // MARK: - Idle UI (presets + custom input)

    private var idleControls: some View {
        VStack(spacing: DSSpacing.lg) {
            presetsRow
            VStack(spacing: DSSpacing.xs) {
                Text("Custom rounds (max 15)")
                    .font(.caption)
                    .foregroundStyle(DSColors.textSecondary)
                HStack(spacing: DSSpacing.sm) {
                    TextField("10", text: $customInput)
                        .keyboardType(.numberPad)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 120)
                        .multilineTextAlignment(.center)
                        .onChange(of: customInput) { _, _ in customError = nil }
                    Button("Start") {
                        startCustom()
                    }
                    .buttonStyle(.dsPrimary)
                    .frame(maxWidth: 140)
                    .disabled(parsedCustomRounds == nil)
                }
                if let customError {
                    Text(customError)
                        .font(.caption)
                        .foregroundStyle(DSColors.destructive)
                }
            }
        }
    }

    private var presetsRow: some View {
        HStack(spacing: DSSpacing.xs) {
            ForEach(presets, id: \.self) { rounds in
                Button {
                    totalRounds = rounds
                    start()
                } label: {
                    Text("\(rounds) Rounds")
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(DSColors.onPrimary)
                        .frame(maxWidth: .infinity)
                        .padding(.vertical, DSSpacing.sm)
                        .background(
                            RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                                .fill(DSColors.accent)
                        )
                }
                .buttonStyle(.plain)
            }
        }
    }

    // MARK: - Running UI (display + controls)

    private var roundHeader: some View {
        Text("Round \(currentRound)/\(totalRounds)")
            .font(.title2.weight(.semibold))
            .foregroundStyle(DSColors.textSecondary)
    }

    private var display: some View {
        TimelineView(.periodic(from: .now, by: 0.1)) { context in
            Text(remainingString(at: context.date))
                .font(.system(size: 84, weight: .semibold, design: .rounded).monospacedDigit())
                .foregroundStyle(DSColors.text)
                .contentTransition(.numericText())
        }
    }

    private var runningControls: some View {
        HStack(spacing: DSSpacing.md) {
            Button(role: .destructive) {
                reset()
            } label: {
                Label("Reset", systemImage: "arrow.counterclockwise")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.dsSecondary)

            Button {
                isRunning ? pause() : resume()
            } label: {
                Label(isRunning ? "Pause" : "Resume", systemImage: isRunning ? "pause.fill" : "play.fill")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.dsPrimary)
        }
    }

    // MARK: - Complete UI

    private var completeState: some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: "checkmark.circle.fill")
                .font(.system(size: 72))
                .foregroundStyle(DSColors.accent)
            Text("Complete!")
                .font(.title.weight(.bold))
                .foregroundStyle(DSColors.accent)
            Text("\(totalRounds) rounds")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
        }
    }

    private var startAnotherButton: some View {
        Button {
            reset()
        } label: {
            Label("Start Another", systemImage: "arrow.counterclockwise")
                .frame(maxWidth: .infinity)
        }
        .buttonStyle(.dsSecondary)
    }

    // MARK: - State machine

    private func start() {
        guard totalRounds > 0 else { return }
        currentRound = 1
        endDate = Date().addingTimeInterval(Self.roundDuration)
        isRunning = true
        hasStarted = true
        isComplete = false
        TimerWakeLock.acquire()
    }

    private func pause() {
        isRunning = false
        endDate = nil
        TimerWakeLock.release()
    }

    private func resume() {
        isRunning = true
        endDate = Date().addingTimeInterval(Self.roundDuration)
        TimerWakeLock.acquire()
    }

    private func reset() {
        endDate = nil
        isRunning = false
        hasStarted = false
        isComplete = false
        currentRound = 1
        totalRounds = 0
        customInput = ""
        customError = nil
        TimerWakeLock.release()
    }

    private func advanceRound() {
        if currentRound < totalRounds {
            currentRound += 1
            endDate = Date().addingTimeInterval(Self.roundDuration)
            TimerFeedback.roundBoundary()
        } else {
            // Final round complete.
            endDate = nil
            isRunning = false
            isComplete = true
            TimerWakeLock.release()
            TimerFeedback.sessionComplete()
        }
    }

    private func startCustom() {
        guard let rounds = parsedCustomRounds else {
            customError = "Enter a number between \(minCustom) and \(maxCustom)."
            return
        }
        totalRounds = rounds
        customInput = "\(rounds)"
        customError = nil
        start()
    }

    private var parsedCustomRounds: Int? {
        let trimmed = customInput.trimmingCharacters(in: .whitespaces)
        guard let value = Int(trimmed),
              (minCustom...maxCustom).contains(value) else {
            return nil
        }
        return value
    }

    // MARK: - Formatting

    private func remainingString(at now: Date) -> String {
        let seconds: Int
        if let endDate {
            seconds = max(0, Int(endDate.timeIntervalSince(now).rounded(.up)))
        } else {
            seconds = Int(Self.roundDuration)
        }
        return formatDuration(seconds)
    }

    private func formatDuration(_ seconds: Int) -> String {
        let minutes = seconds / 60
        let rem = seconds % 60
        return String(format: "%d:%02d", minutes, rem)
    }
}

#Preview {
    EMOMView()
        .padding()
        .background(DSColors.background)
}
