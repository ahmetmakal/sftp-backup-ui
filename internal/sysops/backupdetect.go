package sysops

import (
	"bufio"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/ahmetmakal/sftp-backup-ui/internal/models"
)

var (
	dateDirRegexp      = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	archiveExtRegexp   = regexp.MustCompile(`(?i)\.(tar\.gz|tgz|tar|zip)$`)
	cpmovePrefixRegexp = regexp.MustCompile(`(?i)^cpmove-`)
	legacyCPanelRegexp = regexp.MustCompile(`(?i)^backup-[\d.\-_]+_`)
)

const (
	backupScanMaxDepth   = 3
	backupScanMaxEntries = 5000
)

type masterMeta struct {
	Status string         `json:"Status"`
	Users  map[string]any `json:"users"`
}

// DetectBackup inspects a backup user's upload directory and returns a
// best-effort read of what's being backed up there: what control panel it
// looks like (cPanel/DirectAdmin/custom), when the most recent run landed,
// and how many accounts it covered. Returns nil if nothing recognizable is
// found (empty or unreadable directory).
func DetectBackup(uploadDir string) *models.BackupInfo {
	entries, err := os.ReadDir(uploadDir)
	if err != nil {
		return nil
	}

	// WHM/cPanel "Additional Backup Destinations" layout: a dated folder
	// (YYYY-MM-DD) containing an accounts/ subfolder, ideally with a
	// .master.meta manifest that names every backed-up account explicitly.
	// This is checked first since it's by far the most reliable signal.
	var dateDirs []string
	for _, e := range entries {
		if e.IsDir() && dateDirRegexp.MatchString(e.Name()) {
			dateDirs = append(dateDirs, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(dateDirs)))

	for _, d := range dateDirs {
		if info := detectCPanelRun(filepath.Join(uploadDir, d, "accounts"), d); info != nil {
			return info
		}
	}

	// No dated WHM-style run - fall back to a bounded scan for archive
	// files sitting directly under the upload directory (typical of
	// DirectAdmin transfers and custom rsync/tar setups).
	return detectFlatArchives(uploadDir)
}

func detectCPanelRun(accountsDir, dateName string) *models.BackupInfo {
	entries, err := os.ReadDir(accountsDir)
	if err != nil {
		return nil
	}

	// Prefer the .master.meta manifest's own mtime over the YYYY-MM-DD
	// folder name for the timestamp - it reflects when the run actually
	// completed rather than just implying midnight.
	backupTime := time.Time{}
	if dt, err := time.Parse("2006-01-02", dateName); err == nil {
		backupTime = dt
	}

	metaPath := filepath.Join(accountsDir, ".master.meta")
	if data, err := os.ReadFile(metaPath); err == nil {
		if stat, statErr := os.Stat(metaPath); statErr == nil {
			backupTime = stat.ModTime()
		}
		var meta masterMeta
		if json.Unmarshal(data, &meta) == nil && len(meta.Users) > 0 {
			return &models.BackupInfo{Type: "cpanel", LastBackupAt: backupTime, AccountCount: len(meta.Users)}
		}
	}

	// No manifest, but the accounts/ structure under a dated folder is
	// itself a strong enough cPanel signature - count the archives directly.
	count := 0
	for _, e := range entries {
		if !e.IsDir() && archiveExtRegexp.MatchString(e.Name()) {
			count++
		}
	}
	if count == 0 {
		return nil
	}
	return &models.BackupInfo{Type: "cpanel", LastBackupAt: backupTime, AccountCount: count}
}

func detectFlatArchives(uploadDir string) *models.BackupInfo {
	accounts := map[string]bool{}
	var latest time.Time
	var latestArchive string
	cpanelSignal := false
	scanned := 0

	_ = filepath.WalkDir(uploadDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		scanned++
		if scanned > backupScanMaxEntries {
			return filepath.SkipAll
		}
		rel, relErr := filepath.Rel(uploadDir, path)
		if relErr == nil && strings.Count(rel, string(filepath.Separator)) > backupScanMaxDepth {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || !archiveExtRegexp.MatchString(d.Name()) {
			return nil
		}

		if cpmovePrefixRegexp.MatchString(d.Name()) || legacyCPanelRegexp.MatchString(d.Name()) {
			cpanelSignal = true
		}
		info, infoErr := d.Info()
		if infoErr == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
			latestArchive = path
		}
		accounts[archiveAccountName(d.Name())] = true
		return nil
	})

	if len(accounts) == 0 {
		return nil
	}

	backupType := "custom"
	switch {
	case cpanelSignal:
		backupType = "cpanel"
	case latestArchive != "" && looksLikeDirectAdmin(latestArchive):
		backupType = "directadmin"
	}

	return &models.BackupInfo{Type: backupType, LastBackupAt: latest, AccountCount: len(accounts)}
}

func archiveAccountName(name string) string {
	name = archiveExtRegexp.ReplaceAllString(name, "")
	name = cpmovePrefixRegexp.ReplaceAllString(name, "")
	name = legacyCPanelRegexp.ReplaceAllString(name, "")
	return strings.ToLower(name)
}

// looksLikeDirectAdmin peeks at an archive's top-level entries (without
// extracting it) and checks for DirectAdmin's characteristic backup layout
// (domains/, imap/, email/, database/, backup.conf).
func looksLikeDirectAdmin(archivePath string) bool {
	lower := strings.ToLower(archivePath)
	if !strings.HasSuffix(lower, ".tar.gz") && !strings.HasSuffix(lower, ".tgz") {
		return false
	}
	entries, err := peekArchiveEntries(archivePath, 40)
	if err != nil {
		return false
	}
	signals := 0
	for _, e := range entries {
		top, _, _ := strings.Cut(strings.TrimPrefix(e, "./"), "/")
		switch top {
		case "domains", "imap", "email", "database", "backup.conf":
			signals++
		}
	}
	return signals >= 2
}

// peekArchiveEntries lists up to maxLines entries of a tar.gz archive
// without extracting it to disk, bounded by a short timeout so a huge or
// corrupt archive can't stall a page load.
func peekArchiveEntries(path string, maxLines int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "tar", "-tzf", path)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	var lines []string
	scanner := bufio.NewScanner(stdout)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
		if len(lines) >= maxLines {
			break
		}
	}
	scanErr := scanner.Err()
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	if scanErr != nil && len(lines) == 0 {
		return nil, scanErr
	}
	return lines, nil
}
