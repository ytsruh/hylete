import SwiftUI

/// SF Symbol name constants. Centralised so view code reads
/// `Icons.exercises` instead of the magic string `"dumbbell"`,
/// and so the iOS icon set stays in lockstep with the web's
/// Lucide icon set (the web's `dumbbell`, `plus`, etc. are
/// matched 1:1 to the symbols below).
public enum Icons {

    // MARK: - Tab bar

    public static let today     = "list.bullet.rectangle"
    public static let exercises = "dumbbell"
    /// "Goals" tab — matches the web's nav icon (Lucide's
    /// `target`) so the tab is visually consistent with
    /// the web's top-nav entry.
    public static let goals     = "target"
    /// "Weight" tab — matches the web's nav icon (Lucide's
    /// `scale`) so the tab feels native to the rest of the
    /// app's iconography.
    public static let weight    = "figure.stand"
    public static let profile   = "person.crop.circle"

    // MARK: - Dashboard

    /// Toolbar "add set" button (and the "Add Set" action card).
    public static let addSet = "plus"
    /// "Timer" action card.
    public static let timer = "timer"
    /// Empty-state for the dashboard's "no sets yet" view.
    public static let dumbbellLarge = "dumbbell"
    /// Empty-state for the per-exercise history view.
    public static let chartEmpty = "chart.line.uptrend.xyaxis"

    // MARK: - Forms

    /// "Add another row" affordance in the new-set form.
    public static let plusCircle = "plus.circle"

    // MARK: - Exercise details

    /// "Watch Video" affordance on the exercise history
    /// view. Matches the web's Lucide `play` icon used in
    /// `internal/views/exercise/history.templ:71`.
    public static let play = "play.fill"

    /// Swipe-action "Edit" button on the history rows.
    /// Matches the web's Lucide `edit` icon used in
    /// `internal/views/exercise/history.templ:189`.
    public static let edit = "pencil"

    /// Swipe-action "Delete" button on the history rows.
    /// Matches the same icon used in the dashboard's
    /// swipe-to-delete (client/Hylete/Dashboard/SetList.swift:57).
    public static let trash = "trash"

    // MARK: - Stat cards

    /// "Personal Best" stat card on the exercise history
    /// view. The filled symbol pairs well with the accent
    /// colour treatment for the highlighted PR card.
    public static let trophy = "trophy.fill"
    /// "Last Activity" stat card on the exercise history
    /// view.
    public static let calendar = "calendar"
    /// Smaller variant of the dumbbell for the "Last Set"
    /// stat card. Reuses the same underlying SF Symbol
    /// string as `exercises` but is declared separately so
    /// the two can diverge if the symbol set changes.
    public static let dumbbellSmall = "dumbbell.fill"

    // MARK: - Status

    /// Generic error / load-failure state.
    public static let warning = "exclamationmark.triangle"
    /// Success confirmation (toast icon, etc.).
    public static let success = "checkmark.circle.fill"
    /// Failure in destructive confirmations.
    public static let failure = "xmark.octagon.fill"

    // MARK: - Appearance toggle

    /// "Switch to light mode" icon (shown in dark mode).
    public static let sun  = "sun.max"
    /// "Switch to dark mode" icon (shown in light mode).
    public static let moon = "moon.fill"
}
