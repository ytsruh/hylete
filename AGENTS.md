# Agent Instructions for Hylete

## Product Shape (read this first)
- **The iOS client (`client/`) is the primary user surface.** All user data entry — logging/editing sets, weigh-ins, goals, timers, feedback — happens in the iOS app over the `/api/v1` JSON namespace. That API is critical functionality: never remove or break `/api/v1/*` handlers.
- **The web app is a limited companion surface**: read-only dashboard, per-exercise history/chart pages, profile settings, Data Export page (bulk ZIP downloads), admin (exercises + user management [admin toggle with self-demotion guard + password reset email reusing the forgot-password flow] + feedback inbox), and auth. Do not add web UI for creating or editing workout data.

## Application & Business Logic Instructions
- Weight doesn't have KG or lbs and is stored as a number only. A user's preferred unit is a profile setting; the web reads it via `User.WeightUnitDisplay()` and labels every weight with it. Cardio distances work the same way: stored canonically in **metres**, displayed via the user's `distance_unit` profile setting (`User.DistanceUnitDisplay()`, "km"/"mi"). Pace is never stored — derived as `duration_seconds / (distance_meters/1000)`.
- Every exercise entry created for a workout is a single set (strength) or a single cardio session. Which metric pair carries meaning is decided by the linked exercise's `type` ('strength' | 'cardio' | 'other'), never by client input, and the server zeroes the pair that doesn't apply: strength entries use reps + weight + rest_time; cardio entries require duration_seconds + distance_meters with optional avg_heart_rate + calories_burned (validation in `controllers.ValidateExerciseSetInput`, sentinels map to 400s in `internal/routes/api_v1.go`). The web renders exercise entries read-only and branches its stat cards/charts/tables on type (cardio history shows Best Pace + fastest-pace-per-day charts); creation/update/delete flows live only in the API + iOS client.
- **Naming convention — be explicit.** The domain model is an **exercise entry** (one row in the `exercise_entries` table). The user-facing copy uses **"set"** / **"sets"** for strength and **"session"** for cardio (e.g. "Last 5 Sets" vs "Last 3 Sessions") because that's what the user sees. Code, URLs, HTML ids, comments, and developer-facing strings must always say **"exercise entry"** / **"exercise entries"** — never the bare word `entry` or `entries`. This is deliberate: the SQL table is still `exercise_entries` (and the `ExerciseEntry` struct stays), so anyone reading the codebase can always tell what the row represents without checking a glossary. When in doubt, write the longer form.
- Weight reminders run on an hourly cron defined by `weightReminderCronSpec` in `cmd/main.go`. The per-user schedule lives in the users table (edited on the profile page via a single master toggle + frequency/day/time); the cron is the only trigger — there is no admin UI to fire reminders manually. Reminders are **email-only** — the email column mirrors the master switch — and the web-push stack (VAPID, subscriptions, service worker) was removed along with the admin broadcast screen.
- Bulk data exports live on the dedicated **Data Export page** (`GET /export`, sidebar entry under Account): `GET /export/weight` streams a zip of the user's weight entries + photos, and `GET /export/exercises` streams a zip of the user's exercise entries (CSV + manifest). Both zips are built by `internal/export` (zip + CSV via Go stdlib, no new deps) through an `io.Pipe` in the controllers. The weight zip pulls photos from R2 via `utils.GetObject`; photos that can't be fetched are skipped, with their keys listed in the zip's `manifest.json` `missing_photos` array. Exercise entries have no photos, so that zip is just `exercise_entries.csv` + `manifest.json`. Downloads are plain `<a download>` anchors with a click-triggered loading spinner in the export page's inline script — native downloads fire no completion event JS can observe, so the spinner clears on a fixed timer and real progress lives in the browser's downloads UI; don't convert these to htmx requests (binary responses can't be swapped).
- Transactional email is sent through Cloudflare Email Sending's authenticated SMTP endpoint (`smtp.mx.cloudflare.net:465`, implicit TLS). The whole feature is in `internal/email` (`client.go` does the hand-rolled `crypto/tls` + `net/smtp`, `service.go` composes messages) and the email-specific Templ components live in `internal/email/templates/`. The sender address (`noreply@hyleteapp.com`) and SMTP host are hard-coded in the package; the only required env vars are `CLOUDFLARE_EMAIL_TOKEN` (SMTP auth) and `PUBLIC_URL` (base URL threaded into every link the email contains). **Do not add more env vars for email config without discussing first.**

## Code & Development Instructions
- Use Makefile scripts where possible
- Add and create unit tests wherever possible
- Follow go best practices at all time. Particulary using interfaces, packages should create interfaces that they accept.
- templ is used for templating lanaguage & must be regenerated before running the application. Generated files: `internal/views/*_templ.go` (don't edit directly)
- Avoid adding new dependancies. If they are needed, discuss with user/developer first. If agreed to use then always wrap dependancies in packages so they are isolated and easily edited, removed, updated without codebase wide changes.
- Add documentation/annotations to code wherever possible to explain what functions, structs etc do.
- Make use of `go doc` Go command to fetch documentation for packages/modules
- When using HTMX, use existing patterns such as loading spinners and confirmation modals/dialogs

## Technology & Architecture
- **Tech stack**: Go 1.25+, Echo, Templ, Turso Sync (turso.tech/database/tursogo), htmx, Basecoat CSS
- **Entry point**: `cmd/main.go`
- **HTTP routes**: `internal/routes/*.go` (Echo framework — request parsing, rendering, redirects, middleware)
- **Controllers**: `internal/controllers/*.go` (business logic — auth, exercise entry CRUD orchestration)
- **Repository interfaces**: `internal/models/repository.go` (CRUD operations)
- **Models**: `internal/models/models.go` (ExerciseType, ExerciseEntry, User)
- **Database**: `internal/db/db.go` (Turso Sync wrapper with local-first reads/writes and background cloud sync)
- **Migrations**: `internal/db/migrations/*.sql` managed by [goose](https://github.com/pressly/goose) and embedded in the binary via `//go:embed`
- **Templates**: `internal/views/*.templ` (Templ HTML templates)

## Environment Variables

All environment variables are strictly required and validated on server startup.
Create a `.env` file based on `.env.example` before running the application.

- `PORT` - Server port
- `DB_PATH` - Local database file path (e.g., `/data/strength_tracker.db`)
- `TURSO_DATABASE_URL` - Turso Cloud database URL (`libsql://...`)
- `TURSO_AUTH_TOKEN` - Turso Cloud authentication token
- `CLOUDFLARE_EMAIL_TOKEN` - Cloudflare API token used as the SMTP password for `smtp.mx.cloudflare.net:465`. The token must have the **Email Sending: Edit** permission. Used by `internal/email` for the welcome email on register and the password-reset email.
- `PUBLIC_URL` - Absolute origin the app is served from (e.g. `https://www.hyleteapp.com`). Threaded into every link in transactional emails — the dashboard link in the welcome email, the password-reset link, and the footer's "view on the web" link. Must be a valid http/https URL with no trailing slash; the value is validated at startup by `email.NewService`.
