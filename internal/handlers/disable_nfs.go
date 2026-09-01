package handlers

import (
	"net/http"
	"net/url"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// DisableNFS removes an existing user's NFS export, revoking NFS access
// without touching their SFTP/SSH-key access.
func (h *Handlers) DisableNFS(c *gin.Context) {
	username := c.Param("username")

	if err := sysops.ValidateUsername(username); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	if err := sysops.DisableNFS(h.cfg.NFSExportsDir, username); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("NFS erişimi kapatıldı: "+username))
}
