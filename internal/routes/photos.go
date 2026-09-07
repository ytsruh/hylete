package routes

import (
	"net/http"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
	"github.com/labstack/echo/v4"

	"hylete/internal/utils"
)

// photoUploadRequest is the JSON body sent by the client (iOS app or
// browser) to request a presigned URL for a direct upload to R2.
type photoUploadRequest struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
}

// photoUploadResponse is returned to the client with the URL to PUT to
// and the storage key the server expects to be persisted with the
// resource.
type photoUploadResponse struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// PhotoUploadURL returns a presigned PUT URL for a client to upload a
// weight-progress photo directly to R2. The key is generated server-side
// as `weight/{userID}/{uuid}{ext}` and must be sent back when creating or
// updating the weight entry.
//
// Auth: requires a logged-in user. Reachable via POST /api/v1/weight/
// photo-upload (JWT — used by the iOS client); there is no standalone web
// route any more.
func (h *Handler) PhotoUploadURL(c echo.Context) error {
	claims := GetClaims(c)

	var req photoUploadRequest
	if err := c.Bind(&req); err != nil {
		return echo.NewHTTPError(http.StatusBadRequest, "Invalid request body")
	}
	if req.Filename == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "filename is required")
	}
	if req.ContentType == "" {
		req.ContentType = "application/octet-stream"
	}
	if !strings.HasPrefix(req.ContentType, "image/") {
		return echo.NewHTTPError(http.StatusBadRequest, "content_type must be an image type")
	}

	ext := strings.ToLower(filepath.Ext(req.Filename))
	if ext == "" {
		// Fall back to content-type extension
		switch req.ContentType {
		case "image/jpeg":
			ext = ".jpg"
		case "image/png":
			ext = ".png"
		case "image/webp":
			ext = ".webp"
		case "image/gif":
			ext = ".gif"
		default:
			ext = ".bin"
		}
	}

	key := "weight/" + claims.UserID + "/" + uuid.New().String() + ext

	url, err := utils.CreatePresignedPutURL(key, req.ContentType)
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to generate upload URL")
	}

	return c.JSON(http.StatusOK, photoUploadResponse{URL: url, Key: key})
}
