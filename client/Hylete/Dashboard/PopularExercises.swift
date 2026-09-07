import Foundation
import SwiftUI

/// One slice of the dashboard's "Most Popular Exercises (7d)"
/// donut. `count` is the number of sets for the exercise; the
/// `color` is the brand-palette tint assigned at bucket time so
/// the same slice always renders the same colour.
struct PopularExerciseBucket: Identifiable, Equatable {
    let name: String
    let count: Int
    let color: Color
    /// Buckets flagged `isOther` use the neutral gray slice
    /// colour and the "Other" label.
    let isOther: Bool
    var id: String { name }
}

/// Maximum number of exercises the donut renders individually.
/// Anything beyond is collapsed into a single "Other" bucket so
/// the chart stays readable. Matches the web's
/// `popularExercisesTopN` constant in
/// `internal/views/dashboard/dashboard.templ`.
let popularExercisesTopN = 5

/// Neutral gray for the "Other" slice. Picked to visually
/// de-emphasise the bucket so the eye focuses on the top-N
/// brand shades. Matches the web's
/// `popularExercisesOtherColor = "#9ca3af"`.
private let otherBucketColor = Color(red: 0.61, green: 0.64, blue: 0.69)

/// Brand-orange palette, dark → light, derived by mixing the
/// brand color (#F44900) with white at 0/10/20/30/40/50
/// percent. Matches the web's `donutDefaultPalette` exactly
/// (the web hard-codes the same hex values) so the iOS
/// donut and the web donut are visually identical. We do
/// not use `DSColors.chart1`…`chart5` because those resolve
/// to amber/yellow tones in both light and dark mode — the
/// brand is orange, so the donut uses a hard-coded orange
/// ramp instead of the generic chart palette.
private let brandPalette: [Color] = [
    Color(red: 0.961, green: 0.286, blue: 0.000), // #F54900 brand (darkest)
    Color(red: 0.965, green: 0.357, blue: 0.102), // #F65B1A 10% white
    Color(red: 0.969, green: 0.427, blue: 0.200), // #F76D33 20% white
    Color(red: 0.973, green: 0.498, blue: 0.298), // #F87F4C 30% white
    Color(red: 0.976, green: 0.569, blue: 0.400), // #F99166 40% white
    Color(red: 0.980, green: 0.643, blue: 0.498), // #FAA47F 50% white (lightest)
]

/// Groups a slice of `ExerciseEntryDTO` by exercise name,
/// counts the number of sets per exercise, and returns the
/// `[PopularExerciseBucket]` needed to render the dashboard's
/// "Most Popular Exercises (7d)" donut. Up to
/// `popularExercisesTopN` exercises are returned individually;
/// the rest are collapsed into a single "Other" bucket so the
/// chart stays readable. Each entry is counted as one set to
/// match the dashboard's "Total Sets" stat semantics.
///
/// Sorting is by count desc, then by name asc, so the slice
/// order is fully deterministic and stable.
///
/// An empty input returns an empty array (a donut with zero
/// slices is not meaningful — callers should also gate on
/// `!buckets.isEmpty` before rendering).
func popularExerciseBuckets(_ entries: [ExerciseEntryDTO]) -> [PopularExerciseBucket] {
    var counts: [String: Int] = [:]
    for entry in entries {
        counts[entry.exerciseName, default: 0] += 1
    }

    let sorted = counts
        .map { (name: $0.key, count: $0.value) }
        .sorted { lhs, rhs in
            if lhs.count != rhs.count { return lhs.count > rhs.count }
            return lhs.name < rhs.name
        }

    guard !sorted.isEmpty else { return [] }

    let hasOther = sorted.count > popularExercisesTopN
    let top = Array(sorted.prefix(popularExercisesTopN))
    let tail = Array(sorted.dropFirst(popularExercisesTopN))

    var result: [PopularExerciseBucket] = top.enumerated().map { index, item in
        PopularExerciseBucket(
            name: item.name,
            count: item.count,
            color: brandPalette[index % brandPalette.count],
            isOther: false
        )
    }

    if hasOther {
        let otherCount = tail.reduce(0) { $0 + $1.count }
        result.append(
            PopularExerciseBucket(
                name: "Other",
                count: otherCount,
                color: otherBucketColor,
                isOther: true
            )
        )
    }

    return result
}
