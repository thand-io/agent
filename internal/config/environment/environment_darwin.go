//go:build darwin

package environment

import (
	"runtime"
	"strings"
	"syscall"
)

// detectDarwinVersion detects macOS version
func detectDarwinVersion() string {
	if runtime.GOOS != "darwin" {
		return "unknown"
	}

	// Try to get version from system call
	version, err := syscall.Sysctl("kern.osproductversion")
	if err == nil && len(version) > 0 {
		return version
	}

	// Fallback method using uname
	version, err = syscall.Sysctl("kern.version")
	if err == nil && len(version) > 0 {
		// Extract version number from kernel version string
		parts := strings.Fields(version)
		if len(parts) > 2 {
			return parts[2]
		}
	}

	return "darwin"
}
