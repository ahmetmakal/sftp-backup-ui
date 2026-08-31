package handlers

import (
	"net"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// EnableRsync provisions an rsyncd module + secret for an existing user and
// renders the generated password directly (never via redirect/query string,
// since - unlike the SSH public key - this is a genuine shared secret that
// must never land in a URL, browser history, or the access log).
func (h *Handlers) EnableRsync(c *gin.Context) {
	username := c.Param("username")
	mount := c.PostForm("mount")

	if err := sysops.ValidateUsername(username); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	exists, err := sysops.UserExists(username)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	if !exists {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape("kullanıcı bulunamadı: "+username))
		return
	}
	if sysops.HasRsyncModule(h.cfg.RsyncdDir, username) {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape("bu kullanıcı için zaten bir rsync modülü var: "+username))
		return
	}

	homeDir := filepath.Join(mount, username)
	password, err := sysops.WriteRsyncModule(h.cfg.RsyncdDir, h.cfg.RsyncdService, username, homeDir)
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}

	destHost := c.Request.Host
	if host, _, err := net.SplitHostPort(destHost); err == nil {
		destHost = host
	}

	c.HTML(http.StatusOK, "rsync_enabled.html", gin.H{
		"Username": username,
		"Module":   username,
		"Password": password,
		"DestHost": destHost,
	})
}
