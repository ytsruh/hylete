import SwiftUI

/// Detail view for one block. Header carries the name,
/// description, kind chip, and kind config; the item list shows
/// each planned exercise in order with its free-text target.
///
/// The block loads via `BlockStore.detail(id:)` (cached, so
/// returning from the editor is instant and already fresh —
/// `update` rewrites the cache). Edit opens the editor sheet;
/// delete confirms then pops back to the list.
struct BlockDetailView: View {
    @EnvironmentObject private var env: AppEnvironment
    @Environment(\.dismiss) private var dismiss
    @ObservedObject var store: BlockStore

    let blockID: String

    @State private var editorRequest: EditorRequest?
    @State private var showingDelete: Bool = false
    @State private var didRequestLoad: Bool = false

    /// Which editor to open from the sheet: plain edit or a
    /// duplicate pre-fill. `Identifiable` so it can drive the
    /// sheet directly.
    private struct EditorRequest: Identifiable {
        let id = UUID()
        let duplicate: Bool
    }

    private var block: BlockDTO? {
        store.details[blockID]
    }

    var body: some View {
        content
            .navigationTitle(block?.name ?? "Block")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                if block != nil {
                    ToolbarItem(placement: .topBarTrailing) {
                        Button("Edit") { editorRequest = EditorRequest(duplicate: false) }
                    }
                }
            }
            .sheet(item: $editorRequest) { request in
                if let block {
                    BlockEditorView(
                        mode: request.duplicate ? .duplicate(block) : .edit(block),
                        store: store,
                        onDuplicateSaved: {
                            dismiss()
                        }
                    )
                    .environmentObject(env)
                }
            }
            .alert("Delete this block?", isPresented: $showingDelete) {
                Button("Delete", role: .destructive) {
                    Task {
                        await store.delete(id: blockID)
                        if store.errorMessage == nil {
                            dismiss()
                        }
                    }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This permanently removes the block and its planned exercises. Logged sets are unaffected.")
            }
            .task {
                guard !didRequestLoad else { return }
                didRequestLoad = true
                await store.detail(id: blockID)
            }
    }

    @ViewBuilder
    private var content: some View {
        if let block {
            loadedView(block)
        } else if let error = store.errorMessage {
            VStack(spacing: DSSpacing.md) {
                Image(systemName: Icons.warning)
                    .font(.largeTitle)
                    .foregroundStyle(DSColors.destructive)
                Text(error)
                    .multilineTextAlignment(.center)
                    .foregroundStyle(DSColors.textSecondary)
                Button("Try again") {
                    Task { await store.detail(id: blockID, refresh: true) }
                }
                .buttonStyle(.dsSecondary)
            }
            .padding(DSSpacing.lg)
            .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        }
    }

    private func loadedView(_ block: BlockDTO) -> some View {
        List {
            Section {
                if !block.description.isEmpty {
                    Text(block.description)
                        .font(.body)
                        .foregroundStyle(DSColors.text)
                }
                HStack(spacing: DSSpacing.xs) {
                    Text(block.type.displayName)
                        .font(.subheadline.weight(.semibold))
                        .foregroundStyle(DSColors.accent)
                    if !block.configSummary.isEmpty {
                        Text("·")
                            .foregroundStyle(DSColors.textSecondary)
                        Text(block.configSummary)
                            .font(.subheadline)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                }
            }

            Section {
                ForEach(block.items) { item in
                    VStack(alignment: .leading, spacing: 2) {
                        Text(item.exerciseName)
                            .font(.body)
                            .foregroundStyle(DSColors.text)
                        if !item.targetText.isEmpty {
                            Text(item.targetText)
                                .font(.subheadline)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                    }
                    .padding(.vertical, DSSpacing.xxs)
                }
            } header: {
                Text(itemCountLabel(block.items.count))
            }

            // Full-width action buttons in the DesignSystem
            // primary/secondary idiom: Duplicate is the safe,
            // reversible action (secondary chrome), Delete is
            // filled destructive. Keeps the two visually
            // distinct so a thumb aiming for Duplicate never
            // lands on Delete.
            Section {
                Button {
                    editorRequest = EditorRequest(duplicate: true)
                } label: {
                    Text("Duplicate block")
                }
                .buttonStyle(.dsSecondary)

                Button(role: .destructive) {
                    showingDelete = true
                } label: {
                    Text("Delete block")
                }
                .font(.headline)
                .foregroundStyle(.white)
                .frame(maxWidth: .infinity)
                .frame(height: 48)
                .background(DSColors.destructive)
                .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous))
            }
        }
        .listStyle(.automatic)
    }

    private func itemCountLabel(_ count: Int) -> String {
        count == 1 ? "1 exercise" : "\(count) exercises"
    }
}

#Preview {
    NavigationStack {
        BlockDetailView(
            store: BlockStore(api: APIClient(
                baseURL: URL(string: "http://localhost:8080/api/v1")!,
                tokenProvider: { nil }
            )),
            blockID: "preview"
        )
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
