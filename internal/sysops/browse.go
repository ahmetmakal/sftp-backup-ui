package sysops

import (
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// BrowseEntry is one row in a backup upload directory listing.
type BrowseEntry struct {
	Name    string
	IsDir   bool
	Size    int64
	ModTime time.Time
	RelPath string
}

// ListDirectory reads uploadDir/relPath and returns entries sorted with
// directories first, then files, alphabetically within each group.
// relPath is cleaned against a virtual root so ".." cannot escape uploadDir.
func ListDirectory(uploadDir, relPath string) ([]BrowseEntry, error) {
	cleaned := strings.TrimPrefix(path.Clean("/"+relPath), "/")

	absPath := filepath.Join(uploadDir, cleaned)
	uploadDir = filepath.Clean(uploadDir)
	absPath = filepath.Clean(absPath)
	if absPath != uploadDir && !strings.HasPrefix(absPath, uploadDir+string(filepath.Separator)) {
		return nil, fmt.Errorf("path outside upload directory")
	}

	dirEntries, err := os.ReadDir(absPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory not found")
		}
		return nil, err
	}

	out := make([]BrowseEntry, 0, len(dirEntries))
	for _, entry := range dirEntries {
		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := entry.Name()
		rel := name
		if cleaned != "" {
			rel = cleaned + "/" + name
		}

		isDir := entry.IsDir()
		if entry.Type()&fs.ModeSymlink != 0 {
			isDir = false
		}

		out = append(out, BrowseEntry{
			Name:    name,
			IsDir:   isDir,
			Size:    info.Size(),
			ModTime: info.ModTime(),
			RelPath: rel,
		})
	}

	slices.SortFunc(out, func(a, b BrowseEntry) int {
		if a.IsDir != b.IsDir {
			if a.IsDir {
				return -1
			}
			return 1
		}
		return strings.Compare(a.Name, b.Name)
	})

	return out, nil
}
