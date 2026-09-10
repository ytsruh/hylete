import PhotosUI
import SwiftUI

/// Create / edit sheet for a weight entry. Fields: weight, notes,
/// optional date, and up to three progress photos (front / side /
/// back). Save POSTs (create) or PUTs (edit) via the shared
/// `WeightStore`.
///
/// Photos go through the `WeightPhotoUploader`, which asks the
/// server for a presigned R2 PUT URL (namespaced per angle) and
/// uploads the bytes directly. The form stores the returned keys
/// and submits them to the create / update endpoint alongside the
/// rest of the entry. Each angle slot uploads independently, so a
/// save can involve up to three upload round-trips followed by the
/// single entry POST / PUT.
///
/// The date input uses a `.compact` picker defaulting to today;
/// picking an earlier date backdates the entry.
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

    /// Per-angle photo state. `pickedData` is the bytes the
    /// PhotosPicker just handed us; `useExisting` is true when
    /// the user wants to keep the existing photo in that slot
    /// (the edit-mode default) and false once they've picked a
    /// replacement or explicitly removed it.
    @State private var frontSlot = PhotoSlot(angle: .front)
    @State private var sideSlot = PhotoSlot(angle: .side)
    @State private var backSlot = PhotoSlot(angle: .back)
    @State private var frontPickerItem: PhotosPickerItem?
    @State private var sidePickerItem: PhotosPickerItem?
    @State private var backPickerItem: PhotosPickerItem?
    @State private var uploadingAngles: Set<WeightEntryDTO.PhotoAngle> = []

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

    /// `true` while any angle slot is uploading. Save is
    /// disabled until uploads finish so the submission never
    /// races the presigned PUTs.
    private var isUploadingPhoto: Bool {
        !uploadingAngles.isEmpty
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
                Text("This permanently removes the weight entry and its photos.")
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
    /// past entry.
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

    /// Three photo slots (front / side / back). Each slot is an
    /// independent picker + preview + remove control sharing one
    /// upload pipeline at save time.
    private var photoSection: some View {
        Section {
            photoSlotRow(angle: .front, slot: $frontSlot, pickerItem: $frontPickerItem)
            photoSlotRow(angle: .side, slot: $sideSlot, pickerItem: $sidePickerItem)
            photoSlotRow(angle: .back, slot: $backSlot, pickerItem: $backPickerItem)

            if isUploadingPhoto {
                HStack {
                    ProgressView()
                    Text("Uploading photos…")
                        .font(.caption)
                        .foregroundStyle(DSColors.textSecondary)
                }
            }
        } header: {
            Text("Photos")
        } footer: {
            Text("Optional. Front is required for comparison; side/back add detail.")
        }
    }

    /// One angle slot: picker affordance with preview, plus a
    /// per-slot remove button when the slot holds a photo.
    private func photoSlotRow(
        angle: WeightEntryDTO.PhotoAngle,
        slot: Binding<PhotoSlot>,
        pickerItem: Binding<PhotosPickerItem?>
    ) -> some View {
        VStack(alignment: .leading, spacing: DSSpacing.xs) {
            PhotosPicker(
                selection: pickerItem,
                matching: .images,
                photoLibrary: .shared()
            ) {
                HStack(spacing: DSSpacing.sm) {
                    slotThumbnail(angle: angle, slot: slot.wrappedValue)
                        .frame(width: 64, height: 64)
                        .clipShape(RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous))
                    VStack(alignment: .leading) {
                        Text(slotLabel(angle: angle, slot: slot.wrappedValue))
                            .font(.subheadline)
                            .foregroundStyle(DSColors.text)
                        Text("Tap to change")
                            .font(.caption)
                            .foregroundStyle(DSColors.textSecondary)
                    }
                    Spacer()
                }
            }
            .buttonStyle(.plain)
            .onChange(of: pickerItem.wrappedValue) { _, newValue in
                handlePhotoSelection(newValue, angle: angle)
            }

            if slotHasPhotoToSubmit(angle: angle, slot: slot.wrappedValue) {
                Button(role: .destructive) {
                    clearPhotoSelection(angle: angle)
                } label: {
                    Label("Remove \(angle.rawValue) photo", systemImage: "xmark.circle")
                        .font(.subheadline)
                }
            }
        }
        .padding(.vertical, DSSpacing.xxs)
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

    // MARK: - Slot helpers

    /// Binding-free accessor used by the save path.
    private func slot(for angle: WeightEntryDTO.PhotoAngle) -> PhotoSlot {
        switch angle {
        case .front: return frontSlot
        case .side: return sideSlot
        case .back: return backSlot
        }
    }

    /// `true` when the slot has a photo that will be submitted —
    /// either freshly picked bytes or an existing photo the user
    /// hasn't removed.
    private func slotHasPhotoToSubmit(angle: WeightEntryDTO.PhotoAngle, slot: PhotoSlot) -> Bool {
        if slot.pickedData != nil { return true }
        if case .edit(let entry) = mode, entry.hasPhoto(for: angle), slot.useExisting {
            return true
        }
        return false
    }

    /// `true` when the slot should render a preview thumbnail
    /// instead of the empty "Add" affordance.
    private func slotHasPhotoToShow(angle: WeightEntryDTO.PhotoAngle, slot: PhotoSlot) -> Bool {
        if slot.pickedData != nil { return true }
        if case .edit(let entry) = mode, entry.hasPhoto(for: angle) { return true }
        return false
    }

    private func slotLabel(angle: WeightEntryDTO.PhotoAngle, slot: PhotoSlot) -> String {
        let title = angle.rawValue.capitalized
        if slot.pickedData != nil { return "\(title) — new photo selected" }
        if case .edit(let entry) = mode, entry.hasPhoto(for: angle), slot.useExisting {
            return "Current \(angle.rawValue) photo"
        }
        if slotHasPhotoToShow(angle: angle, slot: slot) {
            return "\(title) photo"
        }
        return "Add \(angle.rawValue) photo"
    }

    @ViewBuilder
    private func slotThumbnail(angle: WeightEntryDTO.PhotoAngle, slot: PhotoSlot) -> some View {
        if let data = slot.pickedData, let uiImage = UIImage(data: data) {
            Image(uiImage: uiImage)
                .resizable()
                .scaledToFill()
        } else if case .edit(let entry) = mode, entry.hasPhoto(for: angle),
                  let url = URL(string: entry.photoURL(for: angle)) {
            AsyncImage(url: url) { phase in
                switch phase {
                case .success(let image):
                    image.resizable().scaledToFill()
                default:
                    slotPlaceholder(angle: angle)
                }
            }
        } else {
            slotPlaceholder(angle: angle)
        }
    }

    private func slotPlaceholder(angle: WeightEntryDTO.PhotoAngle) -> some View {
        RoundedRectangle(cornerRadius: DSSpacing.cornerRadiusSmall, style: .continuous)
            .fill(DSColors.surfaceElevated)
            .overlay(
                VStack(spacing: 2) {
                    Image(systemName: "photo")
                        .foregroundStyle(DSColors.textSecondary)
                    Text(angle.rawValue.capitalized)
                        .font(.caption2)
                        .foregroundStyle(DSColors.textSecondary)
                }
            )
    }

    /// Handles the user picking a new photo for an angle slot.
    /// Stores the bytes plus the inferred content type / filename
    /// so the upload step can reuse them, and marks the existing
    /// photo as replaced.
    private func handlePhotoSelection(_ item: PhotosPickerItem?, angle: WeightEntryDTO.PhotoAngle) {
        guard let item else { return }
        Task { @MainActor in
            do {
                if let data = try await item.loadTransferable(type: Data.self) {
                    let contentType = item.supportedContentTypes.first?.preferredMIMEType ?? "image/jpeg"
                    let ext = item.supportedContentTypes.first?.preferredFilenameExtension ?? "jpg"
                    var updated = slot(for: angle)
                    updated.pickedData = data
                    updated.pickedContentType = contentType
                    updated.pickedFilename = "weight-\(angle.rawValue).\(ext)"
                    updated.useExisting = false
                    setSlot(updated, angle: angle)
                    // Clear the picker item so the same photo can be
                    // re-picked after a remove.
                    clearPickerItem(angle: angle)
                }
            } catch {
                self.errorMessage = "Could not read the selected \(angle.rawValue) photo."
            }
        }
    }

    /// Resets an angle slot. Clears picked bytes and (when editing)
    /// marks the existing photo for deletion.
    private func clearPhotoSelection(angle: WeightEntryDTO.PhotoAngle) {
        var updated = slot(for: angle)
        updated.pickedData = nil
        updated.useExisting = false
        setSlot(updated, angle: angle)
        clearPickerItem(angle: angle)
    }

    private func setSlot(_ slot: PhotoSlot, angle: WeightEntryDTO.PhotoAngle) {
        switch angle {
        case .front: frontSlot = slot
        case .side: sideSlot = slot
        case .back: backSlot = slot
        }
    }

    private func clearPickerItem(angle: WeightEntryDTO.PhotoAngle) {
        switch angle {
        case .front: frontPickerItem = nil
        case .side: sidePickerItem = nil
        case .back: backPickerItem = nil
        }
    }

    // MARK: - Save / Delete

    /// Multi-phase save: upload each picked photo first (namespaced
    /// per angle) to get the storage keys, then POST / PUT the entry.
    /// Each angle uploads sequentially so the spinner can name the
    /// in-flight slot.
    private func save() async {
        errorMessage = nil
        guard let weight = parsedWeight, weight >= 0, weight <= 1000 else {
            errorMessage = "Weight must be between 0 and 1000."
            return
        }

        var keys: [WeightEntryDTO.PhotoAngle: String] = [:]
        for angle in WeightEntryDTO.PhotoAngle.allCases {
            let current = slot(for: angle)
            if let data = current.pickedData {
                uploadingAngles.insert(angle)
                do {
                    keys[angle] = try await WeightPhotoUploader(api: api).upload(
                        data: data,
                        filename: current.pickedFilename,
                        contentType: current.pickedContentType,
                        angle: angle.rawValue
                    )
                } catch let error as APIError {
                    uploadingAngles.remove(angle)
                    errorMessage = error.errorDescription
                    return
                } catch {
                    uploadingAngles.remove(angle)
                    errorMessage = "\(angle.rawValue.capitalized) photo upload failed."
                    return
                }
                uploadingAngles.remove(angle)
            } else if case .edit(let entry) = mode, entry.hasPhoto(for: angle), current.useExisting {
                keys[angle] = entry.photoKey(for: angle)
            }
            // Otherwise the key stays absent, which tells the server
            // to clear that slot (when editing) or leave it empty
            // (when creating).
        }

        // Submit the picked date whenever the user has
        // nudged it off "today" — keeps the round-trip
        // minimal (no field) when the entry is the
        // implicit "logged right now" case, and the
        // server's "default to time.Now()" kicks in.
        let cal = Calendar.current
        let isToday = cal.isDateInToday(createdAt)
        let encodedCreatedAt: Date? = isToday ? nil : createdAt

        isSaving = true
        defer { isSaving = false }

        // `WeightStore` methods are non-throwing: they report
        // failure via a nil result / `errorMessage`, so no
        // do/catch is needed here. (The photo uploads above
        // do throw and keep their own do/catch.)
        switch mode {
        case .create:
            let request = CreateWeightEntryRequest(
                weight: weight,
                notes: trimmedNotes,
                frontPhotoKey: keys[.front] ?? "",
                sidePhotoKey: keys[.side] ?? "",
                backPhotoKey: keys[.back] ?? "",
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
                frontPhotoKey: keys[.front] ?? "",
                removeFrontPhoto: entry.hasPhoto(for: .front) && keys[.front] == nil,
                sidePhotoKey: keys[.side] ?? "",
                removeSidePhoto: entry.hasPhoto(for: .side) && keys[.side] == nil,
                backPhotoKey: keys[.back] ?? "",
                removeBackPhoto: entry.hasPhoto(for: .back) && keys[.back] == nil,
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
        frontSlot.useExisting = entry.hasPhoto(for: .front)
        sideSlot.useExisting = entry.hasPhoto(for: .side)
        backSlot.useExisting = entry.hasPhoto(for: .back)
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

/// Per-angle photo slot state for the weight editor. A value type
/// so each slot can live in its own `@State` without a view model.
struct PhotoSlot: Equatable {
    /// Which angle slot this state belongs to.
    let angle: WeightEntryDTO.PhotoAngle
    /// Freshly-picked bytes awaiting upload (nil = none).
    var pickedData: Data?
    /// MIME type inferred from the picked item.
    var pickedContentType: String = "image/jpeg"
    /// Filename sent with the presigned-URL request.
    var pickedFilename: String = "photo.jpg"
    /// Whether to keep the existing server-side photo in this
    /// slot (edit mode default when the entry has one).
    var useExisting: Bool = false
}
