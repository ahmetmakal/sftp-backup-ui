package sysops

import (
	"fmt"
	"os"
	"path/filepath"
)

const accountCountScriptName = "account-count-check-setup.sh"

// AccountCountRemoteParams configures a pull-and-install run on a source
// server. The source uses its existing backup private key to rsync the script
// from its own upload/ directory on the backup host, then executes it.
type AccountCountRemoteParams struct {
	SourceHost    string // SSH target from backup server (usually the username)
	SourceSSHPort string
	SourceSSHUser string
	Username      string // backup chroot user + key file suffix
	DestHost      string
	DestPort      string
	ScriptPath    string // local path to contrib script on backup server
	HomeDir       string // e.g. /backup1/server.example.com
}

// StageAccountCountScript copies the setup script into upload/ so the source
// server can fetch it with the same SFTP/rsync key used for backups.
func StageAccountCountScript(homeDir, scriptPath string) error {
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return fmt.Errorf("read account count script: %w", err)
	}
	dest := filepath.Join(homeDir, "upload", accountCountScriptName)
	if err := os.WriteFile(dest, content, 0o755); err != nil {
		return fmt.Errorf("write account count script: %w", err)
	}
	return nil
}

// RemoteInstallAccountCount SSHes from the backup server into the source
// server and runs a one-liner that pulls the staged script via the backup
// key and installs the daily cron job.
func RemoteInstallAccountCount(p AccountCountRemoteParams) error {
	if p.SourceSSHPort == "" {
		return fmt.Errorf("SOURCE_SSH_PORT is not configured")
	}
	user := p.SourceSSHUser
	if user == "" {
		user = "root"
	}
	keyPath := fmt.Sprintf("/root/.ssh/backup_%s", p.Username)
	remote := fmt.Sprintf(
		`rsync -az -e "ssh -i %s -p %s -o BatchMode=yes -o ConnectTimeout=10 -o StrictHostKeyChecking=no" %s@%s:upload/%s /root/%s && chmod +x /root/%s && /root/%s --hostname %s --dest-host %s --dest-port %s`,
		keyPath, p.DestPort,
		p.Username, p.DestHost, accountCountScriptName,
		accountCountScriptName, accountCountScriptName, accountCountScriptName,
		p.Username, p.DestHost, p.DestPort,
	)
	target := fmt.Sprintf("%s@%s", user, p.SourceHost)
	_, err := run("ssh",
		"-p", p.SourceSSHPort,
		"-o", "BatchMode=yes",
		"-o", "ConnectTimeout=15",
		"-o", "StrictHostKeyChecking=no",
		target,
		remote,
	)
	if err != nil {
		return fmt.Errorf("remote account count install on %s: %w", p.SourceHost, err)
	}
	return nil
}

// SetupAccountCount stages the script in upload/ and, when SOURCE_SSH_PORT is
// set, triggers pull-and-install on the source server.
func SetupAccountCount(p AccountCountRemoteParams) error {
	if err := StageAccountCountScript(p.HomeDir, p.ScriptPath); err != nil {
		return err
	}
	if p.SourceSSHPort == "" {
		return nil
	}
	return RemoteInstallAccountCount(p)
}
