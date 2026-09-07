import Foundation

/// The two timer modes exposed on the timer's segmented
/// picker. The raw value is the user-facing label so the
/// picker's `tag(...)` lookup stays trivial and the enum
/// doubles as a self-documenting key.
enum TimerMode: String, CaseIterable, Identifiable {
    case timer = "Timer"
    case emom = "EMOM"

    var id: String { rawValue }
}
