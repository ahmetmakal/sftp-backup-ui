package handlers

import (
	"net"

	"github.com/gin-gonic/gin"
)

// backupSFTPEndpoint returns the host:port that source servers use for SFTP/rsync.
// BACKUP_SFTP_HOST/PORT env vars override the HTTP request host when set.
func (h *Handlers) backupSFTPEndpoint(c *gin.Context) (host, port string) {
	host = h.cfg.BackupSFTPHost
	port = h.cfg.BackupSFTPPort
	if host == "" {
		host = c.Request.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
	}
	if port == "" {
		port = "22"
	}
	return host, port
}
