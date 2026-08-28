package sysops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const sshdDropinTemplate = `# Managed by sftp-backup-ui - do not edit by hand.
Match User %s
    ChrootDirectory %s
    ForceCommand internal-sftp
    AllowTcpForwarding no
    X11Forwarding no
`

func dropinPath(sshdConfigDir, username string) string {
	return filepath.Join(sshdConfigDir, "backup-"+username+".conf")
}

// WriteSSHDDropin writes a per-user Match block, validates the resulting
// sshd configuration, and reloads sshd. If validation fails the drop-in is
// removed again so a bad config never gets left behind, and sshd is never
// reloaded with it.
func WriteSSHDDropin(sshdConfigDir, sshdService, username, chrootDir string) error {
	path := dropinPath(sshdConfigDir, username)
	content := fmt.Sprintf(sshdDropinTemplate, username, chrootDir)

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write sshd drop-in: %w", err)
	}

	if _, err := run("sshd", "-t"); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("sshd config validation failed after adding %s, drop-in removed: %w", username, err)
	}

	if _, err := run("systemctl", "reload", sshdService); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("sshd reload failed after adding %s, drop-in removed: %w", username, err)
	}
	return nil
}

// RemoveSSHDDropin deletes a user's Match block and reloads sshd. If the
// drop-in doesn't exist this is a no-op.
func RemoveSSHDDropin(sshdConfigDir, sshdService, username string) error {
	path := dropinPath(sshdConfigDir, username)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove sshd drop-in: %w", err)
	}

	if _, err := run("sshd", "-t"); err != nil {
		return fmt.Errorf("sshd config validation failed after removing %s: %w", username, err)
	}
	if _, err := run("systemctl", "reload", sshdService); err != nil {
		return fmt.Errorf("sshd reload failed after removing %s: %w", username, err)
	}
	return nil
}

// HasSSHDDropin reports whether a user's app-managed Match block (as
// written by WriteSSHDDropin) currently exists.
func HasSSHDDropin(sshdConfigDir, username string) bool {
	_, err := os.Stat(dropinPath(sshdConfigDir, username))
	return err == nil
}

// HasManualMatchBlock scans the main sshd_config for a "Match User <username>"
// line that was NOT written by this app - e.g. one set up by hand before
// this tool existed. Only the exact-username single-user form is detected;
// a comma-separated "Match User a,b,c" list is treated as a match if
// username appears as one of the tokens.
func HasManualMatchBlock(mainConfigPath, username string) bool {
	f, err := os.Open(mainConfigPath)
	if err != nil {
		return false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 3 || fields[0] != "Match" || fields[1] != "User" {
			continue
		}
		if slices.Contains(strings.Split(fields[2], ","), username) {
			return true
		}
	}
	_ = scanner.Err() // best-effort scan; a read error just means "not found"
	return false
}

// SFTPStatus reports whether SFTP chroot access is configured for username,
// distinguishing between the app-managed drop-in and a pre-existing manual
// Match block in the main sshd_config.
func SFTPStatus(sshdConfigDir, mainConfigPath, username string) (active, managed bool) {
	if HasSSHDDropin(sshdConfigDir, username) {
		return true, true
	}
	if HasManualMatchBlock(mainConfigPath, username) {
		return true, false
	}
	return false, false
}
