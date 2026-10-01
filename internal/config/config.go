package config

import (
	"fmt"
	"os"
	"regexp"
	"strconv"
)

// Config holds all runtime configuration, sourced entirely from environment
// variables. There is no config file and no database.
type Config struct {
	ListenAddr    string
	AdminUser     string
	AdminPassword string

	MountRegex     *regexp.Regexp
	SSHDConfigDir  string
	SSHDMainConfig string
	SSHDService    string
	ProjectIDBase  int

	NFSExportsDir  string
	NFSServiceName string

	// BackupSFTPHost is the address source servers use for SFTP/rsync uploads.
	// When empty, handlers fall back to the HTTP request Host header.
	BackupSFTPHost string
	BackupSFTPPort string

	// SourceSSHPort enables automatic account-count setup: after an SSH key is
	// added the backup server SSHes into the source (as SourceSSHUser) and
	// runs a pull+install one-liner using the source's existing backup key.
	SourceSSHPort string
	SourceSSHUser string

	AccountCountScript string
}

func Load() (*Config, error) {
	adminUser := getEnv("ADMIN_USER", "admin")
	adminPassword := os.Getenv("ADMIN_PASSWORD")
	if adminPassword == "" {
		return nil, fmt.Errorf("ADMIN_PASSWORD environment variable must be set")
	}

	mountPattern := getEnv("BACKUP_MOUNT_REGEX", `^/backup[0-9]+$`)
	mountRegex, err := regexp.Compile(mountPattern)
	if err != nil {
		return nil, fmt.Errorf("invalid BACKUP_MOUNT_REGEX %q: %w", mountPattern, err)
	}

	projectIDBase, err := strconv.Atoi(getEnv("PROJECT_ID_BASE", "100"))
	if err != nil {
		return nil, fmt.Errorf("invalid PROJECT_ID_BASE: %w", err)
	}

	return &Config{
		ListenAddr:     getEnv("LISTEN_ADDR", ":8080"),
		AdminUser:      adminUser,
		AdminPassword:  adminPassword,
		MountRegex:     mountRegex,
		SSHDConfigDir:  getEnv("SSHD_CONFIG_DIR", "/etc/ssh/sshd_config.d"),
		SSHDMainConfig: getEnv("SSHD_MAIN_CONFIG", "/etc/ssh/sshd_config"),
		SSHDService:    getEnv("SSHD_SERVICE_NAME", "sshd"),
		ProjectIDBase:  projectIDBase,

		NFSExportsDir:  getEnv("NFS_EXPORTS_DIR", "/etc/exports.d"),
		NFSServiceName: getEnv("NFS_SERVICE_NAME", "nfs-server"),

		BackupSFTPHost: getEnv("BACKUP_SFTP_HOST", ""),
		BackupSFTPPort: getEnv("BACKUP_SFTP_PORT", "22"),

		SourceSSHPort:      getEnv("SOURCE_SSH_PORT", ""),
		SourceSSHUser:      getEnv("SOURCE_SSH_USER", "root"),
		AccountCountScript: getEnv("ACCOUNT_COUNT_SCRIPT", "/opt/sftp-backup-ui/contrib/account-count-check-setup.sh"),
	}, nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
