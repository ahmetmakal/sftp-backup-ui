package models

import "time"

// BackupInfo is a best-effort read of what's actually sitting in a user's
// upload directory: what kind of control-panel backup it looks like, when
// the most recent run landed, and how many accounts it covered.
type BackupInfo struct {
	Type         string // "cpanel", "directadmin", or "custom"
	LastBackupAt time.Time
	AccountCount int
}

// Mount represents a discovered backup pool such as /backup1.
type Mount struct {
	Path           string
	QuotaEnabled   bool
	TotalBytes     uint64
	AvailableBytes uint64
}

// QuotaInfo holds the XFS project quota state for one user on one mount.
type QuotaInfo struct {
	UsedBytes uint64
	SoftBytes uint64
	HardBytes uint64
}

// BackupUser is a system user whose home directory lives under a backup mount.
type BackupUser struct {
	Username    string
	Mount       string
	HomeDir     string
	Quota       *QuotaInfo // nil if no project quota is configured
	SFTPActive  bool       // any sftp chroot Match block found (managed or manual)
	SFTPManaged bool       // Match block is the app-managed drop-in specifically
	HasSSHKey   bool
	Backup      *BackupInfo // nil if the upload directory has no recognizable backup content
}
