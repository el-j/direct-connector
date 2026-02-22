package main

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

// TunnelStatus represents the current state of the SSH tunnel.
type TunnelStatus int

const (
	StatusDisconnected TunnelStatus = iota
	StatusConnecting
	StatusConnected
	StatusReconnecting
)

func (s TunnelStatus) String() string {
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

// TunnelConfig holds all parameters needed to establish the SSH tunnel.
type TunnelConfig struct {
	Host         string
	Port         string
	ForwardPorts string
	User         string
	IPv6         bool
}

// TunnelManager manages the lifecycle of an SSH tunnel with automatic reconnection.
// It is safe for concurrent use.
type TunnelManager struct {
	mu         sync.Mutex
	running    bool
	sshCmd     *exec.Cmd
	cancelFunc context.CancelFunc

	// OnStatus is called from the monitor goroutine whenever the status changes.
	// Callers must dispatch to the UI thread themselves if needed.
	OnStatus func(TunnelStatus)

	// OnLog is called from the monitor goroutine on every log line.
	// Callers must dispatch to the UI thread themselves if needed.
	OnLog func(string)
}

// IsRunning returns whether the tunnel is currently active.
func (t *TunnelManager) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// Start launches the monitor goroutine. Calling Start while already running is a no-op.
func (t *TunnelManager) Start(cfg TunnelConfig) {
	t.mu.Lock()
	if t.running {
		t.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.running = true
	t.cancelFunc = cancel
	t.mu.Unlock()

	go t.monitorLoop(ctx, cfg)
}

// Stop terminates the SSH process and the monitor goroutine.
func (t *TunnelManager) Stop() {
	t.mu.Lock()
	t.running = false
	if t.cancelFunc != nil {
		t.cancelFunc()
	}
	if t.sshCmd != nil && t.sshCmd.Process != nil {
		_ = t.sshCmd.Process.Kill()
	}
	t.mu.Unlock()

	t.emitStatus(StatusDisconnected)
	t.emitLog("Tunnel manually stopped.")
}

// monitorLoop is the background goroutine that repeatedly (re)connects.
func (t *TunnelManager) monitorLoop(ctx context.Context, cfg TunnelConfig) {
	defer func() {
		t.mu.Lock()
		t.running = false
		t.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		t.emitStatus(StatusConnecting)
		t.emitLog(fmt.Sprintf("Connecting to %s:%s...", cfg.Host, cfg.Port))

		args, err := buildSSHArgs(cfg)
		if err != nil {
			t.emitLog(fmt.Sprintf("Invalid configuration: %v", err))
			t.emitStatus(StatusDisconnected)
			return
		}

		cmd := exec.CommandContext(ctx, "ssh", args...)

		stdout, err := cmd.StdoutPipe()
		if err != nil {
			t.emitLog(fmt.Sprintf("StdoutPipe error: %v", err))
			t.waitOrCancel(ctx, 5*time.Second)
			continue
		}
		stderr, err := cmd.StderrPipe()
		if err != nil {
			t.emitLog(fmt.Sprintf("StderrPipe error: %v", err))
			t.waitOrCancel(ctx, 5*time.Second)
			continue
		}

		t.mu.Lock()
		t.sshCmd = cmd
		t.mu.Unlock()

		if startErr := cmd.Start(); startErr != nil {
			t.emitLog(fmt.Sprintf("Failed to start SSH: %v", startErr))
			t.waitOrCancel(ctx, 5*time.Second)
			continue
		}

		t.emitStatus(StatusConnected)

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); t.streamLogs(stdout) }()
		go func() { defer wg.Done(); t.streamLogs(stderr) }()

		waitErr := cmd.Wait()
		wg.Wait()

		// Check whether we are still supposed to be running before logging/reconnecting.
		t.mu.Lock()
		stillRunning := t.running
		t.mu.Unlock()

		if !stillRunning {
			return
		}

		if waitErr != nil {
			t.emitLog(fmt.Sprintf("SSH exited: %v", waitErr))
		}

		t.emitStatus(StatusReconnecting)
		t.emitLog("Connection lost. Retrying in 5 seconds...")
		t.waitOrCancel(ctx, 5*time.Second)
	}
}

// waitOrCancel sleeps for d or returns early when ctx is cancelled.
func (t *TunnelManager) waitOrCancel(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// streamLogs reads line-by-line from stream and forwards to OnLog.
func (t *TunnelManager) streamLogs(stream io.ReadCloser) {
	scanner := bufio.NewScanner(stream)
	for scanner.Scan() {
		t.emitLog("SSH: " + scanner.Text())
	}
}

// emitStatus calls OnStatus if it is set.
func (t *TunnelManager) emitStatus(s TunnelStatus) {
	if t.OnStatus != nil {
		t.OnStatus(s)
	}
}

// emitLog calls OnLog if it is set.
func (t *TunnelManager) emitLog(msg string) {
	if t.OnLog != nil {
		t.OnLog(msg)
	}
}

// buildSSHArgs constructs the ssh argument slice from a TunnelConfig.
// It validates all ports and returns an error if any are invalid.
func buildSSHArgs(cfg TunnelConfig) ([]string, error) {
	// Validate SSH port
	if _, err := strconv.Atoi(cfg.Port); err != nil {
		return nil, fmt.Errorf("SSH port %q is not a valid number", cfg.Port)
	}

	args := make([]string, 0, 16)
	if cfg.IPv6 {
		args = append(args, "-6")
	}
	args = append(args,
		"-p", cfg.Port,
		"-N", // No remote commands — port-forwarding only
		"-o", "ServerAliveInterval=30",
		"-o", "ServerAliveCountMax=3",
		"-o", "ExitOnForwardFailure=yes",
		"-o", "StrictHostKeyChecking=accept-new",
		"-o", "BatchMode=yes", // Never prompt interactively; fail fast instead
	)

	// Parse comma-separated forward ports
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
		args = append(args, "-R", fmt.Sprintf("%s:localhost:%s", p, p))
		forwarded++
	}
	if forwarded == 0 {
		return nil, fmt.Errorf("no valid forward ports specified")
	}

	args = append(args, fmt.Sprintf("%s@%s", cfg.User, cfg.Host))
	return args, nil
}
