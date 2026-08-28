package sysops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/ssh"
)

// ValidateSSHPublicKey parses each non-blank line as an authorized_keys
// entry and rejects the input if any line doesn't parse.
func ValidateSSHPublicKey(pubKey string) error {
	lines := strings.Split(strings.TrimSpace(pubKey), "\n")
	if len(lines) == 0 || strings.TrimSpace(pubKey) == "" {
		return fmt.Errorf("public key is empty")
	}
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if _, _, _, _, err := ssh.ParseAuthorizedKey([]byte(line)); err != nil {
			return fmt.Errorf("invalid SSH public key line %q: %w", line, err)
		}
	}
	return nil
}

// WriteAuthorizedKeys creates <chrootDir>/.ssh/authorized_keys owned by the
// backup user, replacing any previous content. Call ValidateSSHPublicKey
// first.
func WriteAuthorizedKeys(chrootDir, username, pubKey string) error {
	sshDir := filepath.Join(chrootDir, ".ssh")
	if err := os.MkdirAll(sshDir, 0o700); err != nil {
		return fmt.Errorf("create .ssh dir: %w", err)
	}

	authorizedKeysPath := filepath.Join(sshDir, "authorized_keys")
	content := strings.TrimSpace(pubKey) + "\n"
	if err := os.WriteFile(authorizedKeysPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("write authorized_keys: %w", err)
	}

	if err := Chown(sshDir, username, username); err != nil {
		return err
	}
	if err := Chown(authorizedKeysPath, username, username); err != nil {
		return err
	}
	if err := Chmod(sshDir, 0o700); err != nil {
		return err
	}
	if err := Chmod(authorizedKeysPath, 0o600); err != nil {
		return err
	}
	return nil
}

// HasAuthorizedKeys reports whether a user already has an authorized_keys
// file, for display purposes only.
func HasAuthorizedKeys(chrootDir string) bool {
	_, err := os.Stat(filepath.Join(chrootDir, ".ssh", "authorized_keys"))
	return err == nil
}
