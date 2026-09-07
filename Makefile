# Hylete Makefile
# Usage: make [target]

# Variables
APP_NAME=hylete
DB_FILE=strength_tracker.db
PORT=8080

# Tailwind binary selection based on OS
ifeq ($(OS),Linux)
TAILWIND_BIN=./tailwindcss-linux-x64
else
TAILWIND_BIN=./tailwindcss
endif

# Default target
.PHONY: help
help:
	@echo "Hylete - Available commands:"
	@echo ""
	@echo "  make build           - Build the application (includes CSS)"
	@echo "  make css-build       - Build CSS with Tailwind CLI"
	@echo "  make css-watch       - Watch and rebuild CSS on changes"
	@echo "  make download-tailwind - Download Tailwind CLI binaries"
	@echo "  make templ           - Regenerate Templ templates"
	@echo "  make run             - Build and run the application"
	@echo "  make dev             - Run with auto-restart (requires entr)"
	@echo "  make generate        - Regenerate sqlc queries"
	@echo "  make test            - Run tests"
	@echo "  make test-cover      - Run tests with coverage report"
	@echo ""

# Download Tailwind CLI binaries for current platform
.PHONY: download-tailwind
download-tailwind:
	@echo "Downloading Tailwind CLI binaries..."
	@if [ "$(OS)" != "Linux" ]; then \
		echo "Downloading macOS binary..."; \
		curl -sL "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.0/tailwindcss-macos-arm64" -o tailwindcss && chmod +x tailwindcss; \
	fi
	@echo "Downloading Linux binary..."; \
	curl -sL "https://github.com/tailwindlabs/tailwindcss/releases/download/v4.3.0/tailwindcss-linux-x64" -o tailwindcss-linux-x64 && chmod +x tailwindcss-linux-x64
	@echo "✓ Tailwind CLI binaries downloaded"

# Build the application
.PHONY: build
build: templ generate css-build
	@echo "Building $(APP_NAME)..."
	go build -o $(APP_NAME) ./cmd/main.go
	@echo "✓ Build complete: ./$(APP_NAME)"

# Build CSS with Tailwind CLI
.PHONY: css-build
css-build:
	@echo "Building CSS..."
	$(TAILWIND_BIN) -i ./styles/input.css -o ./public/css/styles.css --minify
	@echo "✓ CSS built: ./public/css/styles.css"

# Watch CSS in development
.PHONY: css-watch
css-watch:
	@echo "Watching CSS for changes..."
	$(TAILWIND_BIN) -i ./styles/input.css -o ./public/css/styles.css --watch

# Build and run
.PHONY: run
run: build
	@echo "Starting server on http://localhost:$(PORT)"
	./$(APP_NAME)

# Development mode with auto-restart (requires entr)
.PHONY: dev
dev:
	@if command -v entr >/dev/null 2>&1; then \
		echo "Watching for changes..."; \
		find . -name '*.go' -o -name '*.templ' -o -name 'styles/*.css' | entr -r $(MAKE) run; \
	else \
		echo "Error: 'entr' not installed. Install with: brew install entr (macOS) or apt-get install entr (Linux)"; \
		exit 1; \
	fi

# Regenerate Templ templates
.PHONY: templ
templ:
	@echo "Generating Templ templates..."
	@which templ > /dev/null || (echo "Installing templ..." && go install github.com/a-h/templ/cmd/templ@v0.3.1020)
	templ generate
	@echo "✓ Templates generated"

# Generate sqlc queries
.PHONY: generate
generate:
	@echo "Generating sqlc queries..."
	@which sqlc > /dev/null || (echo "Installing sqlc..." && go install github.com/sqlc-dev/sqlc/cmd/sqlc@latest)
	sqlc generate
	@echo "✓ sqlc queries generated"

# Run tests
.PHONY: test
test:
	@echo "Running tests..."
	go test -v ./...

# Run tests with coverage
.PHONY: test-cover
test-cover:
	@echo "Running tests with coverage..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "✓ Coverage report: coverage.html"
