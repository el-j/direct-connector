# Direct Connector — Copilot Instructions

## Architecture overview

Two parallel GUI implementations share the same backend packages:

| Layer | Path | Stack |
|---|---|---|
| **Active GUI** | `app-wails/` | Go (Wails v2) + Vue 3 + TypeScript + Tailwind |
| **Legacy GUI** | `app/` | Go + Fyne v2 (cross-compiles to native widgets) |
| **Shared logic** | `app-wails/internal/` (and `app/internal/`) | Pure Go, no GUI dependency |

`app-wails/` is the current primary target; `app/` is kept for the headless CLI and Docker image. Both share structurally identical `internal/` packages (config, keygen, p2p, tunnel) but are separate Go modules (`direct-connector` vs `mqtt-tunnel`).

## Two Go modules

- `app/go.mod` — module `mqtt-tunnel`; owns the Fyne GUI and the CLI/Docker binary.
- `app-wails/go.mod` — module `direct-connector`; owns the Wails GUI backend.

Never import across modules. Edits to shared logic (e.g. `tunnel.Manager`) must be mirrored in both `app/internal/` and `app-wails/internal/`.

## Critical build commands

All common workflows go through the root `Makefile`:

```bash
make run          # Wails dev server with hot-reload (primary dev workflow)
make test         # Runs tests in BOTH modules: app-wails + app
make wails-mac    # → dist/direct-connector-mac.app  (arm64)
make wails-windows # → dist/direct-connector-windows.exe (requires mingw-w64)
make clean        # Removes dist/ and app-wails/build/
```

For the legacy Fyne / CLI / Docker builds, use `app/build.sh` directly:

```bash
cd app && ./build.sh test        # Fyne unit tests
cd app && ./build.sh cli         # headless binary (nofyne tag, CGO_ENABLED=0)
cd app && ./build.sh docker      # Docker image (static, no GUI)
```

## Build tags (app/ module only)

- **`!nofyne`** (default) — GUI binary; uses `main.go`, `ssh_tab.go`, `p2p_tab.go`.
- **`nofyne`** — headless CLI/Docker binary; uses `main_nofyne.go` only; no OpenGL/X11.

CLI mode is detected at runtime via `IsCLIMode()` (presence of `--mode` flag or `DC_MODE` env var).

## Wails binding pattern

All exported methods on `App` (`app-wails/app.go`) become async TypeScript functions. Frontend types are defined in `app.go` (not in `internal/`) so Wails generates clean camelCase TS interfaces. Back-end events use `runtime.EventsEmit(ctx, "tunnel:status", …)` and are consumed in Vue components with Wails JS runtime listeners.

## Internal package responsibilities

- `internal/config` — JSON config at `os.UserConfigDir()/MQTTTunnelManager/tunnel_config.json`; uses `AppDirName = "MQTTTunnelManager"` for all app-managed files (SSH keys, config).
- `internal/tunnel` — wraps `os/exec` SSH process with `tunnel.Manager`; emits `tunnel.Status` via `OnStatus`/`OnLog` callbacks; handles automatic reconnect.
- `internal/p2p` — WebRTC via `github.com/pion/webrtc/v3`; SDP blobs are base64-encoded JSON for copy-paste exchange; STUN used only for NAT discovery (no relay traffic).
- `internal/keygen` — Ed25519 keypair generation/loading stored in the config dir.

## Key conventions

- **Port 443 for SSH** — default and recommended; bypasses most firewalls/corporate proxies.
- **`src/` directory is intentionally absent** — Go module at module root is idiomatic; `app/` and `app-wails/` are the correct entry points.
- **CLI flags have `DC_<FLAG>` env var equivalents** — enables pure-env Docker configuration (see `cli.go`).
- **Vue components** (`SshTab.vue`, `P2pTab.vue`) use `v-show` not `v-if` to keep both tabs mounted.
- **Tailwind dark theme** — `bg-gray-900` matches the Wails `BackgroundColour` to prevent white flash on load.
- **`UseWSLSsh`** — Windows-specific flag that invokes `wsl.exe ssh` so SSH runs inside the WSL network namespace (needed when Docker/MQTT is inside WSL).
