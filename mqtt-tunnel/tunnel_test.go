package main

import (
	"strings"
	"testing"
	"time"
)

// ── buildSSHArgs ──────────────────────────────────────────────────────────────

func TestBuildSSHArgs_Valid(t *testing.T) {
	cfg := TunnelConfig{
		Host:         "example.com",
		Port:         "22",
		ForwardPorts: "1883, 3391",
		User:         "alice",
		IPv6:         false,
	}
	args, err := buildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Must contain destination user@host as last element
	last := args[len(args)-1]
	if last != "alice@example.com" {
		t.Errorf("expected last arg 'alice@example.com', got %q", last)
	}

	// Must contain -R flags for both ports
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-R 1883:localhost:1883") {
		t.Errorf("missing -R for port 1883; args: %v", args)
	}
	if !strings.Contains(joined, "-R 3391:localhost:3391") {
		t.Errorf("missing -R for port 3391; args: %v", args)
	}

	// Must NOT contain -6 when IPv6 is off
	if strings.Contains(joined, "-6") {
		t.Errorf("unexpected -6 flag when IPv6 is false; args: %v", args)
	}

	// Must contain -N (no remote commands)
	if !strings.Contains(joined, " -N ") && !strings.HasSuffix(joined, " -N") {
		// Allow -N anywhere
		if !contains(args, "-N") {
			t.Errorf("expected -N in args; args: %v", args)
		}
	}
}

func TestBuildSSHArgs_IPv6(t *testing.T) {
	cfg := TunnelConfig{
		Host:         "example.com",
		Port:         "8080",
		ForwardPorts: "1883",
		User:         "bob",
		IPv6:         true,
	}
	args, err := buildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args[0] != "-6" {
		t.Errorf("expected -6 as first arg when IPv6=true, got %q", args[0])
	}
}

func TestBuildSSHArgs_InvalidSSHPort(t *testing.T) {
	cfg := TunnelConfig{
		Host:         "example.com",
		Port:         "notaport",
		ForwardPorts: "1883",
		User:         "alice",
	}
	_, err := buildSSHArgs(cfg)
	if err == nil {
		t.Error("expected error for non-numeric SSH port, got nil")
	}
}

func TestBuildSSHArgs_InvalidForwardPort(t *testing.T) {
	cfg := TunnelConfig{
		Host:         "example.com",
		Port:         "22",
		ForwardPorts: "abc",
		User:         "alice",
	}
	_, err := buildSSHArgs(cfg)
	if err == nil {
		t.Error("expected error for non-numeric forward port, got nil")
	}
}

func TestBuildSSHArgs_EmptyForwardPorts(t *testing.T) {
	cfg := TunnelConfig{
		Host:         "example.com",
		Port:         "22",
		ForwardPorts: "  ,  ,  ",
		User:         "alice",
	}
	_, err := buildSSHArgs(cfg)
	if err == nil {
		t.Error("expected error when all forward ports are blank, got nil")
	}
}

func TestBuildSSHArgs_BatchModePresent(t *testing.T) {
	cfg := TunnelConfig{Host: "h", Port: "22", ForwardPorts: "1883", User: "u"}
	args, err := buildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsPair(args, "-o", "BatchMode=yes") {
		t.Errorf("expected -o BatchMode=yes in args; args: %v", args)
	}
}

func TestBuildSSHArgs_ServerAliveOptions(t *testing.T) {
	cfg := TunnelConfig{Host: "h", Port: "22", ForwardPorts: "1883", User: "u"}
	args, err := buildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsPair(args, "-o", "ServerAliveInterval=30") {
		t.Errorf("missing ServerAliveInterval; args: %v", args)
	}
	if !containsPair(args, "-o", "ServerAliveCountMax=3") {
		t.Errorf("missing ServerAliveCountMax; args: %v", args)
	}
}

// ── TunnelManager ─────────────────────────────────────────────────────────────

func TestTunnelManager_IsRunning_InitiallyFalse(t *testing.T) {
	tm := &TunnelManager{}
	if tm.IsRunning() {
		t.Error("new TunnelManager should not be running")
	}
}

func TestTunnelManager_StopWhileNotRunning(t *testing.T) {
	tm := &TunnelManager{}
	// Must not panic or deadlock
	done := make(chan struct{})
	go func() {
		tm.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Stop() deadlocked when called on an idle TunnelManager")
	}
}

func TestTunnelManager_OnLog_CalledOnStart(t *testing.T) {
	logs := make(chan string, 10)
	tm := &TunnelManager{
		OnLog: func(msg string) { logs <- msg },
	}

	// Use an invalid config so SSH exits immediately without network access.
	tm.Start(TunnelConfig{
		Host:         "127.0.0.1",
		Port:         "1", // port 1 is privileged, will fail fast
		ForwardPorts: "9999",
		User:         "nobody",
	})

	// Give the goroutine a moment to emit at least the "Connecting" log.
	select {
	case msg := <-logs:
		if msg == "" {
			t.Error("expected non-empty log message")
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for OnLog to be called")
	}

	tm.Stop()
}

func TestTunnelManager_StartIdempotent(t *testing.T) {
	tm := &TunnelManager{
		OnLog: func(string) {},
	}
	cfg := TunnelConfig{
		Host: "127.0.0.1", Port: "1",
		ForwardPorts: "9999", User: "nobody",
	}

	tm.Start(cfg)
	tm.Start(cfg) // second call must be a no-op (no panic, no extra goroutine)

	tm.Stop()
}

func TestTunnelStatus_Strings(t *testing.T) {
	cases := map[TunnelStatus]string{
		StatusDisconnected: "Status: Disconnected",
		StatusConnecting:   "Status: Connecting...",
		StatusConnected:    "Status: Connected & Active",
		StatusReconnecting: "Status: Reconnecting...",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("TunnelStatus(%d).String() = %q, want %q", s, got, want)
		}
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func contains(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}

// containsPair checks whether flag followed by value appear consecutively.
func containsPair(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
