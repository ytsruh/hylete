# Hylete iOS Makefile
# Drives XcodeGen + xcodebuild. All real work happens in
# Xcode/CommandLineTools — this file is just a thin wrapper so
# the day-to-day commands are short and memorable.

# Schemes are defined in project.yml and generated into the
# xcodeproj by `make gen`. `Dev` runs the Debug configuration
# (localhost dev server, com.hyleteapp.hylete.dev bundle ID); `Prod`
# is the ship lane (Release config, production server). All
# day-to-day targets below build Debug via the Dev scheme; the
# `release` target archives through the Prod scheme.
SCHEME = Dev
RELEASE_SCHEME = Prod
# Use the generic simulator destination so xcodebuild doesn't
# try to resolve `OS:latest` to the latest installed runtime
# (which can be a beta iOS that doesn't have the iPhone model
# the user picked). The build is OS-agnostic; the `run` target
# picks a real simulator UUID via `xcrun simctl` after the
# build finishes.
DESTINATION ?= generic/platform=iOS Simulator
XCODEPROJ = Hylete.xcodeproj
# Bundle ID of the DEBUG build (see Configs/Debug.xcconfig).
# `run` installs and launches that one, so `simctl launch`
# must use the `.dev` identifier — the plain com.hyleteapp.hylete
# belongs to Release/production installs.
BUNDLE_ID = com.hyleteapp.hylete.dev

# A regex that matches a simulator UUID. Used by `boot` and
# `run` to extract a usable device id from the (somewhat
# noisy) `xcrun simctl list devices` output.
SIM_UUID_RE = [A-F0-9]{8}-[A-F0-9]{4}-[A-F0-9]{4}-[A-F0-9]{4}-[A-F0-9]{12}

# Absolute path to the Simulator.app inside the active
# developer directory. Resolved once at parse time from
# `xcode-select -p`, so it works for Xcode.app, Xcode-beta.app,
# and any other variant without depending on Launch Services
# having registered the app under its short name. Used by
# `run` to bring the Simulator window to the front after
# `simctl boot` starts the device headless.
SIMULATOR_APP = $(shell xcode-select -p)/Applications/Simulator.app

.PHONY: help gen build test boot run clean release

help:
	@echo "Hylete iOS — available targets:"
	@echo ""
	@echo "  make gen      Regenerate $(XCODEPROJ) from project.yml"
	@echo "  make build    Compile for the booted simulator"
	@echo "  make test     Build and run the unit tests on a simulator (booted device preferred)"
	@echo "  make boot     Boot the first available iPhone simulator (no-op if one is already running)"
	@echo "  make run      Build, install, and launch on the booted simulator (auto-boots one if needed)"
	@echo "  make release  Archive a Release build (Prod scheme) and open Xcode's Organizer to install on a connected iPhone"
	@echo "  make clean    Wipe build artifacts"
	@echo ""
	@echo "Override the destination with DESTINATION=...  e.g."
	@echo "  make run DESTINATION='platform=iOS Simulator,name=iPhone 15 Pro'"

# XcodeGen — re-run after editing project.yml or moving/adding files.
# File globs in project.yml pick up new Swift files automatically;
# you only need this for structural changes (new target, plist keys,
# capabilities).
gen:
	@command -v xcodegen >/dev/null 2>&1 || { echo "xcodegen not installed. Run: brew install xcodegen"; exit 1; }
	xcodegen generate
	@echo "✓ $(XCODEPROJ) regenerated"

build:
	xcodebuild \
		-project $(XCODEPROJ) \
		-scheme $(SCHEME) \
		-configuration Debug \
		-destination '$(DESTINATION)' \
		build

# Unit tests. Unlike `build`, running tests needs a concrete,
# bootable device (the generic destination can't host a test
# run), so this resolves one the same way `run` does: prefer an
# already-booted iPhone, otherwise boot the first available.
#
# IMPORTANT: like `run`, this recipe body MUST be one
# continuous shell block — a `#` comment without a trailing
# `\` would split the script across subshells. Explanatory
# comments live above the recipe, not inside it.
test:
	@DEVICE=$$(xcrun simctl list devices booted 2>/dev/null | grep -E "iPhone" | head -1 | grep -oE "$(SIM_UUID_RE)"); \
	if [ -z "$$DEVICE" ]; then \
		DEVICE=$$(make -s boot); \
	fi; \
	echo "Running unit tests on simulator $$DEVICE..."; \
	xcodebuild \
		-project $(XCODEPROJ) \
		-scheme $(SCHEME) \
		-destination "id=$$DEVICE" \
		test

# `boot` opens the first available iPhone simulator. It's a
# no-op if one is already booted so the user can run it
# freely. The simulator's UUID is written to stdout so any
# caller can chain off it.
#
# IMPORTANT: like `run`, this recipe body MUST be one
# continuous shell block. Every `#` comment without a
# trailing `\` would break the continuation chain and split
# the script across multiple subshells. All explanatory
# comments live above the recipe.
boot:
	@BOOTED=$$(xcrun simctl list devices booted 2>/dev/null | grep -E "iPhone|iPad" | head -1 | grep -oE "$(SIM_UUID_RE)"); \
	if [ -n "$$BOOTED" ]; then \
		echo "$$BOOTED"; \
		exit 0; \
	fi; \
	DEVICE=$$(xcrun simctl list devices available 2>/dev/null | grep -E "iPhone" | grep -v "unavailable" | head -1 | grep -oE "$(SIM_UUID_RE)"); \
	if [ -z "$$DEVICE" ]; then \
		echo "No iPhone simulator available. Open Xcode → Settings → Platforms to download one." >&2; \
		exit 1; \
	fi; \
	echo "Booting simulator $$DEVICE..." >&2; \
	xcrun simctl boot "$$DEVICE" >/dev/null 2>&1 || true; \
	for i in $$(seq 1 30); do \
		if xcrun simctl list devices 2>/dev/null | grep "$$DEVICE" | grep -q "Booted"; then \
			break; \
		fi; \
		sleep 1; \
	done; \
	echo "$$DEVICE"

# `run` is the one-keystroke dev loop. It builds, resolves
# (or boots) a simulator, then installs and launches. If a
# simulator is already booted, that device is used — boot a
# different one first to override.
#
# The boot logic is inlined here rather than delegated to
# the `boot` target via `$(MAKE) -s boot`, because capturing
# stdout from a sub-make through `$$()` is fragile (sub-make
# output buffering + `$$()` pipe handling + the `-s` flag
# can lose the captured value in ways that are hard to
# reproduce). Inlining keeps the entire boot → install →
# launch sequence in one shell, one variable scope, and one
# place to add error reporting. The `boot` target remains
# as a standalone "boot a simulator and print its UUID"
# command for scripts that want to chain off it.
#
# IMPORTANT: the recipe body MUST be one continuous shell
# block. Every line of the recipe is a single `sh -c`
# invocation, so a `#` comment that doesn't end with `\` will
# break the continuation chain and split the script across
# multiple shells (causing "unexpected end of file" errors
# because `if/fi`, `for/done`, etc. get truncated). All
# explanatory comments live above the recipe, not inside it.
run: build
	@BOOTED=$$(xcrun simctl list devices booted 2>/dev/null | grep -E "iPhone" | head -1 | grep -oE "$(SIM_UUID_RE)"); \
	if [ -n "$$BOOTED" ]; then \
		DEVICE="$$BOOTED"; \
		echo "Using already-booted simulator $$DEVICE" >&2; \
	else \
		DEVICE=$$(xcrun simctl list devices available 2>/dev/null | grep -E "iPhone" | grep -v "unavailable" | head -1 | grep -oE "$(SIM_UUID_RE)"); \
		if [ -z "$$DEVICE" ]; then \
			echo "No iPhone simulator available. Open Xcode → Settings → Platforms to download one." >&2; \
			exit 1; \
		fi; \
		echo "Booting simulator $$DEVICE..." >&2; \
		xcrun simctl boot "$$DEVICE" >/dev/null 2>&1 || true; \
		open "$(SIMULATOR_APP)" 2>/dev/null || true; \
		for i in $$(seq 1 30); do \
			if xcrun simctl list devices 2>/dev/null | grep "$$DEVICE" | grep -q "Booted"; then \
				break; \
			fi; \
			sleep 1; \
		done; \
	fi; \
	APP_PATH=$$(xcodebuild -project $(XCODEPROJ) -scheme $(SCHEME) -configuration Debug -destination '$(DESTINATION)' -showBuildSettings 2>/dev/null | awk -F' = ' '/^[[:space:]]+TARGET_BUILD_DIR/ {d=$$2} /^[[:space:]]+FULL_PRODUCT_NAME/ {n=$$2} END {print d"/"n}'); \
	if [ -z "$$DEVICE" ]; then \
		echo "Internal error: device UUID is empty after boot. Please report this." >&2; \
		exit 1; \
	fi; \
	if [ -z "$$APP_PATH" ] || [ ! -d "$$APP_PATH" ]; then \
		echo "Could not locate built .app. Try: make build" >&2; \
		exit 1; \
	fi; \
	echo "Installing $$APP_PATH on $$DEVICE"; \
	xcrun simctl install "$$DEVICE" "$$APP_PATH" && \
	PID=$$(xcrun simctl launch "$$DEVICE" $(BUNDLE_ID) | awk '{print $$NF}') && \
	echo "" && \
	echo "✓ Hylete is running on $$DEVICE (pid $$PID)" && \
	echo "  The Simulator window should be on screen. If not, run: open \"$(SIMULATOR_APP)\""

clean:
	rm -rf build
	xcodebuild \
		-project $(XCODEPROJ) \
		-scheme $(SCHEME) \
		clean 2>/dev/null || true

# `release` produces a signed Release build for a real iOS
# device and hands it off to Xcode's Organizer. The user
# then picks "Distribute App" → "Development" → their
# iPhone to install. It drives the Prod scheme (whose
# archive action is the Release configuration) and passes
# `-configuration Release` explicitly anyway, so the result
# is unambiguous: production bundle ID, "Hylete" display
# name, and the base URL from Hylete/Configs/Release.xcconfig
# (https://www.hyleteapp.com/api/v1).
#
# Signing is automatic via Xcode's personal-team
# provisioning. On first run Xcode will prompt for your
# Apple ID in the Organizer; subsequent runs reuse the
# cached profile. `-allowProvisioningUpdates` is a no-op
# once everything is set up, so it's safe to pass every
# time. To make unattended builds work, set DEVELOPMENT_TEAM
# in the env or in Release.xcconfig.
#
# IMPORTANT: like `run`, this recipe body MUST be one
# continuous shell block. Every `#` comment without a
# trailing `\` would break the continuation chain. All
# explanatory comments live above the recipe, not inside.
release:
	@ARCHIVE_DIR=$$(pwd)/build/Hylete.xcarchive; \
	mkdir -p build && \
	xcodebuild \
	-project $(XCODEPROJ) \
	-scheme $(RELEASE_SCHEME) \
	-configuration Release \
	-destination 'generic/platform=iOS' \
	-archivePath "$$ARCHIVE_DIR" \
	-allowProvisioningUpdates \
	archive && \
	open "$$ARCHIVE_DIR" && \
	echo "" && \
	echo "✓ Release archive at $$ARCHIVE_DIR" && \
	echo "  Organizer should be open. Click 'Distribute App' → 'Development' → pick your iPhone."
