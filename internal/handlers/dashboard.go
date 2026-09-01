package handlers

import (
	"net"
	"net/http"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// Dashboard is the home page: live mounts + users + quotas, re-scanned from
// the system on every request.
func (h *Handlers) Dashboard(c *gin.Context) {
	data, err := sysops.LoadDashboard(h.cfg.MountRegex, h.cfg.SSHDConfigDir, h.cfg.SSHDMainConfig, h.cfg.NFSExportsDir)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Message": "Sistem durumu okunamadı: " + err.Error(),
		})
		return
	}

	destHost := c.Request.Host
	if host, _, err := net.SplitHostPort(destHost); err == nil {
		destHost = host
	}

	c.HTML(http.StatusOK, "dashboard.html", gin.H{
		"Mounts":   data.Mounts,
		"Users":    data.Users,
		"DestHost": destHost,
		"Flash":    c.Query("flash"),
		"Error":    c.Query("error"),
	})
}
