package common

import (
	internal "github.com/thand-io/agent/internal/common"
)

// GetBuildIdentifier returns a string that uniquely identifies the build, including version and commit hash.
var GetBuildIdentifier = internal.GetBuildIdentifier

// GetModuleBuildInfo returns detailed build information for the module, such as version, commit hash, and build time.
var GetModuleBuildInfo = internal.GetModuleBuildInfo
