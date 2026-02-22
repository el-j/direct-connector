package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// AppDirName is the subdirectory created under os.UserConfigDir() for all
// app-managed files (config, SSH keys, …).
const AppDirName = "MQTTTunnelManager"

const appName = AppDirName // internal alias kept for backward compat
const configFileName = "tunnel_config.json"

// Config holds all user-facing settings and is persisted to disk as JSON.
type Config struct {
	Host         string `json:"host"`
	Port         string `json:"port"`
	ForwardPorts string `json:"forward_ports"`
	User         string `json:"user"`
	IPv6         bool   `json:"ipv6"`         // legacy; superseded by IPVersion
	IPVersion    string `json:"ip_version"`   // "" = auto, "4" = force IPv4, "6" = force IPv6
	KeyPath      string `json:"key_path"`     // custom private key path; empty = use app key or SSH default
	ForwardMode  string `json:"forward_mode"` // "R" = remote/reverse (default), "L" = local
	Verbose      bool   `json:"verbose"`      // pass -v to SSH for debug output
	UseWSLSsh    bool   `json:"use_wsl_ssh"`  // Windows: run 'wsl.exe ssh' so SSH runs inside WSL network (where Docker/MQTT lives)
}

// defaultConfig returns safe, sensible defaults.
func defaultConfig() Config {
	return Config{
		Host:         "joevbase.ddns.net",
		Port:         "443", // 443 bypasses most firewalls (indistinguishable from HTTPS to routers)
		ForwardPorts: "1883, 3391",
		User:         "",
		IPv6:         true, // kept for legacy JSON round-trips
		IPVersion:    "6",  // new field — Force IPv6 as default
		ForwardMode:  "R",  // remote/reverse forward by default
		Verbose:      false,
		UseWSLSsh:    false,
	}
}

// AppDirPath returns the OS-appropriate path for the app config directory.
// The directory is created (with 0755 permissions) if it does not exist.
// Falls back to "." if os.UserConfigDir() fails.
func AppDirPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	appDir := filepath.Join(dir, AppDirName)
	_ = os.MkdirAll(appDir, 0o755)
	return appDir
}

// configPath returns the OS-appropriate path to the config file.
// Falls back to the current directory if the config dir cannot be resolved.
func configPath() string {
	d := AppDirPath()
	if d == "." {
		return configFileName
	}
	return filepath.Join(d, configFileName)
}

// Load reads config from disk and merges it onto defaults.
// If the file does not exist the defaults are returned silently.
// Parse errors are printed to stderr so they are visible without a UI.
func Load() Config { return loadConfig() }

// Save writes cfg to disk as indented JSON. Errors are printed to stderr.
func Save(cfg Config) { saveConfig(cfg) }

func loadConfig() Config {
	cfg := defaultConfig()
	path := configPath()

	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			fmt.Fprintf(os.Stderr, "config read error (%s): %v\n", path, err)
		}
		return cfg
	}

	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintf(os.Stderr, "config parse error (%s): %v — using defaults\n", path, err)
		return defaultConfig()
	}
	// Migrate legacy IPv6 bool to IPVersion string.
	if cfg.IPVersion == "" && cfg.IPv6 {
		cfg.IPVersion = "6"
	}
	return cfg
}

// saveConfig writes cfg to disk in indented JSON.
// Errors are written to stderr.
func saveConfig(cfg Config) {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "config marshal error: %v\n", err)
		return
	}
	path := configPath()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "config write error (%s): %v\n", path, err)
	}
}
