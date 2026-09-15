import SwiftUI
import UIKit

/// Block-scoped setup context for the player's timer sheet.
/// Carries the tapped block's plan config so the custom form
/// prefills from it (plan rest / rounds × interval / cap) and
/// quick-starts label the timer with the block's name.
struct PlayerTimerSetupContext: Equatable {
    let blockID: String?
    let blockName: String
    let blockType: BlockTypeDTO
    let restSeconds: Int
    let rounds: Int
    let intervalSeconds: Int
    let timeCapSeconds: Int

    init(block: WorkoutBlockDetailDTO) {
        blockID = block.id
        blockName = block.blockName
        blockType = block.blockType
        restSeconds = block.restSeconds
        rounds = block.rounds
        intervalSeconds = block.intervalSeconds
        timeCapSeconds = block.timeCapSeconds
    }
}

// MARK: - Sticky pill

/// Compact live timer pinned to the top of the player while
/// `PlayerTimerStore.isActive`. Shows the subtitle (kind +
/// round + block), a live `M:SS` countdown, pause/resume and
/// dismiss controls; tapping elsewhere expands the sheet.
///
/// The sleeper `.task(id: endDate)` is the store's single
/// driver: it sleeps until the fire date, then advances the
/// round (EMOM) or completes, firing the matching
/// `TimerFeedback` cue. Re-arms automatically on EMOM round
/// boundaries because `advanceRound` sets a new `endDate`.
/// Pausing nils `endDate`, which cancels the task via the
/// `guard`.
///
/// Visual feedback rides on `store.lastEvent`: every round
/// boundary flashes the pill subtly, swaps the countdown for
/// a temporary "Round X — go!" callout, announces via
/// VoiceOver, and auto-expands the sheet on completion (the
/// caller's `onExpand`). Auto-expand is safe when the sheet
/// is already open (re-presenting is a no-op) and can't fire
/// after a reset — the task's guards fail first.
///
/// The pill carries an explicit expand button alongside the
/// tap gesture (same `onExpand`); the sheet answers with a
/// minimize button next to Done.
struct PlayerTimerPill: View {
    @ObservedObject var store: PlayerTimerStore
    var onExpand: () -> Void

    /// Accent-wash opacity for the event flash. Set to the
    /// event's strength, then animated back to zero.
    @State private var flashOpacity: Double = 0
    /// Temporary round-boundary callout replacing the
    /// countdown. Cleared by the generation-keyed `.task`
    /// below, so rapid rounds can't leave a stale callout up
    /// or clear a fresh one early.
    @State private var interstitialText: String?
    @State private var interstitialGeneration = 0

    var body: some View {
        HStack(spacing: DSSpacing.sm) {
            Image(systemName: Icons.timer)
                .foregroundStyle(DSColors.accent)
                .font(.body.weight(.semibold))
            VStack(alignment: .leading, spacing: 2) {
                Text(store.subtitle)
                    .font(.caption.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                if store.isComplete {
                    Text("Timer Complete!")
                        .font(.headline)
                        .foregroundStyle(DSColors.accent)
                } else if let interstitialText {
                    Text(interstitialText)
                        .font(.headline)
                        .foregroundStyle(DSColors.accent)
                } else {
                    TimelineView(.periodic(from: .now, by: 0.5)) { context in
                        Text(store.displayString(at: context.date))
                            .font(.headline.monospacedDigit())
                            .foregroundStyle(DSColors.text)
                            .contentTransition(.numericText())
                    }
                }
            }
            .animation(.easeInOut(duration: 0.2), value: interstitialText)
            Spacer()
            if !store.isComplete {
                Button {
                    if store.isRunning {
                        store.pause()
                    } else {
                        store.resume()
                    }
                } label: {
                    Image(systemName: store.isRunning ? "pause.fill" : "play.fill")
                        .font(.body.weight(.semibold))
                        .foregroundStyle(DSColors.accent)
                        .frame(width: 32, height: 32)
                }
                .buttonStyle(.plain)
                .accessibilityLabel(store.isRunning ? "Pause timer" : "Resume timer")
            }
            Button {
                onExpand()
            } label: {
                Image(systemName: "arrow.up.right")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Expand timer")
            .accessibilityHint("Opens the full timer view")
            Button {
                store.reset()
            } label: {
                Image(systemName: "xmark")
                    .font(.body.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(width: 32, height: 32)
            }
            .buttonStyle(.plain)
            .accessibilityLabel("Dismiss timer")
        }
        .padding(.horizontal, DSSpacing.md)
        .padding(.vertical, DSSpacing.sm)
        .background(DSColors.surface)
        .overlay(DSColors.accent.opacity(flashOpacity))
        .overlay(Divider().background(DSColors.separator), alignment: .bottom)
        .contentShape(Rectangle())
        .onTapGesture { onExpand() }
        .accessibilityElement(children: .combine)
        .accessibilityLabel("\(store.subtitle), \(store.displayString())")
        .accessibilityHint("Opens the timer")
        .onChange(of: store.lastEvent) { _, event in
            guard let event else { return }
            switch event {
            case .roundBoundary:
                flash(strength: 0.25)
                interstitialText = PlayerTimerStore.boundaryCallout(round: store.currentRound)
                interstitialGeneration += 1
                UIAccessibility.post(
                    notification: .announcement,
                    argument: "Round \(store.currentRound) of \(store.totalRounds)"
                )
            case .sessionComplete:
                flash(strength: 0.45)
                UIAccessibility.post(notification: .announcement, argument: "Timer complete")
                onExpand()
            }
        }
        // Clears the round callout a few seconds after it
        // appears. Keyed on the generation counter so each
        // boundary restarts the countdown; unmounting (reset)
        // cancels it outright.
        .task(id: interstitialGeneration) {
            guard interstitialText != nil else { return }
            try? await Task.sleep(for: .seconds(3))
            guard !Task.isCancelled else { return }
            interstitialText = nil
        }
        .task(id: store.endDate) {
            guard let target = store.endDate, store.isRunning else { return }
            let interval = target.timeIntervalSinceNow
            if interval > 0 {
                try? await Task.sleep(for: .seconds(interval))
            }
            guard !Task.isCancelled,
                  store.isRunning,
                  store.endDate == target else { return }
            if store.kind == .emom {
                let wasLast = store.currentRound >= store.totalRounds
                guard store.advanceRound() else { return }
                if store.isComplete || wasLast {
                    TimerFeedback.sessionComplete()
                } else {
                    TimerFeedback.roundBoundary()
                }
            } else {
                guard store.completeNow() else { return }
                TimerFeedback.sessionComplete()
            }
        }
    }

    /// Briefly washes the pill in accent, then fades. The
    /// completion wash is stronger than the round-boundary
    /// one so the two are distinguishable at a glance.
    private func flash(strength: Double) {
        flashOpacity = strength
        withAnimation(.easeOut(duration: 0.8)) {
            flashOpacity = 0
        }
    }
}

// MARK: - Expanded sheet

/// Full timer sheet for the player, bound to the shared
/// `PlayerTimerStore` (unlike the standalone `TimerView`,
/// which owns local state — the two must never run side by
/// side, so the player presents only this sheet).
///
/// Active timers render a big display with pause/resume +
/// reset; an idle store (fresh "Custom…" or after "Start
/// another") renders the manual setup form — rest / EMOM /
/// AMRAP side by side in sections — prefilled from the
/// originating block's plan config when available.
struct PlayerTimerSheet: View {
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var store: PlayerTimerStore
    var setup: PlayerTimerSetupContext?
    /// Fallback-notification bridge. Observed for the
    /// explainer caption — shown until notifications are
    /// granted, since the fallback only works then.
    @ObservedObject var notifier: PlayerTimerNotifier

    @State private var restInput: String = ""
    @State private var emomRoundsInput: String = ""
    @State private var emomIntervalInput: String = ""
    @State private var amrapInput: String = ""
    @State private var formError: String?
    /// Selected setup tab. Seeded from the originating
    /// block's type on first appear (EMOM blocks open on
    /// EMOM, AMRAP on AMRAP), then user-controlled.
    @State private var setupKind: PlayerTimerStore.Kind = .rest
    /// Interval backing the EMOM chips. Kept in sync with
    /// `emomIntervalInput` both ways so chips and the custom
    /// row never disagree.
    @State private var emomInterval: Int = 60
    @State private var didSeed = false

    var body: some View {
        NavigationStack {
            Group {
                if store.isActive {
                    activeView
                } else {
                    setupView
                }
            }
            .background(DSColors.background.ignoresSafeArea())
            .navigationTitle("Timer")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button {
                        dismiss()
                    } label: {
                        Image(systemName: "chevron.down")
                    }
                    .accessibilityLabel("Minimize timer")
                    .accessibilityHint("Back to the mini timer bar")
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button("Done") { dismiss() }
                }
            }
            .onAppear(perform: seedInputs)
            .task {
                await notifier.refreshStatus()
            }
        }
    }

    // MARK: - Active

    private var activeView: some View {
        VStack(spacing: DSSpacing.lg) {
            Spacer()
            if store.isComplete {
                Image(systemName: Icons.success)
                    .font(.system(size: 72))
                    .foregroundStyle(DSColors.accent)
                Text("Timer Complete!")
                    .font(.title.weight(.bold))
                    .foregroundStyle(DSColors.accent)
                Text(store.subtitle)
                    .font(.body)
                    .foregroundStyle(DSColors.textSecondary)
            } else {
                Text(store.subtitle)
                    .font(.title2.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                TimelineView(.periodic(from: .now, by: 0.1)) { context in
                    Text(store.displayString(at: context.date))
                        .font(.system(size: 84, weight: .semibold, design: .rounded).monospacedDigit())
                        .foregroundStyle(DSColors.text)
                        .contentTransition(.numericText())
                }
            }
            Spacer()
            if store.isComplete {
                // The sheet auto-opens on completion, so its
                // button gets out of the way rather than
                // starting over — the pill keeps the complete
                // state (and its dismiss) behind it.
                Button {
                    dismiss()
                } label: {
                    Text("Dismiss")
                        .frame(maxWidth: .infinity)
                }
                .buttonStyle(.dsSecondary)
            } else {
                HStack(spacing: DSSpacing.md) {
                    Button(role: .destructive) {
                        store.reset()
                    } label: {
                        Label("Reset", systemImage: "arrow.counterclockwise")
                            .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.dsSecondary)
                    Button {
                        if store.isRunning {
                            store.pause()
                        } else {
                            store.resume()
                        }
                    } label: {
                        Label(
                            store.isRunning ? "Pause" : "Resume",
                            systemImage: store.isRunning ? "pause.fill" : "play.fill"
                        )
                        .frame(maxWidth: .infinity)
                    }
                    .buttonStyle(.dsPrimary)
                }
            }
        }
        .padding(DSSpacing.lg)
    }

    // MARK: - Setup (manual picker)

    /// Setup form: a segmented Rest / EMOM / AMRAP picker
    /// (mirroring the standalone `TimerView`), then big
    /// tap-to-start presets prefilled from the block's plan
    /// config, then one custom row per type. Presets start
    /// immediately — no second confirmation — so mid-workout
    /// use stays one tap with sweaty hands.
    private var setupView: some View {
        VStack(spacing: DSSpacing.md) {
            Picker("Timer type", selection: $setupKind) {
                ForEach(PlayerTimerStore.Kind.allCases, id: \.self) { kind in
                    Text(kind.displayName).tag(kind)
                }
            }
            .pickerStyle(.segmented)
            if let setup {
                Text("For \(setup.blockName)")
                    .font(.subheadline.weight(.semibold))
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            // One-line pitch for the fallback notification,
            // shown until granted — the prompt itself fires on
            // the first timer start.
            if notifier.status != .granted {
                Text("Allow notifications to hear the timer when your phone is locked.")
                    .font(.caption)
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
            ScrollView {
                Group {
                    switch setupKind {
                    case .rest:
                        restSetup
                    case .emom:
                        emomSetup
                    case .amrap:
                        amrapSetup
                    }
                }
            }
            if let formError {
                Text(formError)
                    .font(.caption)
                    .foregroundStyle(DSColors.destructive)
                    .frame(maxWidth: .infinity, alignment: .leading)
            }
        }
        .padding(DSSpacing.lg)
    }

    // MARK: Rest setup — countdown presets + custom seconds.

    private var restPresets: [Int] {
        var presets = [30, 60, 90, 120, 180, 300]
        if let plan = setup?.restSeconds, plan > 0, !presets.contains(plan) {
            presets.insert(plan, at: 0)
        }
        return presets
    }

    private var restSetup: some View {
        VStack(alignment: .leading, spacing: DSSpacing.md) {
            LazyVGrid(
                columns: [GridItem(.flexible()), GridItem(.flexible()), GridItem(.flexible())],
                spacing: DSSpacing.xs
            ) {
                ForEach(restPresets, id: \.self) { seconds in
                    presetButton(
                        title: shortDuration(seconds),
                        isPlan: seconds == setup?.restSeconds
                    ) {
                        formError = nil
                        store.startRest(seconds: seconds, blockID: contextBlockID(), blockName: contextName())
                    }
                }
            }
            customRow(
                title: "Custom (seconds)",
                fields: {
                    TextField("90", text: $restInput)
                        .keyboardType(.numberPad)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 120)
                        .multilineTextAlignment(.center)
                },
                startLabel: "Start",
                start: { startRest() }
            )
        }
    }

    // MARK: EMOM setup — interval chips + round presets.

    private var emomIntervals: [Int] {
        var presets = [30, 45, 60, 90, 120]
        if let plan = setup?.intervalSeconds, plan > 0, !presets.contains(plan) {
            presets.insert(plan, at: 0)
        }
        return presets
    }

    private var emomRoundPresets: [Int] {
        var presets = [3, 5, 7, 10, 12, 15]
        if let plan = setup?.rounds, plan > 0, !presets.contains(plan) {
            presets.insert(plan, at: 0)
        }
        return presets
    }

    private var emomSetup: some View {
        VStack(alignment: .leading, spacing: DSSpacing.md) {
            Text("Every")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
            LazyVGrid(
                columns: [GridItem(.flexible()), GridItem(.flexible()), GridItem(.flexible())],
                spacing: DSSpacing.xs
            ) {
                ForEach(emomIntervals, id: \.self) { seconds in
                    intervalChip(seconds)
                }
            }
            Text("Rounds")
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
            LazyVGrid(
                columns: [GridItem(.flexible()), GridItem(.flexible()), GridItem(.flexible())],
                spacing: DSSpacing.xs
            ) {
                ForEach(emomRoundPresets, id: \.self) { rounds in
                    presetButton(
                        title: "\(rounds)",
                        subtitle: rounds == 1 ? "round" : "rounds",
                        isPlan: rounds == setup?.rounds
                    ) {
                        startEMOMPreset(rounds: rounds)
                    }
                }
            }
            customRow(
                title: "Custom (rounds × secs/round)",
                fields: {
                    HStack(spacing: DSSpacing.sm) {
                        TextField("5", text: $emomRoundsInput)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.roundedBorder)
                            .frame(maxWidth: 80)
                            .multilineTextAlignment(.center)
                        Text("×")
                            .foregroundStyle(DSColors.textSecondary)
                        TextField("60", text: $emomIntervalInput)
                            .keyboardType(.numberPad)
                            .textFieldStyle(.roundedBorder)
                            .frame(maxWidth: 80)
                            .multilineTextAlignment(.center)
                    }
                },
                startLabel: "Start",
                start: { startEMOM() }
            )
        }
    }

    // MARK: AMRAP setup — cap presets + custom cap.

    private var amrapPresets: [Int] {
        var presets = [300, 480, 600, 720, 900, 1200]
        if let plan = setup?.timeCapSeconds, plan > 0, !presets.contains(plan) {
            presets.insert(plan, at: 0)
        }
        return presets
    }

    private var amrapSetup: some View {
        VStack(alignment: .leading, spacing: DSSpacing.md) {
            LazyVGrid(
                columns: [GridItem(.flexible()), GridItem(.flexible()), GridItem(.flexible())],
                spacing: DSSpacing.xs
            ) {
                ForEach(amrapPresets, id: \.self) { cap in
                    presetButton(
                        title: minutesLabel(cap),
                        isPlan: cap == setup?.timeCapSeconds
                    ) {
                        formError = nil
                        store.startAMRAP(capSeconds: cap, blockID: contextBlockID(), blockName: contextName())
                    }
                }
            }
            customRow(
                title: "Custom cap (seconds)",
                fields: {
                    TextField("600", text: $amrapInput)
                        .keyboardType(.numberPad)
                        .textFieldStyle(.roundedBorder)
                        .frame(maxWidth: 120)
                        .multilineTextAlignment(.center)
                },
                startLabel: "Start",
                start: { startAMRAP() }
            )
        }
    }

    // MARK: Setup primitives

    /// Big tap-to-start preset cell in a neutral card style.
    /// The grid stays quiet so the accent is reserved for the
    /// plan recommendation (accent edge + tag) and the Start
    /// buttons. The plan value gets a "Plan" tag when it
    /// differs from the standards.
    private func presetButton(
        title: String,
        subtitle: String? = nil,
        isPlan: Bool,
        action: @escaping () -> Void
    ) -> some View {
        Button(action: action) {
            VStack(spacing: 2) {
                Text(title)
                    .font(.headline)
                    .foregroundStyle(isPlan ? DSColors.accent : DSColors.text)
                if let subtitle {
                    Text(subtitle)
                        .font(.caption2)
                        .foregroundStyle(DSColors.textSecondary)
                } else if isPlan {
                    Text("Plan")
                        .font(.caption2.weight(.semibold))
                        .foregroundStyle(DSColors.accent)
                }
            }
            .frame(maxWidth: .infinity, minHeight: 56)
            .background(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surface)
            )
            .overlay(
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .stroke(isPlan ? DSColors.accent : DSColors.separator, lineWidth: isPlan ? 1.5 : 0.5)
            )
        }
        .buttonStyle(.plain)
    }

    /// Single-select interval chip — the chosen interval
    /// applies to every round preset tapped after it.
    private func intervalChip(_ seconds: Int) -> some View {
        let selected = emomInterval == seconds
        return Button {
            emomInterval = seconds
            emomIntervalInput = "\(seconds)"
        } label: {
            Text(shortDuration(seconds))
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(selected ? DSColors.onPrimary : DSColors.text)
                .frame(maxWidth: .infinity, minHeight: 44)
                .background(
                    RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                        .fill(selected ? DSColors.accent : DSColors.surface)
                )
                .overlay(
                    RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                        .stroke(selected ? Color.clear : DSColors.separator, lineWidth: 0.5)
                )
        }
        .buttonStyle(.plain)
    }

    private func customRow<Fields: View>(
        title: String,
        fields: () -> Fields,
        startLabel: String,
        start: @escaping () -> Void
    ) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            Text(title)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.textSecondary)
            HStack(spacing: DSSpacing.sm) {
                fields()
                Spacer()
                Button(startLabel, action: start)
                    .buttonStyle(.dsPrimary)
                    .frame(maxWidth: 120)
            }
        }
    }

    /// Compact duration label: "30s" under a minute, else
    /// "1:00".
    private func shortDuration(_ seconds: Int) -> String {
        if seconds < 60 { return "\(seconds)s" }
        return String(format: "%d:%02d", seconds / 60, seconds % 60)
    }

    /// Whole-minute caps render as "10 min" so the AMRAP grid
    /// scans; odd caps fall back to `shortDuration`.
    private func minutesLabel(_ seconds: Int) -> String {
        if seconds % 60 == 0 { return "\(seconds / 60) min" }
        return shortDuration(seconds)
    }

    // MARK: - Setup logic

    private func contextName() -> String {
        if let setup, !setup.blockName.isEmpty { return setup.blockName }
        if !store.blockName.isEmpty { return store.blockName }
        return "Workout"
    }

    private func contextBlockID() -> String? {
        setup?.blockID ?? store.blockID
    }

    private func seedInputs() {
        guard !didSeed else { return }
        didSeed = true
        if let setup {
            switch setup.blockType {
            case .emom:
                setupKind = .emom
            case .amrap:
                setupKind = .amrap
            case .standard, .circuit:
                setupKind = .rest
            }
        }
        if restInput.isEmpty {
            let plan = setup?.restSeconds ?? 0
            restInput = plan > 0 ? "\(plan)" : "90"
        }
        if emomRoundsInput.isEmpty {
            let plan = setup?.rounds ?? 0
            emomRoundsInput = plan > 0 ? "\(plan)" : "5"
        }
        let intervalPlan = setup?.intervalSeconds ?? 0
        emomInterval = intervalPlan > 0 ? intervalPlan : 60
        if emomIntervalInput.isEmpty {
            emomIntervalInput = "\(emomInterval)"
        }
        if amrapInput.isEmpty {
            let plan = setup?.timeCapSeconds ?? 0
            amrapInput = plan > 0 ? "\(plan)" : "600"
        }
    }

    private func startRest() {
        guard let seconds = parse(restInput, range: 1...1800) else {
            formError = "Enter rest seconds between 1 and 1800."
            return
        }
        formError = nil
        store.startRest(seconds: seconds, blockID: contextBlockID(), blockName: contextName())
    }

    /// Round-preset tap: starts immediately with the
    /// chip-selected interval.
    private func startEMOMPreset(rounds: Int) {
        formError = nil
        store.startEMOM(
            rounds: rounds,
            intervalSeconds: emomInterval,
            blockID: contextBlockID(),
            blockName: contextName()
        )
    }

    private func startEMOM() {
        guard let rounds = parse(emomRoundsInput, range: 1...30) else {
            formError = "Enter rounds between 1 and 30."
            return
        }
        guard let interval = parse(emomIntervalInput, range: 10...600) else {
            formError = "Enter seconds per round between 10 and 600."
            return
        }
        formError = nil
        emomInterval = interval
        store.startEMOM(rounds: rounds, intervalSeconds: interval, blockID: contextBlockID(), blockName: contextName())
    }

    private func startAMRAP() {
        guard let cap = parse(amrapInput, range: 30...7200) else {
            formError = "Enter cap seconds between 30 and 7200."
            return
        }
        formError = nil
        store.startAMRAP(capSeconds: cap, blockID: contextBlockID(), blockName: contextName())
    }

    private func parse(_ text: String, range: ClosedRange<Int>) -> Int? {
        let trimmed = text.trimmingCharacters(in: .whitespaces)
        guard let value = Int(trimmed), range.contains(value) else { return nil }
        return value
    }
}
