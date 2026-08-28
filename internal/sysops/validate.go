package sysops

import (
	"fmt"
	"regexp"
)

// usernameRegex mirrors typical Linux useradd constraints while also
// allowing dots, since backup usernames here are source-server hostnames
// (e.g. server.example.com).
var usernameRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,190}[a-z0-9])?$`)

// quotaSizeRegex requires an explicit unit so admins can't accidentally
// enter a byte count meant as gigabytes. Matches what xfs_quota accepts
// for bsoft=/bhard= (e.g. "200g", "500m").
var quotaSizeRegex = regexp.MustCompile(`^[0-9]+[kKmMgGtT]$`)

func ValidateUsername(username string) error {
	if !usernameRegex.MatchString(username) {
		return fmt.Errorf("invalid username %q: must be lowercase alphanumeric, dots or hyphens, and cannot start/end with a separator", username)
	}
	return nil
}

func ValidateQuotaSize(size string) error {
	if !quotaSizeRegex.MatchString(size) {
		return fmt.Errorf("invalid quota size %q: expected a number followed by a unit (k/m/g/t), e.g. 200g", size)
	}
	return nil
}
