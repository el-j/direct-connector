#!/usr/bin/env bash
# build.sh — Cross-platform build script for Direct Connector
# Usage:
#   ./build.sh run           — Run full GUI app locally on macOS
#   ./build.sh test          — Run all unit tests
#   ./build.sh mac           — Build a native macOS .app bundle
#   ./build.sh windows       — Cross-compile a Windows .exe (requires Docker)
#   ./build.sh cli           — Build a headless CLI binary for the current OS
#   ./build.sh docker        — Build the Docker image (headless, static binary)
#   ./build.sh all           — Run tests, then build for both platforms + CLI
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
  require fyne "Run: go install fyne.io/tools/cmd/fyne@latest"
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

cmd_cli() {
  require go "Install Go from https://go.dev"
  echo "▶  Building headless CLI binary (no GUI, no CGO, static)..."
  CGO_ENABLED=0 go build -tags nofyne -ldflags="-s -w" -o "${APP_NAME}-cli" .
  echo "✅  CLI binary: ${APP_NAME}-cli"
  echo "    Usage: ./${APP_NAME}-cli --mode <ssh|p2p-consumer|p2p-provider|keygen>"
}

cmd_docker() {
  require docker "Install Docker Desktop from https://docker.com"
  if ! docker info &>/dev/null; then
    echo "❌  Docker is not running. Start Docker Desktop and try again."
    exit 1
  fi
  echo "▶  Building Docker image: ${APP_NAME}:latest..."
  docker build -t "${APP_NAME}:latest" .
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
  cmd_windows
  cmd_cli
}

# ── Dispatch ────────────────────────────────────────────────────────────────────

case "${1:-}" in
  run)     cmd_run     ;;
  test)    cmd_test    ;;
  mac)     cmd_mac     ;;
  windows) cmd_windows ;;
  cli)     cmd_cli     ;;
  docker)  cmd_docker  ;;
  all)     cmd_all     ;;
  *)
    echo "Usage: $0 {run|test|mac|windows|cli|docker|all}"
    exit 1
    ;;
esac
