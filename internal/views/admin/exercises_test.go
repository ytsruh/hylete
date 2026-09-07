package admin

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/a-h/templ"

	"hylete/internal/models"
	"hylete/internal/utils"
)

// renderToString renders a templ component to a string for assertions.
func renderToString(t *testing.T, component templ.Component) string {
	t.Helper()
	var buf bytes.Buffer
	if err := component.Render(context.Background(), &buf); err != nil {
		t.Fatalf("failed to render component: %v", err)
	}
	return buf.String()
}

func TestAdminExerciseForm_New_ContainsUploadWidget(t *testing.T) {
	html := renderToString(t, AdminExerciseForm(AdminExerciseFormData{IsEdit: false}, "Admin", true, true))

	// The ImageUpload widget must render with the expected data
	// attribute so the JS can find it.
	if !strings.Contains(html, `data-image-upload="exercise"`) {
		t.Error("expected data-image-upload attribute on the upload widget root")
	}

	// The file input is the upload trigger now — htmx posts the
	// multipart file to the upload endpoint on change. The
	// endpoint was previously a data attribute on the widget
	// root; it's now on the input itself.
	if !strings.Contains(html, `hx-post="/admin/exercises/image-upload"`) {
		t.Error("expected hx-post pointing at the admin image upload route on the file input")
	}
	if !strings.Contains(html, `hx-encoding="multipart/form-data"`) {
		t.Error("expected hx-encoding=\"multipart/form-data\" on the file input")
	}
	if !strings.Contains(html, `hx-target="#toaster"`) {
		t.Error("expected hx-target=\"#toaster\" on the file input")
	}
	if !strings.Contains(html, `hx-swap="beforeend"`) {
		t.Error("expected hx-swap=\"beforeend\" on the file input")
	}
	if !strings.Contains(html, `hx-trigger="change"`) {
		t.Error("expected hx-trigger=\"change\" on the file input")
	}

	// The hidden inputs the form submits back to the server must
	// be present with the right names so the route can pick them
	// up via c.FormValue.
	if !strings.Contains(html, `name="img_key"`) {
		t.Error("expected hidden img_key input on the new form")
	}
	if !strings.Contains(html, `name="img_key_original"`) {
		t.Error("expected hidden img_key_original input on the new form")
	}

	// The legacy free-text "img_url" input must be gone — it was
	// the manual key-paste field that the new widget replaces.
	if strings.Contains(html, `id="img_url"`) {
		t.Error("legacy img_url text input must not appear on the new form")
	}

	// The "Remove current image" button + hidden field should only
	// appear on the edit form when an image exists, never on the
	// new form (the widget gates both on CurrentURL != "").
	if strings.Contains(html, `id="image-remove-exercise"`) {
		t.Error("image-remove button must not appear on the new form")
	}
	if strings.Contains(html, `name="clear_image"`) {
		t.Error("clear_image hidden field must not appear on the new form")
	}
}

func TestAdminExerciseForm_Edit_NoImage_ContainsUploadWidget(t *testing.T) {
	ex := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	html := renderToString(t, AdminExerciseForm(AdminExerciseFormData{Exercise: ex, IsEdit: true}, "Admin", true, true))

	// Upload widget must still be present on the edit form so
	// the admin can add an image to an exercise that has none.
	if !strings.Contains(html, `data-image-upload="exercise"`) {
		t.Error("expected ImageUpload widget on the edit form (no image)")
	}
	// The hidden inputs should render with empty values.
	if !strings.Contains(html, `name="img_key" value=""`) {
		t.Error("expected empty img_key on edit form with no current image")
	}
	// No "Remove current image" control when there's no image.
	if strings.Contains(html, `id="image-remove-exercise"`) {
		t.Error("image-remove button should not appear when there is no current image")
	}
}

func TestAdminExerciseForm_Edit_WithImage_ContainsRemoveButton(t *testing.T) {
	ex := &models.Exercise{
		ID:             "ex-1",
		Name:           "Squat",
		Type:           models.ExerciseTypeStrength,
		ImgURL:         "exercises/abc.jpg",
		ImgURLOriginal: "exercises/abc_original.jpg",
	}
	html := renderToString(t, AdminExerciseForm(AdminExerciseFormData{Exercise: ex, IsEdit: true}, "Admin", true, true))

	// Both keys should be pre-populated so the form submits them
	// back unchanged if the user doesn't touch the upload widget.
	if !strings.Contains(html, `name="img_key" value="exercises/abc.jpg"`) {
		t.Error("expected img_key pre-populated with the current key")
	}
	if !strings.Contains(html, `name="img_key_original" value="exercises/abc_original.jpg"`) {
		t.Error("expected img_key_original pre-populated with the current key")
	}

	// The form must thread the RemoveFieldName / RemoveLabel
	// props through to the widget. The widget then renders the
	// button + hidden field; the component-level test
	// (image_upload_test.go) covers that rendering. Here we just
	// verify the form passes the right values.
	if !strings.Contains(html, `data-remove-field="clear_image"`) {
		t.Error("expected data-remove-field attribute on the upload widget root")
	}
	if !strings.Contains(html, `data-remove-label="Remove current image"`) {
		t.Error("expected data-remove-label attribute on the upload widget root")
	}

	// The legacy checkbox must be gone — only the button + hidden
	// field should carry the "remove" intent.
	if strings.Contains(html, `id="clear_image"`) {
		t.Error("clear_image id must not be on a checkbox any more — it now belongs to a hidden field")
	}
}

func TestAdminExerciseForm_Edit_UploadEndpointHardCoded(t *testing.T) {
	// Regression guard: the ImageUpload widget hardcodes the admin
	// upload endpoint on the file input. If a future refactor
	// changes it to derive from a prop, this test ensures the
	// admin form still POSTs to the admin route (not the generic
	// /api path that the default UploadEndpoint value would
	// produce).
	ex := &models.Exercise{ID: "ex-1", Name: "Squat", Type: models.ExerciseTypeStrength}
	html := renderToString(t, AdminExerciseForm(AdminExerciseFormData{Exercise: ex, IsEdit: true}, "Admin", true, true))

	if !strings.Contains(html, `hx-post="/admin/exercises/image-upload"`) {
		t.Errorf("expected /admin/exercises/image-upload endpoint on file input, got html: %s", html)
	}
}

func TestAdminExerciseForm_PreviewAspectMatchesCard(t *testing.T) {
	// The exercise image is displayed on the history page through
	// components.BannerImage — a 3:1 frame (aspect-[3/1]) — and
	// the server crops uploads to 3:1 too (DefaultExerciseImageConfig,
	// 1200x400). The admin form's preview must match that 3:1
	// aspect so the admin sees what users will see — not the raw
	// file aspect.
	//
	// The preview div only renders when the widget has a CurrentURL
	// (i.e. when storage is configured AND the exercise has an
	// image). For the new form and the edit form without an image
	// there's no preview to assert on. We load the storage config
	// in the relevant subtests so the widget sees a non-empty URL.
	for _, v := range []string{
		"STORAGE_ENDPOINT", "STORAGE_ACCESS_KEY", "STORAGE_SECRET_KEY",
		"STORAGE_BUCKET", "STORAGE_PUBLIC_URL",
	} {
		t.Setenv(v, "test")
	}
	if _, err := utils.LoadStorageConfig(); err != nil {
		t.Fatalf("LoadStorageConfig: %v", err)
	}

	for _, tc := range []struct {
		name         string
		form         AdminExerciseFormData
		wantPreview  bool // true → preview div should be in the HTML
	}{
		{
			name: "new form",
			form: AdminExerciseFormData{IsEdit: false},
			// No exercise, so no preview.
			wantPreview: false,
		},
		{
			name: "edit form with image",
			form: AdminExerciseFormData{
				IsEdit:   true,
				Exercise: &models.Exercise{ID: "ex-1", Name: "Squat", ImgURL: "exercises/abc.jpg"},
			},
			wantPreview: true,
		},
		{
			name: "edit form without image",
			form: AdminExerciseFormData{
				IsEdit:   true,
				Exercise: &models.Exercise{ID: "ex-1", Name: "Squat"},
			},
			// Image URL is empty, so no preview.
			wantPreview: false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			html := renderToString(t, AdminExerciseForm(tc.form, "Admin", true, true))
			hasAspect := strings.Contains(html, "aspect-ratio: 3 / 1")
			if hasAspect != tc.wantPreview {
				t.Errorf("aspect-ratio: 3 / 1 presence = %v, want %v", hasAspect, tc.wantPreview)
			}
			if strings.Contains(html, "aspect-ratio: 16 / 9") {
				t.Errorf("preview still using the old 16:9 aspect, got html:\n%s", html)
			}
			if strings.Contains(html, "padding-bottom") {
				t.Errorf("preview still using the old padding-bottom aspect hack, got html:\n%s", html)
			}
		})
	}
}
