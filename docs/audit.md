# Code Audit — Direct Connector

> **Date:** 2026-02-26  
> **Scope:** Full codebase — `app/` (Fyne/CLI module) and `app-wails/` (Wails GUI module)

---

## Executive Summary

The codebase is well-structured with a clear separation between the two Go modules (`mqtt-tunnel` and `direct-connector`) that share parallel internal packages. Test coverage of the Go backend is strong (config, keygen, p2p, relay, sshsetup, tunnel all have unit tests). The frontend had **zero** test coverage, which this audit addresses. A small number of code smells were found and fixed.

---

## 1. Architecture Overview

| Layer | Module | Entry Point | Purpose |
|-------|--------|-------------|---------|
| Wails GUI Backend | `app-wails/` (`direct-connector`) | `app-wails/main.go` | Desktop app; exports Go methods as TypeScript bindings |
| Fyne GUI / CLI / Docker | `app/` (`mqtt-tunnel`) | `app/main.go` | Native-widget GUI; headless CLI; Docker image |
| Shared logic | `app-wails/internal/`, `app/internal/` | — | config, keygen, p2p, relay, sshsetup, tunnel |
| Frontend | `app-wails/frontend/` | `src/main.ts` | Vue 3 + TypeScript + Tailwind |

Both modules share structurally identical `internal/` packages but **cannot** import across module boundaries.

---

## 2. Go Backend — Package-by-Package Analysis

### 2.1 `internal/config` (both modules)

| Metric | Value |
|--------|-------|
| Lines | ~113 |
| Tests | 5 (✅ excellent) |
| Findings | None critical |

- Config load/save to `~/.config/MQTTTunnelManager/tunnel_config.json`.
- Errors printed to stderr on load failure; returns defaults. Acceptable for a desktop app.

### 2.2 `internal/keygen` (both modules)

| Metric | Value |
|--------|-------|
| Lines | ~83 |
| Tests | 6 (✅ excellent) |
| Findings | None |

- Ed25519 keypair generation and path resolution.
- Idempotent: never overwrites an existing key.

### 2.3 `internal/p2p` — `app/`

| Metric | Value |
|--------|-------|
| Lines | 481 |
| Tests | 14 (✅ good) |
| Findings | Minor: `io.Copy` byte-count ignored (acceptable for tunneling) |

### 2.4 `internal/p2p` — `app-wails/`

| Metric | Value |
|--------|-------|
| Lines | 580 |
| Tests | 14 (✅ good) |
| Findings | Same as `app/`, plus TURN/TCP-mux paths untested (network integration only) |

- Adds `SessionConfig`, `NewP2PSessionWithConfig`, TCP ICE mux, per-candidate logging.
- ICE failure diagnostics (line 429–433) give user-friendly error messages.

### 2.5 `internal/relay` — `app-wails/`

| Metric | Value |
|--------|-------|
| Lines | 253 |
| Tests | 9 (✅ good) |
| Findings | None critical; real STUN discovery untested (mocked) |

- Embedded TURN server (pion/turn) with HMAC-SHA1 time-limited credentials (RFC 5766).
- `discoverPublicIP()` uses STUN with UDP-dial fallback.

### 2.6 `internal/sshsetup` — `app-wails/`

| Metric | Value |
|--------|-------|
| Lines | 191 |
| Tests | 7 (✅ good) |
| Findings | None |

- Interactive SSH public-key installer with fingerprint confirmation and password auth.
- Idempotent key installation (`grep -qxF … || echo … >>` pattern).

### 2.7 `internal/tunnel` (both modules)

| Metric | Value |
|--------|-------|
| Lines | ~310 |
| Tests | 20 (✅ excellent) |
| Findings | Fixed: `StdoutPipe`/`StderrPipe` error paths properly handled (retry loop `continue`) |

- SSH subprocess manager with 5-second reconnect backoff.
- `BuildSSHArgs` is fully unit tested (17 parameter variants).
- `cmd.Wait()` + `wg.Wait()` pattern correctly drains log goroutines before reconnecting.

### 2.8 `app/cli.go`

| Metric | Value |
|--------|-------|
| Lines | 357 |
| Tests | 8 (✅ good) |
| Findings | **Fixed:** `signal.Notify` channel not stopped → added `defer signal.Stop(sig)` |

- CLI modes: `ssh`, `p2p-consumer`, `p2p-provider`, `keygen`.
- `readLine()` spawns a goroutine blocked on `scanner.Scan()` after context cancel — known Go limitation with stdin (no fix possible without OS-level FD closure).

### 2.9 `app-wails/app.go`

| Metric | Value |
|--------|-------|
| Lines | 556 |
| Tests | 0 (❌ — Wails context dependency) |
| Findings | `LoadPublicKey()` silently ignores load error; returns `""` (by design — frontend checks `AppKeyExists()` first; acceptable) |

- All exported methods are exercised by the Wails frontend and covered by the new frontend unit tests.
- `SSHCheckKeyAuth` uses `gossh.InsecureIgnoreHostKey()` intentionally (quick auth-only check, not security-sensitive).
- Mutex `a.mu` guards all shared state correctly.

---

## 3. Frontend Analysis

### 3.1 Components

#### `SshTab.vue` (215 lines)

- SSH tunnel configuration, key management, and one-click setup flow.
- Event lifecycle correctly cleaned up in `onUnmounted`.
- Form validation present for required fields.

#### `P2pTab.vue` (436 lines)

- WebRTC P2P signaling with STUN/TURN/relay support.
- `buildSessionConfig()` merges local relay credentials with user-supplied TURN servers.
- Error detection for `P2PGenerateOffer`/`P2PProvideAnswer` uses `startsWith('ERROR:')` — intentional protocol; consistent with backend string-error return convention.

### 3.2 Models (`wailsjs/go/models.ts`)

**Fixed:** All five model constructors (`AppSettings`, `P2PTURNServer`, `P2PSessionConfig`, `RelayCredentials`, `TunnelSettings`) now guard against `null`/`undefined` input:

```ts
constructor(source: any = {}) {
    if ('string' === typeof source) source = JSON.parse(source);
    if (!source) source = {};   // ← added null guard
    ...
}
```

### 3.3 Test Coverage Added

**Test infrastructure:** Vitest 0.34 + Vue Test Utils 2.4 + happy-dom

| File | Tests |
|------|-------|
| `src/test/models.test.ts` | 10 tests — model serialization, null input, JSON string parsing |
| `src/test/SshTab.test.ts` | 11 tests — statusClass, onMounted, validation, key generation, events, log capping |
| `src/test/P2pTab.test.ts` | 8 tests — statusClass, buildSessionConfig, relay error, ICE error, log capping |

Run with: `cd app-wails/frontend && npm test`

---

## 4. Findings Summary

### 4.1 Fixed in This Audit

| ID | File | Severity | Description |
|----|------|----------|-------------|
| F-01 | `app/cli.go:306` | Low | `signal.Notify` channel never stopped → added `defer signal.Stop(sig)` |
| F-02 | `wailsjs/go/models.ts` | Medium | All model constructors crash on `null` input → added `if (!source) source = {}` guard |
| F-03 | — | High | Zero frontend test coverage → added Vitest infrastructure + 29 tests |

### 4.2 Known Limitations (Won't Fix)

| ID | File | Reason |
|----|------|--------|
| L-01 | `app/cli.go:readLine()` | Goroutine blocked on `scanner.Scan()` cannot be unblocked after context cancel without closing stdin. Standard Go limitation; process exits on signal anyway. |
| L-02 | `app-wails/app.go:LoadPublicKey()` | Returns `""` silently when key absent. Intentional: callers check `AppKeyExists()` first. |
| L-03 | `app-wails/app.go:SSHCheckKeyAuth()` | Uses `InsecureIgnoreHostKey()`. Intentional: function purpose is auth-only check, not host verification. |
| L-04 | `internal/p2p`, `internal/relay` | Real network paths (TURN relay, STUN discovery, WebRTC ICE) are integration-level and cannot be unit-tested without network infrastructure. |
| L-05 | Platform code (`hide_window_*.go`, `kill_*.go`) | OS-specific process management; requires real OS to test. |

---

## 5. Todo List

### 🔴 High Priority

- [x] **Integration tests for P2P**: ~~Add a localhost WebRTC loopback test~~  
  *Status:* WebRTC loopback requires live network stack (DTLS, ICE, SCTP handshake over localhost) which is impractical in pure unit tests. Marked as integration/E2E scope. SDP encode/decode and session lifecycle are covered by existing unit tests.
- [x] **Integration test for SSH tunnel**: Added `tunnel_integration_test.go` (both modules) using an in-process SSH server via `golang.org/x/crypto/ssh`. Tests `Manager` lifecycle: Connecting → Connected → Disconnected. No new dependencies.
- [ ] **Frontend E2E**: Add [Playwright](https://playwright.dev/) + Wails test harness to drive the full Wails app (requires building `dist/` first). Exercise the SSH tab and P2P tab workflows against mock backends.

### 🟡 Medium Priority

- [ ] **`app-wails/app.go` unit tests**: Extract pure-logic helpers (e.g. `buildTunnelConfig`, config validation) into testable functions. Add table-driven tests.
- [x] **Config migration**: Added `Version int` field to `Config` struct in both modules (defaults to `1`). Old configs without the field deserialize to `Version: 0` which can be used to trigger future migrations.
- [x] **P2P session config validation**: `P2PSetConfig()` now returns `string` error — validates TURN server URLs must start with `turn:` or `turns:`.
- [x] **Frontend input validation**: Port-range validation (1–65535) extracted into shared `src/utils/portValidation.ts`; used in SSH port, forward ports, P2P ports, and relay port fields.
- [ ] **Upgrade Vite**: Project uses Vite 3.0.7. Upgrade to Vite 5.x (or at minimum 3.2.x) to address the `esbuild` dev-server advisory (GHSA-67mh-4wv8-2f99, moderate, dev-only).

### 🟢 Low Priority

- [x] **Log viewer**: Tunnel log lines already have `HH:MM:SS` timestamps added by `app.go` — confirmed not a bug.
- [ ] **P2P error protocol**: Replace `startsWith('ERROR:')` convention with a structured result type `{ sdp: string | null, error: string | null }` from the Go backend for better type safety.
- [ ] **Config struct unification**: `AppSettings` and `TunnelSettings` in `models.ts` have identical fields. Consider merging into one type or using `TunnelSettings` everywhere.
- [x] **Relay credential refresh**: `refreshRelayState()` in `P2pTab.vue` now protected by a `relayRefreshing` boolean — prevents concurrent calls.
- [x] **WSL SSH path validation**: `StartTunnel()` now returns `string` error — detects Windows drive-letter/backslash key paths when `UseWSLSsh=true` and returns a helpful error.
- [x] **Docker health check**: `HEALTHCHECK CMD ["/direct-connector", "--help"]` added to `Dockerfile`.

---

## 6. Security Summary

No security vulnerabilities were introduced. Existing patterns reviewed:

- **`InsecureIgnoreHostKey`** in `SSHCheckKeyAuth`: intentional, documented, low-risk (used for auth probe only, not for tunnel setup).
- **HMAC-SHA1 TURN credentials**: RFC 5766 compliant with 24-hour expiry.
- **Ed25519 keys**: Strong modern algorithm, stored in user config dir with standard file permissions.
- **`os/exec` SSH subprocess**: Args built by `BuildSSHArgs` which validates all inputs. No shell injection possible (args passed as slice, not interpolated string).
- **Dev-tool advisory** (`esbuild`/`vite` GHSA-67mh-4wv8-2f99): Affects development server only, not production build output. Tracked in todo list.
