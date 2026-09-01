package sysops

import (
	"fmt"
	"log"
	"path/filepath"
	"slices"
)

// CreateUserParams is everything needed to provision one backup user.
type CreateUserParams struct {
	Username      string
	Mount         string // e.g. /backup1, must be an already-discovered, quota-enabled mount
	QuotaSize     string // e.g. "200g", validated by ValidateQuotaSize
	PubKey        string // optional, validated by ValidateSSHPublicKey if non-empty
	SSHDConfigDir string
	SSHDService   string
	ProjectIDBase int
}

// CreateBackupUser runs every provisioning step in order. If any step fails,
// everything completed so far is unwound on a best-effort basis so a failed
// attempt never leaves a half-configured (and therefore surprising) user
// behind.
func CreateBackupUser(p CreateUserParams) error {
	homeDir := filepath.Join(p.Mount, p.Username)
	uploadDir := filepath.Join(homeDir, "upload")

	var rollback []func()
	undo := func() {
		for _, fn := range slices.Backward(rollback) {
			fn()
		}
	}

	if err := CreateUser(p.Username, homeDir); err != nil {
		return fmt.Errorf("create system user: %w", err)
	}
	rollback = append(rollback, func() {
		if err := DeleteUser(p.Username); err != nil {
			log.Printf("rollback: failed to delete user %s: %v", p.Username, err)
		}
		if err := RemoveAll(homeDir); err != nil {
			log.Printf("rollback: failed to remove home dir %s: %v", homeDir, err)
		}
	})

	if err := MkdirAll(uploadDir, 0o755); err != nil {
		undo()
		return fmt.Errorf("create upload dir: %w", err)
	}

	if err := Chown(homeDir, "root", "root"); err != nil {
		undo()
		return fmt.Errorf("chown home dir: %w", err)
	}
	if err := Chmod(homeDir, 0o755); err != nil {
		undo()
		return fmt.Errorf("chmod home dir: %w", err)
	}
	if err := Chown(uploadDir, p.Username, p.Username); err != nil {
		undo()
		return fmt.Errorf("chown upload dir: %w", err)
	}

	if err := WriteSSHDDropin(p.SSHDConfigDir, p.SSHDService, p.Username, homeDir); err != nil {
		undo()
		return fmt.Errorf("configure sftp chroot: %w", err)
	}
	rollback = append(rollback, func() {
		if err := RemoveSSHDDropin(p.SSHDConfigDir, p.SSHDService, p.Username); err != nil {
			log.Printf("rollback: failed to remove sshd drop-in for %s: %v", p.Username, err)
		}
	})

	if err := EnableChrootShell(p.Username, homeDir); err != nil {
		undo()
		return fmt.Errorf("configure chroot shell: %w", err)
	}
	rollback = append(rollback, func() {
		if err := DisableChrootShell(p.Username, homeDir); err != nil {
			log.Printf("rollback: failed to disable chroot shell for %s: %v", p.Username, err)
		}
	})

	projectID, err := NextProjectID(p.ProjectIDBase)
	if err != nil {
		undo()
		return fmt.Errorf("allocate project id: %w", err)
	}
	if err := AddProjectEntry(projectID, homeDir, p.Username); err != nil {
		undo()
		return fmt.Errorf("register quota project: %w", err)
	}
	rollback = append(rollback, func() {
		if err := RemoveProjectEntry(p.Username); err != nil {
			log.Printf("rollback: failed to remove project entry for %s: %v", p.Username, err)
		}
	})

	if err := SetProjectQuota(p.Mount, p.Username); err != nil {
		undo()
		return fmt.Errorf("activate quota project: %w", err)
	}
	if err := SetQuotaLimit(p.Mount, p.Username, p.QuotaSize); err != nil {
		undo()
		return fmt.Errorf("set quota limit: %w", err)
	}

	if p.PubKey != "" {
		if err := WriteAuthorizedKeys(homeDir, p.Username, p.PubKey); err != nil {
			undo()
			return fmt.Errorf("write ssh key: %w", err)
		}
	}

	return nil
}
