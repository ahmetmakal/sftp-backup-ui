package handlers

import (
	"fmt"
	"net/http"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

type breadcrumbSegment struct {
	Name string
	Path string
}

type browseEntryView struct {
	Name        string
	IsDir       bool
	Size        int64
	ModTime     time.Time
	RelPath     string
	DownloadCmd string
}

// Browse lists a user's upload/ directory and renders ready-to-run rsync
// download commands for each file.
func (h *Handlers) Browse(c *gin.Context) {
	username := c.Param("username")
	mount := c.Query("mount")
	relPath := c.Query("path")

	if err := sysops.ValidateUsername(username); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"Message": err.Error()})
		return
	}
	if !h.cfg.MountRegex.MatchString(mount) {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"Message": "geçersiz mount: " + mount})
		return
	}

	exists, err := sysops.UserExists(username)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"Message": err.Error()})
		return
	}
	if !exists {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"Message": fmt.Sprintf("kullanıcı bulunamadı: %s", username)})
		return
	}

	homeDir := filepath.Join(mount, username)
	uploadDir := filepath.Join(homeDir, "upload")
	entries, err := sysops.ListDirectory(uploadDir, relPath)
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"Message": err.Error()})
		return
	}

	cleanedPath := strings.TrimPrefix(path.Clean("/"+relPath), "/")

	destHost, destPort := h.backupSFTPEndpoint(c)

	hasSSHKey := sysops.HasAuthorizedKeys(homeDir)

	viewEntries := make([]browseEntryView, len(entries))
	for i, e := range entries {
		view := browseEntryView{
			Name:    e.Name,
			IsDir:   e.IsDir,
			Size:    e.Size,
			ModTime: e.ModTime,
			RelPath: e.RelPath,
		}
		if !e.IsDir && hasSSHKey {
			remotePath := "upload/" + e.RelPath
			view.DownloadCmd = fmt.Sprintf(
				`rsync -av -e "ssh -i /root/.ssh/backup_%s -p %s" %s@%s:'%s' .`,
				username, destPort, username, destHost, remotePath,
			)
		}
		viewEntries[i] = view
	}

	c.HTML(http.StatusOK, "browse.html", gin.H{
		"Username":    username,
		"Mount":       mount,
		"CurrentPath": cleanedPath,
		"Breadcrumbs": buildBreadcrumbs(cleanedPath),
		"Entries":     viewEntries,
		"DestHost":    destHost,
		"HasSSHKey":   hasSSHKey,
	})
}

func buildBreadcrumbs(cleanedPath string) []breadcrumbSegment {
	segments := []breadcrumbSegment{{Name: "upload", Path: ""}}
	if cleanedPath == "" {
		return segments
	}
	parts := strings.Split(cleanedPath, "/")
	acc := ""
	for _, part := range parts {
		if acc == "" {
			acc = part
		} else {
			acc += "/" + part
		}
		segments = append(segments, breadcrumbSegment{Name: part, Path: acc})
	}
	return segments
}
