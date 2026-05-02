//go:build !thand_dev

package common

import "github.com/google/uuid"

// GetDeviceID returns the effective device identity for this machine.
func GetDeviceID() uuid.UUID {
	return getMachineDerivedDeviceID()
}
