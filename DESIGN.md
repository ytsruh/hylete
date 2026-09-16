# Hylete Design System

One brand, two surfaces: the **web companion app** (Go + Templ + Tailwind v4 + Basecoat CSS)
and the **iOS app** (SwiftUI). Both read from the same semantic token palette so a colour
means the same thing everywhere.

> Source of truth for colour values: `styles/input.css` (`:root` = light, `.dark` = dark).
> The iOS asset catalog mirrors those values in sRGB. `BrandColors.swift` keeps an auditable
> OKLCH ↔ hex table of both.

## Component libraries

Each platform builds on a component library; the custom design-system code themes and
extends it rather than replacing it.

* **Web — Basecoat CSS** (`styles/basecoat.css`, behaviour in `public/js/basecoat.js`).
  Provides buttons (`.btn`, `.btn-destructive`, …), badges (`.badge`), toasts (`.toast` /
  `.toaster`), dialogs, dropdowns and form primitives. Components are themed entirely through
  the CSS tokens above, so a rebrand flows through with no markup changes. App-specific
  composition (toast/banner wrappers, confirm modal, chart components) lives in
  `internal/views/layout.templ` and `internal/views/components/`. When using a Basecoat
  component, reuse its class + data-attribute conventions rather than inventing new ones —
  e.g. toasts use `data-category`, badges use `data-variant`.
* **iOS — SwiftUI.** All UI is SwiftUI; the `client/Hylete/DesignSystem/` folder is the thin
  custom layer on top: `DSColors` (themed colours), `DSSpacing` / corner radii, type scale
  (`Typography.swift`), icons (`Icons.swift`), button + text-field styles
  (`ButtonStyles.swift`, `TextFieldStyles.swift`), and composite pieces (`StatCard`,
  `WeekCalendarView`, `GoalCompletionCelebration`, …). Prefer SwiftUI system components
  styled with `DSColors`/`DSSpacing` over bespoke drawing, and never hard-code hex in views.

## Colour tokens

| Token | Light | Dark | Used for |
|---|---|---|---|
| `background` | `#FEFEFE` | `#161616` | Page background |
| `foreground` | `#252525` | `#E4E4E4` | Primary text |
| `card` / `card-foreground` | `#FEFEFE` / `#252525` | `#252525` / `#E4E4E4` | Cards, list rows |
| `popover` / `popover-foreground` | `#FEFEFE` / `#252525` | `#252525` / `#E4E4E4` | Elevated surfaces, pills, popovers |
| `primary` / `primary-foreground` | `#F44900` brand orange / white | same | Primary buttons, tab tint, links, profile halo |
| `secondary` / `secondary-foreground` | dark fill `#252525` / white | light fill `#E4E4E4` / dark text | Inverted chips/badges (e.g. cardio type pill) |
| `muted` / `muted-foreground` | `#F8F9FA` / `#6A7180` | `#1F1F1F` / `#A3A3A3` | Subtle surfaces, secondary/caption text, sidebar |
| `accent` / `accent-foreground` | `#FEFBEB` warm tint / `#92400E` | `#92400E` / `#FDE68A` | Soft accent rows, tinted backgrounds |
| `destructive` / `destructive-foreground` | `#EE4444` / white | same | Delete, sign out |
| `success` / `success-foreground` | `#00C389` green / white | same | Confirmations, goal celebration, success toasts |
| `info` / `info-foreground` | `#763EC1` purple / white | same | Informational badges, highlights, info toasts |
| `border` / `input` | `#E5E7EA` | `#404040` | Hairlines, field borders |
| `ring` | `#F59D0A` amber | same | Focus rings, sidebar highlights |
| `chart-1 … chart-5` | `#F59D0A → #78340E` amber/brown ramp | `#FBBF24 …` variant | Charts, sidebar accents |

Brand accents that never change with theme: `primary #F44900`, `destructive #EE4444`,
`success #00C389`, `info #763EC1`, `ring #F59D0A`.

### Donut-chart palette (separate from `chart-*`)

The exercises donut uses its own monochromatic brand-orange ramp so slices read as one family:

`#F54900` → `#F65B1A` → `#F76D33` → `#F87F4C` → `#F99166` → `#FAA47F`
(brand mixed with white at 0/10/20/30/40/50%), plus `#9CA3AF` (gray-400) for the "Other" bucket.

* Web: `internal/views/components/donut_chart.templ` (`donutDefaultPalette`), default line-chart
  colour `#F54900` in `internal/views/components/chart.templ`, Other-bucket `#9ca3af` in
  `internal/views/dashboard/dashboard.templ`.
* iOS: same hexes as `Color` literals in `Dashboard/PopularExercises.swift`.

### Status-colour usage today

* iOS: `DSColors.success` drives the goal-completion checkmark
  (`DesignSystem/GoalCompletionCelebration.swift`); confetti uses brand orange + gray.
  `success`/`info` foregrounds are white in both modes.
* Web: toasts render via `views.Toast(category, …)` / `toastTemplate` in
  `internal/views/layout.templ` with `data-category="success" | "error" | "warning" | "default"`.
  Basecoat styles toasts generically (no per-category colour), so `bg-success` / `bg-info` /
  `text-success` utilities are available now for badges, banners and future toast styling —
  e.g. the admin `badge badge-success` in `internal/views/admin/users.templ`.

## How it works — web

* `styles/input.css` declares every token twice (`:root` light, `.dark` dark) in OKLCH, then
  re-exports them in `@theme inline` as `--color-*` so Tailwind generates utilities:
  `bg-background`, `text-foreground`, `text-muted-foreground`, `bg-card`, `border-border`,
  `bg-success`, `text-info`, etc.
* Stack: `@import "tailwindcss"` + `basecoat.css` (component library: buttons, badges, toasts,
  dialogs) + `basecoat-compat.css`. Fonts: Inter (sans), Source Serif 4 (serif),
  JetBrains Mono (mono); radius `--radius: 0.375rem`; dark mode via `@custom-variant dark`.
* Build: `make css-build` compiles `styles/input.css` → `public/css/styles.css` (Tailwind CLI).
  `make build` runs `templ generate` + `sqlc generate` + `css-build` + `go build`.
* Theme switching: `themeMode` in `localStorage` (`system` | `light` | `dark`), toggled in
  settings; mirrors the iOS `ThemeMode` default of `system`.

## How it works — iOS

* Values live in the asset catalog: `client/Hylete/Assets.xcassets/DS/<token>.colorset/`,
  each with a light (universal) and dark (`luminosity: dark`) variant. SwiftUI picks the right
  one from the current `ColorScheme` automatically.
* Code consumes them through the semantic API in `DesignSystem/Colors.swift` (`DSColors.*`):
  `background`, `surface` (= card), `surfaceElevated` (= popover), `separator` (= border),
  `secondary` / `onSecondary`, `text` / `textSecondary`, `onPrimary`, `onCard`,
  `accent` (= brand) / `accentSubtle` / `onAccentSubtle`, `focusRing`,
  `destructive` / `onDestructive`, `success` / `onSuccess`, `info` / `onInfo`,
  `chart1…chart5`, plus `systemAccent` for system components.
* **Asset-name gotcha:** the runtime name is the colorset name only — Xcode strips the `DS/`
  folder prefix, so the lookup is `Color("background")`, never `Color("DS/background")`
  (a wrong prefix silently yields `.clear`). Three tokens keep a `ds-` prefix to avoid
  collisions with Xcode generated symbols: `ds-primary` (= `accent`), `ds-secondary`,
  `ds-accent` (= `accentSubtle`). `info` / `success` need no prefix.
* `DesignSystem/BrandColors.swift` is the audit table (OKLCH + hex for light and dark) and
  documents the rebrand workflow. `DesignSystem/Theme/ThemeMode.swift` mirrors the web theme
  preference (`system` default, persisted in `@AppStorage`).
* Supporting tokens: `DSSpacing` (4–48 scale, 12pt card / 8pt chip radii in `Spacing.swift`),
  type scale in `Typography.swift`, icons in `Icons.swift`, button/field styles in
  `ButtonStyles.swift` / `TextFieldStyles.swift`, `StatCard` for dashboard cards.

## Adding or changing a colour

1. Update `styles/input.css` first (`:root` + `.dark`), converting the new hex to OKLCH
   (sRGB → linear → LMS → OKLCH per Björn Ottosson's matrices; verify against a converter).
   Mode-invariant accents (brand, destructive, success, info, ring) use the same value twice.
2. Export it in `@theme inline` as `--color-<name>` (+ `--color-<name>-foreground` for text-on-fill).
3. Mirror it in iOS: update/add `Assets.xcassets/DS/<name>.colorset/Contents.json`
   (sRGB 0–1 components, both appearances), expose it in `DSColors`, and update the
   `BrandColors.light` / `dark` tables.
4. Run `make css-build`, `templ generate` if markup changed, `go test ./...`, and check both
   colour schemes on web and in Xcode previews. Change the `.colorset` JSON to rebrand —
   never hard-code hex in views (donut-ramp literals are the documented exception).

## File map

| Area | Files |
|---|---|
| Web tokens | `styles/input.css`, `public/css/styles.css` (generated) |
| Web components | `styles/basecoat.css`, `styles/basecoat-compat.css`, `internal/views/layout.templ` (toast/banner), `internal/views/components/` |
| iOS API | `client/Hylete/DesignSystem/Colors.swift`, `BrandColors.swift`, `Theme/ThemeMode.swift`, `Spacing.swift`, `Typography.swift` |
| iOS values | `client/Hylete/Assets.xcassets/DS/*.colorset/`, `AccentColor.colorset` |
| Charts | `internal/views/components/donut_chart.templ`, `chart.templ`, `client/Hylete/Dashboard/PopularExercises.swift`, `DonutChart.swift` |
