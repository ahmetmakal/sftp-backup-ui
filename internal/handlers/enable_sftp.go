package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// EnableSFTP writes the app-managed sshd chroot drop-in for an existing
// user that currently has no SFTP Match block at all (neither managed nor
// manual). Used to bring pre-existing/manually-created users under
// management without having to recreate them.
func (h *Handlers) EnableSFTP(c *gin.Context) {
	username := c.Param("username")
	mount := c.PostForm("mount")

	if err := h.doEnableSFTP(username, mount); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("SFTP chroot etkinleştirildi: "+username))
}

func (h *Handlers) doEnableSFTP(username, mount string) error {
	if err := sysops.ValidateUsername(username); err != nil {
		return err
	}
	exists, err := sysops.UserExists(username)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kullanıcı bulunamadı: %s", username)
	}

	active, _ := sysops.SFTPStatus(h.cfg.SSHDConfigDir, h.cfg.SSHDMainConfig, username)
	if active {
		return fmt.Errorf("bu kullanıcı için zaten bir SFTP chroot yapılandırması var: %s", username)
	}

	homeDir := filepath.Join(mount, username)
	return sysops.WriteSSHDDropin(h.cfg.SSHDConfigDir, h.cfg.SSHDService, username, homeDir)
}
