package main

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	appconfig "direct-connector/internal/config"
	"direct-connector/internal/keygen"
	"direct-connector/internal/p2p"
	"direct-connector/internal/tunnel"
)

// ── Frontend-facing types ─────────────────────────────────────────────────────
// Defined in main so Wails generates clean camelCase TypeScript interfaces.

// AppSettings is the persisted user configuration.
type AppSettings struct {
	Host         string `json:"host"`
	Port         string `json:"port"`
	ForwardPorts string `json:"forwardPorts"`
	User         string `json:"user"`
	IPVersion    string `json:"ipVersion"`
	ForwardMode  string `json:"forwardMode"`
	KeyPath      string `json:"keyPath"`
	Verbose      bool   `json:"verbose"`
	UseWSLSsh    bool   `json:"useWslSsh"`
}

// TunnelSettings carries SSH tunnel parameters from the frontend.
type TunnelSettings struct {
	Host         string `json:"host"`
	Port         string `json:"port"`
	ForwardPorts string `json:"forwardPorts"`
	User         string `json:"user"`
	IPVersion    string `json:"ipVersion"`
	ForwardMode  string `json:"forwardMode"`
	KeyPath      string `json:"keyPath"`
	Verbose      bool   `json:"verbose"`
	UseWSLSsh    bool   `json:"useWslSsh"`
}

// ── App ───────────────────────────────────────────────────────────────────────

// App is the main application struct. All exported methods become async
// functions callable from the Vue frontend via Wails bindings.
type App struct {
	ctx       context.Context
	tm        *tunnel.Manager
	p2pSess   *p2p.P2PSession
	p2pCancel context.CancelFunc
	mu        sync.Mutex
}

// NewApp creates a new App.
func NewApp() *App { return &App{} }

// startup is the Wails lifecycle hook called once the WebView window is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.tm = &tunnel.Manager{
		OnStatus: func(s tunnel.Status) {
			runtime.EventsEmit(ctx, "tunnel:status", s.String())
		},
		OnLog: func(msg string) {
			ts := time.Now().Format("15:04:05")
			runtime.EventsEmit(ctx, "tunnel:log", ts+" - "+msg)
		},
	}
}

// ── Config ────────────────────────────────────────────────────────────────────

// LoadConfig reads persisted settings and returns them to the frontend.
func (a *App) LoadConfig() AppSettings {
	c := appconfig.Load()
	return AppSettings{
		Host:         c.Host,
		Port:         c.Port,
		ForwardPorts: c.ForwardPorts,
		User:         c.User,
		IPVersion:    c.IPVersion,
		ForwardMode:  c.ForwardMode,
		KeyPath:      c.KeyPath,
		Verbose:      c.Verbose,
		UseWSLSsh:    c.UseWSLSsh,
	}
}

// SaveConfig persists the settings provided by the frontend.
func (a *App) SaveConfig(s AppSettings) {
	appconfig.Save(appconfig.Config{
		Host:         s.Host,
		Port:         s.Port,
		ForwardPorts: s.ForwardPorts,
		User:         s.User,
		IPVersion:    s.IPVersion,
		ForwardMode:  s.ForwardMode,
		KeyPath:      s.KeyPath,
		Verbose:      s.Verbose,
		UseWSLSsh:    s.UseWSLSsh,
	})
}

// ── Key management ────────────────────────────────────────────────────────────

// AppKeyExists reports whether an app-managed keypair is on disk.
func (a *App) AppKeyExists() bool { return keygen.AppKeyExists() }

// GenerateAppKey creates a new Ed25519 keypair. Returns "" on success, error on failure.
func (a *App) GenerateAppKey() string {
	if err := keygen.GenerateAppKey(); err != nil {
		return err.Error()
	}
	return ""
}

// LoadPublicKey returns the public key as an authorized_keys line, or "".
func (a *App) LoadPublicKey() string {
	pk, _ := keygen.LoadAppPublicKey()
	return pk
}

// AppPrivateKeyPath returns the on-disk path to the app-managed private key.
func (a *App) AppPrivateKeyPath() string { return keygen.AppPrivateKeyPath() }

// ── File dialog ───────────────────────────────────────────────────────────────

// BrowseFile opens the native OS file picker (NSOpenPanel on macOS,
// IFileOpenDialog on Windows) and returns the chosen path, or "" if cancelled.
func (a *App) BrowseFile(title string, defaultDir string) string {
	if defaultDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			defaultDir = filepath.Join(home, ".ssh")
		}
	}
	path, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: defaultDir,
	})
	if err != nil {
		return ""
	}
	return path
}

// CopyToClipboard writes text to the OS clipboard.
func (a *App) CopyToClipboard(text string) {
	runtime.ClipboardSetText(a.ctx, text)
}

// ── SSH Tunnel ────────────────────────────────────────────────────────────────

// StartTunnel launches an SSH tunnel from the given settings.
func (a *App) StartTunnel(s TunnelSettings) {
	a.tm.Start(tunnel.Config{
		Host:         s.Host,
		Port:         s.Port,
		ForwardPorts: s.ForwardPorts,
		User:         s.User,
		IPVersion:    s.IPVersion,
		ForwardMode:  s.ForwardMode,
		KeyPath:      s.KeyPath,
		Verbose:      s.Verbose,
		UseWSLSsh:    s.UseWSLSsh,
	})
}

// StopTunnel terminates the running SSH tunnel.
func (a *App) StopTunnel() { a.tm.Stop() }

// IsTunnelRunning reports whether a tunnel is currently active.
func (a *App) IsTunnelRunning() bool { return a.tm.IsRunning() }

// ── P2P ───────────────────────────────────────────────────────────────────────

// P2PGenerateOffer creates a WebRTC offer (Consumer/initiator side).
// ports is comma-separated e.g. "1883,3391".
// Returns base64 SDP on success, "ERROR: ..." on failure.
func (a *App) P2PGenerateOffer(ports string) string {
	a.mu.Lock()
	if a.p2pSess != nil && a.p2pSess.IsActive() {
		a.mu.Unlock()
		return "ERROR: session already active — stop it first"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.p2pCancel = cancel
	sess := a.newP2PSession()
	a.p2pSess = sess
	a.mu.Unlock()

	offer, err := sess.GenerateOffer(ctx, ports)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return offer
}

// P2PAcceptAnswer feeds the Provider's SDP answer into the active Consumer session.
// Returns "" on success, "ERROR: ..." on failure.
func (a *App) P2PAcceptAnswer(answer string) string {
	a.mu.Lock()
	sess := a.p2pSess
	a.mu.Unlock()
	if sess == nil {
		return "ERROR: no active Consumer session — generate an offer first"
	}
	if err := sess.ApplyAnswer(answer); err != nil {
		return "ERROR: " + err.Error()
	}
	return ""
}

// P2PProvideAnswer accepts the Consumer's offer and returns an SDP answer (Provider side).
// Returns base64 SDP on success, "ERROR: ..." on failure.
func (a *App) P2PProvideAnswer(offerSDP string) string {
	a.mu.Lock()
	if a.p2pSess != nil && a.p2pSess.IsActive() {
		a.mu.Unlock()
		return "ERROR: session already active — stop it first"
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.p2pCancel = cancel
	sess := a.newP2PSession()
	a.p2pSess = sess
	a.mu.Unlock()

	answer, err := sess.AcceptOfferAndGenerateAnswer(ctx, offerSDP)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return answer
}

// P2PStop terminates the active P2P session.
func (a *App) P2PStop() {
	a.mu.Lock()
	sess := a.p2pSess
	cancel := a.p2pCancel
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	if sess != nil {
		sess.Stop()
	}
}

// P2PIsActive reports whether a P2P session is live.
func (a *App) P2PIsActive() bool {
	a.mu.Lock()
	sess := a.p2pSess
	a.mu.Unlock()
	return sess != nil && sess.IsActive()
}

func (a *App) newP2PSession() *p2p.P2PSession {
	ctx := a.ctx
	return p2p.NewP2PSession(
		func(s string) { runtime.EventsEmit(ctx, "p2p:status", s) },
		func(msg string) {
			ts := time.Now().Format("15:04:05")
			runtime.EventsEmit(ctx, "p2p:log", ts+" - "+msg)
		},
	)
}
