# Direct Connector

A cross-platform desktop app for creating secure tunnels between two machines — with **no cloud dependency and no subscription**.  
Built with [Go](https://go.dev) and [Fyne](https://fyne.io), packaged as a native `.app` on macOS and a standalone `.exe` on Windows.

---

## What it does

The app provides two independent tunnel methods in a single tabbed UI:

### SSH Tunnel tab
Establishes a persistent, self-healing **reverse SSH tunnel** over an SSH jump host you already control.  
Port-forwards are kept alive with automatic reconnect and keep-alive probes.

| Setting | What it does |
|---|---|
| SSH Host / Port | Your jump server's address. **Use port 443** — routers and corporate firewalls treat it as HTTPS and almost never block it. |
| SSH User | Login username |
| Forward Ports | Comma-separated list of ports to forward (`1883,3391`) |
| IPv6 | Force IPv6 socket (`-6`) — use when on a DS-Lite / IPv6-only network |

Settings are saved to the OS-native config directory (`~/.config/MQTTTunnelManager/` on Linux, `~/Library/Application Support/MQTTTunnelManager/` on macOS, `%AppData%\MQTTTunnelManager\` on Windows).

### P2P Tunnel tab
Creates a **100 % serverless, direct peer-to-peer encrypted tunnel** using WebRTC DataChannels.  
You choose your role, exchange a single line of text with the other machine (via any channel — chat, email, even a sticky note), and the tunnel starts.

No signalling server. No relay. No third-party service.  
The only external contact is a single STUN lookup at startup to discover your public IP — after that, all data flows directly and is encrypted with DTLS.

#### Roles (any OS can be either)

| Role | What it does |
|---|---|
| **Consumer (Initiator)** | Opens local TCP listeners on the specified ports. Traffic is tunnelled *to* the Provider. |
| **Provider (Responder)** | Bridges incoming DataChannels to local services (e.g. Mosquitto on port 1883). |

#### Connection flow

```
Consumer machine                     Provider machine
────────────────                     ────────────────
1. Enter ports
   Click "Generate Offer"
   ── copies a base64 text blob ──▶  Paste into Offer box
                                     Click "Accept & Generate Answer"
                 ◀── base64 text ──  Copy the Answer
2. Paste Answer
   Click "Connect"

      ╔═══════════════════════════════════════════════╗
      ║  Direct DTLS-encrypted peer-to-peer channel  ║
      ║  Zero bytes through any server from here on  ║
      ╚═══════════════════════════════════════════════╝
```

---

## Project layout

```
directSshConnector/
├── README.md
├── Makefile                        ← root entry point (delegates to app/build.sh)
├── Dockerfile                      ← two-stage Docker build (context: app/)
├── docker-compose.yml              ← ready-to-use service examples
├── dist/                           ← built artefacts (git-ignored)
│   ├── direct-connector.app        ← macOS GUI bundle
│   ├── direct-connector.exe        ← Windows GUI executable
│   ├── direct-connector-cli        ← macOS/Linux headless CLI
│   └── direct-connector-linux-amd64
└── app/                            ← Go module root  (module name: mqtt-tunnel)
    ├── FyneApp.toml
    ├── Icon.png
    ├── build.sh                    ← build / test / cross-compile helper
    ├── go.mod / go.sum
    ├── main.go                     ← GUI entry point  (build tag: !nofyne)
    ├── main_nofyne.go              ← CLI entry point  (build tag: nofyne)
    ├── cli.go                      ← headless CLI — all modes, flag/env parsing
    ├── cli_test.go
    ├── ssh_tab.go                  ← SSH tab UI       (build tag: !nofyne)
    ├── p2p_tab.go                  ← P2P tab UI       (build tag: !nofyne)
    └── internal/
        ├── config/                 ← persistent JSON config
        ├── keygen/                 ← Ed25519 keypair management
        ├── p2p/                    ← WebRTC P2P engine
        └── tunnel/                 ← SSH tunnel manager with auto-reconnect
```

> **`src/` is intentionally avoided** — Go's own tooling docs call it a GOPATH-era anti-pattern. `app/` is the idiomatic name when the module cannot sit at the repo root.

---

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| Go | ≥ 1.22 | `brew install go` or [go.dev](https://go.dev/dl/) |
| Fyne CLI (`fyne`) | latest | `go install fyne.io/tools/cmd/fyne@latest` |
| fyne-cross | latest | `go install github.com/fyne-io/fyne-cross@latest` — only needed for Windows cross-compile |
| Docker Desktop | any | [docker.com](https://www.docker.com/products/docker-desktop/) — only needed for Windows cross-compile and Docker image builds |

> **macOS:** Xcode Command Line Tools must be installed (`xcode-select --install`).

---

## Development

```bash
# Run the GUI app locally (no build step)
make run
# or:
cd app && go run .
```

Both tabs are functional immediately. SSH settings are loaded from disk on start and saved on every change.

### Run the test suite

```bash
make test
```

51 tests cover the SSH engine, P2P engine, CLI helpers, config persistence, and bridge/port utilities. The race detector is always enabled.

```
ok  mqtt-tunnel                   (main package — CLI helpers)
ok  mqtt-tunnel/internal/config   (config persistence)
ok  mqtt-tunnel/internal/keygen   (Ed25519 keypair)
ok  mqtt-tunnel/internal/p2p      (WebRTC engine)
ok  mqtt-tunnel/internal/tunnel   (SSH tunnel manager)
```

---

## Building for distribution

All built artefacts land in `dist/` at the project root. Use `make` from the root or `./build.sh` from inside `app/`.

### Quick reference

| Command | Output |
|---|---|
| `make mac` | `dist/direct-connector.app` |
| `make windows` | `dist/direct-connector.exe` |
| `make linux` | `dist/direct-connector-linux-amd64` |
| `make cli` | `dist/direct-connector-cli` (current OS, headless) |
| `make all` | tests + all of the above (Windows skipped gracefully if Docker unavailable) |
| `make clean` | removes `dist/` |

### macOS `.app` bundle

```bash
make mac
```

Produces `dist/direct-connector.app`. Double-click to launch, or drag to `/Applications`.

### Windows `.exe` (cross-compiled from macOS/Linux)

Requires Docker Desktop running and `fyne-cross` installed (see Prerequisites above).

```bash
make windows
```

The first run pulls a Docker image (~1 GB, one-time). Output: `dist/direct-connector.exe`.  
Copy it to a Windows machine — no installer, no runtime, no dependencies.

### Linux headless binary

```bash
make linux
```

Produces `dist/direct-connector-linux-amd64`. Pure-Go static binary, no CGO.

---

## CLI mode & Docker

The same codebase compiles to a fully headless CLI binary — no display server, no GUI libraries.

### Build the headless CLI binary

```bash
make cli
# manual equivalent:
cd app && CGO_ENABLED=0 go build -tags nofyne -ldflags="-s -w" -o ../dist/direct-connector-cli .
```

### CLI modes

| Mode | Description |
|---|---|
| `ssh` | Persistent reverse-SSH tunnel |
| `p2p-consumer` | WebRTC consumer: opens local ports, tunnels to provider |
| `p2p-provider` | WebRTC provider: bridges DataChannels to local services |
| `keygen` | Generate Ed25519 keypair, print public key |

Every flag also reads from a `DC_<FLAG>` environment variable:

```bash
# SSH tunnel
./dist/direct-connector-cli --mode ssh \
  --host jump.example.com --port 443 --user admin \
  --ports 1883,3391 --ip 6

# P2P consumer (interactive stdin/stdout)
./dist/direct-connector-cli --mode p2p-consumer --ports 1883,8080

# P2P provider (file-based — for Docker shared volumes)
./dist/direct-connector-cli --mode p2p-provider \
  --offer-in /data/offer.txt --answer-out /data/answer.txt

# Generate key, print authorized_keys line
./dist/direct-connector-cli --mode keygen
```

### Docker

`Dockerfile` and `docker-compose.yml` live at the project root. The build context is the `app/` directory.

```bash
# Build image
make docker
# or manually:
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

For the P2P case, both containers share a volume at `/data`. The consumer writes `offer.txt`, the provider reads it and writes `answer.txt`, and the consumer polls until the answer appears — no manual SDP copying needed.

---

## Usage walkthrough

### SSH tunnel

1. Launch the app on the machine that needs outbound port-forwarding.
2. Fill in **SSH Host**, **Port** (default 22), **SSH User**, and **Forward Ports**.
3. Click **Start**. The status label turns green and the log shows the SSH command output.
4. The tunnel reconnects automatically after any disconnect (5-second back-off).
5. Click **Stop** when done.

> The app uses your existing SSH key (`~/.ssh/id_ed25519` etc.). Make sure the key is authorised on the jump host. No password prompts are shown — use `ssh-copy-id` beforehand if needed. You can also generate a dedicated key via the **keygen** CLI mode.

### P2P tunnel

#### Consumer machine (wants to *reach* services)

1. Switch to the **P2P Tunnel** tab and select **Consumer**.
2. Enter the ports to forward locally, e.g. `1883,8080`.
3. Click **Generate Offer** → wait ~2 s for ICE gathering.
4. Click **Copy Offer** and send the text to the Provider.
5. Receive the Answer from the Provider, paste it, click **Connect**.

#### Provider machine (has the services)

1. Switch to the **P2P Tunnel** tab and select **Provider**.
2. Paste the Consumer's Offer text.
3. Click **Accept & Generate Answer** → wait ~2 s.
4. Click **Copy Answer** and send it back to the Consumer.

The tunnel is live once both sides complete the exchange.

---

## Key technical properties

| Property | Detail |
|---|---|
| Encryption | DTLS 1.2 (WebRTC mandatory) — all P2P traffic is encrypted end-to-end |
| NAT traversal | ICE via STUN — works through most home/office routers |
| Relay traffic | None — if ICE fails (symmetric NAT on both sides), connection won't establish |
| Signalling | Manual copy-paste — no server involved at any point |
| SSH reconnect | Automatic, 5-second back-off, uses the system `ssh` binary |
| Config storage | OS-native config dir, JSON |
| Race safety | `-race` tested; all shared state guarded by `sync.Mutex`; Fyne UI mutations via `fyne.Do()` |

---

## Dependencies

| Package | Purpose |
|---|---|
| [fyne.io/fyne/v2](https://fyne.io) v2.7.3 | Cross-platform GUI |
| [github.com/pion/webrtc/v3](https://github.com/pion/webrtc) v3.3.6 | WebRTC DataChannels (P2P) |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | Ed25519 keypair generation |

---

## License

This project is unlicensed — use and modify it freely.

