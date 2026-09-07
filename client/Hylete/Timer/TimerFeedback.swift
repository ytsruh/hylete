import AudioToolbox
import UIKit

/// Haptic + system-sound cues for timer state changes. Uses
/// built-in iOS system sounds so no audio asset has to ship
/// with the bundle, and `UINotificationFeedbackGenerator` so
/// the haptic respects the user's "reduce motion / haptics"
/// preferences.
///
/// Both feedback paths pre-warm their generators on first
/// use so the first cue (which is always the round boundary
/// or session complete) fires with zero latency.
@MainActor
enum TimerFeedback {
    /// System sound for a round boundary. "Tink" — short,
    /// light, suitable for the per-round pulse in EMOM.
    private static let roundSoundID: SystemSoundID = 1322
    /// System sound for session complete. "Tweet sent" —
    /// distinct from the round boundary so the user can tell
    /// by ear whether a round ended or the whole session did.
    private static let completeSoundID: SystemSoundID = 1057

    private static let roundHaptic = UINotificationFeedbackGenerator()
    private static let completeHaptic = UINotificationFeedbackGenerator()

    /// Fires at each EMOM round boundary. Lighter haptic +
    /// light system sound.
    static func roundBoundary() {
        roundHaptic.prepare()
        roundHaptic.notificationOccurred(.success)
        AudioServicesPlaySystemSound(roundSoundID)
    }

    /// Fires when the countdown reaches zero or the EMOM
    /// finishes its last round. Stronger haptic + a more
    /// distinct system sound.
    static func sessionComplete() {
        completeHaptic.prepare()
        completeHaptic.notificationOccurred(.warning)
        AudioServicesPlaySystemSound(completeSoundID)
    }
}
