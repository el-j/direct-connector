// Package keygen manages the app-owned Ed25519 SSH keypair.
// Keys are stored alongside the config in the OS user config directory.
package keygen

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gossh "golang.org/x/crypto/ssh"

	"direct-connector/internal/config"
)

// AppKeyDir returns the directory that holds the app-managed keypair.
// It is the same directory returned by config.AppDirPath().
func AppKeyDir() string {
	return config.AppDirPath()
}

// AppPrivateKeyPath returns the path to the app-managed private key file.
func AppPrivateKeyPath() string { return filepath.Join(AppKeyDir(), "id_ed25519") }

// AppPublicKeyPath returns the path to the app-managed public key file.
func AppPublicKeyPath() string { return filepath.Join(AppKeyDir(), "id_ed25519.pub") }

// AppKeyExists reports whether the app-managed private key is present on disk.
func AppKeyExists() bool {
	_, err := os.Stat(AppPrivateKeyPath())
	return err == nil
}

// GenerateAppKey creates a new Ed25519 keypair in the app config directory.
// Any existing files at those paths are overwritten.
// The private key is stored without a passphrase in OpenSSH PEM format (0600).
// The public key is stored as an authorized_keys line (0644).
func GenerateAppKey() error {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return fmt.Errorf("generate key: %w", err)
	}

	if err := os.MkdirAll(AppKeyDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", AppKeyDir(), err)
	}

	// Private key — OpenSSH PEM format, no passphrase.
	pemBlock, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	privBytes := pem.EncodeToMemory(pemBlock)
	if err := os.WriteFile(AppPrivateKeyPath(), privBytes, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Public key — authorized_keys one-liner.
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return fmt.Errorf("new ssh public key: %w", err)
	}
	pubLine := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub)))
	if err := os.WriteFile(AppPublicKeyPath(), []byte(pubLine+"\n"), 0o644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

// LoadAppPublicKey returns the public key as an authorized_keys line,
// ready to be appended to the server's ~/.ssh/authorized_keys file.
func LoadAppPublicKey() (string, error) {
	data, err := os.ReadFile(AppPublicKeyPath())
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
