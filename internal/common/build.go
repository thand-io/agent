package common

import (
	"fmt"
	"runtime/debug"
)

// Version and GitCommit can be set via ldflags at build time
var (
	Version   = "dev"
	GitCommit = "unknown"
)

func GetModuleBuildInfo() (string, string, bool) {
	// If version was set via ldflags, use it
	if Version != "dev" {
		return Version, GitCommit, true
	}

	// Otherwise, try to get from runtime debug info
	if info, ok := debug.ReadBuildInfo(); ok {
		version := info.Main.Version
		var gitCommit string

		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				gitCommit = setting.Value
				break
			}
		}

		return version, gitCommit, true
	}
	return "", "", false
}

func GetClientIdentifier() string {
	version, gitCommit, _ := GetModuleBuildInfo()
	return fmt.Sprintf("%s-%s", version, gitCommit)
}
