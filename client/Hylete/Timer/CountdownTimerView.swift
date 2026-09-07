import SwiftUI

/// The countdown-timer mode of the timer sheet. Mirrors
/// the web's `internal/views/timers/timer.templ`: three
/// preset chips (30s / 60s / 90s), a custom-duration input
/// (1–300 seconds), a big `MM:SS` display, and Start /
/// Pause / Reset controls. Reaches the "Timer Complete!"
/// state when the countdown hits zero.
///
/// All timing is anchored to an absolute `endDate: Date?`
/// rather than a relative counter, so the display stays
/// correct if the user backgrounds the app or the
/// TimelineView ticks late — every redraw recomputes from
/// `Date()`. The completion check is a `.task(id: endDate)`
/// sleep so the haptic + audio cue fire at the right moment
/// even if no frame is being drawn.
struct CountdownTimerView: View {
    /// Preset durations in seconds. Matches the web's
    /// `30s / 60s / 90s` chips.
    private let presets: [Int] = [30, 60, 90]
    /// Inclusive bounds for the custom duration. Matches
    /// the web's `<input min="1" max="300">`.
    private let minCustom = 1
    private let maxCustom = 300

    @State private var selectedDuration: Int = 60
    @State private var customInput: String = ""
    @State private var endDate: Date?
    @State private var isRunning: Bool = false
    @State private var hasStarted: Bool = false
    @State private var isComplete: Bool = false
    @State private var customError: String?

    var body: some View {
        VStack(spacing: DSSpacing.lg) {
            Spacer()
            if isComplete {
                completeState
            } else {
                display
            }
            Spacer()
            if isComplete {
                startAnotherButton
            } else if hasStarted {
                runningControls
            } else {
                idleControls
            }
        }
        .padding(DSSpacing.lg)
        .onDisappear { TimerWakeLock.release() }
        .task(id: endDate) {
            // Sleeps until the running timer is due, then
            // transitions to the complete state. The
            // `Task.isCancelled` check covers pause / reset
            // paths that null out `endDate` before the date
            // is reached; the `endDate ==` re-check covers
            // the case where the user starts a *new* timer
            // before the old one's task wakes up.
            guard let target = endDate, isRunning else { return }
            let interval = target.timeIntervalSinceNow
            if interval > 0 {
                try? await Task.sleep(for: .seconds(interval))
            }
            guard !Task.isCancelled,
                  isRunning,
                  endDate == target else { return }
            completeNow()
        }
    }

    // MARK: - Idle UI (presets + custom input)

    private var idleControls: some View {
        VStack(spacing: DSSpacing.lg) {
            presetsRow
            VStack(spacing: DSSpacing.xs) {
                Text("Custom (seconds)")
                    .font(.caption)
                    .foregroundStyle(DSColors.textSecondary)
                HStack(spacing: DSSpacing.sm) {
                    TextField("30", text: $customInput)
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
                    .disabled(parsedCustomDuration == nil)
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
            ForEach(presets, id: \.self) { seconds in
                Button {
                    selectedDuration = seconds
                    start()
                } label: {
                    Text(formatPreset(seconds))
                        .font(.headline)
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
            Text("Timer Complete!")
                .font(.title.weight(.bold))
                .foregroundStyle(DSColors.accent)
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
        endDate = Date().addingTimeInterval(TimeInterval(selectedDuration))
        isRunning = true
        hasStarted = true
        isComplete = false
        TimerWakeLock.acquire()
    }

    private func pause() {
        guard let endDate else { return }
        let remaining = max(0, endDate.timeIntervalSinceNow)
        selectedDuration = Int(remaining.rounded(.up))
        self.endDate = nil
        isRunning = false
        TimerWakeLock.release()
    }

    private func resume() {
        endDate = Date().addingTimeInterval(TimeInterval(selectedDuration))
        isRunning = true
        isComplete = false
        TimerWakeLock.acquire()
    }

    private func reset() {
        endDate = nil
        isRunning = false
        hasStarted = false
        isComplete = false
        customInput = ""
        customError = nil
        TimerWakeLock.release()
    }

    private func completeNow() {
        isRunning = false
        endDate = nil
        hasStarted = true
        isComplete = true
        TimerWakeLock.release()
        TimerFeedback.sessionComplete()
    }

    private func startCustom() {
        guard let duration = parsedCustomDuration else {
            customError = "Enter a number between \(minCustom) and \(maxCustom)."
            return
        }
        selectedDuration = duration
        customInput = "\(duration)"
        customError = nil
        start()
    }

    private var parsedCustomDuration: Int? {
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
            seconds = selectedDuration
        }
        return formatDuration(seconds)
    }

    private func formatPreset(_ seconds: Int) -> String {
        if seconds < 60 { return "\(seconds)s" }
        let minutes = seconds / 60
        let rem = seconds % 60
        return rem == 0 ? "\(minutes)m" : "\(minutes):\(String(format: "%02d", rem))"
    }

    private func formatDuration(_ seconds: Int) -> String {
        let minutes = seconds / 60
        let rem = seconds % 60
        return String(format: "%d:%02d", minutes, rem)
    }
}

#Preview {
    CountdownTimerView()
        .padding()
        .background(DSColors.background)
}
