package handlers

import (
	"net/http"
	"net/url"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// DisableRsync removes an existing user's rsync module + secret, revoking
// rsync access without touching their SFTP/SSH-key access.
func (h *Handlers) DisableRsync(c *gin.Context) {
	username := c.Param("username")

	if err := sysops.ValidateUsername(username); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	if err := sysops.RemoveRsyncModule(h.cfg.RsyncdDir, h.cfg.RsyncdService, username); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("Rsync erişimi kapatıldı: "+username))
}
