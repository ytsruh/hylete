# Hylete iOS

Minimal SwiftUI client for the Hylete strength tracker. Talks to the
existing Go server's `/api/v1` JSON namespace using the same JWT the
web app uses (sent as `Authorization: Bearer <token>`).

## Requirements

- macOS with **Xcode 15+** (provides the iOS 17 SDK and Simulator)
- [XcodeGen](https://github.com/yonaskolb/XcodeGen) (`brew install xcodegen`)
- A running Hylete server (the parent `../` directory) reachable from
  the simulator

## First-time setup

```bash
# from this directory (client/)
make gen           # generates Hylete.xcodeproj from project.yml
open Hylete.xcodeproj   # optional — only needed for the debugger
```

Edit `Hylete/Configs/*.xcconfig` to change the server a build talks
to. Defaults are already wired up:

| Scheme | Config | API URL | Bundle ID |
|---|---|---|---|
| **Dev** (daily driver) | Debug | `http://localhost:8080/api/v1` | `com.hyleteapp.hylete.dev` |
| **Prod** (ship lane) | Release | `https://www.hyleteapp.com/api/v1` | `com.hyleteapp.hylete` |

The two bundle IDs mean a dev install ("Hylete Dev") and a production
install ("Hylete") can coexist side-by-side on the same device or
simulator, with independent logins. Select the scheme in Xcode's
scheme picker, or override the server per build in scheme editor →
Run → Arguments → Environment Variables.

## Day-to-day workflow

| Command | What it does |
|---|---|
| `make gen`     | Regenerate `Hylete.xcodeproj` after editing `project.yml` or moving files |
| `make build`   | Compile the app for the booted iOS simulator (no run) |
| `make boot`    | Open the first available iPhone simulator (no-op if one is already running) |
| `make run`     | Build, install, and launch on the booted simulator — **auto-boots one if nothing is running** |
| `make release` | Archive a Release build (Prod scheme) and open Xcode's Organizer to install on a connected iPhone |
| `make clean`   | Wipe build artifacts |

`make run` is a one-keystroke dev loop. It first checks whether a
simulator is already booted; if not, it boots the first available
iPhone and opens the Simulator app, then installs and launches the
app on it. If you want to run on a different simulator than the
default, just open it manually first (`make boot` will pick that
one) — the first iPhone in the `Booted` list wins.

Everything above uses the **Dev** scheme (Debug configuration →
localhost server). Shipping is `make release`, which archives via
the **Prod** scheme (Release configuration → production server).
To smoke-test the production API from a simulator, select the Prod
scheme in Xcode and just Run — no archive needed.

Most edits — Swift files, `project.yml`, plist values — don't need
Xcode at all. Re-run `make gen` after structural changes (added
folders, new target, plist keys) and Zed stays usable for ~99% of
the work. The only times you need to open Xcode are:

- The very first time, to let Xcode index the SDK and accept the
  signing certificate
- When the debugger or the SwiftUI canvas would save you time
- When you need to change code signing / capabilities / bundle ID

## Architecture (very brief)

- `App/` — `@main` entry point and dependency container
- `Networking/` — `APIClient` (URLSession + async/await), Codable DTOs,
  `AuthStore` (token in Keychain), error types
- `DesignSystem/` — colors, spacing, button styles
- `Auth/`, `Dashboard/`, `NewSet/`, `Exercises/`, `Profile/` — one
  folder per feature, each with its own views

No third-party Swift packages. No CocoaPods. Just the system SDK.

## Talking to the server

The app sends every request with `Authorization: Bearer <jwt>` where
`<jwt>` is the token returned by `POST /api/v1/auth/login`. The token
is stored in the iOS Keychain (`kSecClassGenericPassword`,
`kSecAttrAccessibleAfterFirstUnlock`) so it survives app restarts but
is never written to UserDefaults or to disk in plaintext.

See `../internal/routes/api_v1.go` for the full server contract.
