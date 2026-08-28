package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) DeleteUser(c *gin.Context) {
	username := c.Param("username")
	mount := c.PostForm("mount")
	removeHome := c.PostForm("remove_home") == "on"

	if err := h.doDeleteUser(username, mount, removeHome); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("Kullanıcı silindi: "+username))
}

func (h *Handlers) doDeleteUser(username, mount string, removeHome bool) error {
	if err := sysops.ValidateUsername(username); err != nil {
		return err
	}

	mounts, err := sysops.DiscoverMounts(h.cfg.MountRegex)
	if err != nil {
		return err
	}
	var mountFound bool
	for _, m := range mounts {
		if m.Path == mount {
			mountFound = true
			break
		}
	}
	if !mountFound {
		return fmt.Errorf("bilinmeyen ya da artık mevcut olmayan mount: %s", mount)
	}

	homeDir := filepath.Join(mount, username)
	return sysops.DeleteBackupUser(h.cfg.SSHDConfigDir, h.cfg.SSHDService, username, homeDir, removeHome)
}
