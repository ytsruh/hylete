import PhotosUI
import SwiftUI

/// Create / edit sheet for a weight entry. Mirrors the
/// web's `/weight/new` and `/weight/:id/edit` forms
/// (`internal/views/weight/form.templ`): weight, notes,
/// optional date, and an optional photo. Save POSTs
/// (create) or PUTs (edit) via the shared `WeightStore`.
///
/// Photos go through the `WeightPhotoUploader`, which
/// asks the server for a presigned R2 PUT URL and uploads
/// the bytes directly. The form stores the returned
/// `photoKey` and submits it to the create / update
/// endpoint alongside the rest of the entry.
///
/// The date input uses a `.compact` picker. The web form
/// toggles between "Set" and "Clear" buttons inline; the
/// iOS editor mirrors that with a "Clear" button that
/// resets the entry's timestamp to the current time.
struct WeightEditorView: View {
    enum Mode {
        case create
        case edit(WeightEntryDTO)
    }

    @EnvironmentObject private var env: AppEnvironment
    @EnvironmentObject private var authStore: AuthStore
    @Environment(\.dismiss) private var dismiss

    let mode: Mode
    /// Shared with `WeightListView` so the list picks up
    /// the new / edited row in place (optimistic insertion
    /// + server confirmation).
    @ObservedObject var store: WeightStore

    @State private var weightText: String = ""
    @State private var notes: String = ""
    @State private var createdAt: Date = Date()

    /// Photo state. The editor seeds from the existing
    /// entry (when editing) and lets the user pick a new
    /// photo, remove the existing one, or leave it
    /// unchanged. `pickedPhotoData` is the bytes the
    /// PhotosPicker just handed us; `useExistingPhoto`
    /// is true when the user wants to keep the existing
    /// photo (the default) and false when they've
    /// explicitly removed it.
    @State private var pickedPhotoData: Data?
    @State private var pickedPhotoContentType: String = "image/jpeg"
    @State private var pickedPhotoFilename: String = "photo.jpg"
    @State private var useExistingPhoto: Bool = true
    @State private var isUploadingPhoto: Bool = false

    @State private var isSaving: Bool = false
    @State private var errorMessage: String?

    /// Tracked across the upload so the editor can show
    /// a small spinner during the PUT. Distinct from the
    /// save spinner so the user can tell which round-trip
    /// is in flight.
    private var isEditing: Bool {
        if case .edit = mode { return true }
        return false
    }

    /// The user's preferred unit — drives the placeholder
    /// and the inline unit label. Falls back to "kg" when
    /// the user isn't signed in (the editor is unreachable
    /// in that state, but the call stays safe).
    private var weightUnit: String {
        authStore.currentUser?.weightUnit ?? "kg"
    }

    private var trimmedNotes: String {
        notes.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var parsedWeight: Double? {
        let trimmed = weightText.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty, let value = Double(trimmed) else { return nil }
        return value
    }

    /// Save is enabled when the weight parses into the
    /// server's accepted range (0–1000) and we're not
    /// already saving. The server enforces the same caps
    /// so a client-side check here just gives the user
    /// instant feedback.
    private var canSave: Bool {
        guard !isSaving, !isUploadingPhoto else { return false }
        guard let weight = parsedWeight, weight >= 0, weight <= 1000 else { return false }
        return true
    }

    /// `true` when the editor has a photo to upload — either
    /// the user just picked one, or the existing entry has
    /// a photo and the user hasn't removed it. Drives the
    /// submitter's choice of `photoKey`.
    private var hasPhotoToSubmit: Bool {
        if pickedPhotoData != nil { return true }
        if case .edit(let entry) = mode, entry.hasPhoto, useExistingPhoto {
            return true
        }
        return false
    }

    /// `true` when the photo section should render the
    /// preview thumbnail (and the "Tap to change" label)
    /// instead of the "Add photo" affordance. Mirrors
    /// `hasPhotoToSubmit` but stays separate so the picker
    /// can offer a preview even when the user has marked
    /// the existing photo for deletion (the preview still
    /// shows what they're about to throw away).
    private var hasPhotoToShow: Bool {
        if pickedPhotoData != nil { return true }
        if case .edit(let entry) = mode, entry.hasPhoto { return true }
        return false
    }

    var body: some View {
        NavigationStack {
            Form {
                weightSection
                notesSection
                dateSection
                photoSection
                if isEditing {
                    deleteSection
                }
                if let errorMessage {
                    Section {
                        Text(errorMessage)
                            .font(.footnote)
                            .foregroundStyle(DSColors.destructive)
                    }
                }
            }
            .navigationTitle(isEditing ? "Edit Weight" : "Log Weight")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .topBarLeading) {
                    Button("Cancel") { dismiss() }
                        .disabled(isSaving || isUploadingPhoto)
                }
                ToolbarItem(placement: .topBarTrailing) {
                    Button {
                        Task { await save() }
                    } label: {
                        if isSaving {
                            ProgressView()
                        } else {
                            Text("Save").bold()
                        }
                    }
                    .disabled(!canSave)
                }
            }
            .confirmationDialog(
                "Delete this entry?",
                isPresented: $showingDeleteConfirm,
                titleVisibility: .visible
            ) {
                Button("Delete entry", role: .destructive) {
                    Task { await deleteAndDismiss() }
                }
                Button("Cancel", role: .cancel) {}
            } message: {
                Text("This permanently removes the weight entry and its photo.")
            }
            .onAppear { seedIfNeeded() }
        }
    }

    // MARK: - Sections

    private var weightSection: some View {
        Section {
            HStack {
                TextField("85.0", text: $weightText)
                    .keyboardType(.decimalPad)
                Text(weightUnit)
                    .foregroundStyle(DSColors.textSecondary)
            }
        } header: {
            Text("Weight")
        } footer: {
            Text("Values in your preferred unit (\(weightUnit)).")
        }
    }

    private var notesSection: some View {
        Section {
            TextField("Notes (optional)", text: $notes, axis: .vertical)
                .lineLimit(1...4)
        } header: {
            Text("Notes")
        }
    }

    /// Date picker. The iOS-native approach: always show
    /// the date with a sensible default (today) so the
    /// user can simply pick a different day to log a
    /// past entry. The web's explicit "backdate" toggle
    /// was a web-form pattern that doesn't translate
    /// well to iOS — a single compact picker is
    /// recognisably native and stays out of the way.
    private var dateSection: some View {
        Section {
            DatePicker(
                "Date",
                selection: $createdAt,
                in: ...Date(),
                displayedComponents: [.date]
            )
        } header: {
            Text("Date")
        } footer: {
            Text("Defaults to today. Pick an earlier date to log a past entry.")
        }
    }

    /// Photo picker. The current preview shows either the
    /// newly-picked photo, the existing photo (when editing),
    /// or a placeholder. The "Remove" button clears the
    /// picked photo (and marks the existing photo for
    /// deletion when editing).
    private var photoSection: some View {
        Section {
            PhotosPicker(
                selection: $photoPickerItem,
                matching: .images,
                photoLibrary: .shared()
            ) {
                if hasPhotoToShow {
                    HStack(spacing: DSSpacing.sm) {
                        previewThumbnail
                            .frame(width: 64, height: 64)
                            .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
                        VStack(alignment: .leading) {
                            Text(photoPickerLabel)
                                .font(.subheadline)
                                .foregroundStyle(DSColors.text)
                            Text("Tap to change")
                                .font(.caption)
                                .foregroundStyle(DSColors.textSecondary)
                        }
                        Spacer()
                    }
                } else {
                    Label("Add photo", systemImage: "photo")
                        .foregroundStyle(DSColors.text)
                }
            }
            .buttonStyle(.plain)
            .onChange(of: photoPickerItem) { _, newValue in
                handlePhotoSelection(newValue)
            }

            if hasPhotoToSubmit {
                Button(role: .destructive) {
                    clearPhotoSelection()
                } label: {
                    Label("Remove photo", systemImage: "xmark.circle")
                }
            }

            if isUploadingPhoto {
                HStack {
                    ProgressView()
                    Text("Uploading photo…")
                        .font(.caption)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
        } header: {
            Text("Photo")
        } footer: {
            Text("Optional. A photo is required to compare two entries.")
        }
    }

    private var deleteSection: some View {
        Section {
            Button(role: .destructive) {
                showingDeleteConfirm = true
            } label: {
                HStack {
                    Spacer()
                    Text("Delete entry")
                    Spacer()
                }
            }
        }
    }

    @State private var showingDeleteConfirm: Bool = false
    @State private var photoPickerItem: PhotosPickerItem?

    /// The thumbnail shown in the photo section. Always
    /// calls `.resizable()` so the parent can size it
    /// freely — the unmodified image's intrinsic size
    /// would otherwise drive the layout. New pickup wins,
    /// otherwise the existing entry's photo (also resizable).
    /// When neither is set, the placeholder thumbnail is
    /// returned so the call site is always a single
    /// uniform `.frame(...)` modifier chain.
    @ViewBuilder
    private var previewThumbnail: some View {
        if let pickedPhotoData, let uiImage = UIImage(data: pickedPhotoData) {
            Image(uiImage: uiImage)
                .resizable()
                .scaledToFill()
        } else if case .edit(let entry) = mode, entry.hasPhoto,
                  let url = URL(string: entry.photoURL) {
            AsyncImage(url: url) { phase in
                switch phase {
                case .success(let image):
                    image.resizable().scaledToFill()
                default:
                    placeholderThumbnail
                }
            }
        } else {
            placeholderThumbnail
        }
    }

    private var placeholderThumbnail: some View {
        RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
            .fill(DSColors.surfaceElevated)
            .overlay(
                Image(systemName: "photo")
                    .foregroundStyle(DSColors.textSecondary)
            )
    }

    private var photoPickerLabel: String {
        if pickedPhotoData != nil { return "New photo selected" }
        if case .edit(let entry) = mode, entry.hasPhoto, useExistingPhoto {
            return "Current photo"
        }
        return "Add photo"
    }

    // MARK: - Photo helpers

    /// Handles the user picking a new photo. Stores the
    /// bytes and the inferred content type / filename so
    /// the upload step can reuse them. Marks the existing
    /// photo as "not kept" so the submission doesn't carry
    /// both the old and new keys.
    private func handlePhotoSelection(_ item: PhotosPickerItem?) {
        guard let item else { return }
        Task { @MainActor in
            do {
                if let data = try await item.loadTransferable(type: Data.self) {
                    pickedPhotoData = data
                    pickedPhotoContentType = item.supportedContentTypes.first?.preferredMIMEType ?? "image/jpeg"
                    let ext = item.supportedContentTypes.first?.preferredFilenameExtension ?? "jpg"
                    pickedPhotoFilename = "weight-photo.\(ext)"
                    useExistingPhoto = false
                }
            } catch {
                self.errorMessage = "Could not read the selected photo."
            }
        }
    }

    /// Resets the user's photo selection. Clears the
    /// picked bytes and (when editing) marks the existing
    /// photo for deletion.
    private func clearPhotoSelection() {
        pickedPhotoData = nil
        photoPickerItem = nil
        if case .edit(let entry) = mode, entry.hasPhoto {
            useExistingPhoto = false
        }
    }

    // MARK: - Save / Delete

    /// Two-phase save: upload the photo (if any) first to
    /// get the storage key, then POST / PUT the entry. Each
    /// phase has its own spinner so the user can tell
    /// which round-trip is in flight.
    private func save() async {
        errorMessage = nil
        guard let weight = parsedWeight, weight >= 0, weight <= 1000 else {
            errorMessage = "Weight must be between 0 and 1000."
            return
        }

        var photoKey: String = ""
        if let data = pickedPhotoData {
            isUploadingPhoto = true
            defer { isUploadingPhoto = false }
            do {
                photoKey = try await WeightPhotoUploader(api: api).upload(
                    data: data,
                    filename: pickedPhotoFilename,
                    contentType: pickedPhotoContentType
                )
            } catch let error as APIError {
                errorMessage = error.errorDescription
                return
            } catch {
                errorMessage = "Photo upload failed."
                return
            }
        } else if case .edit(let entry) = mode, entry.hasPhoto, useExistingPhoto {
            photoKey = entry.photoKey
        }
        // Otherwise photoKey stays empty / cleared, which
        // tells the server to remove the existing photo
        // (when editing) or simply not set one (when creating).

        // Submit the picked date whenever the user has
        // nudged it off "today" — keeps the round-trip
        // minimal (no field) when the entry is the
        // implicit "logged right now" case, and the
        // server's "default to time.Now()" kicks in.
        let cal = Calendar.current
        let isToday = cal.isDateInToday(createdAt)
        let encodedCreatedAt: Date? = isToday ? nil : createdAt
        let removePhoto: Bool
        if case .edit(let entry) = mode {
            removePhoto = entry.hasPhoto && !useExistingPhoto && pickedPhotoData == nil
        } else {
            removePhoto = false
        }

        isSaving = true
        defer { isSaving = false }

        // `WeightStore` methods are non-throwing: they report
        // failure via a nil result / `errorMessage`, so no
        // do/catch is needed here. (The photo upload above
        // does throw and keeps its own do/catch.)
        switch mode {
        case .create:
            let request = CreateWeightEntryRequest(
                weight: weight,
                notes: trimmedNotes,
                photoKey: photoKey,
                createdAt: encodedCreatedAt
            )
            let created = await store.create(request)
            if created == nil {
                errorMessage = store.errorMessage ?? "Could not save the entry."
                return
            }
        case .edit(let entry):
            let request = UpdateWeightEntryRequest(
                weight: weight,
                notes: trimmedNotes,
                photoKey: photoKey,
                removePhoto: removePhoto,
                createdAt: encodedCreatedAt
            )
            await store.update(id: entry.id, request: request)
            if let msg = store.errorMessage {
                errorMessage = msg
                return
            }
        }
        dismiss()
    }

    private func deleteAndDismiss() async {
        guard case .edit(let entry) = mode else { return }
        isSaving = true
        defer { isSaving = false }
        await store.delete(id: entry.id)
        if store.errorMessage != nil {
            errorMessage = store.errorMessage
            return
        }
        dismiss()
    }

    // MARK: - Seeding

    /// Populates the form from the existing entry on
    /// first appear (edit mode only). Skipped when the
    /// weight is already populated so a SwiftUI re-render
    /// mid-edit doesn't wipe the user's typing.
    private func seedIfNeeded() {
        guard case .edit(let entry) = mode else { return }
        guard weightText.isEmpty else { return }
        weightText = String(format: "%.1f", entry.weight)
        notes = entry.notes
        createdAt = entry.createdAt
        useExistingPhoto = entry.hasPhoto
    }

    // MARK: - Computed dependencies

    /// Convenience accessor for the underlying API client.
    /// Mirrors the `env` pattern used by the other editors
    /// so the form stays decoupled from the environment
    /// plumbing.
    private var api: APIClient {
        env.api
    }
}
