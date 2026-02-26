package tunnel

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/pem"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

// TestManager_Integration starts a minimal in-process SSH server and verifies
// that Manager transitions to StatusConnected and then StatusDisconnected.
func TestManager_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if _, err := exec.LookPath("ssh"); err != nil {
		t.Skip("ssh binary not found in PATH")
	}

	// ── Generate RSA host key for the test server ─────────────────────────
	hostKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate host key: %v", err)
	}
	hostSigner, err := gossh.NewSignerFromKey(hostKey)
	if err != nil {
		t.Fatalf("host signer: %v", err)
	}

	// ── Generate Ed25519 client key and write to temp file ────────────────
	_, clientKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate client key: %v", err)
	}
	tmpDir := t.TempDir()
	keyPath := filepath.Join(tmpDir, "id_ed25519")
	pemBlock, err := gossh.MarshalPrivateKey(clientKey, "")
	if err != nil {
		t.Fatalf("marshal private key: %v", err)
	}
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		t.Fatalf("create key file: %v", err)
	}
	if encErr := pem.Encode(keyFile, pemBlock); encErr != nil {
		keyFile.Close()
		t.Fatalf("write key PEM: %v", encErr)
	}
	keyFile.Close()

	// ── Configure and start the test SSH server ───────────────────────────
	serverCfg := &gossh.ServerConfig{
		// Accept any public key for testing purposes.
		PublicKeyCallback: func(_ gossh.ConnMetadata, _ gossh.PublicKey) (*gossh.Permissions, error) {
			return &gossh.Permissions{}, nil
		},
	}
	serverCfg.AddHostKey(hostSigner)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	serverPort := ln.Addr().(*net.TCPAddr).Port
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, acceptErr := ln.Accept()
			if acceptErr != nil {
				return
			}
			go integrationServeConn(conn, serverCfg)
		}
	}()

	// ── Redirect HOME so SSH uses an isolated known_hosts ─────────────────
	sshDir := filepath.Join(tmpDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0700); err != nil {
		t.Fatalf("mkdir .ssh: %v", err)
	}
	origHome := os.Getenv("HOME")
	os.Setenv("HOME", tmpDir)
	t.Cleanup(func() { os.Setenv("HOME", origHome) })

	// ── Start tunnel.Manager ──────────────────────────────────────────────
	statuses := make(chan Status, 20)
	m := &Manager{
		OnStatus: func(s Status) {
			select {
			case statuses <- s:
			default:
			}
		},
		OnLog: func(msg string) { t.Logf("tunnel: %s", msg) },
	}
	m.Start(Config{
		Host:         "127.0.0.1",
		Port:         strconv.Itoa(serverPort),
		ForwardPorts: "19999",
		User:         "testuser",
		KeyPath:      keyPath,
	})
	t.Cleanup(func() { m.Stop() })

	// ── Wait for StatusConnected (up to 10 s) ─────────────────────────────
	deadline := time.After(10 * time.Second)
waitConnected:
	for {
		select {
		case s := <-statuses:
			t.Logf("status: %s", s)
			if s == StatusConnected {
				break waitConnected
			}
		case <-deadline:
			t.Fatal("timed out waiting for StatusConnected")
		}
	}

	// ── Stop and verify StatusDisconnected ───────────────────────────────
	m.Stop()
	deadline2 := time.After(5 * time.Second)
	for {
		select {
		case s := <-statuses:
			t.Logf("status after stop: %s", s)
			if s == StatusDisconnected {
				return
			}
		case <-deadline2:
			t.Fatal("timed out waiting for StatusDisconnected after Stop()")
		}
	}
}

// integrationServeConn handles a single SSH connection for the test server.
// It rejects all incoming SSH channel requests and handles tcpip-forward global requests.
func integrationServeConn(conn net.Conn, cfg *gossh.ServerConfig) {
	sConn, chans, reqs, err := gossh.NewServerConn(conn, cfg)
	if err != nil {
		return
	}
	defer sConn.Close()

	// Use a context so we can stop the goroutines when the connection closes.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		defer cancel()
		for {
			select {
			case req, ok := <-reqs:
				if !ok {
					return
				}
				switch req.Type {
				case "tcpip-forward":
					// Acknowledge the reverse-forward request so SSH stays connected.
					if req.WantReply {
						req.Reply(true, nil)
					}
				default:
					if req.WantReply {
						req.Reply(false, nil)
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	for {
		select {
		case newChan, ok := <-chans:
			if !ok {
				return
			}
			newChan.Reject(gossh.UnknownChannelType, "not supported")
		case <-ctx.Done():
			return
		}
	}
}
