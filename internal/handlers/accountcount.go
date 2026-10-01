package handlers

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/ahmetmakal/sftp-backup-ui/internal/sysops"
	"github.com/gin-gonic/gin"
)

func (h *Handlers) setupAccountCount(c *gin.Context, username, mount string) string {
	destHost, destPort := h.backupSFTPEndpoint(c)
	homeDir := filepath.Join(mount, username)

	err := sysops.SetupAccountCount(sysops.AccountCountRemoteParams{
		SourceHost:    username,
		SourceSSHPort: h.cfg.SourceSSHPort,
		SourceSSHUser: h.cfg.SourceSSHUser,
		Username:      username,
		DestHost:      destHost,
		DestPort:      destPort,
		ScriptPath:    h.cfg.AccountCountScript,
		HomeDir:       homeDir,
	})
	if err != nil {
		log.Printf("account count setup for %s: %v", username, err)
		if h.cfg.SourceSSHPort == "" {
			return " · hesap takibi script'i upload/'a kopyalanamadı"
		}
		return " · hesap takibi otomatik kurulamadı (script upload/'da, elle çalıştırın)"
	}
	if h.cfg.SourceSSHPort == "" {
		return " · hesap takibi script'i upload/'a kopyalandı"
	}
	return " · hesap takibi kuruldu"
}

func (h *Handlers) accountCountPullCmd(username, destHost, destPort string) string {
	keyPath := fmt.Sprintf("/root/.ssh/backup_%s", username)
	return fmt.Sprintf(
		`rsync -az -e "ssh -i %s -p %s -o BatchMode=yes -o StrictHostKeyChecking=no" %s@%s:upload/account-count-check-setup.sh /root/account-count-check-setup.sh && chmod +x /root/account-count-check-setup.sh && /root/account-count-check-setup.sh --hostname %s --dest-host %s --dest-port %s`,
		keyPath, destPort, username, destHost, username, destHost, destPort,
	)
}
