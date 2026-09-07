// Package export produces downloadable archives of the user's data.
//
// The package is intentionally tiny and side-effect-free: callers hand
// it a slice of model records and a PhotoGetter, and it writes a
// complete, self-describing zip to an io.Writer. The HTTP layer wraps
// this in an io.Pipe so the response can stream end-to-end with no
// in-memory buffering of the full archive.
package export

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"time"

	"hylete/internal/models"
)

// PhotoGetter is the read-side of the object store, abstracted so the
// export package can be unit-tested without R2.
type PhotoGetter interface {
	// Get returns a stream of the object's bytes. Callers (this
	// package) are responsible for closing it. A non-nil error means
	// the object could not be retrieved; the export will then skip
	// the photo and record the key in the manifest's missing_photos
	// list.
	Get(ctx context.Context, key string) (io.ReadCloser, error)
}

// Result summarises what BuildWeightZip produced. Returned alongside
// the zip so the caller can log or surface useful diagnostics
// (e.g. "exported 42 entries, 17 photos, 1 photo missing in R2").
type Result struct {
	Entries       int      `json:"entries"`
	PhotosWritten int      `json:"photos_written"`
	MissingPhotos []string `json:"missing_photos"`
}

// manifest is the on-disk shape of manifest.json. userID and
// generatedAt are filled in by the caller (controller) so this package
// stays free of any user-context plumbing.
type manifest struct {
	UserID        string   `json:"user_id"`
	GeneratedAt   string   `json:"generated_at"`
	EntryCount    int      `json:"entry_count"`
	PhotoCount    int      `json:"photo_count"`
	MissingPhotos []string `json:"missing_photos"`
}

// pendingPhoto is the intermediate representation of a photo that has
// been successfully fetched from R2 and is ready to be written into
// the zip. We buffer the bytes in memory because zip.Writer only
// allows one open file body at a time, so we cannot interleave
// "fetch from R2" with "write to zip entry" while still having a
// fully-formed CSV that references the photo names.
type pendingPhoto struct {
	name string
	body []byte
}

// BuildWeightZip writes a zip archive to w containing the user's
// weight entries and any photos that can be fetched from photos.
//
// Layout of the archive:
//   - weight.csv          CSV with one row per entry (header included)
//   - photos/<name>.<ext> one file per entry that has a retrievable photo
//   - manifest.json       export metadata, including missing_photos
//
// Entries are sorted ascending by CreatedAt before being written so
// the resulting files are deterministic regardless of the order in the
// repository.
//
// Photos whose R2 fetch fails are NOT written to the zip; the entry is
// still included in weight.csv (with an empty photo_filename) and the
// offending key is appended to manifest.json's missing_photos list.
//
// weightUnit is the user's preferred display unit ("kg" or "lbs").
// The DB stores weights as a unit-agnostic number, so the CSV's
// `weight` column is written verbatim. The `weight_unit` column
// records the same value on every row so the file is self-describing
// when opened in a spreadsheet.
func BuildWeightZip(ctx context.Context, w io.Writer, entries []models.WeightEntry, photos PhotoGetter, userID string, weightUnit string) (Result, error) {
	// Sort a copy so the caller's slice is untouched.
	sorted := make([]models.WeightEntry, len(entries))
	copy(sorted, entries)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].CreatedAt.Before(sorted[j].CreatedAt)
	})

	result := Result{Entries: len(sorted), MissingPhotos: []string{}}

	// --- Phase 1: pre-fetch every photo. --------------------------------
	// Successful fetches are buffered in memory so the zip-writing
	// phase can do straight byte copies with no further R2 I/O. Failed
	// fetches are recorded so the CSV row's photo_filename stays
	// empty and the manifest reflects the gap.
	//
	// Memory cost: total bytes of photos the user has. For a personal
	// fitness log this is typically tens to a few hundred MB. If that
	// ever becomes a problem the next step is to spool to a temp file,
	// but that's deferred until it actually matters.
	pending := make([]pendingPhoto, 0, len(sorted))
	for i, e := range sorted {
		if e.PhotoKey == "" {
			continue
		}
		rc, err := photos.Get(ctx, e.PhotoKey)
		if err != nil {
			result.MissingPhotos = append(result.MissingPhotos, e.PhotoKey)
			continue
		}
		body, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil {
			result.MissingPhotos = append(result.MissingPhotos, e.PhotoKey)
			continue
		}
		pending = append(pending, pendingPhoto{
			name: buildPhotoFilename(e, i),
			body: body,
		})
		result.PhotosWritten++
	}

	// --- Phase 2: write the zip. ----------------------------------------
	// Zip writers only allow one open file body at a time, so the
	// order is strict: open the CSV, write+flush it fully, then write
	// each photo in turn, then the manifest.
	zw := zip.NewWriter(w)

	csvBody, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "weight.csv",
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: create csv entry: %w", err)
	}
	cw := csv.NewWriter(csvBody)
	if err := cw.Write([]string{"id", "created_at", "date", "weight", "weight_unit", "notes", "photo_filename"}); err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: write csv header: %w", err)
	}
	// Map of entry index -> photo filename ("" if missing/has no photo).
	photoNameFor := make(map[int]string, len(pending))
	for i, p := range pending {
		photoNameFor[i] = p.name
	}
	for i, e := range sorted {
		if err := cw.Write([]string{
			e.ID,
			e.CreatedAt.UTC().Format(time.RFC3339),
			e.CreatedAt.UTC().Format("2006-01-02"),
			fmt.Sprintf("%.1f", e.Weight),
			weightUnit,
			e.Notes,
			photoNameFor[i],
		}); err != nil {
			_ = zw.Close()
			return result, fmt.Errorf("export: write csv row: %w", err)
		}
	}
	cw.Flush()
	if err := cw.Error(); err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: flush csv: %w", err)
	}

	// --- Phase 2b: write the photos. ------------------------------------
	for i, p := range pending {
		photoFile, err := zw.CreateHeader(&zip.FileHeader{
			Name:     "photos/" + p.name,
			Method:   zip.Deflate,
			Modified: sorted[i].CreatedAt,
		})
		if err != nil {
			_ = zw.Close()
			return result, fmt.Errorf("export: create photo entry: %w", err)
		}
		if _, copyErr := io.Copy(photoFile, bytes.NewReader(p.body)); copyErr != nil {
			_ = zw.Close()
			return result, fmt.Errorf("export: write photo %s: %w", p.name, copyErr)
		}
	}

	// --- Phase 2c: write the manifest. ----------------------------------
	if result.MissingPhotos == nil {
		result.MissingPhotos = []string{}
	}
	mf, err := zw.CreateHeader(&zip.FileHeader{
		Name:     "manifest.json",
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: create manifest: %w", err)
	}
	enc := json.NewEncoder(mf)
	enc.SetIndent("", "  ")
	if err := enc.Encode(manifest{
		UserID:        userID,
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339),
		EntryCount:    result.Entries,
		PhotoCount:    result.PhotosWritten,
		MissingPhotos: result.MissingPhotos,
	}); err != nil {
		_ = zw.Close()
		return result, fmt.Errorf("export: encode manifest: %w", err)
	}

	if err := zw.Close(); err != nil {
		return result, fmt.Errorf("export: close zip: %w", err)
	}
	return result, nil
}

// buildPhotoFilename produces a stable, human-readable filename for a
// photo inside the zip. The form is:
//
//	YYYY-MM-DD_<short-id>.<ext>
//
// where <short-id> is the first 8 characters of the entry ID and
// <ext> is the lowercased extension of the original photo key (or .bin
// when the key has no extension). Using the date in the filename keeps
// photos in chronological order when the zip is extracted and viewed
// in a file browser.
func buildPhotoFilename(e models.WeightEntry, index int) string {
	date := e.CreatedAt.UTC().Format("2006-01-02")
	shortID := e.ID
	if len(shortID) > 8 {
		shortID = shortID[:8]
	}
	if shortID == "" {
		shortID = fmt.Sprintf("row-%d", index)
	}
	ext := strings.ToLower(path.Ext(e.PhotoKey))
	if ext == "" {
		ext = ".bin"
	}
	return fmt.Sprintf("%s_%s%s", date, shortID, ext)
}
