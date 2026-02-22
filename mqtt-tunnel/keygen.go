package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	gossh "golang.org/x/crypto/ssh"
)

// appKeyDir returns the directory where the managed keypair lives.
// It is the same directory that holds the config file.
func appKeyDir() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "."
	}
	return filepath.Join(dir, appName)
}

// appPrivateKeyPath returns the path to the app-managed private key.
func appPrivateKeyPath() string { return filepath.Join(appKeyDir(), "id_ed25519") }

// appPublicKeyPath returns the path to the app-managed public key.
func appPublicKeyPath() string { return filepath.Join(appKeyDir(), "id_ed25519.pub") }

// AppKeyExists reports whether the app-managed private key is present on disk.
func AppKeyExists() bool {
	_, err := os.Stat(appPrivateKeyPath())
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

	if err := os.MkdirAll(appKeyDir(), 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", appKeyDir(), err)
	}

	// Private key — OpenSSH PEM format, no passphrase.
	pemBlock, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}
	privBytes := pem.EncodeToMemory(pemBlock)
	if err := os.WriteFile(appPrivateKeyPath(), privBytes, 0o600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Public key — authorized_keys format.
	sshPub, err := gossh.NewPublicKey(pub)
	if err != nil {
		return fmt.Errorf("new ssh public key: %w", err)
	}
	pubLine := strings.TrimSpace(string(gossh.MarshalAuthorizedKey(sshPub)))
	if err := os.WriteFile(appPublicKeyPath(), []byte(pubLine+"\n"), 0o644); err != nil {
		return fmt.Errorf("write public key: %w", err)
	}

	return nil
}

// LoadAppPublicKey returns the public key as an authorized_keys line,
// ready to be appended to the server's ~/.ssh/authorized_keys file.
func LoadAppPublicKey() (string, error) {
	data, err := os.ReadFile(appPublicKeyPath())
	if err != nil {
		return "", fmt.Errorf("read public key: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}
