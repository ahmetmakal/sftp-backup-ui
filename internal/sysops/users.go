package sysops

import (
	"bufio"
	"errors"
	"os"
	"os/user"
	"strings"
)

type passwdEntry struct {
	Username string
	HomeDir  string
	Shell    string
}

// listPasswdEntries reads /etc/passwd directly (not via cgo/nss) so this
// works identically in minimal containers and on bare metal.
func listPasswdEntries() ([]passwdEntry, error) {
	f, err := os.Open("/etc/passwd")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []passwdEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Split(line, ":")
		if len(fields) < 7 {
			continue
		}
		entries = append(entries, passwdEntry{
			Username: fields[0],
			HomeDir:  fields[5],
			Shell:    fields[6],
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return entries, nil
}

// ListBackupUsers returns every system user whose home directory lives
// directly under one of the given backup mounts (mount/<username>).
func ListBackupUsers(mountPaths []string) ([]passwdEntry, error) {
	entries, err := listPasswdEntries()
	if err != nil {
		return nil, err
	}

	var backupUsers []passwdEntry
	for _, e := range entries {
		for _, mount := range mountPaths {
			if e.HomeDir == mount+"/"+e.Username {
				backupUsers = append(backupUsers, e)
				break
			}
		}
	}
	return backupUsers, nil
}

// UserExists reports whether a system user with this name already exists.
func UserExists(username string) (bool, error) {
	_, err := user.Lookup(username)
	if err == nil {
		return true, nil
	}
	var unknownErr user.UnknownUserError
	if errors.As(err, &unknownErr) {
		return false, nil
	}
	return false, err
}

// CreateUser runs useradd for a no-login SFTP-only account.
func CreateUser(username, homeDir string) error {
	_, err := run("useradd", "-d", homeDir, "-s", "/usr/sbin/nologin", username)
	return err
}

// DeleteUser removes the system account only. Deliberately never passes
// userdel's -r: our chroot home directories are root:root-owned (sshd
// requires this), which GNU userdel refuses to recursively remove even
// with -r ("not owned by <user>, not removing"), exiting non-zero and
// leaving the whole call looking like it failed even though the account
// itself was in fact deleted. Removing the directory tree, when wanted, is
// handled separately by the caller (see DeleteBackupUser).
func DeleteUser(username string) error {
	_, err := run("userdel", username)
	return err
}
