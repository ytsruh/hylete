// Package routes: api_dto.go defines the JSON shapes the /api/v1
// namespace sends and receives. They are intentionally separate
// from the domain models so the on-the-wire contract is decoupled
// from the database schema, and so we never accidentally leak a
// field the client does not need (e.g. password hashes). The
// From… helpers below are the only way a model becomes a DTO.
package routes

import (
	"time"

	"hylete/internal/models"
	"hylete/internal/utils"
)

// APIError is the body returned for every non-2xx response from
// the /api/v1 namespace. The auth middleware and the per-handler
// error paths both produce this shape so the iOS client only has
// to parse a single error format.
type APIError struct {
	Error string `json:"error"`
}

// LoginRequest is the JSON body for POST /api/v1/auth/login.
type LoginRequest struct {
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required"`
}

// RegisterRequest is the JSON body for POST /api/v1/auth/register.
type RegisterRequest struct {
	Name     string `json:"name"     validate:"required,min=1,max=100"`
	Email    string `json:"email"    validate:"required,email"`
	Password string `json:"password" validate:"required,min=6"`
}

// PasswordResetRequest is the JSON body for
// POST /api/v1/auth/password-reset/request.
type PasswordResetRequest struct {
	Email string `json:"email" validate:"required,email"`
}

// PasswordResetResponse is returned for every accepted password-reset
// request. The message deliberately does not reveal whether the email
// belongs to an account.
type PasswordResetResponse struct {
	Message string `json:"message"`
}

// AuthResponse is the JSON body returned by login and register.
// Token is a signed JWT the client stores in the Keychain; User
// is the safe public-facing view of the user record.
type AuthResponse struct {
	Token string  `json:"token"`
	User  UserDTO `json:"user"`
}

// UserDTO is the safe public-facing view of a user. It deliberately
// omits PasswordHash and the internal reminder scheduling fields
// (which are only useful to the server's tick job).
type UserDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Email        string   `json:"email"`
	IsAdmin      bool     `json:"is_admin"`
	WeightUnit   string   `json:"weight_unit"`
	DistanceUnit string   `json:"distance_unit"`
	TargetWeight *float64 `json:"target_weight,omitempty"`
}

// UserFromModel converts a models.User into the safe UserDTO.
// nil is mapped to a zero-value DTO so callers don't have to
// branch before encoding the response.
func UserFromModel(u *models.User) UserDTO {
	if u == nil {
		return UserDTO{WeightUnit: "kg", DistanceUnit: models.DistanceUnitKm}
	}
	return UserDTO{
		ID:           u.ID,
		Name:         u.Name,
		Email:        u.Email,
		IsAdmin:      u.IsAdmin,
		WeightUnit:   u.WeightUnitDisplay(),
		DistanceUnit: u.DistanceUnitDisplay(),
		TargetWeight: u.TargetWeight,
	}
}

// UpdateMeRequest is the JSON body for PUT /api/v1/me. Mirrors
// the user-editable subset of the HTML profile form (name,
// target weight, weight unit, distance unit) so the iOS app can update the
// same fields the web app exposes. Reminder preferences are
// deliberately omitted: they live on the web app only for now, and the
// iOS client surfaces no UI for them. Add fields here (and to UserDTO)
// when the iOS app grows that surface.
//
// TargetWeight is a pointer so an omitted JSON field (or an
// explicit null) clears the user's goal, matching the form's
// empty-input semantics.
type UpdateMeRequest struct {
	Name         string   `json:"name"          validate:"required,min=2,max=100"`
	TargetWeight *float64 `json:"target_weight" validate:"omitempty,gte=0,lte=1000"`
	WeightUnit   string   `json:"weight_unit"   validate:"omitempty,oneof=kg lbs"`
	DistanceUnit string   `json:"distance_unit" validate:"omitempty,oneof=km mi"`
}

// ExerciseDTO is the JSON shape for an exercise in any list or
// lookup response.
//
// `ImgURL` is the raw storage key (e.g. "exercises/<id>.webp"),
// suitable for fetching the object via the storage client. Clients
// that want to render the image directly should use `ImageURL`,
// which is the fully-qualified public URL resolved via
// `utils.PublicURLFor("exercises/<id>.webp")` — i.e. the same
// value the web app's `internal/views/exercise/history.templ`
// feeds into its `<img src=…>`. Keeping both fields lets older
// clients keep reading `img_url` while newer clients (the iOS
// app) get a ready-to-render URL with no extra config.
//
// `ImageURLOriginal` mirrors the higher-resolution
// "img_url_original" variant (see DefaultExerciseImageConfig).
// It is omitted from the JSON entirely for exercises without an
// original, so older clients never see an empty-URL key. The iOS
// full-screen image viewer prefers it for sharper pinch-zoom,
// falling back to `ImageURL` on older server builds.
type ExerciseDTO struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Description      string `json:"description"`
	VideoURL         string `json:"video_url"`
	ImgURL           string `json:"img_url"`
	ImageURL         string `json:"image_url"`
	ImageURLOriginal string `json:"image_url_original,omitempty"`
	Type             string `json:"type"`
}

// ExerciseFromModel converts a models.Exercise into its DTO.
func ExerciseFromModel(e models.Exercise) ExerciseDTO {
	dto := ExerciseDTO{
		ID:          e.ID,
		Name:        e.Name,
		Description: e.Description,
		VideoURL:    e.VideoURL,
		ImgURL:      e.ImgURL,
		ImageURL:    utils.PublicURLFor(e.ImgURL),
		Type:        string(e.Type),
	}
	// Only resolve the original's public URL when an original
	// exists — PublicURLFor on an empty key would build a
	// meaningless bare-bucket URL.
	if e.ImgURLOriginal != "" {
		dto.ImageURLOriginal = utils.PublicURLFor(e.ImgURLOriginal)
	}
	return dto
}

// ExercisesFromModels converts a slice of exercises into the
// DTO slice. The empty case returns an empty (non-nil) slice
// so the JSON encoder writes `[]` rather than `null` — easier
// for the Swift `Codable` decoder to consume.
func ExercisesFromModels(es []models.Exercise) []ExerciseDTO {
	out := make([]ExerciseDTO, 0, len(es))
	for _, e := range es {
		out = append(out, ExerciseFromModel(e))
	}
	return out
}

// ExerciseEntryDTO is the JSON shape for a single set. The metric pair that
// carries meaning depends on ExerciseType: strength entries use reps/weight/
// rest_time, cardio entries use duration_seconds/distance_meters (+ optional
// avg_heart_rate/calories_burned). The server always sends every field with 0
// for the non-applicable pair, so clients can decode one struct and branch on
// exercise_type instead of probing for nulls. Distance is metres; pace is not
// sent — derive it as duration_seconds / (distance_meters/1000).
type ExerciseEntryDTO struct {
	ID              string    `json:"id"`
	ExerciseID      string    `json:"exercise_id"`
	ExerciseName    string    `json:"exercise_name"`
	ExerciseType    string    `json:"exercise_type"`
	Reps            int       `json:"reps"`
	Weight          float64   `json:"weight"`
	Notes           string    `json:"notes"`
	RestTime        int       `json:"rest_time"`
	DurationSeconds int       `json:"duration_seconds"`
	DistanceMeters  float64   `json:"distance_meters"`
	AvgHeartRate    int       `json:"avg_heart_rate"`
	CaloriesBurned  float64   `json:"calories_burned"`
	CreatedAt       time.Time `json:"created_at"`
}

// ExerciseEntryFromModel converts a models.ExerciseEntry into its DTO.
func ExerciseEntryFromModel(e models.ExerciseEntry) ExerciseEntryDTO {
	return ExerciseEntryDTO{
		ID:              e.ID,
		ExerciseID:      e.ExerciseID,
		ExerciseName:    e.ExerciseName,
		ExerciseType:    string(e.ExerciseType),
		Reps:            e.Reps,
		Weight:          e.Weight,
		Notes:           e.Notes,
		RestTime:        e.RestTime,
		DurationSeconds: e.DurationSeconds,
		DistanceMeters:  e.DistanceMeters,
		AvgHeartRate:    e.AvgHeartRate,
		CaloriesBurned:  e.CaloriesBurned,
		CreatedAt:       e.CreatedAt,
	}
}

// ExerciseEntriesFromModels converts a slice of exercise entries
// into the DTO slice. The empty case returns an empty (non-nil)
// slice so the JSON encoder writes `[]` rather than `null`.
func ExerciseEntriesFromModels(es []models.ExerciseEntry) []ExerciseEntryDTO {
	out := make([]ExerciseEntryDTO, 0, len(es))
	for _, e := range es {
		out = append(out, ExerciseEntryFromModel(e))
	}
	return out
}

// CreateSetInput is one set within a CreateExerciseEntriesRequest.
// The numeric limits match the HTML form heritage (reps 0–1000, weight
// 0–5000, rest 0–3600) extended with cardio fields (duration 0–86400s,
// distance 0–500km, HR 0–300bpm, calories 0–10000kcal). Minimums are 0
// here because which fields are mandatory depends on the exercise's type
// — that check lives in the controller's ValidateExerciseSetInput so the
// JSON and any future surface share one rule set.
type CreateSetInput struct {
	Reps            int     `json:"reps"             validate:"gte=0,lte=1000"`
	Weight          float64 `json:"weight"           validate:"gte=0,lte=5000"`
	RestTime        int     `json:"rest_time"        validate:"gte=0,lte=3600"`
	DurationSeconds int     `json:"duration_seconds" validate:"gte=0,lte=86400"`
	DistanceMeters  float64 `json:"distance_meters"  validate:"gte=0,lte=500000"`
	AvgHeartRate    int     `json:"avg_heart_rate"   validate:"gte=0,lte=300"`
	CaloriesBurned  float64 `json:"calories_burned"  validate:"gte=0,lte=10000"`
}

// CreateExerciseEntriesRequest is the body for
// POST /api/v1/exercise-entries. Sets is the multi-set payload
// (one entry created per set, all sharing the same exercise,
// notes and timestamp — same semantics as the web form).
// CreatedAt is optional and defaults to time.Now() on the
// server when omitted, so a client that just wants "log it
// now" can send an empty body field. For cardio exercises each
// set must carry duration_seconds and distance_meters; for
// strength exercises reps must be at least 1 (enforced after
// binding, keyed off the linked exercise's type).
type CreateExerciseEntriesRequest struct {
	ExerciseID string           `json:"exercise_id" validate:"required"`
	Notes      string           `json:"notes"       validate:"max=500"`
	CreatedAt  *time.Time       `json:"created_at,omitempty"`
	Sets       []CreateSetInput `json:"sets"        validate:"required,min=1,dive"`
}

// UpdateExerciseEntryRequest is the body for
// PUT /api/v1/exercise-entries/:id. The single-set shape
// matches the create request; the same type-driven validation
// applies using the entry's current exercise.
type UpdateExerciseEntryRequest struct {
	ExerciseID      string     `json:"exercise_id"      validate:"required"`
	Notes           string     `json:"notes"            validate:"max=500"`
	Reps            int        `json:"reps"             validate:"gte=0,lte=1000"`
	Weight          float64    `json:"weight"           validate:"gte=0,lte=5000"`
	RestTime        int        `json:"rest_time"        validate:"gte=0,lte=3600"`
	DurationSeconds int        `json:"duration_seconds" validate:"gte=0,lte=86400"`
	DistanceMeters  float64    `json:"distance_meters"  validate:"gte=0,lte=500000"`
	AvgHeartRate    int        `json:"avg_heart_rate"   validate:"gte=0,lte=300"`
	CaloriesBurned  float64    `json:"calories_burned"  validate:"gte=0,lte=10000"`
	CreatedAt       *time.Time `json:"created_at,omitempty"`
}

// HistoryStatsDTO is the lifetime-stats header shown above
// the history list. Strength clients read max_weight; cardio
// clients read best_pace_sec_per_km (0 = none) and
// longest_distance_meters. Both sets are always populated so
// the client only needs exercise_type to pick.
type HistoryStatsDTO struct {
	MaxWeight             float64           `json:"max_weight"`
	BestPaceSecPerKm      float64           `json:"best_pace_sec_per_km"`
	LongestDistanceMeters float64           `json:"longest_distance_meters"`
	LastSet               *ExerciseEntryDTO `json:"last_set,omitempty"`
}

// HistoryStatsFromModel converts a models.HistoryStats into
// the DTO. The zero-value LastSet (when the user has no
// exercise entries for the exercise) is mapped to a nil
// pointer so the field is omitted from the JSON.
func HistoryStatsFromModel(s models.HistoryStats) HistoryStatsDTO {
	out := HistoryStatsDTO{
		MaxWeight:             s.MaxWeight,
		BestPaceSecPerKm:      s.BestPaceSecPerKm,
		LongestDistanceMeters: s.LongestDistanceMeters,
	}
	if s.LastSet.ID != "" {
		dto := ExerciseEntryFromModel(s.LastSet)
		out.LastSet = &dto
	}
	return out
}

// HistoryPageDTO is the body of a paginated history
// response. The HasPrev/HasNext flags let the iOS pager
// render the same "Previous / Next" buttons as the web
// history view without a separate HEAD request.
// ExerciseType tells the client which metric pair the stats
// and entries carry ("strength" | "cardio" | "other").
type HistoryPageDTO struct {
	ExerciseType string             `json:"exercise_type"`
	Entries      []ExerciseEntryDTO `json:"entries"`
	Stats        HistoryStatsDTO    `json:"stats"`
	Page         int                `json:"page"`
	HasPrev      bool               `json:"has_prev"`
	HasNext      bool               `json:"has_next"`
}

// HistoryPageFromModel converts a models.ExerciseHistoryPage
// into the DTO. The empty-cases return empty (non-nil) slices
// so the JSON encoder writes `[]` rather than `null`. The page's
// exercise type is read from the first entry (every entry in a
// page belongs to the same exercise); an empty page falls back
// to "other".
func HistoryPageFromModel(p *models.ExerciseHistoryPage) HistoryPageDTO {
	exerciseType := string(models.ExerciseTypeOther)
	for _, e := range p.ExerciseEntries {
		if e.ExerciseType != "" {
			exerciseType = string(e.ExerciseType)
			break
		}
	}
	return HistoryPageDTO{
		ExerciseType: exerciseType,
		Entries:      ExerciseEntriesFromModels(p.ExerciseEntries),
		Stats:        HistoryStatsFromModel(p.Stats),
		Page:         p.Page,
		HasPrev:      p.HasPrev,
		HasNext:      p.HasNext,
	}
}

// --- Goals ---

// GoalDTO is the JSON shape for a single goal in the
// /api/v1/goals namespace. Mirrors models.Goal but exposes
// only the fields the iOS client needs. StartDate, TargetDate,
// EndDate and CompletedAt are pointers so a missing date is
// rendered as JSON null (not zero-time, which would imply
// "1970-01-01" and break the UI's conditional rendering).
type GoalDTO struct {
	ID          string     `json:"id"`
	Title       string     `json:"title"`
	Description string     `json:"description"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	TargetDate  *time.Time `json:"target_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

// CreateGoalRequest is the body for POST /api/v1/goals. The
// validation limits match the HTML form (title 1-200,
// description 0-2000) so the JSON and HTML surfaces reject
// the same inputs.
type CreateGoalRequest struct {
	Title       string     `json:"title"       validate:"required,min=1,max=200"`
	Description string     `json:"description" validate:"max=2000"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	TargetDate  *time.Time `json:"target_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

// UpdateGoalRequest is the body for PUT /api/v1/goals/:id.
// CompletedAt is intentionally NOT editable here — clients
// must use POST /goals/:id/complete or /reopen so the server
// owns the completion timestamp (matches the HTML surface).
type UpdateGoalRequest struct {
	Title       string     `json:"title"       validate:"required,min=1,max=200"`
	Description string     `json:"description" validate:"max=2000"`
	StartDate   *time.Time `json:"start_date,omitempty"`
	TargetDate  *time.Time `json:"target_date,omitempty"`
	EndDate     *time.Time `json:"end_date,omitempty"`
}

// GoalFromModel converts a models.Goal into its DTO. Pass by
// value so callers don't have to dereference.
func GoalFromModel(g models.Goal) GoalDTO {
	return GoalDTO{
		ID:          g.ID,
		Title:       g.Title,
		Description: g.Description,
		StartDate:   g.StartDate,
		TargetDate:  g.TargetDate,
		EndDate:     g.EndDate,
		CompletedAt: g.CompletedAt,
		CreatedAt:   g.CreatedAt,
		UpdatedAt:   g.UpdatedAt,
	}
}

// GoalsFromModels converts a slice of goals into the DTO
// slice. Returns a non-nil empty slice so the JSON encoder
// writes `[]` rather than `null` when the user has no goals.
func GoalsFromModels(gs []models.Goal) []GoalDTO {
	out := make([]GoalDTO, 0, len(gs))
	for _, g := range gs {
		out = append(out, GoalFromModel(g))
	}
	return out
}

// --- Weight ---

// WeightEntryDTO is the JSON shape for a single body-weight
// entry on the /api/v1/weight namespace. Mirrors the
// models.WeightEntry domain type but resolves the storage key
// into a fully-qualified `PhotoURL` so the iOS client can hand
// it straight to AsyncImage without any extra config.
//
// `PhotoKey` is included alongside `PhotoURL` so the iOS
// editor can re-submit it on update (the server's PUT handler
// expects to be told the existing key explicitly — see
// `UpdateWeight` in `internal/routes/weight.go:127`). When
// `HasPhoto` is false the `PhotoKey` and `PhotoURL` fields are
// omitted from the JSON so a client can rely on `HasPhoto`
// as the single source of truth (rather than checking both
// fields for emptiness).
type WeightEntryDTO struct {
	ID        string    `json:"id"`
	Weight    float64   `json:"weight"`
	Notes     string    `json:"notes"`
	PhotoKey  string    `json:"photo_key,omitempty"`
	PhotoURL  string    `json:"photo_url,omitempty"`
	HasPhoto  bool      `json:"has_photo"`
	CreatedAt time.Time `json:"created_at"`
}

// WeightEntryFromModel converts a models.WeightEntry into its
// JSON DTO. The photo URL is resolved via utils.PublicURLFor
// so the iOS client never sees raw storage keys in the
// photo_url field.
func WeightEntryFromModel(e models.WeightEntry) WeightEntryDTO {
	dto := WeightEntryDTO{
		ID:        e.ID,
		Weight:    e.Weight,
		Notes:     e.Notes,
		PhotoKey:  e.PhotoKey,
		HasPhoto:  e.HasPhoto(),
		CreatedAt: e.CreatedAt,
	}
	if e.HasPhoto() {
		dto.PhotoURL = utils.PublicURLFor(e.PhotoKey)
	}
	return dto
}

// WeightEntriesFromModels converts a slice of weight entries
// into the DTO slice. Returns an empty (non-nil) slice when
// the input is empty so the JSON encoder writes `[]` rather
// than `null` — easier for the Swift Codable decoder.
func WeightEntriesFromModels(es []models.WeightEntry) []WeightEntryDTO {
	out := make([]WeightEntryDTO, 0, len(es))
	for _, e := range es {
		out = append(out, WeightEntryFromModel(e))
	}
	return out
}

// WeightEntriesResponse wraps the weight entry slice in a
// named envelope so the server can add fields (pagination
// metadata, summary stats, etc.) without breaking the iOS
// contract. The iOS view treats the response as opaque and
// only reads `entries`.
type WeightEntriesResponse struct {
	Entries []WeightEntryDTO `json:"entries"`
}

// CreateWeightEntryRequest is the body for POST /api/v1/weight.
// Mirrors the web form's three fields (weight, notes,
// photo_key) and the optional `created_at` that lets the
// client backdate an entry without manually re-issuing the
// server's time.Now() default.
//
// The numeric and length limits match the HTML form
// (`weightFormInput` at `internal/routes/weight.go:19`) so
// the JSON and HTML surfaces reject the same inputs.
type CreateWeightEntryRequest struct {
	Weight    float64    `json:"weight"     validate:"required,gte=0,lte=1000"`
	Notes     string     `json:"notes"      validate:"max=1000"`
	PhotoKey  string     `json:"photo_key,omitempty"`
	CreatedAt *time.Time `json:"created_at,omitempty"`
}

// UpdateWeightEntryRequest is the body for PUT /api/v1/weight/:id.
// Same shape as the create request, plus `remove_photo` so the
// iOS editor can clear the existing photo without first having
// to delete the entry and re-create it. When `remove_photo` is
// true the server ignores `photo_key` and clears the
// associated column. When `photo_key` is non-empty the server
// replaces the existing key with the new one. When both are
// empty / false the existing key is preserved.
type UpdateWeightEntryRequest struct {
	Weight      float64    `json:"weight"       validate:"required,gte=0,lte=1000"`
	Notes       string     `json:"notes"        validate:"max=1000"`
	PhotoKey    string     `json:"photo_key,omitempty"`
	RemovePhoto bool       `json:"remove_photo,omitempty"`
	CreatedAt   *time.Time `json:"created_at,omitempty"`
}

// WeightCompareResponse is the body for GET /api/v1/weight/compare.
// Returned in the same order the entries occurred on the
// timeline (Before/After) so the iOS view can render the slider
// with the historical photo on the left and the more recent one
// on the right without having to sort client-side.
// `DeltaText` is the formatted "+2.5 kg" / "−1.5 kg" / ""
// string the server already produces for the web compare
// modal — see `formatWeightDelta` in `internal/routes/weight.go:241`.
type WeightCompareResponse struct {
	Before    WeightEntryDTO `json:"before"`
	After     WeightEntryDTO `json:"after"`
	DeltaText string         `json:"delta_text"`
}

// WeightPhotoUploadRequest is the JSON body for POST
// /api/v1/weight/photo-upload. Mirrors the existing
// `photoUploadRequest` at `internal/routes/photos.go:16` but
// is exposed as a public type so the iOS DTO and the Go
// validation tags live next to the rest of the weight API.
type WeightPhotoUploadRequest struct {
	Filename    string `json:"filename"     validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
}

// WeightPhotoUploadResponse is the JSON body returned by POST
// /api/v1/weight/photo-upload. The client PUTs the file bytes
// directly to `URL`, then submits `Key` back to the create or
// update form. `URL` is a presigned R2 PUT URL and is valid
// for one hour.
type WeightPhotoUploadResponse struct {
	URL string `json:"url"`
	Key string `json:"key"`
}

// --- Feedback ---

// SubmitFeedbackRequest is the body for POST /api/v1/feedback.
// Mirrors the two fields the web app's /feedback form collects
// (`title`, `message`); validation is delegated to
// FeedbackController.Submit so the JSON and HTML surfaces
// enforce the exact same length rules (title 5–100,
// message 10–1000, both trimmed). The user_id is taken from
// the JWT — clients cannot submit feedback on someone else's
// behalf.
type SubmitFeedbackRequest struct {
	Title   string `json:"title"`
	Message string `json:"message"`
}
