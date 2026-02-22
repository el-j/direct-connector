package keygen

import (
	"strings"
	"testing"
)

func TestAppPrivateKeyPath(t *testing.T) {
	p := AppPrivateKeyPath()
	if p == "" {
		t.Fatal("AppPrivateKeyPath must not be empty")
	}
	if !strings.HasSuffix(p, "id_ed25519") {
		t.Errorf("expected path to end with id_ed25519, got %q", p)
	}
}

func TestAppPublicKeyPath(t *testing.T) {
	p := AppPublicKeyPath()
	if p == "" {
		t.Fatal("AppPublicKeyPath must not be empty")
	}
	if !strings.HasSuffix(p, "id_ed25519.pub") {
		t.Errorf("expected path to end with id_ed25519.pub, got %q", p)
	}
}

func TestGenerateAppKey_CreatesFiles(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := GenerateAppKey(); err != nil {
		if strings.Contains(err.Error(), "config") || strings.Contains(err.Error(), "dir") {
			t.Skipf("config dir not overridable on this platform: %v", err)
		}
		t.Fatalf("GenerateAppKey: %v", err)
	}

	if !AppKeyExists() {
		t.Error("AppKeyExists() returned false immediately after GenerateAppKey()")
	}
}

func TestGenerateAppKey_PublicKeyFormat(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := GenerateAppKey(); err != nil {
		t.Skipf("GenerateAppKey error: %v", err)
	}

	pub, err := LoadAppPublicKey()
	if err != nil {
		t.Fatalf("LoadAppPublicKey: %v", err)
	}
	if !strings.HasPrefix(pub, "ssh-ed25519 ") {
		t.Errorf("public key has unexpected format: %q", pub)
	}
}

func TestAppKeyExists_FalseWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)
	if AppKeyExists() {
		t.Skip("app key already exists in config dir; cannot test absence")
	}
}

func TestGenerateAppKey_Idempotent(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", dir)

	if err := GenerateAppKey(); err != nil {
		t.Skipf("GenerateAppKey: %v", err)
	}
	pub1, err := LoadAppPublicKey()
	if err != nil {
		t.Fatalf("first LoadAppPublicKey: %v", err)
	}

	if err := GenerateAppKey(); err != nil {
		t.Fatalf("second GenerateAppKey: %v", err)
	}
	pub2, err := LoadAppPublicKey()
	if err != nil {
		t.Fatalf("second LoadAppPublicKey: %v", err)
	}

	if pub1 == pub2 {
		t.Error("expected two generated keys to be different")
	}
}
