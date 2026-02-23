# app-wails — Direct Connector (Wails GUI)

This is the primary GUI module for Direct Connector.  
Stack: **Go 1.24 + [Wails v2](https://wails.io) + Vue 3 + TypeScript + Tailwind CSS**.

See the [root README](../README.md) for the full feature overview, prerequisites, and distribution build commands.

---

## Module

```
module: direct-connector
go:     1.24
```

---

## Development

```bash
# Hot-reload dev server (opens a native window + localhost:34115 for browser devtools)
wails dev

# Or from the repo root:
make run
```

Changes to `.go` files restart the backend; changes to `frontend/src/` are hot-reloaded by Vite without a restart.

---

## Project structure

```
app-wails/
├── app.go              ← All Wails-bound methods (LoadConfig, TunnelStart, P2PGenerateOffer, …)
├── main.go             ← Wails entry point (window config, lifecycle hooks)
├── tray_darwin.go/.m   ← macOS menu-bar status item via Obj-C/CGO (no delegate conflict)
├── tray_windows.go     ← Windows tray via energye/systray
├── tray_other.go       ← Linux stub
├── tray.go             ← Shared tray helpers (open, help, quit)
├── wails.json          ← Wails project config (window size, app name, …)
├── frontend/
│   ├── src/
│   │   ├── App.vue              ← Tab bar + root layout
│   │   └── components/
│   │       ├── SshTab.vue       ← SSH tunnel UI + key-install wizard modal
│   │       └── P2pTab.vue       ← P2P tunnel UI + embedded relay panel
│   ├── wailsjs/                 ← Auto-generated Go→TS bindings — do NOT edit
│   ├── tailwind.config.js
│   ├── vite.config.ts
│   └── tsconfig.json
└── internal/
    ├── config/     ← Persistent JSON config (OS config dir / MQTTTunnelManager/)
    ├── keygen/     ← Ed25519 keypair generation and loading
    ├── p2p/        ← WebRTC P2P engine (DataChannels, ICE, TURN, TCP mux)
    ├── relay/      ← Embedded TURN server (pion/turn v2; UDP + TCP)
    ├── sshsetup/   ← Pure-Go interactive SSH public-key installer
    └── tunnel/     ← SSH tunnel manager (os/exec ssh, auto-reconnect)
```

---

## Wails binding pattern

All exported methods on `App` in `app.go` become async TypeScript functions automatically.  
Frontend types are defined as Go structs in `app.go` (not in `internal/`) so Wails generates clean camelCase TS interfaces in `frontend/wailsjs/go/`.

Events flow from Go → Vue via `runtime.EventsEmit`:

| Event | Payload | Emitted by |
|---|---|---|
| `tunnel:status` | `string` | `tunnel.Manager.OnStatus` |
| `tunnel:log` | `string` | `tunnel.Manager.OnLog` |
| `p2p:status` | `string` | P2P session status callback |
| `p2p:log` | `string` | P2P session log callback |
| `relay:log` | `string` | `relay.Relay.OnLog` |
| `setup:prompt` | `SetupPrompt` | `sshsetup.Installer` prompts |
| `setup:log` | `string` | `sshsetup.Installer` log |
| `setup:done` | `SetupResult` | `sshsetup.Installer` completion |

---

## Building

```bash
# macOS arm64 bundle → ../dist/direct-connector-mac.app
wails build -platform darwin/arm64

# macOS universal (requires Xcode)
wails build -platform darwin/universal

# Windows amd64 (requires mingw-w64)
CC=x86_64-w64-mingw32-gcc wails build -platform windows/amd64

# Or use the root Makefile targets:
make wails-mac
make wails-windows
make build-all
```

---

## Tests

```bash
go test -race -count=1 ./...
# Or from repo root:
make wails-test
```

Tests cover `internal/config`, `internal/keygen`, `internal/p2p`, `internal/relay`, `internal/sshsetup`, and `internal/tunnel`.
