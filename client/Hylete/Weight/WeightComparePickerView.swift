import SwiftUI

/// Picker sheet for the weight photo comparison flow. Lists every
/// entry that has at least one photo (newest first); the user taps
/// up to two, picks the angle they share, and hits Compare.
///
/// The compared angle auto-selects the first shared angle
/// (preferring front) whenever the pair changes. Pairs with no
/// shared angle leave Compare disabled with a hint. The actual
/// fetch runs here so the parent list stays free of comparison
/// state; success hands the server response back via `onCompared`.
struct WeightComparePickerView: View {
    /// Entries with at least one photo, newest first.
    let entries: [WeightEntryDTO]
    let weightUnit: String
    /// Called with the server-compared pair. The caller presents
    /// the comparison sheet and dismisses this picker.
    let onCompared: (WeightCompareResponse) -> Void

    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss

    @State private var selectedIDs: [String] = []
    @State private var selectedAngle: WeightEntryDTO.PhotoAngle = .front
    @State private var comparisonError: String?
    @State private var isLoading: Bool = false

    /// The selected entries in tap order.
    private var selectedEntries: [WeightEntryDTO] {
        selectedIDs.compactMap { id in entries.first { $0.id == id } }
    }

    /// Angles both selected entries share, in preference order.
    /// Empty when fewer than two are selected.
    private var sharedAngles: [WeightEntryDTO.PhotoAngle] {
        guard selectedEntries.count == 2 else { return [] }
        return WeightEntryDTO.PhotoAngle.allCases.filter {
            selectedEntries[0].hasPhoto(for: $0) && selectedEntries[1].hasPhoto(for: $0)
        }
    }

    var body: some View {
        NavigationStack {
            VStack(spacing: DSSpacing.md) {
                selectionSummary
                entriesList
                angleSection
                if let comparisonError {
                    Text(comparisonError)
                        .font(.caption)
                        .foregroundStyle(DSColors.destructive)
                        .frame(maxWidth: .infinity, alignment: .leading)
                }
                compareButton
            }
            .padding(DSSpacing.md)
            .frame(maxWidth: .infinity, maxHeight: .infinity, alignment: .top)
            .background(DSColors.background.ignoresSafeArea())
            .navigationTitle("Compare Photos")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                        .disabled(isLoading)
                }
            }
        }
        .presentationDragIndicator(.visible)
    }

    private var selectionSummary: some View {
        Text("\(selectedIDs.count) of 2 selected")
            .font(.subheadline.weight(.semibold))
            .foregroundStyle(DSColors.textSecondary)
            .frame(maxWidth: .infinity, alignment: .leading)
    }

    private var entriesList: some View {
        ScrollView {
            VStack(spacing: DSSpacing.xs) {
                ForEach(entries) { entry in
                    Button {
                        toggleSelection(entry.id)
                    } label: {
                        HStack(spacing: DSSpacing.sm) {
                            if selectedIDs.contains(entry.id) {
                                Image(systemName: "checkmark.circle.fill")
                                    .foregroundStyle(BrandColors.brandOrange)
                            } else {
                                Image(systemName: "circle")
                                    .foregroundStyle(DSColors.textSecondary)
                            }
                            Text(entry.formattedWeight(in: weightUnit))
                                .font(.subheadline.weight(.semibold).monospacedDigit())
                                .foregroundStyle(DSColors.text)
                            Spacer()
                            Text(entry.createdAt.formatted(.dateTime.day().month(.abbreviated).year()))
                                .font(.caption)
                                .foregroundStyle(DSColors.textSecondary)
                            Text(angleBadges(entry))
                                .font(.caption2)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                        .padding(DSSpacing.sm)
                        .background(
                            RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
                                .fill(selectedIDs.contains(entry.id) ? DSColors.surfaceElevated : DSColors.surface)
                        )
                        .overlay(
                            RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
                                .stroke(selectedIDs.contains(entry.id) ? BrandColors.brandOrange : DSColors.separator, lineWidth: selectedIDs.contains(entry.id) ? 1.5 : 0.5)
                        )
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel(selectedIDs.contains(entry.id) ? "Deselect entry" : "Select entry for comparison")
                }
            }
        }
    }

    /// Compact "F · S · B" badges naming the angles the entry has.
    private func angleBadges(_ entry: WeightEntryDTO) -> String {
        entry.photoAngles.map { String($0.rawValue.prefix(1).uppercased()) }.joined(separator: " · ")
    }

    @ViewBuilder
    private var angleSection: some View {
        if selectedEntries.count == 2 {
            if sharedAngles.isEmpty {
                Text("These entries share no photo angle.")
                    .font(.caption)
                    .foregroundStyle(DSColors.textSecondary)
                    .frame(maxWidth: .infinity, alignment: .leading)
            } else {
                Picker("Angle", selection: $selectedAngle) {
                    ForEach(sharedAngles, id: \.self) { angle in
                        Text(angle.rawValue.capitalized).tag(angle)
                    }
                }
                .pickerStyle(.segmented)
                .accessibilityLabel("Photo angle")
            }
        }
    }

    private var compareButton: some View {
        Button {
            Task { await loadComparison() }
        } label: {
            if isLoading {
                ProgressView().tint(.white)
            } else {
                Text("Compare")
            }
        }
        .buttonStyle(.dsPrimary)
        .disabled(selectedIDs.count != 2 || sharedAngles.isEmpty || isLoading)
    }

    private func toggleSelection(_ id: String) {
        comparisonError = nil
        if let index = selectedIDs.firstIndex(of: id) {
            selectedIDs.remove(at: index)
        } else if selectedIDs.count < 2 {
            selectedIDs.append(id)
            // Auto-pick the first shared angle whenever the pair
            // becomes complete.
            if selectedIDs.count == 2, let first = sharedAngles.first {
                selectedAngle = first
            }
        }
        // Keep the selected angle valid when the pair changes.
        if selectedIDs.count == 2, !sharedAngles.contains(selectedAngle), let first = sharedAngles.first {
            selectedAngle = first
        }
    }

    private func loadComparison() async {
        guard selectedIDs.count == 2 else { return }
        comparisonError = nil
        isLoading = true
        defer { isLoading = false }

        do {
            let response = try await env.api.compareWeightEntries(
                a: selectedIDs[0],
                b: selectedIDs[1],
                angle: selectedAngle.rawValue
            )
            onCompared(response)
        } catch let error as APIError {
            comparisonError = error.errorDescription
        } catch {
            comparisonError = "Could not compare the selected photos."
        }
    }
}
