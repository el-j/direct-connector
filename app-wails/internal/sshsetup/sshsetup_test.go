// Package sshsetup unit tests — no real SSH server required.
package sshsetup

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestNew verifies that New returns a non-nil Installer with a non-nil reply channel.
func TestNew(t *testing.T) {
	ins := New(func(PromptKind, string) {}, func(string) {})
	if ins == nil {
		t.Fatal("New returned nil")
	}
	if ins.replyCh == nil {
		t.Fatal("replyCh is nil")
	}
	if ins.OnPrompt == nil {
		t.Fatal("OnPrompt is nil")
	}
	if ins.OnLog == nil {
		t.Fatal("OnLog is nil")
	}
}

// TestReplyNoOp verifies that calling Reply when no prompt is pending
// does not block (buffered channel with select-default).
func TestReplyNoOp(t *testing.T) {
	ins := New(func(PromptKind, string) {}, func(string) {})

	// First Reply fills the buffer.
	ins.Reply("first")

	// Second Reply must not block even though the buffer is already full.
	done := make(chan struct{})
	go func() {
		ins.Reply("second") // would deadlock without select-default
		close(done)
	}()

	select {
	case <-done:
		// good: did not block
	case <-time.After(time.Second):
		t.Fatal("Reply blocked unexpectedly when channel was full")
	}

	// Drain and verify only the first value is present.
	select {
	case got := <-ins.replyCh:
		if got != "first" {
			t.Fatalf("expected 'first', got %q", got)
		}
	default:
		t.Fatal("expected a value in replyCh but channel was empty")
	}
}

// TestRunCancelledContext verifies that Run returns an error immediately
// when the context is already cancelled before calling Run.
func TestRunCancelledContext(t *testing.T) {
	ins := New(
		func(kind PromptKind, msg string) {
			t.Errorf("OnPrompt called unexpectedly: kind=%s msg=%s", kind, msg)
		},
		func(msg string) { /* log is fine, ignore */ },
	)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	err := ins.Run(ctx, Config{
		Host:      "localhost",
		Port:      "2222",
		User:      "test",
		PublicKey: "ssh-ed25519 AAAA test",
	})
	if err == nil {
		t.Fatal("expected error for cancelled context, got nil")
	}
	if !strings.Contains(err.Error(), "cancel") && !strings.Contains(err.Error(), "context") {
		t.Fatalf("expected cancellation error, got: %v", err)
	}
}

// TestRunEmptyHostReturnsError verifies that Run with an empty host
// returns an error rather than panicking.
func TestRunEmptyHostReturnsError(t *testing.T) {
	ins := New(func(PromptKind, string) {}, func(string) {})

	ctx := context.Background()
	err := ins.Run(ctx, Config{}) // zero Config: empty host
	if err == nil {
		t.Fatal("expected error for empty host, got nil")
	}
	if !strings.Contains(err.Error(), "host") {
		t.Fatalf("expected 'host' in error, got: %v", err)
	}
}

// TestRunDialError verifies that Run returns an error (not a panic) when
// it cannot connect to the given address (nothing listening on port 1).
func TestRunDialError(t *testing.T) {
	ins := New(func(PromptKind, string) {}, func(string) {})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 127.0.0.1:1 should refuse immediately on any OS.
	err := ins.Run(ctx, Config{
		Host:      "127.0.0.1",
		Port:      "1",
		User:      "test",
		PublicKey: "ssh-ed25519 AAAA test",
	})
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

// TestReplyUnblocksAfterPrompt ensures that a goroutine blocked on replyCh
// is unblocked by a call to Reply.
func TestReplyUnblocksAfterPrompt(t *testing.T) {
	ins := New(func(PromptKind, string) {}, func(string) {})

	received := make(chan string, 1)
	go func() {
		select {
		case v := <-ins.replyCh:
			received <- v
		case <-time.After(2 * time.Second):
			received <- "TIMEOUT"
		}
	}()

	// Give the goroutine a moment to start blocking.
	time.Sleep(10 * time.Millisecond)
	ins.Reply("yes")

	got := <-received
	if got != "yes" {
		t.Fatalf("expected 'yes', got %q", got)
	}
}

// TestLogHelperNilSafe verifies that log does not panic when OnLog is nil.
func TestLogHelperNilSafe(t *testing.T) {
	ins := &Installer{
		OnPrompt: func(PromptKind, string) {},
		OnLog:    nil,
		replyCh:  make(chan string, 1),
	}
	// Must not panic.
	ins.log("test message")
	ins.log("formatted %s", "value")
}
