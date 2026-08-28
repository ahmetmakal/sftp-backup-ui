package handlers

import (
	"fmt"
	"net/http"
	"net/url"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) NewUserForm(c *gin.Context) {
	mounts, err := sysops.DiscoverMounts(h.cfg.MountRegex)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{
			"Message": "Backup mount'ları keşfedilemedi: " + err.Error(),
		})
		return
	}
	c.HTML(http.StatusOK, "create_user.html", gin.H{
		"Mounts": mounts,
		"Error":  c.Query("error"),
	})
}

func (h *Handlers) CreateUser(c *gin.Context) {
	username := c.PostForm("username")
	mount := c.PostForm("mount")
	quotaSize := c.PostForm("quota_size")
	pubKey := c.PostForm("pub_key")

	if err := h.validateCreateUser(username, mount, quotaSize, pubKey); err != nil {
		c.Redirect(http.StatusSeeOther, "/users/new?error="+url.QueryEscape(err.Error()))
		return
	}

	err := sysops.CreateBackupUser(sysops.CreateUserParams{
		Username:      username,
		Mount:         mount,
		QuotaSize:     quotaSize,
		PubKey:        pubKey,
		SSHDConfigDir: h.cfg.SSHDConfigDir,
		SSHDService:   h.cfg.SSHDService,
		ProjectIDBase: h.cfg.ProjectIDBase,
	})
	if err != nil {
		c.Redirect(http.StatusSeeOther, "/users/new?error="+url.QueryEscape(err.Error()))
		return
	}

	keyNote := "SSH key sonradan eklenebilir"
	if pubKey != "" {
		keyNote = "SSH key eklendi"
	}
	msg := fmt.Sprintf("Kullanıcı oluşturuldu: %s · %s · %s kota · %s", username, mount, quotaSize, keyNote)
	c.Redirect(http.StatusSeeOther, "/?flash="+url.QueryEscape(msg))
}

func (h *Handlers) validateCreateUser(username, mount, quotaSize, pubKey string) error {
	if err := sysops.ValidateUsername(username); err != nil {
		return err
	}
	if err := sysops.ValidateQuotaSize(quotaSize); err != nil {
		return err
	}
	if pubKey != "" {
		if err := sysops.ValidateSSHPublicKey(pubKey); err != nil {
			return err
		}
	}

	exists, err := sysops.UserExists(username)
	if err != nil {
		return err
	}
	if exists {
		return fmt.Errorf("kullanıcı zaten mevcut: %s", username)
	}

	mounts, err := sysops.DiscoverMounts(h.cfg.MountRegex)
	if err != nil {
		return err
	}
	var found bool
	for _, m := range mounts {
		if m.Path == mount {
			found = true
			if !m.QuotaEnabled {
				return fmt.Errorf("seçilen mount'ta proje kotası aktif değil: %s", mount)
			}
			break
		}
	}
	if !found {
		return fmt.Errorf("bilinmeyen ya da artık mevcut olmayan mount: %s", mount)
	}
	return nil
}
