package handlers

import (
	"fmt"
	"net/http"
	"net/url"
	"path/filepath"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

// UpdateSSHKey adds or replaces a user's authorized_keys after creation.
func (h *Handlers) UpdateSSHKey(c *gin.Context) {
	username := c.Param("username")
	mount := c.PostForm("mount")
	pubKey := c.PostForm("pub_key")

	if err := h.doUpdateSSHKey(username, mount, pubKey); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("SSH anahtarı güncellendi: "+username))
}

func (h *Handlers) doUpdateSSHKey(username, mount, pubKey string) error {
	if err := sysops.ValidateUsername(username); err != nil {
		return err
	}
	if err := sysops.ValidateSSHPublicKey(pubKey); err != nil {
		return err
	}
	exists, err := sysops.UserExists(username)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kullanıcı bulunamadı: %s", username)
	}
	homeDir := filepath.Join(mount, username)
	return sysops.WriteAuthorizedKeys(homeDir, username, pubKey)
}
