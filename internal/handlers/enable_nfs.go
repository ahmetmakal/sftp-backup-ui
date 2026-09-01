package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// EnableNFS exports an already-SFTP-managed user's upload directory over
// NFSv4, restricted to the client IPs/CIDRs the admin provides - for
// source servers too low on local disk space to stage a backup archive
// before shipping it over SFTP/rsync.
func (h *Handlers) EnableNFS(c *gin.Context) {
	username := c.Param("username")
	mount := c.PostForm("mount")
	clientIPsRaw := c.PostForm("client_ips")

	if err := h.doEnableNFS(username, mount, clientIPsRaw); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("NFS erişimi etkinleştirildi: "+username))
}

func (h *Handlers) doEnableNFS(username, mount, clientIPsRaw string) error {
	if err := sysops.ValidateUsername(username); err != nil {
		return err
	}
	clientIPs, err := sysops.ValidateClientIPs(clientIPsRaw)
	if err != nil {
		return err
	}
	_, managed := sysops.SFTPStatus(h.cfg.SSHDConfigDir, h.cfg.SSHDMainConfig, username)
	if !managed {
		return fmt.Errorf("NFS için önce bu kullanıcının panel-yönetimli SFTP erişimi olmalı: %s", username)
	}

	homeDir := filepath.Join(mount, username)
	return sysops.EnableNFS(h.cfg.NFSExportsDir, h.cfg.NFSServiceName, username, homeDir, clientIPs)
}
