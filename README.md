# Direct Connector

A cross-platform desktop app for creating secure tunnels between two machines — with **no cloud dependency and no subscription**.

The primary GUI is built with [Go](https://go.dev) + [Wails v2](https://wails.io) (Vue 3 + TypeScript + Tailwind CSS), packaged as a native `.app` on macOS and a standalone `.exe` on Windows.  
The same codebase also compiles to a fully **headless CLI binary** and a **Docker image** (no GUI, no display server required).

---

## What it does

The app provides three independent tunnel methods in a single tabbed UI:

### SSH Tunnel tab

Establishes a persistent, self-healing **reverse SSH tunnel** over an SSH jump host you already control.  
Port-forwards are kept alive with automatic reconnect and keep-alive probes.

| Setting | What it does |
|---|---|
| SSH Host / Port | Your jump server's address. **Use port 443** — routers and corporate firewalls treat it as HTTPS and almost never block it. |
| SSH User | Login username on the jump host |
| Forward Ports | Comma-separated list of ports to forward (`1883,3391`) |
| Forward Mode | `local` (expose remote port locally) or `remote` (expose local port on the server) |
| SSH Key | Path to private key; defaults to the app-managed `id_ed25519` in the config dir |
| IPv6 | Force IPv6 socket (`-6`) — use when on a DS-Lite / IPv6-only network |
| Verbose | Pass `-v` to the SSH process for detailed logging |
| Use WSL SSH | Windows only: invoke `wsl.exe ssh` so SSH runs inside the WSL network namespace (needed when services are inside WSL/Docker) |

#### One-click key installer

The **"Install Key on Server"** button runs a pure-Go SSH setup wizard (`internal/sshsetup`):
1. Generates an app-managed Ed25519 keypair if one does not exist yet.
2. Dials the server via `x/crypto/ssh` and installs the public key into `~/.ssh/authorized_keys`.
3. Interactive prompts (host fingerprint confirmation, password) appear as modal dialogs in the UI — no terminal required.

After installation, **"Check Key Auth"** does a short key-only dial to confirm the server accepts the key.

### P2P Tunnel tab

Creates a **100 % serverless, direct peer-to-peer encrypted tunnel** using WebRTC DataChannels.  
You choose your role, exchange a single blob of text with the other machine (via any channel — chat, email, even a sticky note), and the tunnel starts.

No mandatory signalling server. No relay. No third-party service.  
The only external contact by default is a STUN lookup to discover your public IP — after that, all data flows directly and is encrypted with DTLS 1.2.

#### Roles

| Role | What it does |
|---|---|
| **Consumer (Initiator)** | Opens local TCP listeners on the specified ports. Traffic is tunnelled *to* the Provider. |
| **Provider (Responder)** | Bridges incoming DataChannels to local services (e.g. Mosquitto on port 1883). |

#### Connection flow

```
Consumer machine                         Provider machine
────────────────                         ────────────────
1. Enter ports
   Click "Generate Offer"
   ── copies a base64 text blob ──────▶  Paste into Offer box
                                         Click "Accept & Generate Answer"
                   ◀──  base64 text ──  Copy the Answer
2. Paste Answer
   Click "Connect"

      ╔════════════════════════════════════════════════════╗
      ║  Direct DTLS-encrypted peer-to-peer channel        ║
      ║  Zero bytes through any server from this point on  ║
      ╚════════════════════════════════════════════════════╝
```

#### Advanced ICE options (optional)

| Option | What it does |
|---|---|
| TURN Servers | Add one or more TURN relay servers to work through symmetric NATs / CGNAT |
| TCP Mux Port | Bind a passive ICE-TCP listener (port 443 recommended for maximum firewall penetration) |
| Gather Timeout | Override the default 30-second ICE candidate gathering window |

### Relay tab

Runs an **embedded TURN server** directly on this machine so you can provide your own relay without any external service.

- Discovers the machine's public IP via STUN on startup.
- Listens on a configurable UDP port (and attempts TCP on the same port for firewall traversal).
- Generates time-limited HMAC credentials for the P2P tab automatically and copies them in one click.
- Logs relay activity in real time.

---

## Project layout

```
directSshConnector/
├── README.md
├── Makefile                        ← root entry point (all common workflows)
├── Dockerfile                      ← two-stage Docker build (context: app/)
├── docker-compose.yml              ← ready-to-use service examples
├── dist/                           ← built artefacts (git-ignored)
│   ├── direct-connector-mac.app    ← macOS GUI bundle
│   ├── direct-connector-windows.exe← Windows GUI executable
│   └── direct-connector-linux-amd64← headless Linux CLI / server
│
├── app-wails/                      ← Go module: direct-connector  ← PRIMARY GUI
│   ├── app.go                      ← Wails backend; all exported methods → TS bindings
│   ├── main.go                     ← Wails entry point
│   ├── tray_darwin.go / .m         ← macOS tray (custom Obj-C/CGO shim, avoids delegate conflicts)
│   ├── tray_windows.go             ← Windows tray (energye/systray)
│   ├── tray_other.go               ← stub for Linux
│   ├── go.mod / go.sum             ← module: direct-connector (Go 1.24)
│   ├── wails.json
│   ├── frontend/                   ← Vue 3 + TypeScript + Tailwind CSS
│   │   ├── src/
│   │   │   ├── App.vue
│   │   │   └── components/
│   │   │       ├── SshTab.vue      ← SSH tunnel UI + interactive key-install wizard
│   │   │       └── P2pTab.vue      ← P2P tunnel + embedded relay UI
│   │   └── wailsjs/                ← auto-generated Go→TS bindings (do not edit)
│   └── internal/
│       ├── config/                 ← persistent JSON config (JSON at OS config dir)
│       ├── keygen/                 ← Ed25519 keypair generation / loading
│       ├── p2p/                    ← WebRTC P2P engine + SessionConfig (TURN, TCP mux)
│       ├── relay/                  ← embedded TURN server (pion/turn v2)
│       ├── sshsetup/               ← pure-Go one-time SSH public key installer
│       └── tunnel/                 ← SSH tunnel manager with auto-reconnect
│
└── app/                            ← Go module: mqtt-tunnel  ← LEGACY GUI + CLI/Docker
    ├── main.go                     ← Fyne GUI entry point  (build tag: !nofyne)
    ├── main_nofyne.go              ← CLI entry point       (build tag: nofyne)
    ├── cli.go                      ← headless CLI — all modes, flag/env parsing
    ├── ssh_tab.go                  ← Fyne SSH tab UI
    ├── p2p_tab.go                  ← Fyne P2P tab UI
    ├── build.sh                    ← build / test / cross-compile helper
    ├── go.mod / go.sum             ← module: mqtt-tunnel
    └── internal/
        ├── config/                 ← persistent JSON config (same schema)
        ├── keygen/                 ← Ed25519 keypair management
        ├── p2p/                    ← WebRTC P2P engine
        └── tunnel/                 ← SSH tunnel manager
```

> **Two separate Go modules.** `app-wails/` (module `direct-connector`) and `app/` (module `mqtt-tunnel`) are independent. Never import across modules. Logic shared between both lives in structurally identical `internal/` packages that must be kept in sync manually.

---

## Prerequisites

### Wails GUI (`app-wails/`)

| Tool | Version | Install |
|---|---|---|
| Go | ≥ 1.24 | `brew install go` or [go.dev](https://go.dev/dl/) |
| Node.js + npm | ≥ 20 | `brew install node` |
| Wails CLI | v2 latest | `go install github.com/wailsapp/wails/v2/cmd/wails@latest` |
| mingw-w64 | any | `brew install mingw-w64` — Windows cross-compile only |

> **macOS:** Xcode Command Line Tools must be installed (`xcode-select --install`).

### Legacy Fyne GUI / CLI / Docker (`app/`)

| Tool | Version | Install |
|---|---|---|
| Go | ≥ 1.22 | as above |
| Fyne CLI | latest | `go install fyne.io/tools/cmd/fyne@latest` |
| fyne-cross | latest | `go install github.com/fyne-io/fyne-cross@latest` — Windows cross-compile only |
| Docker Desktop | any | [docker.com](https://www.docker.com/products/docker-desktop/) — Windows cross-compile or image builds |

---

## Development

```bash
# Run the Wails GUI with hot-reload (primary dev workflow)
make run
# Equivalent:
cd app-wails && wails dev
```

Both the Vue frontend and Go backend hot-reload on save. The embedded dev server also runs on `http://localhost:34115` for browser devtools access.

### Run the test suite

```bash
# Tests in both modules (Wails backend + legacy app)
make test

# Wails backend tests only
make wails-test

# Legacy app tests only
cd app && ./build.sh test
```

---

## Building for distribution

All built artefacts land in `dist/` at the project root.

### Quick reference

| Command | Output |
|---|---|
| `make wails-mac` | `dist/direct-connector-mac.app` (arm64) |
| `make wails-windows` | `dist/direct-connector-windows.exe` (requires mingw-w64) |
| `make build-all` | macOS + Linux + Windows (Linux/Windows skip gracefully if tools missing) |
| `make clean` | removes `dist/` and `app-wails/build/` |

### macOS `.app` bundle

```bash
make wails-mac
```

Produces `dist/direct-connector-mac.app`. Double-click to launch, or drag to `/Applications`.  
For a universal Intel + Apple Silicon build, install Xcode and run:

```bash
cd app-wails && wails build -platform darwin/universal
```

### Windows `.exe` (cross-compiled from macOS)

Requires `mingw-w64`:

```bash
brew install mingw-w64
make wails-windows
```

Output: `dist/direct-connector-windows.exe`. Copy to a Windows machine — no installer, no runtime, no dependencies.

### Linux

Wails v2 does not support Linux cross-compilation from macOS. Build natively on Linux or via Docker:

```bash
docker run --rm -v $(pwd):/src -w /src/app-wails \
  ghcr.io/wailsapp/wails:latest wails build -platform linux/amd64
```

---

## CLI mode & Docker

The `app/` module compiles to a fully headless CLI binary — no display server, no GUI libraries.

### Build

```bash
cd app && ./build.sh cli
# Manual equivalent:
cd app && CGO_ENABLED=0 go build -tags nofyne -ldflags="-s -w" \
  -o ../dist/direct-connector-cli .
```

### CLI modes

| Mode | Description |
|---|---|
| `ssh` | Persistent reverse-SSH tunnel |
| `p2p-consumer` | WebRTC consumer: opens local ports and tunnels to provider |
| `p2p-provider` | WebRTC provider: bridges DataChannels to local services |
| `keygen` | Generate Ed25519 keypair, print public key |

Every flag also reads from a `DC_<FLAG>` environment variable:

```bash
# SSH tunnel
./dist/direct-connector-cli --mode ssh \
  --host jump.example.com --port 443 --user admin \
  --ports 1883,3391

# P2P consumer (interactive stdin/stdout)
./dist/direct-connector-cli --mode p2p-consumer --ports 1883,8080

# P2P provider (file-based — for Docker shared volumes)
./dist/direct-connector-cli --mode p2p-provider \
  --offer-in /data/offer.txt --answer-out /data/answer.txt

# Generate key, print authorized_keys line
./dist/direct-connector-cli --mode keygen
```

### Docker

```bash
# Build image
make docker
# or:
docker build -f Dockerfile -t direct-connector:latest app/

# Run SSH tunnel
docker run --rm \
  -e DC_MODE=ssh \
  -e DC_HOST=jump.example.com \
  -e DC_PORT=443 \
  -e DC_USER=admin \
  -e DC_PORTS=1883,3391 \
  -v dc-config:/config \
  direct-connector:latest
```

The image is built in two stages and runs `FROM scratch` — just the static binary + CA certificates. Final image is ~10 MB.

### Docker Compose

```bash
# Start SSH tunnel service
docker compose up ssh-tunnel

# Start P2P consumer + provider (share SDPs via a named volume)
docker compose --profile p2p up
```

Both P2P containers share a volume at `/data`. The consumer writes `offer.txt`, the provider reads it and writes `answer.txt`, and the consumer polls until the answer appears — no manual SDP copying needed.

---

## Config storage

Settings are persisted to a JSON file in the OS-native config directory:

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/MQTTTunnelManager/tunnel_config.json` |
| Linux | `~/.config/MQTTTunnelManager/tunnel_config.json` |
| Windows | `%AppData%\MQTTTunnelManager\tunnel_config.json` |

The app-managed SSH keypair lives in the same directory (`id_ed25519` / `id_ed25519.pub`).

---

## System tray

On macOS, the app installs a status-bar item via a custom Objective-C/CGO shim (`tray_darwin.go` + `tray_darwin.m`). This avoids NSApp delegate conflicts with Wails' own Cocoa layer that affect third-party systray libraries.

On Windows, the tray uses `energye/systray`.

Menu actions: **Open** (bring window to front), **Help**, **Exit**.

---

## Key technical properties

| Property | Detail |
|---|---|
| P2P encryption | DTLS 1.2 — mandatory for WebRTC; all P2P data is encrypted end-to-end |
| NAT traversal | ICE via STUN by default; optional TURN relay (external or embedded) |
| Embedded relay | pion/turn v2; UDP + TCP listeners; HMAC time-limited credentials |
| SSH reconnect | Automatic, 5-second back-off, spawns the system `ssh` binary |
| Race safety | `-race` tested; all shared state guarded by `sync.Mutex`; Wails events safe from goroutines |
| Config dir | `MQTTTunnelManager/` in the OS config dir; same path in both modules |

---

## Dependencies (app-wails module)

| Package | Version | Purpose |
|---|---|---|
| [github.com/wailsapp/wails/v2](https://wails.io) | v2.11.0 | Native desktop window + Go↔JS bridge |
| [github.com/pion/webrtc/v3](https://github.com/pion/webrtc) | v3.3.6 | WebRTC DataChannels (P2P) |
| [github.com/pion/turn/v2](https://github.com/pion/turn) | v2.1.6 | Embedded TURN relay server |
| [github.com/pion/stun](https://github.com/pion/stun) | v0.6.1 | STUN client (public IP discovery) |
| [github.com/pion/ice/v2](https://github.com/pion/ice) | v2.3.38 | ICE transport |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | v0.48.0 | Ed25519 keypair + pure-Go SSH client |

Frontend: Vue 3 + TypeScript + Tailwind CSS 3 + Vite.

---

## License

This project is unlicensed — use and modify it freely.
