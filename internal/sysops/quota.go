package sysops

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/ahmetmakal/sftp-backup-ui/internal/models"
)

const (
	projectsFile = "/etc/projects"
	projidFile   = "/etc/projid"

	// limitsFile mirrors the block limits kept in the xfs quota inodes
	// ("project:size" lines). xfs_repair can rebuild those inodes and zero
	// every limit, so this file lets RestoreQuotaLimits put them back.
	limitsFile = "/etc/projquota-limits"
)

// NextProjectID scans /etc/projid for the highest numeric project id in use
// and returns one past it, never going below base.
func NextProjectID(base int) (int, error) {
	entries, err := readColonFile(projidFile)
	if err != nil {
		return 0, err
	}

	next := base
	for _, e := range entries {
		id, err := strconv.Atoi(e.value)
		if err != nil {
			continue
		}
		if id >= next {
			next = id + 1
		}
	}
	return next, nil
}

// AddProjectEntry records a new project in /etc/projects and /etc/projid.
func AddProjectEntry(projectID int, homeDir, username string) error {
	if err := appendLine(projectsFile, fmt.Sprintf("%d:%s", projectID, homeDir)); err != nil {
		return fmt.Errorf("update %s: %w", projectsFile, err)
	}
	if err := appendLine(projidFile, fmt.Sprintf("%s:%d", username, projectID)); err != nil {
		return fmt.Errorf("update %s: %w", projidFile, err)
	}
	return nil
}

// RemoveProjectEntry drops a user's lines from /etc/projects and /etc/projid.
// Best-effort: existing on-disk data and any already-applied xfs quota limit
// are left untouched.
func RemoveProjectEntry(username string) error {
	if err := removeLinesWithKey(projidFile, username); err != nil {
		return fmt.Errorf("update %s: %w", projidFile, err)
	}
	if err := removeLinesWithKey(projectsFile, username); err != nil {
		return fmt.Errorf("update %s: %w", projectsFile, err)
	}
	if err := removeLinesWithKey(limitsFile, username); err != nil {
		return fmt.Errorf("update %s: %w", limitsFile, err)
	}
	return nil
}

// SetProjectQuota associates the (already-registered) project with its xfs
// quota tracking on mount.
func SetProjectQuota(mount, username string) error {
	_, err := run("xfs_quota", "-x", "-c", "project -s "+username, mount)
	return err
}

// SetQuotaLimit sets (or updates) both the soft and hard block limits for a
// user's project on mount. size must already be validated (ValidateQuotaSize).
func SetQuotaLimit(mount, username, size string) error {
	cmd := fmt.Sprintf("limit -p bsoft=%s bhard=%s %s", size, size, username)
	if _, err := run("xfs_quota", "-x", "-c", cmd, mount); err != nil {
		return err
	}
	return recordQuotaLimit(username, size)
}

// recordQuotaLimit upserts username's limit in limitsFile.
func recordQuotaLimit(username, size string) error {
	if err := removeLinesWithKey(limitsFile, username); err != nil {
		return err
	}
	return appendLine(limitsFile, username+":"+size)
}

// RestoreQuotaLimits reconciles the xfs quota limits on every backup mount
// with limitsFile: a project whose hard limit is 0 but has a recorded limit
// gets it re-applied, and a project with a limit that isn't recorded yet is
// added to the file. It returns the usernames it restored.
func RestoreQuotaLimits(pattern *regexp.Regexp) ([]string, error) {
	mounts, err := DiscoverMounts(pattern)
	if err != nil {
		return nil, err
	}
	entries, err := readColonFile(limitsFile)
	if err != nil {
		return nil, err
	}
	recorded := make(map[string]string, len(entries))
	for _, e := range entries {
		recorded[e.key] = e.value
	}

	var restored []string
	for _, m := range mounts {
		report, err := ReportProjectQuotas(m.Path)
		if err != nil {
			continue
		}
		for name, q := range report {
			size, ok := recorded[name]
			switch {
			case q.HardBytes == 0 && ok:
				if err := SetQuotaLimit(m.Path, name, size); err != nil {
					return restored, err
				}
				restored = append(restored, name)
			case q.HardBytes != 0 && !ok:
				if err := recordQuotaLimit(name, strconv.FormatUint(q.HardBytes/1024, 10)+"k"); err != nil {
					return restored, err
				}
			}
		}
	}
	return restored, nil
}

// ReportProjectQuotas parses `xfs_quota report -p` for mount into a map of
// project name -> usage, all in bytes. xfs_quota reports blocks in KiB by
// default, which is what's converted here.
func ReportProjectQuotas(mount string) (map[string]models.QuotaInfo, error) {
	out, err := run("xfs_quota", "-x", "-c", "report -p -N", mount)
	if err != nil {
		return nil, err
	}

	result := make(map[string]models.QuotaInfo)
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		name := fields[0]
		if name == "#0" {
			continue // the default/unset project bucket
		}
		used, err1 := strconv.ParseUint(fields[1], 10, 64)
		soft, err2 := strconv.ParseUint(fields[2], 10, 64)
		hard, err3 := strconv.ParseUint(fields[3], 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			continue
		}
		result[name] = models.QuotaInfo{
			UsedBytes: used * 1024,
			SoftBytes: soft * 1024,
			HardBytes: hard * 1024,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

type colonEntry struct {
	key   string
	value string
	raw   string
}

func readColonFile(path string) ([]colonEntry, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []colonEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, ":", 2)
		if len(parts) != 2 {
			continue
		}
		entries = append(entries, colonEntry{key: parts[0], value: parts[1], raw: line})
	}
	return entries, scanner.Err()
}

func appendLine(path, line string) error {
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line + "\n")
	return err
}

// removeLinesWithKey rewrites path keeping only lines whose colon-separated
// key does not equal target.
func removeLinesWithKey(path, target string) error {
	entries, err := readColonFile(path)
	if err != nil {
		return err
	}
	if entries == nil {
		return nil
	}

	var kept []string
	for _, e := range entries {
		if e.key == target {
			continue
		}
		kept = append(kept, e.raw)
	}

	content := ""
	if len(kept) > 0 {
		content = strings.Join(kept, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(content), 0o644)
}
