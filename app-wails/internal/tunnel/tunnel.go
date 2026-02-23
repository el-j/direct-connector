// Package tunnel manages the lifecycle of a persistent reverse-SSH tunnel.
package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Status represents the current state of the SSH tunnel.
type Status int

const (
	StatusDisconnected Status = iota
	StatusConnecting
	StatusConnected
	StatusReconnecting
)

// String returns a human-readable label for the status, suitable for UI display.
func (s Status) String() string {
	switch s {
	case StatusConnecting:
		return "Status: Connecting..."
	case StatusConnected:
		return "Status: Connected & Active"
	case StatusReconnecting:
		return "Status: Reconnecting..."
	default:
		return "Status: Disconnected"
	}
}

// Config holds all parameters needed to establish the SSH tunnel.
type Config struct {
	Host         string
	Port         string
	ForwardPorts string
	User         string
	IPVersion    string // "" = auto, "4" = force IPv4 (-4), "6" = force IPv6 (-6)
	KeyPath      string // path to private key file; empty = use SSH default (~/.ssh/id_*)
	ForwardMode  string // "" or "R" = remote/reverse forward (-R, default); "L" = local forward (-L)
	Verbose      bool   // when true, pass -v to SSH for verbose debug output
	UseWSLSsh    bool   // Windows only: invoke 'wsl.exe ssh' so SSH runs inside the WSL network namespace
}

// Manager manages the lifecycle of an SSH tunnel with automatic reconnection.
// It is safe for concurrent use.
type Manager struct {
	mu         sync.Mutex
	running    bool
	sshCmd     *exec.Cmd
	cancelFunc context.CancelFunc

	// OnStatus is called from the monitor goroutine whenever the status changes.
	// Callers must dispatch to the UI thread themselves if needed.
	OnStatus func(Status)

	// OnLog is called from the monitor goroutine on every log line.
	// Callers must dispatch to the UI thread themselves if needed.
	OnLog func(string)
}

// IsRunning returns whether the tunnel is currently active.
func (m *Manager) IsRunning() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running
}

// Start launches the monitor goroutine. Calling Start while already running is a no-op.
func (m *Manager) Start(cfg Config) {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.running = true
	m.cancelFunc = cancel
	m.mu.Unlock()

	go m.monitorLoop(ctx, cfg)
}

// Stop terminates the SSH process and the monitor goroutine.
func (m *Manager) Stop() {
	m.mu.Lock()
	m.running = false
	if m.cancelFunc != nil {
		m.cancelFunc()
	}
	if m.sshCmd != nil && m.sshCmd.Process != nil {
		pid := m.sshCmd.Process.Pid
		_ = m.sshCmd.Process.Kill()
		killProcessTree(pid) // Windows: kills wsl.exe + all children; no-op elsewhere
	}
	m.mu.Unlock()

	m.emitStatus(StatusDisconnected)
	m.emitLog("Tunnel manually stopped.")
}

// monitorLoop is the background goroutine that repeatedly (re)connects.
func (m *Manager) monitorLoop(ctx context.Context, cfg Config) {
	defer func() {
		m.mu.Lock()
		m.running = false
		m.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		m.emitStatus(StatusConnecting)
		m.emitLog(fmt.Sprintf("Connecting to %s:%s...", cfg.Host, cfg.Port))

		args, err := BuildSSHArgs(cfg)
		if err != nil {
			m.emitLog(fmt.Sprintf("Invalid configuration: %v", err))
			m.emitStatus(StatusDisconnected)
			return
		}

		// Log the exact command being run so the user can compare / debug.
		// On Windows with UseWSLSsh, invoke 'wsl.exe ssh' so the SSH process runs
		// inside the WSL network namespace — the same context where Docker/MQTT lives.
		binary := "ssh"
		execArgs := args
		if cfg.UseWSLSsh {
			binary = "wsl.exe"
			execArgs = append([]string{"ssh"}, args...)
			m.emitLog("Running: wsl.exe ssh " + strings.Join(args, " "))
		} else {
			m.emitLog("Running: ssh " + strings.Join(args, " "))
		}

		cmd := exec.CommandContext(ctx, binary, execArgs...)
		hideWindow(cmd) // no-op on macOS/Linux; suppresses console popup on Windows

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			m.emitLog(fmt.Sprintf("StdoutPipe error: %v", err))
			m.waitOrCancel(ctx, 5*time.Second)
			continue
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			m.emitLog(fmt.Sprintf("StderrPipe error: %v", err))
			m.waitOrCancel(ctx, 5*time.Second)
			continue
		}

		m.mu.Lock()
		m.sshCmd = cmd
		m.mu.Unlock()

		if startErr := cmd.Start(); startErr != nil {
			m.emitLog(fmt.Sprintf("Failed to start SSH: %v", startErr))
			if cfg.UseWSLSsh {
				m.emitLog("Hint: 'wsl.exe' not found — make sure WSL is installed on this Windows machine.")
			} else {
				m.emitLog("Hint: make sure 'ssh' is installed and in your PATH.")
			}
			m.waitOrCancel(ctx, 5*time.Second)
			continue
		}

		// Don't declare Connected immediately — wait a moment to see if SSH
		// dies instantly (auth failure, bad host key, etc.).
		connectedAt := time.Now()
		m.emitStatus(StatusConnecting)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); m.streamLogs(stdout) }()
		go func() { defer wg.Done(); m.streamLogs(stderr) }()

		// Mark as connected after 2 s — if SSH exits before that we never
		// show "Connected" (avoids a misleading flash on Windows auth failures).
		connectTimer := time.AfterFunc(2*time.Second, func() {
			m.mu.Lock()
			running := m.running
			m.mu.Unlock()
			if running {
				m.emitStatus(StatusConnected)
			}
		})

		waitErr := cmd.Wait()
		connectTimer.Stop()
		uptime := time.Since(connectedAt)
		wg.Wait()

		// Check whether we are still supposed to be running before logging/reconnecting.
		m.mu.Lock()
		stillRunning := m.running
		m.mu.Unlock()

		if !stillRunning {
			return
		}

		if waitErr != nil {
			m.emitLog(fmt.Sprintf("SSH exited: %v", waitErr))
		}

		// If SSH died very quickly it is almost certainly a config/auth problem,
		// not a transient network blip — give the user an actionable hint.
		if uptime < 3*time.Second {
			m.emitLog("─── SSH exited in under 3 s — likely causes: ───")
			m.emitLog("  • Key not authorized: copy the public key from the SSH Key card")
			m.emitLog("    and paste it into ~/.ssh/authorized_keys on the server.")
			m.emitLog("  • Wrong username or host.")
			m.emitLog("  • Server not listening on port " + cfg.Port + ".")
			m.emitLog("  • Host key mismatch: delete ~/.ssh/known_hosts entry for " + cfg.Host + ".")
			m.emitLog("────────────────────────────────────────────────")
		}

		m.emitStatus(StatusReconnecting)
		m.emitLog("Connection lost. Retrying in 5 seconds...")
		m.waitOrCancel(ctx, 5*time.Second)
	}
}

// waitOrCancel sleeps for d or returns early when ctx is cancelled.
func (m *Manager) waitOrCancel(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// streamLogs reads line-by-line from stream and forwards to OnLog.
func (m *Manager) streamLogs(stream io.ReadCloser) {
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		m.emitLog("SSH: " + scanner.Text())
	}
}

// emitStatus calls OnStatus if it is set.
func (m *Manager) emitStatus(s Status) {
	if m.OnStatus != nil {
		m.OnStatus(s)
	}
}

// emitLog calls OnLog if it is set.
func (m *Manager) emitLog(msg string) {
	if m.OnLog != nil {
		m.OnLog(msg)
	}
}

// BuildSSHArgs constructs the ssh argument slice from a Config.
// It validates all ports and returns an error if any are invalid.
func BuildSSHArgs(cfg Config) ([]string, error) {
	// Validate SSH port.
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return nil, fmt.Errorf("SSH port %q is not a valid number", cfg.Port)
	}

	args := make([]string, 0, 20)
	switch cfg.IPVersion {
	case "4":
		args = append(args, "-4")
	case "6":
		args = append(args, "-6")
		// default "": let SSH choose
	}
	if cfg.KeyPath != "" {
		args = append(args, "-i", cfg.KeyPath)
	}
	fwdFlag := "-R" // remote/reverse forward by default
	if strings.ToUpper(cfg.ForwardMode) == "L" {
		fwdFlag = "-L"
	}
	baseArgs := []string{
		"-p", cfg.Port,
		"-N", // No remote commands — port-forwarding only
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StrictHostKeyChecking=accept-new",
	}
	if cfg.Verbose {
		baseArgs = append(baseArgs, "-v") // verbose: SSH logs auth method and forwarding details
	}
	args = append(args, baseArgs...)

	// Parse comma-separated forward ports.
	rawPorts := strings.Split(cfg.ForwardPorts, ",")
	forwarded := 0
	for _, raw := range rawPorts {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if _, err := strconv.Atoi(p); err != nil {
			return nil, fmt.Errorf("forward port %q is not a valid number", p)
		}
		args = append(args, fwdFlag, fmt.Sprintf("%s:localhost:%s", p, p))
		forwarded++
	}
	if forwarded == 0 {
		return nil, fmt.Errorf("no valid forward ports specified")
	}

	args = append(args, fmt.Sprintf("%s@%s", cfg.User, cfg.Host))
	return args, nil
}
