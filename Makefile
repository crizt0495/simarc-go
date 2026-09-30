# SIMARC — build and packaging
#
# The web build is unchanged:  make run / make build
# The desktop build lives in desktop/ and produces native installers.
#
# Wails apps link against the platform webview through cgo, so a binary can
# only be produced on the OS it will run on. Use `make desktop` to build the
# host's binary, or `make desktop-dist` for the full installer where the host
# tooling supports it. For all three platforms at once, push a tag and let
# .github/workflows/desktop-release.yml build them on their own runners.

SHELL := /bin/bash
.DEFAULT_GOAL := help

# Wails refuses to start unless the `production` tag is present, and on Linux
# the webkit2_41 tag selects WebKitGTK 4.1 (4.0 is EOL and not built here).
# WAILS_TAGS is overridden per-OS by the recipe that needs it.
WAILS_TAGS := desktop,webkit2_41
GO_TAGS    := desktop webkit2_41 production

BIN        := desktop/build/bin
BINARY     := SIMARC
GO         ?= go
WAILS      ?= $(shell $(GO) env GOPATH)/bin/wails

.PHONY: help
help: ## Show this help
	@echo "SIMARC — available targets"
	@echo
	@grep -hE '^[a-zA-Z_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-18s\033[0m %s\n", $$1, $$2}'
	@echo
	@echo "  desktop-builds must run on the target OS (cgo + platform webview)."

# ── Web build (unchanged) ────────────────────────────────────────────────────

.PHONY: run
run: ## Run the web server locally
	$(GO) run ./cmd/server

.PHONY: build
build: ## Build the web server binary
	$(GO) build -o server ./cmd/server

.PHONY: test
test: ## Run the Go test suite (needs MySQL from .env)
	$(GO) test ./...

.PHONY: assets
assets: ## Rebuild the minified CSS that the pages actually load
	npm run css:minify

.PHONY: assets-check
assets-check: ## Fail if a .min.css is out of sync with its source
	npm run css:check

.PHONY: test-desktop
test-desktop: ## Test the desktop shell contract (needs MySQL from .env)
	cd desktop && $(GO) test -tags "$(GO_TAGS)" ./...

.PHONY: vet
vet: ## Vet the web code
	$(GO) vet ./...

.PHONY: fmt
fmt: ## Format the Go code
	$(GO) fmt ./...

# ── Desktop prerequisites ────────────────────────────────────────────────────

.PHONY: desktop-tools
desktop-tools: ## Install the Wails CLI and its Linux system dependencies
	$(GO) install github.com/wailsapp/wails/v2/cmd/wails@v2.16.0
	@echo
	@echo "Linux also needs the webview development headers (Debian/Ubuntu):"
	@echo "  sudo apt install libwebkit2gtk-4.1-dev build-essential \\"
	@echo "      libgtk-3-dev pkg-config"

.PHONY: desktop-icons
desktop-icons: ## Regenerate the desktop app icons for all platforms
	python3 scripts/make-desktop-icons.py

# ── Desktop build ────────────────────────────────────────────────────────────

.PHONY: desktop
desktop: desktop-tools ## Build the desktop app for this OS (binary in desktop/build/bin)
	cd desktop && $(WAILS) build -tags "$(WAILS_TAGS)" -skipbindings

.PHONY: desktop-run
desktop-run: desktop ## Build and launch the desktop app
	cd desktop && ./build/bin/$(BINARY)

# Plain go build, for a quick binary with no installer step. The `production`
# tag is mandatory: without it wails.Run returns an error and exits.
.PHONY: desktop-binary
desktop-binary: ## Build just the desktop binary with go build (no installer)
	$(GO) build -tags "$(GO_TAGS)" -ldflags "-w -s" -o $(BINARY) ./desktop

# ── Desktop installers ───────────────────────────────────────────────────────

.PHONY: desktop-dist
desktop-dist: desktop-tools ## Build the native installer for this OS
ifeq ($(shell uname -s),Darwin)
	cd desktop && $(WAILS) build -tags "$(WAILS_TAGS)" -skipbindings -package
	@echo "Installer: desktop/build/bin/SIMARC.app and SIMARC.dmg"
else ifeq ($(OS),Windows_NT)
	cd desktop && $(WAILS) build -tags "desktop" -skipbindings -nsis
	@echo "Installer: desktop/build/bin/SIMARC-amd64-setup.exe"
else
	cd desktop && $(WAILS) build -tags "$(WAILS_TAGS)" -skipbindings -nsis -platform linux/amd64
	@echo "Binary: desktop/build/bin/SIMARC"
	@echo "For .deb / AppImage, install nfpm and appimagetool and run scripts/package-linux.sh"
endif

# ── Install on this machine ──────────────────────────────────────────────────

.PHONY: desktop-install
desktop-install: desktop ## Build and install the desktop app for the current user (Linux)
	install -Dm755 $(BIN)/$(BINARY) ~/.local/bin/$(BINARY)
	install -Dm644 .desktop/simarc.desktop ~/.local/share/applications/simarc.desktop
	install -Dm644 desktop/build/appicon.png ~/.local/share/icons/hicolor/512x512/apps/simarc.png
	update-desktop-database ~/.local/share/applications 2>/dev/null || true
	@echo
	@echo "Installed. Start it from the application menu, or run: $(HOME)/.local/bin/$(BINARY)"

.PHONY: desktop-uninstall
desktop-uninstall: ## Remove the user-level desktop install
	rm -f ~/.local/bin/$(BINARY) \
	      ~/.local/share/applications/simarc.desktop \
	      ~/.local/share/icons/hicolor/512x512/apps/simarc.png
	@echo "Removed."

# ── Housekeeping ─────────────────────────────────────────────────────────────

.PHONY: clean
clean: ## Remove build output
	rm -rf $(BIN) server
