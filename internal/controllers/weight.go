package controllers

import (
	"context"
	"fmt"
	"io"
	"log"
	"sort"
	"strings"
	"time"

	"hylete/internal/export"
	"hylete/internal/models"
	"hylete/internal/utils"
)

// WeightController handles body weight entry business logic.
type WeightController struct {
	repo   models.WeightRepo
	photos export.PhotoGetter
}

// NewWeightController creates a new WeightController instance.
// photos is used by ExportWeightZip to pull photos out of R2. It is
// accepted as an interface (not a concrete type) so the export logic
// can be unit-tested without R2; production wiring is done in main.go.
func NewWeightController(repo models.WeightRepo, photos export.PhotoGetter) *WeightController {
	return &WeightController{repo: repo, photos: photos}
}

// ListWeightEntries returns all weight entries for a user ordered by date descending.
func (wc *WeightController) ListWeightEntries(userID string) ([]models.WeightEntry, error) {
	return wc.repo.List(userID)
}

// CreateWeightEntry creates a new weight entry for the given user with the current timestamp.
// Each of the three photo keys is optional; pass "" for angles without a photo.
func (wc *WeightController) CreateWeightEntry(userID string, weight float64, notes string, frontPhotoKey, sidePhotoKey, backPhotoKey string) (*models.WeightEntry, error) {
	entry := &models.WeightEntry{
		Weight:        weight,
		Notes:         notes,
		FrontPhotoKey: frontPhotoKey,
		SidePhotoKey:  sidePhotoKey,
		BackPhotoKey:  backPhotoKey,
		UserID:        userID,
		CreatedAt:     time.Now(),
	}
	if err := wc.repo.Create(entry); err != nil {
		return nil, err
	}
	return entry, nil
}

// GetWeightEntry retrieves a single weight entry by ID, scoped to the user.
func (wc *WeightController) GetWeightEntry(id, userID string) (*models.WeightEntry, error) {
	return wc.repo.GetByID(id, userID)
}

// GetWeightEntriesForCompare fetches two weight entries by ID for the
// image-comparison feature. Both must belong to the given user and both
// must have a photo in the requested angle slot. Returned slice is sorted
// by created_at ascending so callers can treat [0] as "before" and [1]
// as "after". Returns an error when the pair is incomplete, the entries
// are missing, or either entry lacks that angle's photo — these are
// presented to the user inline, not as a 500.
func (wc *WeightController) GetWeightEntriesForCompare(idA, idB, userID string, angle models.WeightPhotoAngle) ([]models.WeightEntry, error) {
	if !angle.IsValid() {
		return nil, fmt.Errorf("unknown photo angle %q", string(angle))
	}
	if idA == "" || idB == "" || idA == idB {
		return nil, fmt.Errorf("please choose two different weight entries to compare")
	}
	entries, err := wc.repo.GetByIDs(idA, idB, userID)
	if err != nil {
		return nil, err
	}
	if len(entries) < 2 {
		return nil, fmt.Errorf("could not find both weight entries")
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].CreatedAt.Before(entries[j].CreatedAt)
	})
	for _, e := range entries {
		if !e.HasPhotoForAngle(angle) {
			shared := models.SharedWeightPhotoAngles(&entries[0], &entries[1])
			if len(shared) == 0 {
				return nil, fmt.Errorf("these entries share no photo angle to compare")
			}
			names := make([]string, 0, len(shared))
			for _, a := range shared {
				names = append(names, string(a))
			}
			return nil, fmt.Errorf("one entry has no %s photo — both entries share: %s", string(angle), strings.Join(names, ", "))
		}
	}
	return entries, nil
}

// UpdateWeightEntry updates an existing weight entry including its timestamp.
// Each photo key is set verbatim; pass "" to clear a slot.
func (wc *WeightController) UpdateWeightEntry(id, userID string, weight float64, notes string, frontPhotoKey, sidePhotoKey, backPhotoKey string, createdAt time.Time) (*models.WeightEntry, error) {
	entry := &models.WeightEntry{
		ID:            id,
		Weight:        weight,
		Notes:         notes,
		FrontPhotoKey: frontPhotoKey,
		SidePhotoKey:  sidePhotoKey,
		BackPhotoKey:  backPhotoKey,
		UserID:        userID,
		CreatedAt:     createdAt,
	}
	if err := wc.repo.Update(entry, userID); err != nil {
		return nil, err
	}
	return entry, nil
}

// DeleteWeightEntry removes a weight entry by ID, scoped to the user.
// Associated photos in R2 are deleted best-effort across all three
// angle slots (errors are logged but do not block the DB delete).
func (wc *WeightController) DeleteWeightEntry(id, userID string) error {
	existing, err := wc.repo.GetByID(id, userID)
	if err != nil {
		return err
	}
	if existing != nil {
		for _, key := range existing.PhotoKeys() {
			if delErr := utils.DeleteObject(key); delErr != nil {
				log.Printf("warning: failed to delete weight photo %q from R2: %v", key, delErr)
			}
		}
	}
	return wc.repo.Delete(id, userID)
}

// SuggestedExportFilename returns the recommended filename for a
// user's weight export zip. Exposed so the route layer (and tests)
// don't need to know the format.
func SuggestedExportFilename(now time.Time) string {
	return "hylete-weight-export-" + now.UTC().Format("2006-01-02") + ".zip"
}

// ExportWeightZip builds a streaming zip archive of the user's weight
// entries (and any photos that can be fetched) and returns a reader
// the caller can pipe straight to the HTTP response. The string
// returned alongside is the suggested filename for the
// Content-Disposition header.
//
// The export is built in a background goroutine writing into an
// io.Pipe, so the HTTP layer can start sending bytes as soon as the
// first zip entry is written — no in-memory buffering of the full
// archive.
//
// weightUnit is the user's preferred display unit ("kg" or "lbs") and
// is recorded in the CSV's weight_unit column so the file is
// self-describing — the weight column is otherwise a unit-agnostic
// number.
//
// The error returned covers failures up to the point of returning the
// pipe reader; once streaming has started, errors are surfaced by the
// goroutine closing the pipe with the error, which io.Copy will
// propagate to the response writer.
func (wc *WeightController) ExportWeightZip(ctx context.Context, userID string, weightUnit string) (io.Reader, string, error) {
	entries, err := wc.repo.List(userID)
	if err != nil {
		return nil, "", fmt.Errorf("load weight entries: %w", err)
	}

	pr, pw := io.Pipe()
	filename := SuggestedExportFilename(time.Now())

	go func() {
		// Use a detached context for the actual build so an
		// in-flight export isn't cancelled when the request
		// context is. The pipe closure is the only way to
		// signal a mid-stream error to io.Copy.
		buildCtx := context.Background()
		_, buildErr := export.BuildWeightZip(buildCtx, pw, entries, wc.photos, userID, weightUnit)
		if buildErr != nil {
			_ = pw.CloseWithError(buildErr)
			return
		}
		_ = pw.Close()
	}()

	return pr, filename, nil
}
