package sysops

import (
	"path/filepath"
	"regexp"

	"github.com/ahmetmakal/sftp-backup-ui/internal/models"
)

// DashboardData is the full, live snapshot of backup mounts and their users,
// assembled fresh on every request from passwd, xfs quota reports, and sshd
// drop-in files - there is no stored/cached copy anywhere.
type DashboardData struct {
	Mounts []models.Mount
	Users  []models.BackupUser
}

func LoadDashboard(mountPattern *regexp.Regexp, sshdConfigDir, sshdMainConfig, nfsExportsDir string) (*DashboardData, error) {
	mounts, err := DiscoverMounts(mountPattern)
	if err != nil {
		return nil, err
	}

	mountPaths := make([]string, len(mounts))
	quotaByMount := make(map[string]map[string]models.QuotaInfo)
	for i, m := range mounts {
		mountPaths[i] = m.Path
		if m.QuotaEnabled {
			if report, err := ReportProjectQuotas(m.Path); err == nil {
				quotaByMount[m.Path] = report
			}
		}
	}

	entries, err := ListBackupUsers(mountPaths)
	if err != nil {
		return nil, err
	}

	users := make([]models.BackupUser, 0, len(entries))
	for _, e := range entries {
		mount := filepath.Dir(e.HomeDir)
		active, managed := SFTPStatus(sshdConfigDir, sshdMainConfig, e.Username)
		nfsEnabled, nfsClientIPs := IsNFSEnabled(nfsExportsDir, e.Username)
		uploadDir := filepath.Join(e.HomeDir, "upload")
		bu := models.BackupUser{
			Username:      e.Username,
			Mount:         mount,
			HomeDir:       e.HomeDir,
			SFTPActive:    active,
			SFTPManaged:   managed,
			HasSSHKey:     HasAuthorizedKeys(e.HomeDir),
			NFSEnabled:    nfsEnabled,
			NFSClientIPs:  nfsClientIPs,
			Backup:        DetectBackup(uploadDir),
			AccountStatus: ReadAccountStatus(uploadDir),
		}
		if report, ok := quotaByMount[mount]; ok {
			if q, ok := report[e.Username]; ok {
				qCopy := q
				bu.Quota = &qCopy
			}
		}
		users = append(users, bu)
	}

	return &DashboardData{Mounts: mounts, Users: users}, nil
}
