package common

import (
	"crypto/sha256"

	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
)

func getMachineDerivedDeviceID() uuid.UUID {
	// TODO(hugh): Check if the thand.io config exists and use that for an identifier.

	id, err := machineid.ID()
	if err != nil {
		// Fallback to a random ephemeral UUID if machine ID cannot be obtained.
		return uuid.New()
	}

	// Hash the machine ID and convert to UUID format.
	hash := sha256.Sum256([]byte(id))
	return uuid.UUID(hash[:16])
}

// GetClientIdentifier returns the stable machine-derived identifier used by
// legacy call sites. Device registration and routing should prefer GetDeviceID.
func GetClientIdentifier() uuid.UUID {
	return GetDeviceID()
}
