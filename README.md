# Hylete - The Fitness Tracker

A simple fitness tracking application built with Go, Echo, htmx, Tailwind CSS + Basecoat CSS, and SQLite/Turso. HTMX and Basecoat give a simple, clean semantic view layer for the web interface & SwiftUI is used for the iOS app. Built using an MVC architecture and focused on using as few dependencies as possible.

## Product shape

The iOS app (`client/`) is the primary user surface — logging sets, weigh-ins,
goals, timers, and feedback all happen there over the `/api/v1` JSON namespace.

This web app is a limited companion surface:
- Read-only dashboard (recent activity + 7-day stats)
- Per-exercise history and chart pages
- Profile settings (name, appearance, target weight & unit, weight-reminder preferences)
- Data Export page (`GET /export`, sidebar under Account) with weight data export (`/export/weight`) and exercise entries export (`/export/exercises`) ZIP downloads
- Admin: exercise management (+ image upload), users, feedback inbox
- Authentication (register / login / password reset)

## Image uploads

Images are displayed through ratio-enforced components on both surfaces —
`BannerImage` / `LandscapeImage` / `PortraitImage` / `SquareImage` in
`internal/views/components/image.templ` (web) and
`client/Hylete/DesignSystem/Image.swift` (iOS). Upload images already in the
ratio of the component that displays them so nothing important is cropped:

- Exercise images — **3:1** (e.g. **3000x1000**), the same ratio on web and
  iOS. Admin uploads are centre-cropped server-side to 3:1
  (1200x400 display / 2400x800 original), and both hero banners render an
  exact 3:1 window, so a 3:1 upload survives the whole pipeline untouched.
- Weight progress photos (iOS) — **16:9** (e.g. 1920x1080). They are never
  shown as banners (comparison is 3:4, list thumbnails are small), so 16:9
  is safe.
- Portrait — **3:4** (e.g. 1080x1440).
- Square — **1:1** (e.g. 1080x1080).

Other ratios still render, but are centre-cropped to fill the frame — they
are never stretched or distorted.

## Environment Variables

All variables are required and validated at startup. See `.env.example`.

- `PORT` - Server port (default: 8080)
- `JWT_SECRET` - Secret used to sign auth info
- `DB_PATH` - SQLite/Turso database file path (default: strength_tracker.db)
- `TURSO_DATABASE_URL` - Remote URL from Turso
- `TURSO_AUTH_TOKEN` - Auth token for Turso
- `STORAGE_ENDPOINT` - Storage endpoint URL
- `STORAGE_ACCESS_KEY` - Storage access key
- `STORAGE_SECRET_KEY` - Storage secret key
- `STORAGE_BUCKET` - Storage bucket name
- `STORAGE_PUBLIC_URL` - Public URL for the storage bucket
- `CLOUDFLARE_EMAIL_TOKEN` - Cloudflare email token
- `PUBLIC_URL` - Public URL for the app
