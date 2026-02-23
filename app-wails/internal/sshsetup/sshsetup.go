// Package sshsetup performs a one-time SSH public-key installation onto a
// remote server using interactive prompts for host-key verification and
// password authentication.
package sshsetup

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// PromptKind distinguishes between interactive prompt types.
type PromptKind string

const (
	PromptKindFingerprint PromptKind = "fingerprint"
	PromptKindPassword    PromptKind = "password"
)

// Config holds the parameters for the one-time key installation.
type Config struct {
	Host      string // SSH server hostname
	Port      string // SSH server port (e.g. "443")
	User      string // SSH username
	PublicKey string // The full authorized_keys line to install
}

// Installer performs the one-time SSH public key installation.
// It is NOT safe for concurrent use; create a new instance per operation.
type Installer struct {
	OnPrompt func(kind PromptKind, message string) // called to emit prompt event to UI
	OnLog    func(string)                          // called to emit log messages
	replyCh  chan string                           // receives user replies
}

// New returns a new Installer.
func New(onPrompt func(PromptKind, string), onLog func(string)) *Installer {
	return &Installer{
		OnPrompt: onPrompt,
		OnLog:    onLog,
		replyCh:  make(chan string, 1),
	}
}

// Reply sends the user's answer for the current blocked prompt.
// Safe to call from any goroutine. No-op if no prompt is pending (non-blocking).
func (i *Installer) Reply(answer string) {
	select {
	case i.replyCh <- answer:
	default:
	}
}

// Run connects to the SSH server, confirms host key interactively,
// authenticates with password interactively, then runs an idempotent shell
// command to append the public key to ~/.ssh/authorized_keys.
// Returns nil on success, wrapped error on failure.
// Respects ctx cancellation.
func (i *Installer) Run(ctx context.Context, cfg Config) error {
	if cfg.Host == "" {
		return fmt.Errorf("sshsetup: host is required")
	}
	if cfg.Port == "" {
		cfg.Port = "22"
	}
	if cfg.User == "" {
		cfg.User = "root"
	}

	addr := net.JoinHostPort(cfg.Host, cfg.Port)
	i.log("Connecting to %s ...", addr)

	// hostKeyCallback is called exactly once by the SSH handshake.
	// It blocks until the user confirms or cancels.
	hostKeyCallback := func(hostname string, remote net.Addr, key gossh.PublicKey) error {
		fingerprint := gossh.FingerprintSHA256(key)
		msg := fmt.Sprintf(
			"The authenticity of host '%s' cannot be established.\n"+
				"Key fingerprint: %s\n"+
				"Do you want to continue connecting? (yes/no)",
			hostname, fingerprint,
		)
		i.OnPrompt(PromptKindFingerprint, msg)

		select {
		case reply := <-i.replyCh:
			if strings.TrimSpace(strings.ToLower(reply)) != "yes" {
				return fmt.Errorf("sshsetup: host key rejected by user")
			}
			i.log("Host key accepted.")
			return nil
		case <-ctx.Done():
			return fmt.Errorf("sshsetup: cancelled during host key verification: %w", ctx.Err())
		}
	}

	// passwordCallback is invoked by the x/crypto/ssh library when the server
	// requests password authentication.
	passwordCallback := func() (secret string, err error) {
		i.log("Server is requesting password authentication.")
		i.OnPrompt(PromptKindPassword, fmt.Sprintf("Password for %s@%s:", cfg.User, cfg.Host))

		select {
		case pw := <-i.replyCh:
			return pw, nil
		case <-ctx.Done():
			return "", fmt.Errorf("sshsetup: cancelled during password prompt: %w", ctx.Err())
		}
	}

	clientCfg := &gossh.ClientConfig{
		User: cfg.User,
		Auth: []gossh.AuthMethod{
			gossh.PasswordCallback(passwordCallback),
		},
		HostKeyCallback: hostKeyCallback,
		Timeout:         30 * time.Second,
	}

	// Check for cancellation before dialing.
	select {
	case <-ctx.Done():
		return fmt.Errorf("sshsetup: cancelled before dial: %w", ctx.Err())
	default:
	}

	client, err := gossh.Dial("tcp", addr, clientCfg)
	if err != nil {
		return fmt.Errorf("sshsetup: dial %s: %w", addr, err)
	}
	defer client.Close()

	i.log("Authenticated successfully. Installing public key ...")

	// Escape single-quotes in the public key for safe shell embedding.
	safeKey := strings.ReplaceAll(cfg.PublicKey, `'`, `'\''`)

	cmd := fmt.Sprintf(
		"mkdir -p ~/.ssh && chmod 700 ~/.ssh && "+
			"grep -qxF '%s' ~/.ssh/authorized_keys 2>/dev/null || "+
			"echo '%s' >> ~/.ssh/authorized_keys && "+
			"chmod 600 ~/.ssh/authorized_keys",
		safeKey, safeKey,
	)

	session, err := client.NewSession()
	if err != nil {
		return fmt.Errorf("sshsetup: new session: %w", err)
	}
	defer session.Close()

	// Run the command in a goroutine so we can respect ctx cancellation.
	type runResult struct {
		err error
	}
	done := make(chan runResult, 1)
	go func() {
		done <- runResult{err: session.Run(cmd)}
	}()

	select {
	case res := <-done:
		if res.err != nil {
			return fmt.Errorf("sshsetup: install command: %w", res.err)
		}
	case <-ctx.Done():
		// Ask the server to end the remote process gracefully.
		_ = session.Signal(gossh.SIGTERM)
		return fmt.Errorf("sshsetup: cancelled during key install: %w", ctx.Err())
	}

	i.log("Public key installed successfully.")
	return nil
}

// log is a convenience helper that calls OnLog if set.
func (i *Installer) log(format string, args ...any) {
	if i.OnLog == nil {
		return
	}
	if len(args) == 0 {
		i.OnLog(format)
		return
	}
	i.OnLog(fmt.Sprintf(format, args...))
}
