package sysops

import (
	"bufio"
	"os"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/ahmetmakal/sftp-backup-ui/internal/models"
)

// DiscoverMounts scans /proc/mounts for XFS filesystems mounted at a path
// matching pattern (e.g. /backup1, /backup2, ...). Nothing is cached: every
// call re-reads the live mount table, since new backup pools can appear at
// any time.
func DiscoverMounts(pattern *regexp.Regexp) ([]models.Mount, error) {
	f, err := os.Open("/proc/mounts")
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var mounts []models.Mount
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}
		mountPoint, fsType, options := fields[1], fields[2], fields[3]
		if fsType != "xfs" || !pattern.MatchString(mountPoint) {
			continue
		}

		m := models.Mount{
			Path:         mountPoint,
			QuotaEnabled: hasProjectQuota(options),
		}
		if total, avail, err := statfs(mountPoint); err == nil {
			m.TotalBytes = total
			m.AvailableBytes = avail
		}
		mounts = append(mounts, m)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Path < mounts[j].Path })
	return mounts, nil
}

func hasProjectQuota(mountOptions string) bool {
	for opt := range strings.SplitSeq(mountOptions, ",") {
		if opt == "prjquota" || opt == "pquota" {
			return true
		}
	}
	return false
}

func statfs(path string) (total, available uint64, err error) {
	var stat syscall.Statfs_t
	if err = syscall.Statfs(path, &stat); err != nil {
		return 0, 0, err
	}
	blockSize := uint64(stat.Bsize)
	return stat.Blocks * blockSize, stat.Bavail * blockSize, nil
}
