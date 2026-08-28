package sysops

import "fmt"

// DeleteBackupUser reverses provisioning: sshd chroot access first (so no
// new sftp session can start mid-teardown), then the system account, then
// (optionally) the backed-up data itself, then the quota project
// registration. homeDir's tree is removed directly by this function rather
// than via userdel -r, since the chroot dir is root:root-owned (required by
// sshd) and userdel refuses to recursively remove a directory the deleted
// user doesn't own.
func DeleteBackupUser(sshdConfigDir, sshdService, username, homeDir string, removeHome bool) error {
	if err := RemoveSSHDDropin(sshdConfigDir, sshdService, username); err != nil {
		return fmt.Errorf("remove sftp chroot config: %w", err)
	}
	if err := DeleteUser(username); err != nil {
		return fmt.Errorf("delete system user: %w", err)
	}
	if removeHome {
		if err := RemoveAll(homeDir); err != nil {
			return fmt.Errorf("remove home directory: %w", err)
		}
	}
	if err := RemoveProjectEntry(username); err != nil {
		return fmt.Errorf("remove quota project entry: %w", err)
	}
	return nil
}
