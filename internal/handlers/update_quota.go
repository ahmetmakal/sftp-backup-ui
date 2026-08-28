package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) UpdateQuota(c *gin.Context) {
	username := c.Param("username")
	mount := c.PostForm("mount")
	quotaSize := c.PostForm("quota_size")

	if err := h.doUpdateQuota(username, mount, quotaSize); err != nil {
		c.Redirect(http.StatusSeeOther, "/?error="+url.QueryEscape(err.Error()))
		return
	}
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape("Kota güncellendi: "+username))
}

func (h *Handlers) doUpdateQuota(username, mount, quotaSize string) error {
	if err := sysops.ValidateUsername(username); err != nil {
		return err
	}
	if err := sysops.ValidateQuotaSize(quotaSize); err != nil {
		return err
	}
	exists, err := sysops.UserExists(username)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("kullanıcı bulunamadı: %s", username)
	}
	return sysops.SetQuotaLimit(mount, username, quotaSize)
}
