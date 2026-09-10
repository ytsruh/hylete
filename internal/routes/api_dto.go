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
	// HeightCm is the user's height in centimetres; nil means unset.
	HeightCm *float64 `json:"height_cm,omitempty"`
	// Gender is one of "male" | "female" | "non-binary" |
	// "prefer-not-to-say"; empty means unset.
	Gender string `json:"gender,omitempty"`
	// DateOfBirth is the user's birth date as "YYYY-MM-DD"; nil
	// means unset. Age is never sent — derive it if needed.
	DateOfBirth *string `json:"date_of_birth,omitempty"`
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
		HeightCm:     u.HeightCm,
		Gender:       u.GenderDisplay(),
		DateOfBirth:  u.DateOfBirth,
	}
}

// UpdateMeRequest is the JSON body for PUT /api/v1/me. Mirrors
// the user-editable subset of the HTML profile form (name,
// target weight, weight unit, distance unit, height, gender,
// date of birth) so the iOS app can update the same fields the web
// app exposes. Reminder preferences are deliberately omitted: they
// live on the dedicated GET/PUT /api/v1/me/reminders endpoints so an
// older client PUTting /me without reminder fields can never clobber
// them.
//
// TargetWeight / HeightCm / DateOfBirth are pointers so an omitted
// JSON field (or an explicit null) clears the value, matching the
// form's empty-input semantics. Gender is a string where empty means
// unset. DateOfBirth is a "YYYY-MM-DD" string validated by
// models.ParseDateOfBirth plus the min-age rule (not by tags — the
// same pattern as the web form).
type UpdateMeRequest struct {
	Name         string   `json:"name"          validate:"required,min=2,max=100"`
	TargetWeight *float64 `json:"target_weight" validate:"omitempty,gte=0,lte=1000"`
	WeightUnit   string   `json:"weight_unit"   validate:"omitempty,oneof=kg lbs"`
	DistanceUnit string   `json:"distance_unit" validate:"omitempty,oneof=km mi"`
	HeightCm     *float64 `json:"height_cm"     validate:"omitempty,gte=0,lte=300"`
	Gender       string   `json:"gender"        validate:"omitempty,oneof=male female non-binary prefer-not-to-say"`
	DateOfBirth  *string  `json:"date_of_birth"`
}

// ReminderPreferencesDTO is the JSON shape for the user's
// weight-reminder schedule. Mirrors models.ReminderPreferences
// but speaks the on-the-wire strings the web profile form uses:
// frequency is one of "off" | "daily" | "weekly" | "biweekly",
// day_of_week is 0–6 (Sunday=0) for weekly/biweekly and omitted
// otherwise, and time is "HH:00" in 24h UTC (hour-only by design).
//
// Reminders are email-only: enabled IS the opt-in, there is no
// separate channel flag. Returned by GET /api/v1/me/reminders
// and accepted (same shape) by PUT /api/v1/me/reminders.
type ReminderPreferencesDTO struct {
	Enabled   bool   `json:"enabled"`
	Frequency string `json:"frequency"`
	DayOfWeek *int   `json:"day_of_week,omitempty"`
	Time      string `json:"time"`
}

// ReminderPreferencesFromModel converts the stored user row into
// the DTO. Empty frequency reads as "off" and empty time as
// "09:00" (matching the SQL column defaults in migrations 00009
// and the web form's empty-input behavior) so a never-touched
// row decodes to a usable "reminders off" shape instead of a
// blank picker.
func ReminderPreferencesFromModel(u *models.User) ReminderPreferencesDTO {
	if u == nil {
		return ReminderPreferencesDTO{Frequency: string(models.ReminderOff), Time: defaultReminderTime}
	}
	frequency := string(u.ReminderFrequency)
	if frequency == "" {
		frequency = string(models.ReminderOff)
	}
	timeValue := u.ReminderTime
	if timeValue == "" {
		timeValue = defaultReminderTime
	}
	return ReminderPreferencesDTO{
		Enabled:   u.ReminderEnabled,
		Frequency: frequency,
		DayOfWeek: u.ReminderDayOfWeek,
		Time:      timeValue,
	}
}

// UpdateReminderPreferencesRequest is the JSON body for
// PUT /api/v1/me/reminders. Same shape as ReminderPreferencesDTO;
// validation mirrors the web profile form (profileInput in
// internal/routes/profile.go): frequency must be a known value,
// day_of_week must be 0–6 when present, and time must be a valid
// "HH:00" hour (checked via models.ParseReminderTimeForRoute so
// the same accept-set is enforced in one place).
type UpdateReminderPreferencesRequest struct {
	Enabled   bool   `json:"enabled"`
	Frequency string `json:"frequency"   validate:"required,oneof=off daily weekly biweekly"`
	DayOfWeek *int   `json:"day_of_week,omitempty" validate:"omitempty,gte=0,lte=6"`
	Time      string `json:"time"         validate:"required"`
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
//
// `Aliases` is the comma-separated alternate-name list (never rendered
// visibly). The iOS picker uses it for client-side search filtering
// the same way the web lists use their `data-aliases` attribute.
type ExerciseDTO struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Aliases          string `json:"aliases"`
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
		Aliases:     e.Aliases,
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
// the history list. Strength clients read max_weight and
// best_set_volume (best single-set reps * weight, 0 = none);
// cardio clients read best_pace_sec_per_km (0 = none) and
// longest_distance_meters. Both sets are always populated so
// the client only needs exercise_type to pick.
type HistoryStatsDTO struct {
	MaxWeight             float64           `json:"max_weight"`
	BestSetVolume         float64           `json:"best_set_volume"`
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
		BestSetVolume:         s.BestSetVolume,
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
// models.WeightEntry domain type but resolves each angle's storage
// key into a fully-qualified `*PhotoURL` so the iOS client can hand
// it straight to AsyncImage without any extra config.
//
// The `*PhotoKey` fields are included alongside the URLs so the iOS
// editor can re-submit them on update (the server's PUT handler
// expects to be told the existing keys explicitly). When an angle
// slot is empty its key and URL fields are omitted from the JSON so
// a client can rely on `Has*Photo` as the single source of truth
// (rather than checking both fields for emptiness).
type WeightEntryDTO struct {
	ID            string    `json:"id"`
	Weight        float64   `json:"weight"`
	Notes         string    `json:"notes"`
	FrontPhotoKey string    `json:"front_photo_key,omitempty"`
	FrontPhotoURL string    `json:"front_photo_url,omitempty"`
	HasFrontPhoto bool      `json:"has_front_photo"`
	SidePhotoKey  string    `json:"side_photo_key,omitempty"`
	SidePhotoURL  string    `json:"side_photo_url,omitempty"`
	HasSidePhoto  bool      `json:"has_side_photo"`
	BackPhotoKey  string    `json:"back_photo_key,omitempty"`
	BackPhotoURL  string    `json:"back_photo_url,omitempty"`
	HasBackPhoto  bool      `json:"has_back_photo"`
	HasPhoto      bool      `json:"has_photo"`
	PhotoCount    int       `json:"photo_count"`
	CreatedAt     time.Time `json:"created_at"`
}

// WeightEntryFromModel converts a models.WeightEntry into its
// JSON DTO. Photo URLs are resolved via utils.PublicURLFor
// so the iOS client never sees raw storage keys in the
// photo_url fields.
func WeightEntryFromModel(e models.WeightEntry) WeightEntryDTO {
	dto := WeightEntryDTO{
		ID:            e.ID,
		Weight:        e.Weight,
		Notes:         e.Notes,
		FrontPhotoKey: e.FrontPhotoKey,
		HasFrontPhoto: e.HasPhotoForAngle(models.WeightPhotoFront),
		SidePhotoKey:  e.SidePhotoKey,
		HasSidePhoto:  e.HasPhotoForAngle(models.WeightPhotoSide),
		BackPhotoKey:  e.BackPhotoKey,
		HasBackPhoto:  e.HasPhotoForAngle(models.WeightPhotoBack),
		HasPhoto:      e.HasPhoto(),
		PhotoCount:    e.PhotoCount(),
		CreatedAt:     e.CreatedAt,
	}
	if e.HasPhotoForAngle(models.WeightPhotoFront) {
		dto.FrontPhotoURL = utils.PublicURLFor(e.FrontPhotoKey)
	}
	if e.HasPhotoForAngle(models.WeightPhotoSide) {
		dto.SidePhotoURL = utils.PublicURLFor(e.SidePhotoKey)
	}
	if e.HasPhotoForAngle(models.WeightPhotoBack) {
		dto.BackPhotoURL = utils.PublicURLFor(e.BackPhotoKey)
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
// Carries the weight, notes, one optional photo key per angle, and
// the optional `created_at` that lets the client backdate an entry
// without manually re-issuing the server's time.Now() default.
//
// The numeric and length limits match the HTML form heritage so
// the JSON surface rejects the same inputs.
type CreateWeightEntryRequest struct {
	Weight        float64    `json:"weight"          validate:"required,gte=0,lte=1000"`
	Notes         string     `json:"notes"           validate:"max=1000"`
	FrontPhotoKey string     `json:"front_photo_key,omitempty"`
	SidePhotoKey  string     `json:"side_photo_key,omitempty"`
	BackPhotoKey  string     `json:"back_photo_key,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
}

// UpdateWeightEntryRequest is the body for PUT /api/v1/weight/:id.
// Same shape as the create request, plus per-angle `remove_*_photo`
// flags so the iOS editor can clear a single slot without first
// having to delete the entry and re-create it. When a remove flag
// is true the server ignores that angle's key and clears the
// associated column. When a key is non-empty the server replaces
// the existing key with the new one. When both are empty / false
// the existing key is preserved.
type UpdateWeightEntryRequest struct {
	Weight           float64    `json:"weight"             validate:"required,gte=0,lte=1000"`
	Notes            string     `json:"notes"              validate:"max=1000"`
	FrontPhotoKey    string     `json:"front_photo_key,omitempty"`
	RemoveFrontPhoto bool       `json:"remove_front_photo,omitempty"`
	SidePhotoKey     string     `json:"side_photo_key,omitempty"`
	RemoveSidePhoto  bool       `json:"remove_side_photo,omitempty"`
	BackPhotoKey     string     `json:"back_photo_key,omitempty"`
	RemoveBackPhoto  bool       `json:"remove_back_photo,omitempty"`
	CreatedAt        *time.Time `json:"created_at,omitempty"`
}

// WeightCompareResponse is the body for GET /api/v1/weight/compare.
// Returned in the same order the entries occurred on the
// timeline (Before/After) so the iOS view can render the slider
// with the historical photo on the left and the more recent one
// on the right without having to sort client-side.
// `Angle` echoes the compared angle slot. `DeltaText` is the
// formatted "+2.5 kg" / "−1.5 kg" / "" string for the weight delta.
type WeightCompareResponse struct {
	Before    WeightEntryDTO `json:"before"`
	After     WeightEntryDTO `json:"after"`
	Angle     string         `json:"angle"`
	DeltaText string         `json:"delta_text"`
}

// WeightPhotoUploadRequest is the JSON body for POST
// /api/v1/weight/photo-upload. Mirrors the existing
// `photoUploadRequest` at `internal/routes/photos.go:16` but
// is exposed as a public type so the iOS DTO and the Go
// validation tags live next to the rest of the weight API.
// Angle optionally namespaces the storage key (front/side/back).
type WeightPhotoUploadRequest struct {
	Filename    string `json:"filename"     validate:"required"`
	ContentType string `json:"content_type" validate:"required"`
	Angle       string `json:"angle,omitempty" validate:"omitempty,oneof=front side back"`
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

// --- Health snapshots ---

// HealthSnapshotDTO is one day of Apple Health vitals. Metric
// units are canonical (metres, kcal, bpm, ms, ml/kg/min,
// unitless mass); the iOS client converts to display units.
// MeasuredAt timestamps are omitted when nil (value carried
// forward or never measured) so Swift decodes them as nil via
// decodeIfPresent rather than tripping on null.
type HealthSnapshotDTO struct {
	ID                  string     `json:"id"`
	SnapshotDate        string     `json:"snapshot_date"`
	Tz                  string     `json:"tz"`
	Steps               int64      `json:"steps"`
	DistanceMeters      float64    `json:"distance_meters"`
	ActiveEnergyKcal    float64    `json:"active_energy_kcal"`
	BasalEnergyKcal     float64    `json:"basal_energy_kcal"`
	ExerciseMinutes     float64    `json:"exercise_minutes"`
	SleepSeconds        float64    `json:"sleep_seconds"`
	Weight              float64    `json:"weight"`
	WeightMeasuredAt    *time.Time `json:"weight_measured_at,omitempty"`
	BMI                 float64    `json:"bmi"`
	BMIMeasuredAt       *time.Time `json:"bmi_measured_at,omitempty"`
	BodyFatPercentage   float64    `json:"body_fat_percentage"`
	BodyFatMeasuredAt   *time.Time `json:"body_fat_measured_at,omitempty"`
	LeanBodyMass        float64    `json:"lean_body_mass"`
	LeanMassMeasuredAt  *time.Time `json:"lean_mass_measured_at,omitempty"`
	HeartRate           float64    `json:"heart_rate"`
	HeartRateMeasuredAt *time.Time `json:"heart_rate_measured_at,omitempty"`
	RestingHeartRate    float64    `json:"resting_heart_rate"`
	RestingHRMeasuredAt *time.Time `json:"resting_hr_measured_at,omitempty"`
	WalkingHeartRateAvg float64    `json:"walking_heart_rate_avg"`
	WalkingHRMeasuredAt *time.Time `json:"walking_hr_measured_at,omitempty"`
	HRV                 float64    `json:"hrv_ms"`
	HRVMeasuredAt       *time.Time `json:"hrv_measured_at,omitempty"`
	CardioRecoveryBPM   float64    `json:"cardio_recovery_bpm"`
	CardioRecoveryAt    *time.Time `json:"cardio_recovery_measured_at,omitempty"`
	VO2Max              float64    `json:"vo2_max"`
	VO2MeasuredAt       *time.Time `json:"vo2_measured_at,omitempty"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// HealthSnapshotFromModel converts a models.HealthSnapshot into
// its JSON DTO. Field-for-field copy — no conversion, since
// both sides already speak canonical units.
func HealthSnapshotFromModel(e models.HealthSnapshot) HealthSnapshotDTO {
	return HealthSnapshotDTO{
		ID:                  e.ID,
		SnapshotDate:        e.SnapshotDate,
		Tz:                  e.Tz,
		Steps:               e.Steps,
		DistanceMeters:      e.DistanceMeters,
		ActiveEnergyKcal:    e.ActiveEnergyKcal,
		BasalEnergyKcal:     e.BasalEnergyKcal,
		ExerciseMinutes:     e.ExerciseMinutes,
		SleepSeconds:        e.SleepSeconds,
		Weight:              e.Weight,
		WeightMeasuredAt:    e.WeightMeasuredAt,
		BMI:                 e.BMI,
		BMIMeasuredAt:       e.BMIMeasuredAt,
		BodyFatPercentage:   e.BodyFatPercentage,
		BodyFatMeasuredAt:   e.BodyFatMeasuredAt,
		LeanBodyMass:        e.LeanBodyMass,
		LeanMassMeasuredAt:  e.LeanMassMeasuredAt,
		HeartRate:           e.HeartRate,
		HeartRateMeasuredAt: e.HeartRateMeasuredAt,
		RestingHeartRate:    e.RestingHeartRate,
		RestingHRMeasuredAt: e.RestingHRMeasuredAt,
		WalkingHeartRateAvg: e.WalkingHeartRateAvg,
		WalkingHRMeasuredAt: e.WalkingHRMeasuredAt,
		HRV:                 e.HRV,
		HRVMeasuredAt:       e.HRVMeasuredAt,
		CardioRecoveryBPM:   e.CardioRecoveryBPM,
		CardioRecoveryAt:    e.CardioRecoveryAt,
		VO2Max:              e.VO2Max,
		VO2MeasuredAt:       e.VO2MeasuredAt,
		CreatedAt:           e.CreatedAt,
		UpdatedAt:           e.UpdatedAt,
	}
}

// HealthSnapshotsFromModels converts a slice of snapshots into
// the DTO slice. Returns an empty (non-nil) slice when the
// input is empty so the JSON encoder writes `[]` rather than
// `null` — easier for the Swift Codable decoder.
func HealthSnapshotsFromModels(es []models.HealthSnapshot) []HealthSnapshotDTO {
	out := make([]HealthSnapshotDTO, 0, len(es))
	for _, e := range es {
		out = append(out, HealthSnapshotFromModel(e))
	}
	return out
}

// HealthSnapshotsResponse wraps the snapshot slice in a named
// envelope so the server can add fields without breaking the
// iOS contract.
type HealthSnapshotsResponse struct {
	Snapshots []HealthSnapshotDTO `json:"snapshots"`
}

// HealthSnapshotItem is one snapshot in an upsert batch. Range
// bounds reject physically implausible values; the calendar
// date shape and future-date rule are enforced by the
// controller (validateSnapshotDate) so the error surfaces as a
// sentinel-mapped 400.
type HealthSnapshotItem struct {
	SnapshotDate         string     `json:"snapshot_date"          validate:"required"`
	Tz                   string     `json:"tz"                     validate:"max=64"`
	Steps                int64      `json:"steps"                  validate:"gte=0,lte=200000"`
	DistanceMeters       float64    `json:"distance_meters"        validate:"gte=0,lte=500000"`
	ActiveEnergyKcal     float64    `json:"active_energy_kcal"     validate:"gte=0,lte=20000"`
	BasalEnergyKcal      float64    `json:"basal_energy_kcal"      validate:"gte=0,lte=20000"`
	ExerciseMinutes      float64    `json:"exercise_minutes"       validate:"gte=0,lte=1440"`
	SleepSeconds         float64    `json:"sleep_seconds"          validate:"gte=0,lte=86400"`
	Weight               float64    `json:"weight"                 validate:"gte=0,lte=1000"`
	WeightMeasuredAt     *time.Time `json:"weight_measured_at,omitempty"`
	BMI                  float64    `json:"bmi"                    validate:"gte=0,lte=100"`
	BMIMeasuredAt        *time.Time `json:"bmi_measured_at,omitempty"`
	BodyFatPercentage    float64    `json:"body_fat_percentage"    validate:"gte=0,lte=100"`
	BodyFatMeasuredAt    *time.Time `json:"body_fat_measured_at,omitempty"`
	LeanBodyMass         float64    `json:"lean_body_mass"         validate:"gte=0,lte=1000"`
	LeanMassMeasuredAt   *time.Time `json:"lean_mass_measured_at,omitempty"`
	HeartRate            float64    `json:"heart_rate"             validate:"gte=0,lte=300"`
	HeartRateMeasuredAt  *time.Time `json:"heart_rate_measured_at,omitempty"`
	RestingHeartRate     float64    `json:"resting_heart_rate"     validate:"gte=0,lte=300"`
	RestingHRMeasuredAt  *time.Time `json:"resting_hr_measured_at,omitempty"`
	WalkingHeartRateAvg  float64    `json:"walking_heart_rate_avg" validate:"gte=0,lte=300"`
	WalkingHRMeasuredAt  *time.Time `json:"walking_hr_measured_at,omitempty"`
	HRV                  float64    `json:"hrv_ms"                 validate:"gte=0,lte=1000"`
	HRVMeasuredAt        *time.Time `json:"hrv_measured_at,omitempty"`
	CardioRecoveryBPM    float64    `json:"cardio_recovery_bpm"    validate:"gte=0,lte=300"`
	CardioRecoveryAt     *time.Time `json:"cardio_recovery_measured_at,omitempty"`
	VO2Max               float64    `json:"vo2_max"                validate:"gte=0,lte=100"`
	VO2MeasuredAt        *time.Time `json:"vo2_measured_at,omitempty"`
}

// UpsertHealthSnapshotsRequest is the body for POST
// /api/v1/health-snapshots: a batch of daily snapshots (the
// 90-day backfill fits with headroom; larger syncs chunk
// client-side). Same dates re-uploaded overwrite via the
// (user_id, snapshot_date) upsert, so retries are safe.
type UpsertHealthSnapshotsRequest struct {
	Snapshots []HealthSnapshotItem `json:"snapshots" validate:"required,min=1,max=120,dive"`
}
