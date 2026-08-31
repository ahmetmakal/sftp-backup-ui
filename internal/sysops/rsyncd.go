package sysops

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
)

const rsyncModuleTemplate = `# Managed by sftp-backup-ui - do not edit by hand.
[%s]
path = %s
comment = %s
uid = %s
gid = %s
read only = false
list = false
auth users = %s
secrets file = %s
`

func rsyncConfPath(rsyncdDir, username string) string {
	return filepath.Join(rsyncdDir, username+".conf")
}

func rsyncSecretPath(rsyncdDir, username string) string {
	return filepath.Join(rsyncdDir, username+".secret")
}

// GenerateRsyncPassword returns a random URL-safe token suitable for an
// rsyncd secrets file entry.
func GenerateRsyncPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate rsync password: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// WriteRsyncModule writes an rsyncd module + secrets file for username and
// reloads (or, if the running rsyncd doesn't support a live reload,
// restarts) the daemon so it picks up the new module. If the daemon isn't
// active afterward, the written files are removed so a failed attempt never
// leaves a stale module behind. Returns the generated password - callers
// must show it to the admin immediately, since it is never stored anywhere
// this app can read back in plaintext form for display again.
func WriteRsyncModule(rsyncdDir, rsyncdService, username, chrootDir string) (password string, err error) {
	password, err = GenerateRsyncPassword()
	if err != nil {
		return "", err
	}

	confPath := rsyncConfPath(rsyncdDir, username)
	secretPath := rsyncSecretPath(rsyncdDir, username)

	if err := os.MkdirAll(rsyncdDir, 0o755); err != nil {
		return "", fmt.Errorf("create rsyncd dir: %w", err)
	}

	uploadDir := filepath.Join(chrootDir, "upload")
	confContent := fmt.Sprintf(rsyncModuleTemplate, username, uploadDir, username, username, username, username, secretPath)
	if err := os.WriteFile(confPath, []byte(confContent), 0o644); err != nil {
		return "", fmt.Errorf("write rsync module: %w", err)
	}

	secretContent := fmt.Sprintf("%s:%s\n", username, password)
	if err := os.WriteFile(secretPath, []byte(secretContent), 0o600); err != nil {
		_ = os.Remove(confPath)
		return "", fmt.Errorf("write rsync secret: %w", err)
	}

	if err := reloadRsyncd(rsyncdService); err != nil {
		_ = os.Remove(confPath)
		_ = os.Remove(secretPath)
		return "", fmt.Errorf("rsyncd reload failed after adding %s, module removed: %w", username, err)
	}
	return password, nil
}

// RemoveRsyncModule deletes a user's rsync module + secret and reloads
// rsyncd. If neither file exists this is a no-op.
func RemoveRsyncModule(rsyncdDir, rsyncdService, username string) error {
	confPath := rsyncConfPath(rsyncdDir, username)
	secretPath := rsyncSecretPath(rsyncdDir, username)

	_, confErr := os.Stat(confPath)
	_, secretErr := os.Stat(secretPath)
	if os.IsNotExist(confErr) && os.IsNotExist(secretErr) {
		return nil
	}

	if confErr == nil {
		if err := os.Remove(confPath); err != nil {
			return fmt.Errorf("remove rsync module: %w", err)
		}
	}
	if secretErr == nil {
		if err := os.Remove(secretPath); err != nil {
			return fmt.Errorf("remove rsync secret: %w", err)
		}
	}

	if err := reloadRsyncd(rsyncdService); err != nil {
		return fmt.Errorf("rsyncd reload failed after removing %s: %w", username, err)
	}
	return nil
}

// HasRsyncModule reports whether a user's rsync module currently exists.
func HasRsyncModule(rsyncdDir, username string) bool {
	_, err := os.Stat(rsyncConfPath(rsyncdDir, username))
	return err == nil
}

// reloadRsyncd tries a live config reload first; not every rsyncd unit
// supports that (it depends on the packaged systemd unit / rsync version),
// so this falls back to a full restart, then confirms the service is
// actually active afterward either way.
func reloadRsyncd(service string) error {
	if _, err := run("systemctl", "reload", service); err != nil {
		if _, err := run("systemctl", "restart", service); err != nil {
			return err
		}
	}
	if _, err := run("systemctl", "is-active", service); err != nil {
		return fmt.Errorf("service not active: %w", err)
	}
	return nil
}
