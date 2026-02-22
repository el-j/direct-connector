#!/usr/bin/env bash
# build.sh — Cross-platform build script for MQTT Tunnel Manager
# Usage:
#   ./build.sh run           — Run locally on macOS (for development)
#   ./build.sh test          — Run all unit tests
#   ./build.sh mac           — Build a native macOS .app bundle
#   ./build.sh windows       — Cross-compile a Windows .exe (requires Docker)
#   ./build.sh all           — Run tests, then build for both platforms
set -euo pipefail

APP_NAME="direct-connector"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# ── Helpers ────────────────────────────────────────────────────────────────────

require() {
  if ! command -v "$1" &>/dev/null; then
    echo "❌  '$1' not found. $2"
    exit 1
  fi
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
  require fyne "Run: go install fyne.io/fyne/v2/cmd/fyne@latest"
  echo "▶  Building macOS .app bundle..."
  fyne package -os darwin -name "${APP_NAME}" -appVersion "1.0.0"
  echo "✅  macOS build complete: ${APP_NAME}.app"
}

cmd_windows() {
  require go          "Install Go from https://go.dev"
  require fyne-cross  "Run: go install github.com/fyne-io/fyne-cross@latest"
  require docker      "Install Docker Desktop from https://docker.com"
  if ! docker info &>/dev/null; then
    echo "❌  Docker is not running. Start Docker Desktop and try again."
    exit 1
  fi
  echo "▶  Cross-compiling Windows .exe (this may take a while on first run)..."
  fyne-cross windows -arch=amd64
  EXE_PATH="fyne-cross/bin/windows-amd64/${APP_NAME}.exe"
  if [[ -f "$EXE_PATH" ]]; then
    echo "✅  Windows build complete: ${EXE_PATH}"
  else
    echo "⚠️  Build finished but .exe not found at expected path. Check fyne-cross output."
  fi
}

cmd_all() {
  cmd_test
  cmd_mac
  cmd_windows
}

# ── Dispatch ────────────────────────────────────────────────────────────────────

case "${1:-}" in
  run)     cmd_run     ;;
  test)    cmd_test    ;;
  mac)     cmd_mac     ;;
  windows) cmd_windows ;;
  all)     cmd_all     ;;
  *)
    echo "Usage: $0 {run|test|mac|windows|all}"
    exit 1
    ;;
esac
