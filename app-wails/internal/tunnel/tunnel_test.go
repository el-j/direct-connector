package tunnel

import (
	"strings"
	"testing"
	"time"
)

// ── BuildSSHArgs ──────────────────────────────────────────────────────────────

func TestBuildSSHArgs_Valid(t *testing.T) {
	cfg := Config{
		Host:         "example.com",
		Port:         "22",
		ForwardPorts: "1883, 3391",
		User:         "alice",
		IPVersion:    "",
	}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	last := args[len(args)-1]
	if last != "alice@example.com" {
		t.Errorf("expected last arg 'alice@example.com', got %q", last)
	}

	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-R 1883:localhost:1883") {
		t.Errorf("missing -R for port 1883; args: %v", args)
	}
	if !strings.Contains(joined, "-R 3391:localhost:3391") {
		t.Errorf("missing -R for port 3391; args: %v", args)
	}
	if strings.Contains(joined, "-6") {
		t.Errorf("unexpected -6 flag when IPVersion is empty; args: %v", args)
	}
	if strings.Contains(joined, "-4") {
		t.Errorf("unexpected -4 flag when IPVersion is empty; args: %v", args)
	}
	if !contains(args, "-N") {
		t.Errorf("expected -N in args; args: %v", args)
	}
}

func TestBuildSSHArgs_IPv6(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "8080", ForwardPorts: "1883", User: "bob", IPVersion: "6"}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args[0] != "-6" {
		t.Errorf("expected -6 as first arg when IPVersion=\"6\", got %q", args[0])
	}
}

func TestBuildSSHArgs_IPv4(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "alice", IPVersion: "4"}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args[0] != "-4" {
		t.Errorf("expected -4 as first arg when IPVersion=\"4\", got %q", args[0])
	}
	if strings.Contains(strings.Join(args, " "), "-6") {
		t.Errorf("unexpected -6 when IPVersion=\"4\"; args: %v", args)
	}
}

func TestBuildSSHArgs_IPAuto(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u", IPVersion: ""}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(args, " ")
	if strings.Contains(joined, "-4") || strings.Contains(joined, "-6") {
		t.Errorf("expected no IP flag when IPVersion is empty; args: %v", args)
	}
}

func TestBuildSSHArgs_WithKeyPath(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u", KeyPath: "/home/u/.ssh/id_ed25519"}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsPair(args, "-i", "/home/u/.ssh/id_ed25519") {
		t.Errorf("expected -i /home/u/.ssh/id_ed25519 in args; args: %v", args)
	}
}

func TestBuildSSHArgs_NoKeyPath(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u"}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(args, "-i") {
		t.Errorf("unexpected -i flag when KeyPath is empty; args: %v", args)
	}
}

func TestBuildSSHArgs_InvalidSSHPort(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "notaport", ForwardPorts: "1883", User: "alice"}
	_, err := BuildSSHArgs(cfg)
	if err == nil {
		t.Error("expected error for non-numeric SSH port, got nil")
	}
}

func TestBuildSSHArgs_InvalidForwardPort(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "abc", User: "alice"}
	_, err := BuildSSHArgs(cfg)
	if err == nil {
		t.Error("expected error for non-numeric forward port, got nil")
	}
}

func TestBuildSSHArgs_EmptyForwardPorts(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "  ,  ,  ", User: "alice"}
	_, err := BuildSSHArgs(cfg)
	if err == nil {
		t.Error("expected error when all forward ports are blank, got nil")
	}
}

func TestBuildSSHArgs_VerboseAbsentByDefault(t *testing.T) {
	cfg := Config{Host: "h", Port: "22", ForwardPorts: "1883", User: "u"} // Verbose not set
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if contains(args, "-v") {
		t.Errorf("expected -v absent when Verbose=false; args: %v", args)
	}
}

func TestBuildSSHArgs_VerbosePresentWhenEnabled(t *testing.T) {
	cfg := Config{Host: "h", Port: "22", ForwardPorts: "1883", User: "u", Verbose: true}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !contains(args, "-v") {
		t.Errorf("expected -v flag when Verbose=true; args: %v", args)
	}
}

func TestBuildSSHArgs_LocalForward(t *testing.T) {
	cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u", ForwardMode: "L"}
	args, err := BuildSSHArgs(cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "-L 1883:localhost:1883") {
		t.Errorf("expected -L for local forward; args: %v", args)
	}
	if strings.Contains(joined, "-R") {
		t.Errorf("unexpected -R when ForwardMode=\"L\"; args: %v", args)
	}
}

func TestBuildSSHArgs_RemoteForwardDefault(t *testing.T) {
	// Both empty and "R" should produce -R
	for _, mode := range []string{"", "R"} {
		cfg := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u", ForwardMode: mode}
		args, err := BuildSSHArgs(cfg)
		if err != nil {
			t.Fatalf("mode=%q unexpected error: %v", mode, err)
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "-R 1883:localhost:1883") {
			t.Errorf("mode=%q expected -R; args: %v", mode, args)
		}
	}
}

func TestBuildSSHArgs_UseWSLSsh_DoesNotChangeArgs(t *testing.T) {
	// UseWSLSsh only changes the executable, not the SSH argument list itself.
	without := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u"}
	with := Config{Host: "example.com", Port: "22", ForwardPorts: "1883", User: "u", UseWSLSsh: true}
	argsWithout, err := BuildSSHArgs(without)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	argsWith, err := BuildSSHArgs(with)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Join(argsWithout, " ") != strings.Join(argsWith, " ") {
		t.Errorf("UseWSLSsh should not change SSH args; without=%v with=%v", argsWithout, argsWith)
	}
}

func TestBuildSSHArgs_ServerAliveOptions(t *testing.T) {
	cfg := Config{Host: "h", Port: "22", ForwardPorts: "1883", User: "u"}
	args, err := BuildSSHArgs(cfg)
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

// ── Manager ───────────────────────────────────────────────────────────────────

func TestManager_IsRunning_InitiallyFalse(t *testing.T) {
	m := &Manager{}
	if m.IsRunning() {
		t.Error("new Manager should not be running")
	}
}

func TestManager_StopWhileNotRunning(t *testing.T) {
	m := &Manager{}
	done := make(chan struct{})
	go func() {
		m.Stop()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Error("Stop() deadlocked when called on an idle Manager")
	}
}

func TestManager_OnLog_CalledOnStart(t *testing.T) {
	logs := make(chan string, 10)
	m := &Manager{
		OnLog: func(msg string) { logs <- msg },
	}
	m.Start(Config{Host: "127.0.0.1", Port: "1", ForwardPorts: "9999", User: "nobody"})
	select {
	case msg := <-logs:
		if msg == "" {
			t.Error("expected non-empty log message")
		}
	case <-time.After(3 * time.Second):
		t.Error("timed out waiting for OnLog to be called")
	}
	m.Stop()
}

func TestManager_StartIdempotent(t *testing.T) {
	m := &Manager{OnLog: func(string) {}}
	cfg := Config{Host: "127.0.0.1", Port: "1", ForwardPorts: "9999", User: "nobody"}
	m.Start(cfg)
	m.Start(cfg) // second call must be a no-op
	m.Stop()
}

func TestStatus_Strings(t *testing.T) {
	cases := map[Status]string{
		StatusDisconnected: "Status: Disconnected",
		StatusConnecting:   "Status: Connecting...",
		StatusConnected:    "Status: Connected & Active",
		StatusReconnecting: "Status: Reconnecting...",
	}
	for s, want := range cases {
		if got := s.String(); got != want {
			t.Errorf("Status(%d).String() = %q, want %q", s, got, want)
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

func containsPair(args []string, flag, value string) bool {
	for i := 0; i < len(args)-1; i++ {
		if args[i] == flag && args[i+1] == value {
			return true
		}
	}
	return false
}
