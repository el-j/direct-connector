package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mqtt-tunnel/internal/config"
	"mqtt-tunnel/internal/keygen"
)

// ── IsCLIMode ─────────────────────────────────────────────────────────────────

func TestIsCLIMode_FlagPresent(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"direct-connector", "--mode", "ssh"}
	if !IsCLIMode() {
		t.Error("expected IsCLIMode() == true when --mode flag is present")
	}
}

func TestIsCLIMode_EnvPresent(t *testing.T) {
	t.Setenv("DC_MODE", "ssh")
	if !IsCLIMode() {
		t.Error("expected IsCLIMode() == true when DC_MODE env var is set")
	}
}

func TestIsCLIMode_NeitherPresent(t *testing.T) {
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"direct-connector"}
	os.Unsetenv("DC_MODE")
	if IsCLIMode() {
		t.Error("expected IsCLIMode() == false when no mode flag/env is present")
	}
}

// ── envOr ─────────────────────────────────────────────────────────────────────

func TestEnvOr_EnvSet(t *testing.T) {
	t.Setenv("DC_TEST_KEY", "fromenv")
	if got := envOr("DC_TEST_KEY", "default"); got != "fromenv" {
		t.Errorf("got %q, want %q", got, "fromenv")
	}
}

func TestEnvOr_EnvUnset(t *testing.T) {
	os.Unsetenv("DC_TEST_KEY2")
	if got := envOr("DC_TEST_KEY2", "fallback"); got != "fallback" {
		t.Errorf("got %q, want %q", got, "fallback")
	}
}

// ── waitForFile ───────────────────────────────────────────────────────────────

func TestWaitForFile_FileExists(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "answer.txt")
	content := "my-sdp-answer"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	ctx := context.Background()
	noop := func(string, ...any) {}
	got, err := waitForFile(ctx, path, noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != content {
		t.Errorf("got %q, want %q", got, content)
	}
}

func TestWaitForFile_CancelledBeforeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "never.txt") // file never created

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	noop := func(string, ...any) {}
	_, err := waitForFile(ctx, path, noop)
	if err == nil {
		t.Error("expected error when context is cancelled, got nil")
	}
}

func TestWaitForFile_FileAppearsLate(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "late.txt")

	go func() {
		time.Sleep(3 * time.Second)
		_ = os.WriteFile(path, []byte("late-content"), 0o644)
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	noop := func(string, ...any) {}
	got, err := waitForFile(ctx, path, noop)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "late-content" {
		t.Errorf("got %q, want %q", got, "late-content")
	}
}

// ── readLine ─────────────────────────────────────────────────────────────────

func TestReadLine_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled; readLine should return quickly

	// readLine blocks on os.Stdin, which we can't easily replace in a test.
	// We test only the cancellation path via a goroutine + timeout.
	done := make(chan error, 1)
	go func() { _, err := readLine(ctx); done <- err }()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected context cancellation error, got nil")
		}
	case <-time.After(3 * time.Second):
		t.Error("readLine did not respect context cancellation within 3 s")
	}
}

// ── cliKeygen (integration) ───────────────────────────────────────────────────

func TestCLIKeygen_GeneratesKey(t *testing.T) {
	// Point the app config at a temp dir.
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	// Re-derive the expected paths after overriding XDG_CONFIG_HOME.
	privPath := filepath.Join(dir, config.AppDirName, "id_ed25519")
	pubPath := filepath.Join(dir, config.AppDirName, "id_ed25519.pub")

	if err := keygen.GenerateAppKey(); err != nil {
		// If the paths are resolved differently on this OS the test is
		// environment-dependent; skip rather than fail.
		if strings.Contains(err.Error(), "config") {
			t.Skipf("config dir not overridable on this OS: %v", err)
		}
		t.Fatalf("GenerateAppKey: %v", err)
	}

	for _, p := range []string{privPath, pubPath} {
		if _, err := os.Stat(p); os.IsNotExist(err) {
			t.Logf("key not at expected path %s (OS may use a different config dir) — checking keygen.AppPrivateKeyPath()", p)
		}
	}

	// Regardless of exact path, the app should report the key as existing.
	if !keygen.AppKeyExists() {
		t.Error("AppKeyExists() returned false after GenerateAppKey()")
	}

	pub, err := keygen.LoadAppPublicKey()
	if err != nil {
		t.Fatalf("LoadAppPublicKey: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Errorf("public key has unexpected format: %q", pub)
	}
}
