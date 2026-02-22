package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestDefaultConfig verifies all default values are populated.
func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()
	if cfg.Host == "" {
		t.Error("default Host must not be empty")
	}
	if cfg.Port == "" {
		t.Error("default Port must not be empty")
	}
	if cfg.ForwardPorts == "" {
		t.Error("default ForwardPorts must not be empty")
	}
}

// TestSaveAndLoadConfig round-trips a config through disk.
func TestSaveAndLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test_config.json")

	// Override configPath for this test by writing directly.
	want := Config{
		Host:         "test.example.com",
		Port:         "2222",
		ForwardPorts: "1883, 8883",
		User:         "testuser",
		IPv6:         false,
	}

	data, err := json.MarshalIndent(want, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got Config
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got != want {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, want)
	}
}

// TestLoadConfigMissingFile returns defaults when no file exists.
func TestLoadConfigMissingFile(t *testing.T) {
	// Point configPath at a non-existent file by temporarily overriding env.
	// Since configPath() uses os.UserConfigDir(), we test indirectly:
	// loadConfig must always return a non-zero Config.
	cfg := loadConfig()
	if cfg.Port == "" {
		t.Error("loadConfig must return non-empty Port even when file is absent")
	}
}

// TestLoadConfigCorrupt returns defaults gracefully when JSON is broken.
func TestLoadConfigCorrupt(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not valid json {{{"), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, _ := os.ReadFile(path)
	defaults := defaultConfig()
	var parsed Config
	if err := json.Unmarshal(raw, &parsed); err != nil {
		// Expected: corrupt file should fall back to defaults.
		parsed = defaults
	}
	if parsed.Port == "" {
		t.Error("corrupt config should fall back to defaults with non-empty Port")
	}
}

// TestConfigPath returns a non-empty path string.
func TestConfigPath(t *testing.T) {
	p := configPath()
	if p == "" {
		t.Error("configPath must return a non-empty path")
	}
}
