//go:build !darwin

package environment

// detectDarwinVersion detects macOS version (stub for non-Darwin platforms)
func detectDarwinVersion() string {
	return "unknown"
}
