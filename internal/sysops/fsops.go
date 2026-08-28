package sysops

import (
	"fmt"
	"os"
	"os/user"
)

func MkdirAll(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

func Chmod(path string, perm os.FileMode) error {
	return os.Chmod(path, perm)
}

// Chown resolves ownerUser:ownerGroup by name (not uid/gid) and applies it
// to path. Using the system's own name resolution avoids ever having to
// trust a caller-supplied numeric id.
func Chown(path, ownerUser, ownerGroup string) error {
	u, err := user.Lookup(ownerUser)
	if err != nil {
		return fmt.Errorf("lookup user %q: %w", ownerUser, err)
	}
	g, err := user.LookupGroup(ownerGroup)
	if err != nil {
		return fmt.Errorf("lookup group %q: %w", ownerGroup, err)
	}
	uid, err := parseID(u.Uid)
	if err != nil {
		return err
	}
	gid, err := parseID(g.Gid)
	if err != nil {
		return err
	}
	return os.Chown(path, uid, gid)
}

func parseID(s string) (int, error) {
	var id int
	if _, err := fmt.Sscanf(s, "%d", &id); err != nil {
		return 0, fmt.Errorf("parse id %q: %w", s, err)
	}
	return id, nil
}

// RemoveAll is used only for best-effort rollback of directories this app
// itself just created during a failed creation flow.
func RemoveAll(path string) error {
	return os.RemoveAll(path)
}
