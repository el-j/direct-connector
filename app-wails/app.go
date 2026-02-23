package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	gossh "golang.org/x/crypto/ssh"

	appconfig "direct-connector/internal/config"
	"direct-connector/internal/keygen"
	"direct-connector/internal/p2p"
	"direct-connector/internal/relay"
	"direct-connector/internal/sshsetup"
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

// SetupPrompt is emitted as the "setup:prompt" event payload.
type SetupPrompt struct {
	Kind        string `json:"kind"` // "fingerprint" | "password"
	Message     string `json:"message"`
	Fingerprint string `json:"fingerprint"` // non-empty when kind=="fingerprint"
}

// SetupResult is emitted as the "setup:done" event payload.
type SetupResult struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// P2PTURNServer describes a single TURN relay server.
type P2PTURNServer struct {
	URL        string `json:"url"`
	Username   string `json:"username"`
	Credential string `json:"credential"`
}

// P2PSessionConfig carries per-session ICE tuning parameters from the frontend.
type P2PSessionConfig struct {
	TURNServers       []P2PTURNServer `json:"turnServers"`
	TCPMuxPort        int             `json:"tcpMuxPort"`
	GatherTimeoutSecs int             `json:"gatherTimeoutSecs"`
}

// RelayCredentials is returned by RelayGetCredentials.
type RelayCredentials struct {
	TURNAddr    string `json:"turnAddr"`    // UDP transport URL
	TURNTCPAddr string `json:"turnTcpAddr"` // TCP transport URL (empty if TCP listener failed)
	Username    string `json:"username"`
	Password    string `json:"password"`
	IsRunning   bool   `json:"isRunning"`
}

// ── App ───────────────────────────────────────────────────────────────────────

// App is the main application struct. All exported methods become async
// functions callable from the Vue frontend via Wails bindings.
type App struct {
	ctx             context.Context
	tm              *tunnel.Manager
	p2pSess         *p2p.P2PSession
	p2pCancel       context.CancelFunc
	p2pConfig       P2PSessionConfig // guarded by mu
	relay           *relay.Relay
	mu              sync.Mutex
	sshInstaller    *sshsetup.Installer
	setupCancelFunc context.CancelFunc
	setupBusy       bool // guarded by mu
}

// NewApp creates a new App.
func NewApp() *App { return &App{} }

// startup is the Wails lifecycle hook called once the WebView window is ready.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.relay = relay.New(func(msg string) {
		ts := time.Now().Format("15:04:05")
		runtime.EventsEmit(ctx, "relay:log", ts+" - "+msg)
	})
	a.tm = &tunnel.Manager{
		OnStatus: func(s tunnel.Status) {
			runtime.EventsEmit(ctx, "tunnel:status", s.String())
		},
		OnLog: func(msg string) {
			ts := time.Now().Format("15:04:05")
			runtime.EventsEmit(ctx, "tunnel:log", ts+" - "+msg)
		},
	}
	a.setupTray()
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

// P2PSetConfig stores per-session ICE tuning parameters.  Call before
// P2PGenerateOffer or P2PProvideAnswer.  Safe to call at any time.
func (a *App) P2PSetConfig(cfg P2PSessionConfig) {
	a.mu.Lock()
	a.p2pConfig = cfg
	a.mu.Unlock()
}

// RelayStart starts the embedded TURN relay on the given UDP port.
// Returns "" on success, an error string on failure.
func (a *App) RelayStart(port int) string {
	if err := a.relay.Start(port); err != nil {
		return err.Error()
	}
	return ""
}

// RelayStop shuts down the embedded TURN relay.  Safe when not running.
func (a *App) RelayStop() { a.relay.Stop() }

// RelayIsRunning reports whether the embedded TURN relay is running.
func (a *App) RelayIsRunning() bool { return a.relay.IsRunning() }

// GetPublicIP returns the machine's current public IP as discovered via STUN
// when the relay was last started.  Returns empty string if the relay has
// never been started or is not running.  The frontend displays this to guide
// the user through firewall/port-forward setup for cross-network TURN use.
func (a *App) GetPublicIP() string {
	return a.relay.PublicIP()
}

// RelayGetCredentials returns the current TURN relay address and short-lived
// credentials.  All fields are empty strings when the relay is not running.
func (a *App) RelayGetCredentials() RelayCredentials {
	user, pass := a.relay.Credentials()
	return RelayCredentials{
		TURNAddr:    a.relay.TURNAddr(),
		TURNTCPAddr: a.relay.TURNTCPAddr(),
		Username:    user,
		Password:    pass,
		IsRunning:   a.relay.IsRunning(),
	}
}

func (a *App) newP2PSession() *p2p.P2PSession {
	a.mu.Lock()
	cfg := a.p2pConfig
	a.mu.Unlock()

	serverCfg := p2p.SessionConfig{
		TCPMuxPort:    cfg.TCPMuxPort,
		GatherTimeout: time.Duration(cfg.GatherTimeoutSecs) * time.Second,
	}
	for _, t := range cfg.TURNServers {
		serverCfg.TURNServers = append(serverCfg.TURNServers, p2p.TURNServer{
			URL:        t.URL,
			Username:   t.Username,
			Credential: t.Credential,
		})
	}

	ctx := a.ctx
	return p2p.NewP2PSessionWithConfig(
		serverCfg,
		func(s string) { runtime.EventsEmit(ctx, "p2p:status", s) },
		func(msg string) {
			ts := time.Now().Format("15:04:05")
			runtime.EventsEmit(ctx, "p2p:log", ts+" - "+msg)
		},
	)
}

// shutdown stops all active connections and tears down the tray.
// Called from OnShutdown in main.go via both the tray Exit action and the OS-level quit.
func (a *App) shutdown() {
	// 1. Stop tunnel — kills the SSH / wsl.exe process
	if a.tm != nil {
		a.tm.Stop()
	}
	// 2. Stop P2P session
	a.P2PStop()
	// 3. Stop relay
	if a.relay != nil {
		a.relay.Stop()
	}
	// 4. Tear down tray
	a.teardownTray()
}

// ── SSH Key Setup ─────────────────────────────────────────────────────────────

// SSHSetupStart begins the one-time SSH key installation asynchronously.
// It generates an app key if none exists, then tries to install it on the
// remote server. Interactive prompts (fingerprint, password) are emitted as
// "setup:prompt" events; the frontend must call SSHSetupReply to unblock.
// Progress is logged via "setup:log" events. Completion is signalled via
// "setup:done".
// Returns "" on successful start (async), or an error string if preconditions fail.
func (a *App) SSHSetupStart(s TunnelSettings) string {
	a.mu.Lock()
	if a.setupBusy {
		a.mu.Unlock()
		return "setup already in progress"
	}
	if !keygen.AppKeyExists() {
		if err := keygen.GenerateAppKey(); err != nil {
			a.mu.Unlock()
			return fmt.Sprintf("generate app key: %v", err)
		}
	}
	pubKey, err := keygen.LoadAppPublicKey()
	if err != nil {
		a.mu.Unlock()
		return fmt.Sprintf("load public key: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.setupCancelFunc = cancel
	a.setupBusy = true

	wCtx := a.ctx // Wails context for EventsEmit
	installer := sshsetup.New(
		func(kind sshsetup.PromptKind, message string) {
			fp := ""
			if kind == sshsetup.PromptKindFingerprint {
				fp = message
			}
			runtime.EventsEmit(wCtx, "setup:prompt", SetupPrompt{
				Kind:        string(kind),
				Message:     message,
				Fingerprint: fp,
			})
		},
		func(msg string) {
			ts := time.Now().Format("15:04:05")
			runtime.EventsEmit(wCtx, "setup:log", ts+" - "+msg)
		},
	)
	a.sshInstaller = installer
	a.mu.Unlock()

	go func() {
		runErr := installer.Run(ctx, sshsetup.Config{
			Host:      s.Host,
			Port:      s.Port,
			User:      s.User,
			PublicKey: pubKey,
		})
		a.mu.Lock()
		a.setupBusy = false
		a.sshInstaller = nil
		a.mu.Unlock()

		result := SetupResult{OK: runErr == nil}
		if runErr != nil {
			result.Message = runErr.Error()
		} else {
			result.Message = "Public key installed successfully."
		}
		runtime.EventsEmit(wCtx, "setup:done", result)
	}()

	return ""
}

// SSHSetupReply forwards the user's answer (e.g. "yes" or a password) to the
// currently blocked sshsetup.Installer.Reply channel.
func (a *App) SSHSetupReply(answer string) {
	a.mu.Lock()
	installer := a.sshInstaller
	a.mu.Unlock()
	if installer != nil {
		installer.Reply(answer)
	}
}

// SSHSetupCancel aborts a running setup.
func (a *App) SSHSetupCancel() {
	a.mu.Lock()
	cancel := a.setupCancelFunc
	a.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// SSHSetupBusy reports whether a key setup is in progress.
func (a *App) SSHSetupBusy() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.setupBusy
}

// SSHCheckKeyAuth attempts a short no-op SSH dial using key authentication only
// (no password). Returns "" if auth succeeds, error string otherwise.
// This is used by the frontend to detect whether the installed key is accepted.
func (a *App) SSHCheckKeyAuth(s TunnelSettings) string {
	keyPath := keygen.AppPrivateKeyPath()
	keyBytes, err := os.ReadFile(keyPath)
	if err != nil {
		return fmt.Sprintf("read private key: %v", err)
	}
	signer, err := gossh.ParsePrivateKey(keyBytes)
	if err != nil {
		return fmt.Sprintf("parse private key: %v", err)
	}

	port := s.Port
	if port == "" {
		port = "22"
	}
	addr := fmt.Sprintf("%s:%s", s.Host, port)

	clientCfg := &gossh.ClientConfig{
		User: s.User,
		Auth: []gossh.AuthMethod{
			gossh.PublicKeys(signer),
		},
		// Accept any host key — we're checking auth, not host identity.
		HostKeyCallback: gossh.InsecureIgnoreHostKey(), //nolint:gosec
		Timeout:         10 * time.Second,
	}

	client, err := gossh.Dial("tcp", addr, clientCfg)
	if err != nil {
		if isAuthError(err.Error()) {
			return "key authentication failed: server did not accept the app key"
		}
		return err.Error()
	}
	client.Close()
	return ""
}

// isAuthError returns true when the SSH error message indicates an authentication failure.
func isAuthError(msg string) bool {
	return containsFold(msg, "unable to authenticate") ||
		containsFold(msg, "handshake failed") ||
		containsFold(msg, "no supported methods")
}

func containsFold(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
