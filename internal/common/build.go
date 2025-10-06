package common

import (
	"fmt"
	"runtime/debug"
)

func GetModuleBuildInfo() (string, string, bool) {
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
