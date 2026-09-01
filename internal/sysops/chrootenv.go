package sysops

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

const (
	sftpServerPath = "/usr/libexec/openssh/sftp-server"
	rsyncPath      = "/usr/bin/rsync"
	bashHostPath   = "/usr/bin/bash"
	bashChrootPath = "/bin/bash"
	devNullPath    = "/dev/null"

	// noLoginShell is what CreateUser sets new accounts' shell to before
	// EnableChrootShell switches it to bashChrootPath.
	noLoginShell = "/usr/sbin/nologin"
)

// coreutilsPaths are the handful of basic file-management commands some
// rsync/sftp automation clients (cPanel's Rsync destination among them) run
// directly over the SSH session rather than through the SFTP protocol -
// e.g. "mkdir -p" before a transfer, "mv" to atomically rename an uploaded
// file into place. ls is included for interactive convenience when an
// admin logs in directly. This is deliberately not the full coreutils
// package: no vim, ps, or anything beyond basic file listing/management.
var coreutilsPaths = []string{
	"/usr/bin/mkdir",
	"/usr/bin/mv",
	"/usr/bin/rm",
	"/usr/bin/cat",
	"/usr/bin/chmod",
	"/usr/bin/stat",
	"/usr/bin/df",
	"/usr/bin/test",
	"/usr/bin/ls",
}

// bindPair is one file that needs to be reachable inside a chroot jail: src
// is its real path on the host, dst is where it must appear relative to the
// chroot root. For system binaries/libraries src and dst are the same path
// (so paths baked into the binary, like the dynamic linker's expectations,
// still resolve correctly inside the jail).
type bindPair struct {
	src string
	dst string
}

// EnableChrootShell turns a bare SFTP chroot into one where the user also
// has a real shell and rsync: writes a minimal per-user /etc/passwd and
// /etc/group (both bash and the external sftp-server binary - unlike sshd's
// built-in internal-sftp - resolve the connecting uid via NSS), bind-mounts
// (read-only) bash, rsync, sftp-server, coreutilsPaths, /dev/null, and their
// current shared library dependencies into the jail, persists those mounts
// in /etc/fstab so they survive a reboot, and switches the account's login
// shell to the jailed bash. Every step is idempotent, so re-running this
// (e.g. after an OS update replaced /usr/bin/rsync, or to pick up a newly
// added binary) refreshes any stale bind mounts.
//
// This does not touch sshd configuration - the caller is responsible for
// the chroot's Match block (see WriteSSHDDropin). With no ForceCommand,
// sshd runs the SFTP subsystem (configured globally as the external
// sftp-server binary, not the "internal-sftp" special case) directly, and
// falls back to the user's real shell for anything else - direct command
// exec (e.g. rsync -e ssh, which execs "$SHELL -c 'rsync --server ...'") or
// an interactive login.
func EnableChrootShell(username, chrootDir string) error {
	targets, err := resolveBindTargets()
	if err != nil {
		return fmt.Errorf("resolve rsync/sftp-server dependencies: %w", err)
	}

	var mountedDests []string
	shellChanged := false
	undo := func() {
		for _, dest := range slices.Backward(mountedDests) {
			unbindOne(dest)
		}
		_ = removeFstabEntriesByChroot(chrootDir)
		removeIdentityFiles(chrootDir)
		if shellChanged {
			_, _ = run("usermod", "-s", noLoginShell, username)
		}
	}

	// A synthetic file holding only this user's own entry - not a
	// bind-mount of the host's real /etc/passwd - keeps every other system
	// account invisible from inside the jail.
	if err := writeIdentityFiles(username, chrootDir); err != nil {
		undo()
		return fmt.Errorf("write chroot identity files: %w", err)
	}

	for _, t := range targets {
		dest := filepath.Join(chrootDir, t.dst)
		if err := bindOne(t.src, dest); err != nil {
			undo()
			return fmt.Errorf("bind-mount %s: %w", t.src, err)
		}
		mountedDests = append(mountedDests, dest)
	}

	if err := writeFstabEntries(username, chrootDir, targets); err != nil {
		undo()
		return fmt.Errorf("persist fstab entries: %w", err)
	}

	if _, err := run("usermod", "-s", bashChrootPath, username); err != nil {
		undo()
		return fmt.Errorf("set login shell: %w", err)
	}
	shellChanged = true

	return nil
}

// DisableChrootShell tears down everything EnableChrootShell set up:
// unmounts everything under chrootDir, removes the persisted fstab entries
// and identity files, and restores the account's shell to noLoginShell.
// Safe to call on a chroot that was never enabled (every step is a no-op in
// that case).
func DisableChrootShell(username, chrootDir string) error {
	_, _ = run("usermod", "-s", noLoginShell, username)
	unmountAllUnder(chrootDir)
	if err := removeFstabEntriesByChroot(chrootDir); err != nil {
		return fmt.Errorf("clean up fstab entries: %w", err)
	}
	removeIdentityFiles(chrootDir)
	return nil
}

// IsChrootShellEnabled reports whether bash is currently mounted into
// chrootDir.
func IsChrootShellEnabled(chrootDir string) bool {
	return isMounted(filepath.Join(chrootDir, bashChrootPath))
}

// resolveBindTargets returns bash, rsync, sftp-server, coreutilsPaths,
// /dev/null, and their current shared library dependencies (via ldd),
// deduplicated. Doing this at call time rather than hardcoding the library
// list means a future OS/package upgrade that renames a dependency doesn't
// require a code change.
func resolveBindTargets() ([]bindPair, error) {
	binaries := append([]string{sftpServerPath, rsyncPath, bashHostPath}, coreutilsPaths...)

	pairs := []bindPair{
		{src: sftpServerPath, dst: sftpServerPath},
		{src: rsyncPath, dst: rsyncPath},
		{src: bashHostPath, dst: bashChrootPath},
		{src: devNullPath, dst: devNullPath},
	}
	seen := map[string]bool{sftpServerPath: true, rsyncPath: true, bashHostPath: true, devNullPath: true}
	for _, p := range coreutilsPaths {
		pairs = append(pairs, bindPair{src: p, dst: p})
		seen[p] = true
	}

	for _, bin := range binaries {
		deps, err := lddDeps(bin)
		if err != nil {
			return nil, fmt.Errorf("ldd %s: %w", bin, err)
		}
		for _, dep := range deps {
			if seen[dep] {
				continue
			}
			seen[dep] = true
			pairs = append(pairs, bindPair{src: dep, dst: dep})
		}
	}
	return pairs, nil
}

// lddDeps parses `ldd <path>` output for the real file paths it resolved -
// skipping virtual entries like linux-vdso.so.1 that have no backing file.
func lddDeps(binPath string) ([]string, error) {
	out, err := run("ldd", binPath)
	if err != nil {
		return nil, err
	}

	var deps []string
	for line := range strings.SplitSeq(out, "\n") {
		fields := strings.Fields(line)
		var path string
		switch {
		case len(fields) >= 3 && fields[1] == "=>" && strings.HasPrefix(fields[2], "/"):
			path = fields[2]
		case len(fields) >= 1 && strings.HasPrefix(fields[0], "/"):
			path = fields[0]
		default:
			continue
		}
		if !slices.Contains(deps, path) {
			deps = append(deps, path)
		}
	}
	return deps, nil
}

// bindOne bind-mounts src at dst (creating dst as an empty placeholder file
// first if needed) and remounts it read-only. A no-op if dst is already a
// mount point.
func bindOne(src, dst string) error {
	if isMounted(dst) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}
	if _, err := os.Stat(dst); os.IsNotExist(err) {
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY, 0o644)
		if err != nil {
			return fmt.Errorf("create placeholder: %w", err)
		}
		f.Close()
	}
	if _, err := run("mount", "--bind", src, dst); err != nil {
		return fmt.Errorf("mount --bind: %w", err)
	}
	if _, err := run("mount", "-o", "remount,ro,bind", dst); err != nil {
		_, _ = run("umount", dst)
		return fmt.Errorf("remount read-only: %w", err)
	}
	return nil
}

func unbindOne(dst string) {
	if isMounted(dst) {
		_, _ = run("umount", dst)
	}
}

// unmountAllUnder unmounts every current mount point under chrootDir,
// deepest paths first.
func unmountAllUnder(chrootDir string) {
	points := mountPointsUnder(chrootDir)
	slices.SortFunc(points, func(a, b string) int { return len(b) - len(a) })
	for _, p := range points {
		_, _ = run("umount", p)
	}
}

func mountPointsUnder(chrootDir string) []string {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return nil
	}
	prefix := chrootDir + "/"
	var points []string
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[1], prefix) {
			points = append(points, fields[1])
		}
	}
	return points
}

func isMounted(path string) bool {
	data, err := os.ReadFile("/proc/mounts")
	if err != nil {
		return false
	}
	needle := " " + path + " "
	return strings.Contains(string(data), needle)
}

func fstabHeader(username, chrootDir string) string {
	return fmt.Sprintf("# sftp-backup-ui chroot-shell: %s (%s)", username, chrootDir)
}

func writeFstabEntries(username, chrootDir string, targets []bindPair) error {
	if err := removeFstabEntriesByChroot(chrootDir); err != nil {
		return err
	}
	f, err := os.OpenFile("/etc/fstab", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err := fmt.Fprintln(f, fstabHeader(username, chrootDir)); err != nil {
		return err
	}
	for _, t := range targets {
		dest := filepath.Join(chrootDir, t.dst)
		if _, err := fmt.Fprintf(f, "%s %s none bind,ro 0 0\n", t.src, dest); err != nil {
			return err
		}
	}
	return nil
}

// removeFstabEntriesByChroot drops every fstab line whose mount point falls
// under chrootDir, plus our header comment, and rewrites the file. Any
// other admin-managed fstab content is left untouched.
func removeFstabEntriesByChroot(chrootDir string) error {
	data, err := os.ReadFile("/etc/fstab")
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}

	prefix := chrootDir + "/"
	var kept []string
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && strings.HasPrefix(fields[1], prefix) {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "# sftp-backup-ui chroot-shell:") && strings.Contains(trimmed, prefix[:len(prefix)-1]) {
			continue
		}
		kept = append(kept, line)
	}
	return os.WriteFile("/etc/fstab", []byte(strings.Join(kept, "\n")), 0o644)
}

// writeIdentityFiles creates a minimal /etc/passwd and /etc/group inside
// chrootDir containing only username's own entry, copied from the host's
// real records. This is deliberately not a bind-mount of the host's actual
// /etc/passwd: that would let bash and the external sftp-server binary
// (and anyone with a shell inside the jail) resolve every system account's
// name, not just this user's own.
func writeIdentityFiles(username, chrootDir string) error {
	etcDir := filepath.Join(chrootDir, "etc")
	if err := os.MkdirAll(etcDir, 0o755); err != nil {
		return fmt.Errorf("create etc dir: %w", err)
	}
	passwd, err := run("getent", "passwd", username)
	if err != nil {
		return fmt.Errorf("getent passwd %s: %w", username, err)
	}
	if err := os.WriteFile(filepath.Join(etcDir, "passwd"), []byte(strings.TrimSpace(passwd)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write chroot passwd: %w", err)
	}
	group, err := run("getent", "group", username)
	if err != nil {
		return fmt.Errorf("getent group %s: %w", username, err)
	}
	if err := os.WriteFile(filepath.Join(etcDir, "group"), []byte(strings.TrimSpace(group)+"\n"), 0o644); err != nil {
		return fmt.Errorf("write chroot group: %w", err)
	}
	return nil
}

func removeIdentityFiles(chrootDir string) {
	_ = os.Remove(filepath.Join(chrootDir, "etc", "passwd"))
	_ = os.Remove(filepath.Join(chrootDir, "etc", "group"))
}
