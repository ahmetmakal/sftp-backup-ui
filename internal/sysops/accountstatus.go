package sysops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/ahmetmakal/sftp-backup-ui/internal/models"
)

const accountStatusFileName = ".account-status.json"

// accountStatusFile mirrors the JSON written by
// contrib/account-count-check-setup.sh on the source server.
type accountStatusFile struct {
	TotalAccounts int    `json:"total_accounts"`
	Panel         string `json:"panel"`
	CheckedAt     string `json:"checked_at"`
}

// ReadAccountStatus reads and parses the source server's self-reported
// account count from uploadDir, if it has ever uploaded one. Returns nil
// if the file is missing or unreadable/malformed - this is a best-effort
// read of untrusted-but-cooperative input, matching DetectBackup's style.
func ReadAccountStatus(uploadDir string) *models.AccountStatus {
	data, err := os.ReadFile(filepath.Join(uploadDir, accountStatusFileName))
	if err != nil {
		return nil
	}

	var f accountStatusFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil
	}
	checkedAt, err := time.Parse(time.RFC3339, f.CheckedAt)
	if err != nil {
		return nil
	}

	return &models.AccountStatus{
		TotalAccounts: f.TotalAccounts,
		Panel:         f.Panel,
		CheckedAt:     checkedAt,
	}
}
