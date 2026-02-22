#!/usr/bin/env bash
# build.sh — Cross-platform build script for Direct Connector
# Usage:
#   ./build.sh run           — Run full GUI app locally on macOS
#   ./build.sh test          — Run all unit tests
#   ./build.sh mac           — Build a native macOS .app bundle        → dist/
#   ./build.sh linux         — Build a headless Linux binary (no CGO)  → dist/
#   ./build.sh windows       — Cross-compile a Windows .exe (requires Docker + fyne-cross)  → dist/
#   ./build.sh cli           — Build a headless CLI binary for the current OS  → dist/
#   ./build.sh docker        — Build the Docker image (headless, static binary)
#   ./build.sh all           — Run tests + build mac/linux/cli (skips windows/docker if unavailable)
#   ./build.sh clean         — Remove the dist/ directory
set -euo pipefail

APP_NAME="direct-connector"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
DIST_DIR="${PROJECT_ROOT}/dist"
cd "$SCRIPT_DIR"

# ── Helpers ────────────────────────────────────────────────────────────────────

require() {
  if ! command -v "$1" &>/dev/null; then
    echo "❌  '$1' not found. $2"
    exit 1
  fi
}

require_soft() {
  # Like require() but prints a warning and returns 1 instead of exiting.
  if ! command -v "$1" &>/dev/null; then
    echo "⚠️  '$1' not found — skipping. $2"
    return 1
  fi
  return 0
}

mkdir_dist() {
  mkdir -p "${DIST_DIR}"
}

# ── Commands ────────────────────────────────────────────────────────────────────

cmd_run() {
  require go "Install Go from https://go.dev"
  echo "▶  Running ${APP_NAME} locally..."
  go run .
}

cmd_test() {
  require go "Install Go from https://go.dev"
  echo "▶  Running unit tests..."
  go test -v -race -count=1 ./...
  echo "✅  All tests passed."
}

cmd_mac() {
  require go   "Install Go from https://go.dev"
  require fyne "Run: go install fyne.io/tools/cmd/fyne@latest"
  mkdir_dist
  echo "▶  Building macOS .app bundle → dist/..."
  # fyne package always outputs to the current directory; we move it to dist/ after.
  fyne package --target darwin --name "${APP_NAME}" --app-version "1.0.0"
  mv "${APP_NAME}.app" "${DIST_DIR}/${APP_NAME}.app"
  echo "✅  macOS build complete: dist/${APP_NAME}.app"
}

cmd_linux() {
  require go "Install Go from https://go.dev"
  mkdir_dist
  echo "▶  Building headless Linux binary (no GUI, no CGO) → dist/..."
  GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
    go build -tags nofyne -ldflags="-s -w" -o "${DIST_DIR}/${APP_NAME}-linux-amd64" .
  echo "✅  Linux CLI binary: dist/${APP_NAME}-linux-amd64"
}

cmd_windows() {
  require go "Install Go from https://go.dev"
  require_soft fyne-cross "Run: go install github.com/fyne-io/fyne-cross@latest" || exit 1
  require_soft docker     "Install Docker Desktop from https://docker.com"       || exit 1
  if ! docker info &>/dev/null; then
    echo "❌  Docker is not running. Start Docker Desktop and try again."
    exit 1
  fi
  mkdir_dist
  echo "▶  Cross-compiling Windows .exe (this may take a while on first run)..."
  # fyne-cross outputs to fyne-cross/bin/windows-amd64/ automatically; no -output flag.
  fyne-cross windows -arch=amd64 -name "${APP_NAME}"
  EXE_PATH="fyne-cross/bin/windows-amd64/${APP_NAME}.exe"
  if [[ -f "$EXE_PATH" ]]; then
    cp "$EXE_PATH" "${DIST_DIR}/${APP_NAME}.exe"
    echo "✅  Windows build complete: dist/${APP_NAME}.exe"
  else
    echo "⚠️  fyne-cross finished but .exe not found at expected path. Check fyne-cross output."
  fi
}

cmd_cli() {
  require go "Install Go from https://go.dev"
  mkdir_dist
  # Determine output extension for the current OS.
  local EXT=""
  if [[ "$(go env GOOS)" == "windows" ]]; then EXT=".exe"; fi
  echo "▶  Building headless CLI binary (no GUI, no CGO, static) → dist/..."
  CGO_ENABLED=0 go build -tags nofyne -ldflags="-s -w" \
    -o "${DIST_DIR}/${APP_NAME}-cli${EXT}" .
  echo "✅  CLI binary: dist/${APP_NAME}-cli${EXT}"
  echo "    Usage: ./dist/${APP_NAME}-cli${EXT} --mode <ssh|p2p-consumer|p2p-provider|keygen>"
}

cmd_docker() {
  require docker "Install Docker Desktop from https://docker.com"
  if ! docker info &>/dev/null; then
    echo "❌  Docker is not running. Start Docker Desktop and try again."
    exit 1
  fi
  echo "▶  Building Docker image: ${APP_NAME}:latest..."
  # Dockerfile lives at the project root; build context is the app/ source dir.
  docker build -f "${PROJECT_ROOT}/Dockerfile" -t "${APP_NAME}:latest" "${SCRIPT_DIR}"
  echo "✅  Docker image built: ${APP_NAME}:latest"
  echo ""
  echo "Quick start (SSH tunnel):"
  echo "  docker run --rm \\"
  echo "    -e DC_MODE=ssh \\"
  echo "    -e DC_HOST=jump.example.com \\"
  echo "    -e DC_PORT=8080 \\"
  echo "    -e DC_USER=admin \\"
  echo "    -e DC_PORTS=1883,3391 \\"
  echo "    -v dc-config:/config \\"
  echo "    ${APP_NAME}:latest"
}

cmd_all() {
  cmd_test
  cmd_mac
  cmd_linux
  cmd_cli

  # Windows requires Docker + fyne-cross; skip gracefully when unavailable.
  if command -v fyne-cross &>/dev/null && command -v docker &>/dev/null && docker info &>/dev/null 2>&1; then
    cmd_windows
  else
    echo "⚠️  Skipping Windows build (fyne-cross / Docker not available)."
  fi

  # Docker image build is optional.
  if command -v docker &>/dev/null && docker info &>/dev/null 2>&1; then
    cmd_docker
  else
    echo "⚠️  Skipping Docker image build (Docker not available)."
  fi
}

cmd_clean() {
  echo "▶  Removing dist/..."
  rm -rf "${DIST_DIR}"
  echo "✅  dist/ removed."
}

# ── Dispatch ────────────────────────────────────────────────────────────────────

case "${1:-}" in
  run)     cmd_run     ;;
  test)    cmd_test    ;;
  mac)     cmd_mac     ;;
  linux)   cmd_linux   ;;
  windows) cmd_windows ;;
  cli)     cmd_cli     ;;
  docker)  cmd_docker  ;;
  all)     cmd_all     ;;
  clean)   cmd_clean   ;;
  *)
    echo "Usage: $0 {run|test|mac|linux|windows|cli|docker|all|clean}"
    exit 1
    ;;
esac
