package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"hylete/internal/imaging"
	"hylete/internal/utils"
	"hylete/internal/views"
)

// ExerciseImageConfig controls the two image variants the admin
// upload route produces. Both variants are derived from the same
// upload (a single source image goes in) and share the same 3:1
// banner aspect ratio, matching the ratio-enforced display
// components on both surfaces (components.BannerImage on the web,
// BannerImage in client/Hylete/DesignSystem/Image.swift) so an
// upload in the expected ratio survives the whole pipeline
// untouched. Uploads are expected to already be 3:1 (e.g.
// 3000x1000) — anything else is centre-cropped to fit.
type ExerciseImageConfig struct {
	// DisplayWidth / DisplayHeight is the size written to the
	// "img_url" storage key. Used in cards, dialogs, and any other
	// UI surface that needs the exercise image. Quality 85 keeps
	// file size modest for repeated network fetches.
	DisplayWidth  int
	DisplayHeight int
	// OriginalWidth / OriginalHeight is the size of the
	// "img_url_original" storage key. A 2400x800 source surrogate
	// is large enough to look good in a future full-screen viewer
	// without bloating R2 storage. Quality 95 preserves detail.
	OriginalWidth  int
	OriginalHeight int
}

// DefaultExerciseImageConfig is the production configuration for the
// admin exercise image upload. Centralised so tests can refer to the
// same numbers.
var DefaultExerciseImageConfig = ExerciseImageConfig{
	DisplayWidth:   1200,
	DisplayHeight:  400,
	OriginalWidth:  2400,
	OriginalHeight: 800,
}

// exerciseImageProcessor is the subset of imaging.Processor the
// route uses. Declared as a local interface so the route is unit
// testable with a fake without depending on the full package.
type exerciseImageProcessor interface {
	Process(ctx context.Context, src io.Reader, opts imaging.ProcessOptions) (*imaging.Result, error)
}

// exerciseImageUploader is the subset of utils.PutObject the route
// uses. Declared as a local interface so tests can substitute a
// fake without touching the S3 client.
type exerciseImageUploader interface {
	PutObject(ctx context.Context, key, contentType string, body io.Reader) error
}

// imageUploadedEvent is the detail payload of the `image-uploaded`
// custom event fired via the HX-Trigger response header. The widget
// listens for it on the document to copy the new keys into the form's
// hidden inputs. Field tags match the JS property names so
// json.Marshal emits display_key / original_key.
type imageUploadedEvent struct {
	DisplayKey  string `json:"display_key"`
	OriginalKey string `json:"original_key"`
}

// AdminExerciseImageUpload handles the server-side upload flow for
// exercise images. The browser file input is rendered with htmx
// attributes that POST the multipart form to this route, target
// `#toaster`, and swap the response into the toaster. The route
// therefore returns HTML (a views.Toast), not JSON.
//
// On success: the response body is a success toast, and the
// HX-Trigger header carries an `image-uploaded` event with the
// new storage keys. The widget's JS listens for that event and
// copies the keys into the form's hidden inputs so they survive
// until the form is saved.
//
// On any failure (bad MIME, oversize, decode error, R2 write
// failure): the response body is an error toast. There is no
// HX-Trigger. The widget's hidden inputs are left untouched, so
// the existing image (if any) is preserved.
//
// The 10 MB body cap is enforced via http.MaxBytesReader so a
// hostile client can't exhaust memory before the multipart parser
// runs. The MIME gate (image/jpeg or image/png) is enforced by
// the imaging processor's own sniff — that's the source of truth
// for the supported types.
//
// On any failure mid-flight (e.g. the second upload to R2 errors
// after the first succeeded) the route does best-effort cleanup
// of the partially-written objects so R2 doesn't accumulate
// orphan files.
func (h *Handler) AdminExerciseImageUpload(c echo.Context) error {
	ctx := c.Request().Context()

	// 10 MB cap, matching the widget's client-side pre-check.
	const maxBody = 10 << 20 // 10 MiB
	c.Request().Body = http.MaxBytesReader(c.Response().Writer, c.Request().Body, maxBody)

	// Max multipart memory before spooling to disk. Echo's default
	// is 32 MB which is well above our body cap, so 8 MB is
	// comfortable and keeps the in-memory footprint small.
	if err := c.Request().ParseMultipartForm(8 << 20); err != nil {
		return h.renderToast(c, "error", "Upload failed", "Could not parse upload (file may exceed 10 MB)")
	}

	file, err := c.FormFile("file")
	if err != nil {
		return h.renderToast(c, "error", "Upload failed", "Missing 'file' field")
	}
	if file.Size <= 0 {
		return h.renderToast(c, "error", "Upload failed", "Uploaded file is empty")
	}
	if file.Size > maxBody {
		return h.renderToast(c, "error", "Upload failed", "File exceeds 10 MB limit")
	}

	src, err := file.Open()
	if err != nil {
		return h.renderToast(c, "error", "Upload failed", "Could not read uploaded file")
	}
	defer src.Close()

	// The display variant is the small, frequently-loaded one, so
	// it gets processed first. If the original variant later fails
	// to upload we still have something useful to return, and we
	// can clean up the orphan display object.
	displayRes, err := h.imageProcessor.Process(ctx, src, imaging.ProcessOptions{
		TargetWidth:  h.imageConfig.DisplayWidth,
		TargetHeight: h.imageConfig.DisplayHeight,
		Quality:      85,
		Format:       imaging.FormatJPEG,
	})
	if err != nil {
		return h.renderImagingError(c, err)
	}

	// Re-open the file for the second Process call — the first
	// Process read src to EOF. Multipart File implements io.Seeker
	// (it's backed by a temp file in Echo's default config), so
	// Seek(0) is reliable.
	if _, seekErr := src.Seek(0, io.SeekStart); seekErr != nil {
		return h.renderToast(c, "error", "Upload failed", "Could not re-read uploaded file")
	}
	originalRes, err := h.imageProcessor.Process(ctx, src, imaging.ProcessOptions{
		TargetWidth:  h.imageConfig.OriginalWidth,
		TargetHeight: h.imageConfig.OriginalHeight,
		Quality:      95,
		Format:       imaging.FormatJPEG,
	})
	if err != nil {
		return h.renderImagingError(c, err)
	}

	// Both variants are JPEG (forced by the imaging package's
	// Format constant in this route), so the file extensions and
	// content types are fixed.
	id := uuid.New().String()
	displayKey := "exercises/" + id + ".jpg"
	originalKey := "exercises/" + id + "_original.jpg"

	displayBody := bytes.NewReader(displayRes.Data)
	if err := h.imageUploader.PutObject(ctx, displayKey, displayRes.MIMEType, displayBody); err != nil {
		return h.renderToast(c, "error", "Upload failed", "Could not upload display image")
	}

	if err := h.imageUploader.PutObject(ctx, originalKey, originalRes.MIMEType, bytes.NewReader(originalRes.Data)); err != nil {
		// Best-effort cleanup: drop the display object we just
		// uploaded so we don't leave an orphan in R2.
		if delErr := utils.DeleteObject(displayKey); delErr != nil {
			c.Logger().Warnf("failed to clean up display image %q after original upload failed: %v", displayKey, delErr)
		}
		return h.renderToast(c, "error", "Upload failed", "Could not upload original image")
	}

	// Success: render the toast and tell the widget's JS which
	// keys to copy into the hidden inputs. The HX-Trigger header
	// is a JSON-encoded event name + detail object; htmx parses
	// it and fires the event on the triggering element. We listen
	// on the document in the widget.
	trigger, err := json.Marshal(map[string]imageUploadedEvent{
		"image-uploaded": {
			DisplayKey:  displayKey,
			OriginalKey: originalKey,
		},
	})
	if err != nil {
		// Highly unlikely (the struct is a plain string-string
		// map), but fall back to the toast without the trigger
		// rather than failing the upload.
		c.Logger().Warnf("failed to marshal image-uploaded trigger: %v", err)
		return render(c, views.Toast("success", "Image uploaded", "Ready to save."))
	}
	c.Response().Header().Set("HX-Trigger", string(trigger))
	return render(c, views.Toast("success", "Image uploaded", "Ready to save."))
}

// renderToast is a small wrapper that renders the shared views.Toast
// component. Centralised so every failure path reads the same and
// the comment explaining the flow lives in one place.
func (h *Handler) renderToast(c echo.Context, category, title, description string) error {
	return render(c, views.Toast(category, title, description))
}

// renderImagingError translates an imaging error into the
// user-facing toast. Keeps the imaging error vocabulary in one
// place so adding a new imaging error type means changing one
// switch.
func (h *Handler) renderImagingError(c echo.Context, err error) error {
	switch {
	case err == imaging.ErrUnsupportedFormat, strings.Contains(err.Error(), imaging.ErrUnsupportedFormat.Error()):
		return h.renderToast(c, "error", "Upload failed", "File must be a JPEG or PNG image")
	case err == imaging.ErrDecodeFailed, strings.Contains(err.Error(), imaging.ErrDecodeFailed.Error()):
		return h.renderToast(c, "error", "Upload failed", "Image could not be decoded (file may be corrupt)")
	case err == imaging.ErrInvalidOptions, strings.Contains(err.Error(), imaging.ErrInvalidOptions.Error()):
		return h.renderToast(c, "error", "Upload failed", "Image processor misconfigured")
	default:
		return h.renderToast(c, "error", "Upload failed", fmt.Sprintf("Image processing failed: %v", err))
	}
}
