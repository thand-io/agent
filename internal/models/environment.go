package models

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
)

// EnvironmentPlatform enum
type EnvironmentPlatform string

const (
	AWS        EnvironmentPlatform = "aws"
	GCP        EnvironmentPlatform = "gcp"
	Azure      EnvironmentPlatform = "azure"
	Kubernetes EnvironmentPlatform = "kubernetes"
	Local      EnvironmentPlatform = "local"
)

// NOTE: providers cannot be supported as they are loaded in
// post environment initialisation
type EnvironmentConfig struct {
	// This is the name of the environment or hostname
	Name string `mapstructure:"name" default:"development"`
	// AWS, GCP, Kubernetes, Local
	Platform EnvironmentPlatform `mapstructure:"platform" default:"aws"` // aws, gcp, kubernetes
	// Operating System details
	OperatingSystem        string `mapstructure:"os" default:"linux"`          // windows, mac, linux
	OperatingSystemVersion string `mapstructure:"os_version" default:"latest"` // e.g. 10, 11, 12 for macOS
	Architecture           string `mapstructure:"arch" default:"amd64"`        // amd64, arm64
	Ephemeral              bool   `mapstructure:"ephemeral" default:"false"`   // true if running in an ephemeral environment

	// Settings for the platform
	Config   *BasicConfig `mapstructure:"config"`   // Additional environment-specific config
	MetaData *BasicConfig `mapstructure:"metadata"` // Metadata for the environment

}

func (e *EnvironmentConfig) GetIdentifier() string {
	return common.ConvertToSnakeCase(
		fmt.Sprintf("thand-%s-%s", e.Platform, e.Name))
}
