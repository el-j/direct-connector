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
| SSH Host / Port | Your jump server's address |
| SSH User | Login username |
| Forward Ports | Comma-separated list of ports to forward (`1883,8080,22`) |
| IPv6 | Wrap the host in `[…]` automatically |

Settings are saved to the OS-native config directory (`~/.config/MQTTTunnelManager/` on macOS/Linux, `%AppData%\MQTTTunnelManager\` on Windows) so they survive restarts.

### P2P Tunnel tab
Creates a **100 % serverless, direct peer-to-peer encrypted tunnel** using WebRTC DataChannels.  
You choose your role on this machine, exchange a single line of text with the other machine (via any channel — chat, email, even a sticky note), and the tunnel starts.

No signalling server. No relay. No third-party service.  
The only external contact is a single STUN lookup at startup to discover your public IP — after that, all data flows directly and is encrypted with DTLS.

#### Roles (any OS can be either role)

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
├── README.md                   ← you are here
├── MQTT Tunnel Manager (Go).md ← original design spec
└── mqtt-tunnel/                ← Go module root
    ├── main.go                 ← GUI entry point (build tag: !nofyne)
    ├── main_nofyne.go          ← CLI-only entry point (build tag: nofyne)
    ├── cli.go                  ← headless CLI: all modes, flag/env parsing
    ├── tunnel.go               ← SSH tunnel engine (TunnelManager)
    ├── ssh_tab.go              ← SSH tab UI (build tag: !nofyne)
    ├── p2p.go                  ← WebRTC P2P engine (P2PSession)
    ├── p2p_tab.go              ← P2P tab UI (build tag: !nofyne)
    ├── keygen.go               ← Ed25519 key generation
    ├── config.go               ← persistent SSH config (JSON)
    ├── tunnel_test.go          ← SSH engine tests
    ├── p2p_test.go             ← P2P engine tests
    ├── config_test.go          ← config tests
    ├── cli_test.go             ← CLI helpers tests
    ├── Dockerfile              ← two-stage build → scratch image
    ├── docker-compose.yml      ← ready-to-use service examples
    ├── build.sh                ← build / test / cross-compile helper
    ├── FyneApp.toml            ← Fyne app metadata
    ├── go.mod
    └── go.sum
```

---

## Prerequisites

| Tool | Version | Install |
|---|---|---|
| Go | ≥ 1.21 (developed on 1.25) | `brew install go` or [go.dev](https://go.dev/dl/) |
| Fyne CLI | latest | `go install fyne.io/fyne/v2/cmd/fyne@latest` |
| fyne-cross | latest | `go install github.com/fyne-io/fyne-cross@latest` |
| Docker Desktop | any | [docker.com](https://www.docker.com/products/docker-desktop/) — **only** needed for Windows cross-compile |

> **macOS:** Xcode Command Line Tools must be installed (`xcode-select --install`).  
> **Windows (native build):** Install [TDM-GCC](https://jmeubank.github.io/tdm-gcc/) or any MinGW toolchain.

---

## Development

```bash
cd mqtt-tunnel

# Run locally (no build step needed)
./build.sh run
# or
go run .
```

The app starts immediately. Both tabs are functional. SSH settings are loaded from disk on start and saved on every change.

### Run the test suite

```bash
./build.sh test
# or
go test -v -race -count=1 ./...
```

51 tests cover the SSH engine, P2P engine, CLI helpers, config persistence, and bridge/port utilities. The race detector is always enabled.

---

## Building for distribution

All build commands are in `mqtt-tunnel/build.sh`. Run from the `mqtt-tunnel/` directory.

### macOS `.app` bundle

```bash
./build.sh mac
```

Produces `direct-connector.app` in the current directory. Double-click to launch, or drag to `/Applications`.

### Windows `.exe` (cross-compiled from macOS)

Docker Desktop must be running.

```bash
./build.sh windows
```

The first run pulls a Docker image (~1 GB, one-time). Output:

```
fyne-cross/bin/windows-amd64/direct-connector.exe
```

Copy the `.exe` to the target Windows machine — no installer, no runtime, no dependencies.

### Build both at once

```bash
./build.sh all   # runs tests first, then mac + windows + cli
```

---

## CLI mode & Docker

The same binary supports a fully headless CLI mode — no display server, no GUI libraries. Use it in terminal scripts, cron jobs, or Docker containers.

### Build the headless CLI binary

```bash
./build.sh cli
# or manually:
CGO_ENABLED=0 go build -tags nofyne -ldflags="-s -w" -o direct-connector-cli .
```

The `nofyne` build tag excludes all Fyne code. The result is a **pure-Go, fully static binary** with no CGO, no OpenGL, no display dependency.

### CLI modes

| Mode | Description |
|---|---|
| `ssh` | Persistent reverse-SSH tunnel |
| `p2p-consumer` | WebRTC consumer: opens local ports, tunnels to provider |
| `p2p-provider` | WebRTC provider: bridges DataChannels to local services |
| `keygen` | Generate app Ed25519 keypair, print public key |

Every flag also reads from a `DC_<FLAG>` environment variable:

```bash
# SSH tunnel
direct-connector --mode ssh \
  --host jump.example.com --port 8080 --user admin \
  --ports 1883,3391 --ip 6

# P2P consumer (interactive stdin/stdout)
direct-connector --mode p2p-consumer --ports 1883,8080

# P2P provider (file-based — for Docker shared volumes)
direct-connector --mode p2p-provider \
  --offer-in /data/offer.txt --answer-out /data/answer.txt

# Generate key, print authorized_keys line
direct-connector --mode keygen
```

### Docker

```bash
# Build image
./build.sh docker          # or: docker build -t direct-connector:latest .

# Run SSH tunnel
docker run --rm \
  -e DC_MODE=ssh \
  -e DC_HOST=jump.example.com \
  -e DC_PORT=8080 \
  -e DC_USER=admin \
  -e DC_PORTS=1883,3391 \
  -v dc-config:/config \
  direct-connector:latest
```

The Docker image is built in two stages and runs `FROM scratch` — just the static binary + CA certificates. Final image is ~10 MB.

### Docker Compose

A ready-to-use [docker-compose.yml](mqtt-tunnel/docker-compose.yml) is included with three service examples:

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

> The app uses your existing SSH key (`~/.ssh/id_rsa` etc.). Make sure the key is authorised on the jump host. No password prompts are shown — use `ssh-copy-id` beforehand if needed.

### P2P tunnel

#### On the Consumer machine (the one that wants to *reach* services)

1. Switch to the **P2P Tunnel** tab.
2. Select **Consumer (Initiator — opens local ports)**.
3. Enter the ports you want forwarded locally, e.g. `1883,8080`.
4. Click **Generate Offer**. Wait ~2 seconds for ICE gathering.
5. Click **Copy Offer** and send the text to the Provider (chat, email, anything).
6. Wait for the Provider to send back an Answer, paste it, and click **Connect**.

#### On the Provider machine (the one that *has* the services)

1. Switch to the **P2P Tunnel** tab.
2. Select **Provider (Responder — bridges local services)**.
3. Paste the Consumer's Offer text.
4. Click **Accept & Generate Answer**. Wait ~2 seconds.
5. Click **Copy Answer** and send the text back to the Consumer.

The tunnel is live once both sides complete the exchange. No further coordination is needed until you click **Stop**.

#### What "ports" means

- **Consumer side:** `1883` means `localhost:1883` on *this* machine will be forwarded to the Provider.
- **Provider side:** Incoming connections for port `1883` are bridged to `localhost:1883` on *that* machine.

So if the Provider is running Mosquitto on 1883, the Consumer can connect its MQTT client to its own `localhost:1883` and communicate directly.

---

## Key technical properties

| Property | Detail |
|---|---|
| Encryption | DTLS 1.2 (WebRTC mandatory) — all P2P traffic is encrypted |
| NAT traversal | ICE via STUN — works through most home/office routers |
| Relay traffic | None — if ICE fails (symmetric NAT on both sides), the connection won't establish |
| Signalling | Manual copy-paste — no server involved at any point |
| SSH reconnect | Automatic, 5-second back-off, uses system `ssh` binary |
| Config storage | OS native config dir, JSON |
| Race safety | Tested with `-race`; all shared state guarded by `sync.Mutex`; Fyne UI mutations via `fyne.Do()` |

---

## Dependencies

| Package | Purpose |
|---|---|
| [fyne.io/fyne/v2](https://fyne.io) v2.7.3 | Cross-platform GUI |
| [github.com/pion/webrtc/v3](https://github.com/pion/webrtc) v3.3.6 | WebRTC DataChannels (P2P) |

---

## License

This project is unlicensed — use and modify it freely.
