## Direct Connector — root Makefile

ROOT_DIR    := $(patsubst %/,%,$(dir $(abspath $(lastword $(MAKEFILE_LIST)))))
PROJECT_DIR := $(ROOT_DIR)/app
WAILS_DIR   := $(ROOT_DIR)/app-wails
DIST_DIR    := $(ROOT_DIR)/dist

.PHONY: run test clean \
        wails-dev wails-test \
        wails-mac wails-linux wails-windows build-all

# ── Dev / test ────────────────────────────────────────────────────────────────

## Run the Wails dev server with hot-reload (opens a native window)
run:
	cd "$(WAILS_DIR)" && wails dev

## Run all unit tests (Wails backend + original internal packages)
test:
	cd "$(WAILS_DIR)"   && go test -race -count=1 ./...
	cd "$(PROJECT_DIR)" && go test -race -count=1 ./...

## Alias: run tests on Wails backend only
wails-test:
	cd "$(WAILS_DIR)" && go test -race -count=1 ./...

## Remove dist/ and all Wails intermediate build output
clean:
	rm -rf "$(DIST_DIR)" "$(WAILS_DIR)/build"

# ── Wails production builds ───────────────────────────────────────────────────
# All outputs land in dist/ at the project root.
# • wails-mac     — always works natively (universal = Intel + Apple Silicon)
# • wails-linux   — requires musl cross-compiler:
#                   brew install FiloSottile/musl-cross/musl-cross
# • wails-windows — requires mingw-w64 cross-compiler:
#                   brew install mingw-w64
# • build-all     — runs all three; Linux/Windows steps skip gracefully if the
#                   required cross-compiler is not installed.

## Build macOS arm64 bundle  → dist/direct-connector-mac.app
## (For a universal Intel+ARM build, install Xcode and run:
##  wails build -platform darwin/universal)
wails-mac:
	@mkdir -p "$(DIST_DIR)"
	@rm -rf "$(WAILS_DIR)/build"
	cd "$(WAILS_DIR)" && wails build -platform darwin/arm64 -clean
	@cp -r "$(WAILS_DIR)/build/bin/direct-connector.app" \
	       "$(DIST_DIR)/direct-connector-mac.app"
	@rm -rf "$(WAILS_DIR)/build"
	@echo "✓  macOS  →  $(DIST_DIR)/direct-connector-mac.app"

## Build Linux amd64 binary  → dist/direct-connector-linux-amd64
## NOTE: Wails v2 does not support Linux cross-compilation from macOS.
##       Build on a Linux machine or in Docker:
##         docker run --rm -v $(ROOT_DIR):/src -w /src/app-wails \
##           ghcr.io/wailsapp/wails:latest wails build -platform linux/amd64
wails-linux:
	@echo "⚠  Linux build skipped — Wails v2 does not support cross-compilation from macOS."
	@echo "   Build natively on Linux, or use the Docker command shown in the Makefile comment."

## Build Windows amd64 .exe  → dist/direct-connector-windows.exe
wails-windows:
	@mkdir -p "$(DIST_DIR)"
	@if ! command -v x86_64-w64-mingw32-gcc >/dev/null 2>&1; then \
	    echo "⚠  Windows build skipped — mingw-w64 cross-compiler not found."; \
	    echo "   Install with:  brew install mingw-w64"; \
	else \
	    rm -rf "$(WAILS_DIR)/build/bin" && \
	    cd "$(WAILS_DIR)" && \
	        CC=x86_64-w64-mingw32-gcc \
	        wails build -platform windows/amd64 -skipbindings -clean && \
	    cp "$(WAILS_DIR)/build/bin/direct-connector.exe" \
	       "$(DIST_DIR)/direct-connector-windows.exe" && \
	    rm -rf "$(WAILS_DIR)/build" && \
	    echo "✓  Windows →  $(DIST_DIR)/direct-connector-windows.exe"; \
	fi

## Build for all platforms (skips Linux/Windows gracefully if tools missing)
build-all: wails-mac wails-linux wails-windows
	@echo ""
	@echo "Build complete. Artefacts in $(DIST_DIR):"
	@ls -lh "$(DIST_DIR)" 2>/dev/null || true
