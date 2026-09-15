import SwiftUI

/// Searchable block picker sheet used by `WorkoutEditorView`.
/// Mirrors `ExercisePickerSheet`: a search field over rows showing
/// the name plus a "Kind · N exercises" subtitle. Tapping a row
/// selects the block and dismisses; the caller owns the selection
/// binding so cancel leaves the form unchanged.
struct BlockPickerSheet: View {
    let blocks: [BlockSummaryDTO]
    @Binding var selectedBlockID: String?
    @Environment(\.dismiss) private var dismiss

    @State private var search: String = ""

    private var filtered: [BlockSummaryDTO] {
        let query = search.trimmingCharacters(in: .whitespacesAndNewlines).lowercased()
        guard !query.isEmpty else { return blocks }
        return blocks.filter { $0.name.lowercased().contains(query) }
    }

    var body: some View {
        NavigationStack {
            content
                .navigationTitle("Choose Block")
                .navigationBarTitleDisplayMode(.inline)
                .searchable(text: $search, placement: .navigationBarDrawer(displayMode: .always), prompt: "Search blocks")
                .toolbar {
                    ToolbarItem(placement: .topBarLeading) {
                        Button("Cancel") { dismiss() }
                    }
                }
        }
    }

    @ViewBuilder
    private var content: some View {
        if blocks.isEmpty {
            VStack(spacing: DSSpacing.md) {
                Spacer()
                Image(systemName: "rectangle.3.group")
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.textSecondary)
                Text("No blocks yet. Build a block first, then add it to a workout.")
                    .foregroundStyle(DSColors.textSecondary)
                    .multilineTextAlignment(.center)
                Spacer()
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if filtered.isEmpty {
            VStack(spacing: DSSpacing.md) {
                Spacer()
                Text("No blocks match your search.")
                    .foregroundStyle(DSColors.textSecondary)
                    .multilineTextAlignment(.center)
                Spacer()
            }
            .padding()
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            List {
                ForEach(filtered) { block in
                    Button {
                        selectedBlockID = block.id
                        dismiss()
                    } label: {
                        HStack {
                            VStack(alignment: .leading, spacing: 2) {
                                Text(block.name)
                                    .font(.body)
                                    .foregroundStyle(DSColors.text)
                                Text(subtitle(for: block))
                                    .font(.subheadline)
                                    .foregroundStyle(DSColors.textSecondary)
                            }
                            Spacer()
                            if block.id == selectedBlockID {
                                Image(systemName: "checkmark")
                                    .foregroundStyle(DSColors.accent)
                                    .accessibilityLabel("Selected")
                            }
                        }
                        .contentShape(Rectangle())
                    }
                    .buttonStyle(.plain)
                    .listRowSeparator(.hidden)
                }
            }
            .listStyle(.insetGrouped)
            .listRowSeparator(.hidden)
        }
    }

    private func subtitle(for block: BlockSummaryDTO) -> String {
        let exercises = block.itemCount == 1 ? "1 exercise" : "\(block.itemCount) exercises"
        return "\(block.type.displayName) · \(exercises)"
    }
}

#Preview {
    BlockPickerSheet(
        blocks: [
            BlockSummaryDTO(
                id: "1", name: "Push Day", description: "", type: .standard,
                rounds: 0, restSeconds: 0, timeCapSeconds: 0, intervalSeconds: 0,
                itemCount: 4, createdAt: Date(), updatedAt: Date()
            ),
        ],
        selectedBlockID: .constant("1")
    )
}
