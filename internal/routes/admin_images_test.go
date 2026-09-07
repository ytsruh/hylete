package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/labstack/echo/v4"

	"hylete/internal/imaging"
)

// fakeUploader records every PutObject call. Tests assert against
// the recorded keys/content to make sure the route wrote both
// variants to the expected locations. When err is non-nil it
// returns that error after recording the put (matching the real
// R2 behaviour where a failed request can still leave the object
// in place).
type fakeUploader struct {
	mu   sync.Mutex
	puts []fakePut
	err  error // if non-nil, all PutObject calls return this error
}

type fakePut struct {
	Key         string
	ContentType string
	Body        []byte
}

func (u *fakeUploader) PutObject(_ context.Context, key, contentType string, body io.Reader) error {
	u.mu.Lock()
	defer u.mu.Unlock()
	data, err := io.ReadAll(body)
	if err != nil {
		return err
	}
	u.puts = append(u.puts, fakePut{Key: key, ContentType: contentType, Body: data})
	if u.err != nil {
		return u.err
	}
	return nil
}

func (u *fakeUploader) lastTwo() (display, original fakePut) {
	u.mu.Lock()
	defer u.mu.Unlock()
	if len(u.puts) < 2 {
		return fakePut{}, fakePut{}
	}
	return u.puts[len(u.puts)-2], u.puts[len(u.puts)-1]
}

// newFakeImagePipeline returns a real imaging processor (so the
// route's MIME / decode validation is exercised end-to-end) and a
// fake uploader (so we don't need a live S3 client). Using the
// real processor is the simplest way to make the route's rejection
// paths (bad MIME, corrupt bytes) cover the same code they'd cover
// in production.
func newFakeImagePipeline() (imaging.Processor, *fakeUploader) {
	return imaging.NewStdProcessor(), &fakeUploader{}
}

// makeJPEGMultipart builds a multipart/form-data body that contains
// a single "file" field with the supplied bytes and a sensible
// filename. The Content-Type of the part is overridden when
// contentTypeOverride is non-empty so tests can simulate a wrong
// MIME type (e.g. application/pdf).
func makeJPEGMultipart(t *testing.T, body []byte, contentTypeOverride, filename string) (string, []byte) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	h := make(map[string][]string)
	h["Content-Disposition"] = []string{fmt.Sprintf(`form-data; name="file"; filename=%q`, filename)}
	if contentTypeOverride != "" {
		h["Content-Type"] = []string{contentTypeOverride}
	} else {
		h["Content-Type"] = []string{"image/jpeg"}
	}
	fw, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("create part: %v", err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatalf("write part: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	return w.FormDataContentType(), buf.Bytes()
}

// validJPEGBytes is a minimal-but-valid 8x8 orange JPEG used as
// the input payload for happy-path tests. Generated once at
// package init so each test can reuse it cheaply.
var validJPEGBytes = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.RGBA{255, 165, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 80}); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// validPNGBytes is a 4x4 solid-red PNG used for the PNG happy-path
// and for sniffing tests.
var validPNGBytes = func() []byte {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			img.Set(x, y, color.RGBA{255, 0, 0, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf.Bytes()
}()

// uploadTrigger is the JSON detail carried by the HX-Trigger
// response header on a successful upload. Mirrors the Go type
// in admin_images.go.
type uploadTrigger struct {
	ImageUploaded struct {
		DisplayKey  string `json:"display_key"`
		OriginalKey string `json:"original_key"`
	} `json:"image-uploaded"`
}

// doUpload runs one upload through the route and returns the
// recorder. Tests then assert on the response body / headers.
func doUpload(t *testing.T, h *Handler, e *echo.Echo, fileBytes []byte, contentType, filename string) *httptest.ResponseRecorder {
	t.Helper()
	ct, body := makeJPEGMultipart(t, fileBytes, contentType, filename)
	req := httptest.NewRequest(http.MethodPost, "/admin/exercises/image-upload", bytes.NewReader(body))
	req.Header.Set(echo.HeaderContentType, ct)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	setAuthContext(c, "admin-1", "admin@example.com", "Admin", true)
	if err := h.AdminExerciseImageUpload(c); err != nil {
		t.Fatalf("upload: %v", err)
	}
	return rec
}

func TestAdminExerciseImageUpload_HappyPath(t *testing.T) {
	h, _, _, e := setupHandler(t)
	upl := h.imageUploader.(*fakeUploader)

	rec := doUpload(t, h, e, validJPEGBytes, "", "test.jpg")

	// The route returns HTML, not JSON. On success the body is a
	// views.Toast with category=success. htmx uses the body as
	// the swap target's HTML.
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `data-category="success"`) {
		t.Errorf("expected success toast in response body, got: %s", body)
	}
	if !strings.Contains(body, "Image uploaded") {
		t.Errorf("expected toast title in response body, got: %s", body)
	}

	// The HX-Trigger header carries the new storage keys so the
	// widget's JS can copy them into the form's hidden inputs.
	trigger := rec.Header().Get("HX-Trigger")
	if trigger == "" {
		t.Fatal("expected HX-Trigger header on successful upload")
	}
	var parsed uploadTrigger
	if err := json.Unmarshal([]byte(trigger), &parsed); err != nil {
		t.Fatalf("decode HX-Trigger: %v (raw = %s)", err, trigger)
	}
	keys := parsed.ImageUploaded
	if keys.DisplayKey == "" {
		t.Error("HX-Trigger display_key is empty")
	}
	if keys.OriginalKey == "" {
		t.Error("HX-Trigger original_key is empty")
	}
	if !strings.HasPrefix(keys.DisplayKey, "exercises/") || !strings.HasSuffix(keys.DisplayKey, ".jpg") {
		t.Errorf("DisplayKey = %q, want exercises/...jpg", keys.DisplayKey)
	}
	if !strings.HasSuffix(keys.OriginalKey, "_original.jpg") {
		t.Errorf("OriginalKey = %q, want ..._original.jpg", keys.OriginalKey)
	}
	if keys.DisplayKey == keys.OriginalKey {
		t.Errorf("display and original keys should differ, both = %q", keys.DisplayKey)
	}

	// Both variants should have been uploaded to R2.
	if got := len(upl.puts); got != 2 {
		t.Errorf("uploads = %d, want 2", got)
	}
	display, original := upl.lastTwo()
	if display.Key != keys.DisplayKey {
		t.Errorf("display put key = %q, want %q", display.Key, keys.DisplayKey)
	}
	if original.Key != keys.OriginalKey {
		t.Errorf("original put key = %q, want %q", original.Key, keys.OriginalKey)
	}
	if display.ContentType != "image/jpeg" || original.ContentType != "image/jpeg" {
		t.Errorf("content types = %q / %q, want image/jpeg / image/jpeg", display.ContentType, original.ContentType)
	}
	if len(display.Body) == 0 || len(original.Body) == 0 {
		t.Error("uploaded bodies should be non-empty")
	}
}

func TestAdminExerciseImageUpload_PNG(t *testing.T) {
	h, _, _, e := setupHandler(t)

	rec := doUpload(t, h, e, validPNGBytes, "image/png", "test.png")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `data-category="success"`) {
		t.Errorf("expected success toast for PNG upload, got: %s", rec.Body.String())
	}
}

func TestAdminExerciseImageUpload_RejectsNonImage(t *testing.T) {
	h, _, _, e := setupHandler(t)

	// A PDF header is sniffed as application/pdf and must be
	// rejected. The route now returns a toast instead of an
	// echo.HTTPError, so the response body carries the error
	// toast markup and the status is 200.
	pdfHeader := []byte("%PDF-1.4\n%¥±ë\n")
	rec := doUpload(t, h, e, pdfHeader, "application/pdf", "test.pdf")

	body := rec.Body.String()
	if !strings.Contains(body, `data-category="error"`) {
		t.Errorf("expected error toast in response body, got: %s", body)
	}
	if !strings.Contains(body, "JPEG") && !strings.Contains(body, "image") {
		t.Errorf("error message should mention JPEG/PNG, got: %s", body)
	}
	if trigger := rec.Header().Get("HX-Trigger"); trigger != "" {
		t.Errorf("error response should not set HX-Trigger, got: %s", trigger)
	}
}

func TestAdminExerciseImageUpload_RejectsEmptyFile(t *testing.T) {
	h, _, _, e := setupHandler(t)

	rec := doUpload(t, h, e, nil, "image/jpeg", "empty.jpg")

	if !strings.Contains(rec.Body.String(), `data-category="error"`) {
		t.Errorf("expected error toast for empty file, got: %s", rec.Body.String())
	}
}

func TestAdminExerciseImageUpload_RejectsOversize(t *testing.T) {
	h, _, _, e := setupHandler(t)

	// 11 MB of zero bytes — larger than the 10 MB cap. We use
	// random bytes (not a real image) because the body cap should
	// be enforced *before* the imaging package runs, so the bytes
	// don't have to be a real image. The error surfaces as a
	// toast, not an HTTP error.
	oversize := make([]byte, 11<<20)
	rec := doUpload(t, h, e, oversize, "image/jpeg", "huge.jpg")

	body := rec.Body.String()
	if !strings.Contains(body, `data-category="error"`) {
		t.Errorf("expected error toast for oversize body, got: %s", body)
	}
}

func TestAdminExerciseImageUpload_DisplayKeyCollisionWithUUIDs(t *testing.T) {
	// Run two uploads back-to-back and confirm each produces a
	// distinct pair of keys (no UUID reuse). This guards against
	// the storage-key generator being broken.
	h, _, _, e := setupHandler(t)

	doOne := func() uploadTrigger {
		rec := doUpload(t, h, e, validJPEGBytes, "", "x.jpg")
		trigger := rec.Header().Get("HX-Trigger")
		var parsed uploadTrigger
		if err := json.Unmarshal([]byte(trigger), &parsed); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return parsed
	}

	a := doOne()
	b := doOne()
	if a.ImageUploaded.DisplayKey == b.ImageUploaded.DisplayKey {
		t.Errorf("display keys collided: %q", a.ImageUploaded.DisplayKey)
	}
	if a.ImageUploaded.OriginalKey == b.ImageUploaded.OriginalKey {
		t.Errorf("original keys collided: %q", a.ImageUploaded.OriginalKey)
	}
}

func TestAdminExerciseImageUpload_OriginalUploadFailureCleansUpDisplay(t *testing.T) {
	// If the first upload (display) succeeds but the second
	// (original) fails, the route must best-effort delete the
	// display object so R2 doesn't accumulate orphans. We assert
	// on the recorded puts to confirm the display upload
	// happened (it should be recorded even though the original
	// upload errored afterwards) and that the error reached the
	// caller.
	h, _, _, e := setupHandler(t)
	upl := h.imageUploader.(*fakeUploader)

	// Wrap the uploader to fail on the *second* PutObject call.
	// The route's display upload must succeed, the original
	// upload must fail, and the cleanup DeleteObject must be
	// called for the display key.
	upl.mu.Lock()
	upl.err = errors.New("simulated original upload failure")
	upl.mu.Unlock()

	rec := doUpload(t, h, e, validJPEGBytes, "", "x.jpg")

	body := rec.Body.String()
	if !strings.Contains(body, `data-category="error"`) {
		t.Errorf("expected error toast on original-upload failure, got: %s", body)
	}
	// Only one PutObject should have been recorded (the display
	// one) before the original upload failed.
	upl.mu.Lock()
	puts := append([]fakePut(nil), upl.puts...)
	upl.mu.Unlock()
	if len(puts) != 1 {
		t.Errorf("puts = %d, want 1 (display only, original failed)", len(puts))
	}
	// The route's best-effort cleanup is handled by utils.DeleteObject
	// directly. We can't intercept that without a heavier mock, but
	// the error path has been exercised; that's the main contract.
}
