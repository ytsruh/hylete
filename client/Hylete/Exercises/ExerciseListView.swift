import SwiftUI

/// The "Exercises" tab. Lists every exercise in the
/// catalogue with a search field at the top. Tap an
/// exercise to navigate to `ExerciseHistoryView` for that
/// exercise. Each row shows the exercise's image (or a
/// muted placeholder), name, and a type chip. The
/// description is intentionally not shown here — the
/// per-exercise history view is where the description
/// lives, so it's not duplicated in the catalogue list.
struct ExerciseListView: View {
    @EnvironmentObject private var env: AppEnvironment

    @State private var exercises: [ExerciseDTO] = []
    @State private var isLoading: Bool = true
    @State private var errorMessage: String?
    @State private var search: String = ""

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Exercises")
                .searchable(text: $search, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search exercises")
        }
        .task { await load() }
        .refreshable { await load() }
    }

    @ViewBuilder
    private var content: some View {
        if isLoading && exercises.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let errorMessage, exercises.isEmpty {
            VStack(spacing: DSSpacing.md) {
                Image(systemName: Icons.warning)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.destructive)
                Text(errorMessage)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") { Task { await load() } }
                    .buttonStyle(.dsSecondary)
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List {
                ForEach(filtered) { exercise in
                    NavigationLink(value: exercise) {
                        ExerciseRow(exercise: exercise)
                    }
                    .listRowSeparator(.hidden)
                }
            }
            .listStyle(.insetGrouped)
            .listRowSeparator(.hidden)
            .navigationDestination(for: ExerciseDTO.self) { exercise in
                ExerciseHistoryView(exercise: exercise)
            }
        }
    }

    private var filtered: [ExerciseDTO] {
        let q = search.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        if q.isEmpty { return exercises }
        return exercises.filter { $0.name.lowercased().contains(q) }
    }

    private func load() async {
        errorMessage = nil
        isLoading = true
        defer { isLoading = false }
        do {
            exercises = try await env.api.listExercises()
        } catch let error as APIError {
            if case .unauthorized = error { return }
            errorMessage = error.errorDescription
        } catch {
            errorMessage = "Could not load exercises."
        }
    }
}

/// One row in the exercise list. Layout (left → right):
///
///   [ thumbnail ]   [ name                 ]   [ type chip ]
///
/// The thumbnail is filled with the exercise's image when
/// one exists; otherwise a muted placeholder tile with the
/// dumbbell icon is shown in the same slot so every row
/// has the same visual rhythm. The wrapping
/// `NavigationLink` in the parent view pushes the
/// per-exercise history view.
struct ExerciseRow: View {
    let exercise: ExerciseDTO

    var body: some View {
        HStack(spacing: DSSpacing.md) {
            thumbnail

            Text(exercise.name)
                .font(.subheadline.weight(.semibold))
                .foregroundStyle(DSColors.text)
                .lineLimit(2)
                .multilineTextAlignment(.leading)
                .frame(maxWidth: .infinity, alignment: .leading)

            ExerciseTypeChip(type: exercise.type)
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    @ViewBuilder
    private var thumbnail: some View {
        if exercise.hasImage, let url = URL(string: exercise.imageURL) {
            AsyncImage(url: url) { phase in
                switch phase {
                case .success(let image):
                    image
                        .resizable()
                        .scaledToFill()
                case .failure, .empty:
                    placeholder
                @unknown default:
                    placeholder
                }
            }
            .frame(width: 60, height: 40)
            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
        } else {
            placeholder
                .frame(width: 60, height: 40)
        }
    }

    /// Muted tile with the dumbbell icon shown when the
    /// exercise has no image (or its image hasn't loaded
    /// yet). Keeps every row's leading edge at the same
    /// width so the name alignment is consistent across the
    /// list. Matches the web's muted placeholder in
    /// `internal/views/exercise/history.templ:65-67`.
    private var placeholder: some View {
        RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
            .fill(DSColors.surfaceElevated)
            .overlay(
                Image(systemName: Icons.exercises)
                    .font(.system(size: 16))
                    .foregroundStyle(DSColors.textSecondary)
            )
    }
}

/// Pill-shaped badge showing the exercise type. Mirrors the web's
/// `ExerciseTypeBadge` in
/// `internal/views/components/exercise_type_badge.templ`. The badge
/// background adapts to the exercise type so the row scans at a glance:
///   - **Strength** → brand orange (`DSColors.accent`).
///   - **Cardio**   → inverted chip fill (`DSColors.secondary`): dark
///                    pill in light mode, light pill in dark mode — the
///                    same foreground/background swap the web theme uses,
///                    so cardio reads distinctly from both the row and
///                    the strength chips in either appearance.
///   - **Other**    → a neutral grey.
///
/// The text colour is picked per type for contrast:
/// strength uses the brand-aware `DSColors.onPrimary`
/// (white in both modes — the orange is dark enough that
/// black text fails contrast), cardio uses `DSColors.onSecondary`
/// (flips with the fill), and other uses `DSColors.text`.
struct ExerciseTypeChip: View {
    let type: String

    var body: some View {
        Text(displayName)
            .font(.caption2.weight(.semibold))
            .foregroundStyle(foregroundStyle)
            .padding(.horizontal, DSSpacing.xs)
            .padding(.vertical, DSSpacing.xxs)
            .background(
                Capsule(style: .continuous)
                    .fill(backgroundColor)
            )
            .overlay(
                Capsule(style: .continuous)
                    .stroke(DSColors.separator, lineWidth: 0.5)
            )
    }

    private var displayName: String {
        switch type.lowercased() {
        case "strength": return "Strength"
        case "cardio":   return "Cardio"
        case "other":    return "Other"
        default:         return type.capitalized
        }
    }

    /// Per-type background colour. See the type doc
    /// comment at the top of the badge for the rationale.
    private var backgroundColor: Color {
        switch type.lowercased() {
        case "strength": return DSColors.accent
        case "cardio":   return DSColors.secondary
        default:         return Color(.systemGray4)
        }
    }

    /// Per-type text colour. Strength always uses the
    /// brand-on-primary (white) for contrast against the
    /// orange; cardio rides on the inverted secondary pairing;
    /// other uses the adaptive label colour.
    private var foregroundStyle: Color {
        switch type.lowercased() {
        case "strength": return DSColors.onPrimary
        case "cardio":   return DSColors.onSecondary
        default:         return DSColors.text
        }
    }
}

private extension String {
    /// Locale-aware first-letter-uppercase. Avoids pulling in
    /// `Foundation.NSString.capitalizedString` differences and
    /// matches the web's `capitalize` CSS for ASCII.
    var capitalized: String {
        guard let first = first else { return self }
        return first.uppercased() + dropFirst()
    }
}

#Preview {
    ExerciseListView()
        .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
        .environmentObject(AuthStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
}
