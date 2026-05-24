package identity

import (
	"regexp"
)

const (
	// MaxAccountNameLength caps local usernames and group names accepted by the helper.
	MaxAccountNameLength = 32
	// MaxWindowsAdminGroupLength caps the configured Windows admin group name.
	MaxWindowsAdminGroupLength = 64
)

var (
	accountNamePattern       = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*[$]?$`)
	windowsAdminGroupPattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9 ._-]*$`)
)

// ValidAccountName reports whether v is a conservative helper-safe local account or group name.
func ValidAccountName(v string) bool {
	if v == "" || len(v) > MaxAccountNameLength {
		return false
	}
	return accountNamePattern.MatchString(v)
}

// ValidWindowsAdminGroup reports whether v is a helper-safe Windows local admin group name.
func ValidWindowsAdminGroup(v string) bool {
	if v == "" || len(v) > MaxWindowsAdminGroupLength {
		return false
	}
	return windowsAdminGroupPattern.MatchString(v)
}
