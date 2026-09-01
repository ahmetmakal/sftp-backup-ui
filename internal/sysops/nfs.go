package sysops

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

// exportsFilePath is the per-user NFS export drop-in, following the same
// pattern as sshd_config.d: nfs-utils automatically includes every
// *.exports file under exportsDir (see `man exports`), so enabling/
// disabling NFS for one user never touches any other user's export or the
// shared /etc/exports file.
func exportsFilePath(exportsDir, username string) string {
	return filepath.Join(exportsDir, username+".exports")
}

// ValidateClientIPs parses a newline/comma-separated list of IPv4/IPv6
// addresses or CIDR blocks - the set of source hosts allowed to mount an
// NFS export. Mirrors ValidateSSHPublicKey's per-line parse-and-report
// style (sshkeys.go).
func ValidateClientIPs(raw string) ([]string, error) {
	var ips []string
	for _, field := range strings.FieldsFunc(raw, func(r rune) bool { return r == '\n' || r == '\r' || r == ',' }) {
		entry := strings.TrimSpace(field)
		if entry == "" {
			continue
		}
		if _, _, err := net.ParseCIDR(entry); err == nil {
			ips = append(ips, entry)
			continue
		}
		if net.ParseIP(entry) == nil {
			return nil, fmt.Errorf("invalid IP or CIDR %q", entry)
		}
		ips = append(ips, entry)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("at least one client IP or CIDR is required")
	}
	return ips, nil
}

// EnableNFS exports <homeDir>/upload over NFSv4 to clientIPs only. Every
// write through this export - regardless of the uid the client claims - is
// mapped (all_squash) to this user's own uid/gid, the same account already
// created for SFTP: file ownership stays consistent, and even a fully
// compromised client can only write into this one user's own
// quota-bounded directory, never anything else on the backup server.
func EnableNFS(exportsDir, nfsServiceName, username, homeDir string, clientIPs []string) error {
	u, err := user.Lookup(username)
	if err != nil {
		return fmt.Errorf("look up system user %s: %w", username, err)
	}

	exportPath := filepath.Join(homeDir, "upload")
	if _, err := os.Stat(exportPath); err != nil {
		return fmt.Errorf("upload directory does not exist: %w", err)
	}

	if _, err := run("systemctl", "is-active", nfsServiceName); err != nil {
		return fmt.Errorf("%s is not active: %w", nfsServiceName, err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "# Managed by sftp-backup-ui - do not edit by hand.\n%s", exportPath)
	for _, ip := range clientIPs {
		// insecure: don't require the client to source its connection from
		// a reserved (<1024) port. The default "secure" behavior breaks
		// for any client behind NAT/a load balancer that doesn't preserve
		// the original source port - confirmed against a real mount
		// attempt, which failed with a bare "Operation not permitted" and
		// no server-side log line explaining why.
		fmt.Fprintf(&b, " %s(rw,sync,no_subtree_check,insecure,all_squash,anonuid=%s,anongid=%s)", ip, u.Uid, u.Gid)
	}
	b.WriteString("\n")

	if err := os.MkdirAll(exportsDir, 0o755); err != nil {
		return fmt.Errorf("create exports dir: %w", err)
	}
	path := exportsFilePath(exportsDir, username)
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return fmt.Errorf("write export file: %w", err)
	}

	if _, err := run("exportfs", "-ra"); err != nil {
		_ = os.Remove(path)
		_, _ = run("exportfs", "-ra")
		return fmt.Errorf("exportfs -ra failed, export removed: %w", err)
	}
	return nil
}

// DisableNFS removes username's export drop-in and re-exports. A no-op if
// NFS was never enabled for this user.
func DisableNFS(exportsDir, username string) error {
	path := exportsFilePath(exportsDir, username)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove export file: %w", err)
	}
	if _, err := run("exportfs", "-ra"); err != nil {
		return fmt.Errorf("exportfs -ra failed after removing export: %w", err)
	}
	return nil
}

// IsNFSEnabled reports whether username currently has an export drop-in
// and, if so, the client IPs/CIDRs it's restricted to (for display).
func IsNFSEnabled(exportsDir, username string) (bool, []string) {
	data, err := os.ReadFile(exportsFilePath(exportsDir, username))
	if err != nil {
		return false, nil
	}

	var ips []string
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		fields := strings.Fields(line)
		for _, f := range fields[1:] {
			if ip, _, ok := strings.Cut(f, "("); ok {
				ips = append(ips, ip)
			}
		}
	}
	return true, ips
}
