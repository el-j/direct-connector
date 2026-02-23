# Direct Connector v1.0.0

**First public release.** Direct Connector is a cross-platform desktop app for creating secure, persistent tunnels between two machines — with no cloud dependency, no subscription, and no third-party relay required.

---

## What's included

### 🔐 SSH Reverse Tunnel
- Persistent, self-healing reverse SSH tunnel to any jump host you control
- Automatic reconnect with 5-second back-off — survives sleep, network changes, and interruptions
- Configurable port forwarding in `local` or `remote` mode
- IPv6 support (`-6` flag) for DS-Lite and IPv6-only networks
- WSL SSH mode (Windows): routes the SSH process through `wsl.exe` for correct networking when services live inside WSL or Docker
- Verbose mode passes `-v` to the SSH binary for detailed diagnostics

### 🔑 One-Click SSH Key Installer
- Generates an app-managed Ed25519 keypair on first use, stored in the OS config directory
- Pure-Go SSH key installation via `x/crypto/ssh` — no terminal, no `ssh-copy-id`
- Interactive in-app wizard with host fingerprint confirmation and password prompts
- Post-install key auth check confirms the server accepted the key

### 🌐 Serverless P2P Tunnel
- 100 % serverless peer-to-peer encrypted tunnel using WebRTC DataChannels
- Copy-paste SDP exchange — no signalling server at any point
- DTLS 1.2 end-to-end encryption (WebRTC mandatory)
- ICE NAT traversal via STUN — works through most home and office routers
- Optional TURN relay servers for symmetric NAT / CGNAT environments
- Optional ICE-TCP mux listener (port 443 recommended for maximum firewall penetration)
- Configurable ICE gather timeout (default 30 seconds)
- Consumer and Provider roles with independent port configuration

### ⚡ Embedded TURN Relay Server
- Run your own TURN relay directly inside the app — no external service needed
- Public IP discovery via STUN on startup
- UDP listener + optional TCP listener on the same port for firewall-blocked sites
- Time-limited HMAC credentials generated automatically (one-click copy into P2P settings)
- Real-time relay activity log

### 🖥️ Native Desktop GUI
- Built with Go + Wails v2 (Vue 3 + TypeScript + Tailwind CSS) — native WebView, not Electron
- macOS: custom Objective-C/CGO menu-bar status item (Open / Help / Exit) with no Cocoa delegate conflicts
- Windows: system tray via `energye/systray`
- Settings persisted automatically to the OS config directory

### 🐳 Headless CLI & Docker
- Same tunnelling engine compiles to a fully static binary (`CGO_ENABLED=0`, `-tags nofyne`)
- Zero CGO, no GUI libraries — runs on headless servers and in containers
- `FROM scratch` Docker image (~10 MB): just the binary and CA certificates
- All flags available as `DC_*` environment variables for pure-env Docker / Compose configuration
- Docker Compose examples for SSH tunnel and P2P consumer/provider pairs (file-based SDP exchange via shared volume)

### 🤖 CI / CD
- GitHub Actions workflow: tests on every push and pull request (race detector enabled)
- Automated multi-platform builds: macOS arm64 `.app`, Windows amd64 `.exe`, macOS CLI, Linux amd64 CLI
- Automated GitHub Releases on `v*` tags with all artefacts attached
- GitHub Pages project site with live download links at https://el-j.github.io/directConnector/

---

## Downloads

| Platform | File | Notes |
|---|---|---|
| macOS (Apple Silicon) | `direct-connector-mac.zip` | Unzip and drag to Applications |
| Windows (x64) | `direct-connector-windows.exe` | No installer — run directly |
| Linux (amd64) | `direct-connector-linux-amd64` | CLI / headless binary |
| macOS CLI (arm64) | `direct-connector-cli-darwin-arm64` | Headless, no GUI |

> macOS users: on first launch, right-click → Open to bypass Gatekeeper (the binary is not yet notarised).

---

## Config storage

| OS | Path |
|---|---|
| macOS | `~/Library/Application Support/MQTTTunnelManager/` |
| Windows | `%AppData%\MQTTTunnelManager\` |
| Linux | `~/.config/MQTTTunnelManager/` |

---

## Dependencies

| Package | Version |
|---|---|
| [Wails v2](https://wails.io) | v2.11.0 |
| [pion/webrtc](https://github.com/pion/webrtc) | v3.3.6 |
| [pion/turn](https://github.com/pion/turn) | v2.1.6 |
| [pion/stun](https://github.com/pion/stun) | v0.6.1 |
| [pion/ice](https://github.com/pion/ice) | v2.3.38 |
| [golang.org/x/crypto](https://pkg.go.dev/golang.org/x/crypto) | v0.48.0 |

---

## License

Unlicensed — use and modify freely.
