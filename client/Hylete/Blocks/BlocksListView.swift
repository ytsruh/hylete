import SwiftUI

/// The Blocks list (Beta). A plain `List` of the user's planned
/// blocks, newest first. Tapping a row pushes the detail view;
/// trailing swipe deletes.
///
/// Stack-less content: the More hub's `NavigationStack` provides
/// the single stack — never wrap this view. Sheets (the editor)
/// keep their own stacks.
///
/// All networking and state lives in `BlockStore`; this view is
/// purely presentational.
struct BlocksListView: View {
    @EnvironmentObject private var env: AppEnvironment
    @ObservedObject var store: BlockStore

    @State private var showingNewBlock: Bool = false
    @State private var deletingBlock: BlockSummaryDTO?
    @State private var showingDeleteAlert: Bool = false
    @State private var duplicatingBlock: BlockDTO?
    @State private var search: String = ""
    @State private var typeFilter: BlockTypeFilter = .all

    /// Client-side name + type filter over the loaded summaries.
    /// Trimmed, case-insensitive contains match — same as the
    /// exercise catalogue filter.
    private var filtered: [BlockSummaryDTO] {
        filterBlocks(store.blocks, search: search, typeFilter: typeFilter)
    }

    var body: some View {
        content
            .navigationTitle("Blocks")
            .toolbar {
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        showingNewBlock = true
                    } label: {
                        Image(systemName: Icons.addSet)
                    }
                    .accessibilityLabel("Add block")
                }
            }
            .sheet(isPresented: $showingNewBlock) {
                BlockEditorView(mode: .create, store: store)
                    .environmentObject(env)
            }
            .sheet(item: $duplicatingBlock) { block in
                BlockEditorView(mode: .duplicate(block), store: store)
                    .environmentObject(env)
            }
            .alert("Delete this block?", isPresented: $showingDeleteAlert) {
                Button("Delete", role: .destructive) {
                    if let block = deletingBlock {
                        Task { await store.delete(id: block.id) }
                    }
                }
                Button("Cancel", role: .cancel) {
                    deletingBlock = nil
                }
            } message: {
                Text("This permanently removes the block and its planned exercises. Logged sets are unaffected.")
            }
            .task { await store.load() }
            .refreshable { await store.load() }
    }

    @ViewBuilder
    private var content: some View {
        if store.isLoading && store.blocks.isEmpty {
            ProgressView()
                .frame(maxWidth: .infinity, maxHeight: .infinity)
        } else if let error = store.errorMessage, store.blocks.isEmpty {
            errorState(error)
        } else if store.blocks.isEmpty {
            emptyState
        } else {
            loadedList
        }
    }

    private var loadedList: some View {
        VStack(spacing: 0) {
            Text("Blocks are sets of exercises that can be reused across workouts")
                .font(.title3)
                .foregroundStyle(DSColors.textSecondary)
                .frame(maxWidth: .infinity, alignment: .leading)
                .padding(.horizontal)
                .padding(.top, DSSpacing.sm)
                .padding(.bottom, DSSpacing.xs)
            InlineSearchField(text: $search, prompt: "Search blocks")
                .padding(.horizontal)
                .padding(.bottom, DSSpacing.xs)
            BlockTypeFilterView(selection: $typeFilter)
                .padding(.bottom, DSSpacing.xs)
            if filtered.isEmpty {
                noMatches
            } else {
                List {
            // Mutation errors (e.g. the 409 when deleting a block
            // still used by a workout) surface inline — the list
            // itself is non-empty so the full-screen error state
            // never shows. Tapping dismisses.
            if let error = store.errorMessage {
                Section {
                    Button {
                        store.errorMessage = nil
                    } label: {
                        HStack(spacing: DSSpacing.xs) {
                            Image(systemName: Icons.warning)
                                .foregroundStyle(DSColors.destructive)
                            Text(error)
                                .font(.footnote)
                                .foregroundStyle(DSColors.text)
                            Spacer()
                            Image(systemName: "xmark")
                                .font(.caption)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                    }
                    .buttonStyle(.plain)
                    .accessibilityLabel("Dismiss error")
                }
            }
            Section {
                ForEach(filtered) { block in
                    NavigationLink {
                        BlockDetailView(store: store, blockID: block.id)
                    } label: {
                        blockRow(for: block)
                    }
                    .swipeActions(edge: .leading, allowsFullSwipe: false) {
                        Button {
                            Task { duplicatingBlock = await store.detail(id: block.id) }
                        } label: {
                            Label("Duplicate", systemImage: "plus.square.on.square")
                        }
                        .tint(DSColors.accent)
                    }
                    .swipeActions(edge: .trailing, allowsFullSwipe: true) {
                        Button(role: .destructive) {
                            deletingBlock = block
                            showingDeleteAlert = true
                        } label: {
                            Label("Delete", systemImage: "trash")
                        }
                    }
                }
            }
                }
                .listStyle(.automatic)
            }
        }
    }

    /// Shown when the list itself is non-empty but the current
    /// search/type combination hides every row.
    private var noMatches: some View {
        VStack(spacing: DSSpacing.md) {
            Spacer()
            Text("No blocks match your search.")
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Spacer()
        }
        .padding()
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    /// Two-line row: name + "Kind · N exercises" subtitle with
    /// the kind config appended when it carries meaning
    /// ("Circuit · 3 exercises · 4 rounds").
    private func blockRow(for block: BlockSummaryDTO) -> some View {
        VStack(alignment: .leading, spacing: 2) {
            Text(block.name)
                .font(.body)
                .foregroundStyle(DSColors.text)
            Text(blockSubtitle(for: block))
                .font(.subheadline)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(.vertical, DSSpacing.xxs)
    }

    private func blockSubtitle(for block: BlockSummaryDTO) -> String {
        let exercises = block.itemCount == 1 ? "1 exercise" : "\(block.itemCount) exercises"
        var parts = [block.type.displayName, exercises]
        switch block.type {
        case .standard:
            break
        case .circuit:
            parts.append("\(block.rounds) rounds")
        case .amrap:
            parts.append("\(block.timeCapSeconds / 60) min cap")
        case .emom:
            parts.append("\(block.rounds) min")
        }
        return parts.joined(separator: " · ")
    }

    private var emptyState: some View {
        VStack(spacing: DSSpacing.md) {
            ZStack {
                RoundedRectangle(cornerRadius: DSSpacing.cornerRadius, style: .continuous)
                    .fill(DSColors.surfaceElevated)
                Image(systemName: "rectangle.3.group")
                    .font(.system(size: 24))
                    .foregroundStyle(DSColors.text)
            }
            .frame(width: 48, height: 48)

            Text("No blocks yet")
                .font(.title3.weight(.semibold))
                .foregroundStyle(DSColors.text)
            Text("Group exercises you repeat — a strength day, a circuit, an AMRAP or an EMOM — and plan the targets up front.")
                .font(.body)
                .foregroundStyle(DSColors.textSecondary)
                .multilineTextAlignment(.center)
            Text("Tap + to build your first block.")
                .font(.footnote)
                .foregroundStyle(DSColors.textSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }

    private func errorState(_ message: String) -> some View {
        VStack(spacing: DSSpacing.md) {
            Image(systemName: Icons.warning)
                .font(.largeTitle)
                .foregroundStyle(DSColors.destructive)
            Text(message)
                .multilineTextAlignment(.center)
                .foregroundStyle(DSColors.textSecondary)
            Button("Try again") {
                Task { await store.load() }
            }
            .buttonStyle(.dsSecondary)
        }
        .padding(DSSpacing.lg)
        .frame(maxWidth: .infinity, maxHeight: .infinity)
    }
}

#Preview {
    NavigationStack {
        BlocksListView(store: BlockStore(api: APIClient(
            baseURL: URL(string: "http://localhost:8080/api/v1")!,
            tokenProvider: { nil }
        )))
    }
    .environmentObject(AppEnvironment.live(baseURL: URL(string: "http://localhost:8080/api/v1")!))
}
