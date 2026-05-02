//go:build thand_dev

package common

import (
	"os"
	"strings"

	"github.com/google/uuid"
)

const deviceIDOverrideEnvVar = "THAND_DEV_DEVICE_ID_OVERRIDE"

// GetDeviceID returns the effective device identity for this machine.
// Dev-tagged builds may override the machine-derived ID for deterministic tests.
func GetDeviceID() uuid.UUID {
	override := strings.TrimSpace(os.Getenv(deviceIDOverrideEnvVar))
	if override != "" {
		if parsed, err := uuid.Parse(override); err == nil {
			return parsed
		}
	}

	return getMachineDerivedDeviceID()
}
