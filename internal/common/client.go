package common

import (
	"crypto/sha256"
	"os"
	"strings"

	"github.com/denisbrodbeck/machineid"
	"github.com/google/uuid"
)

const clientIdentifierEnvVar = "THAND_AGENT_ID"

// GetClientIdentifier returns a UUID that uniquely identifies this system.
// It uses the machine's hardware ID to generate a consistent, system-specific UUID.
func GetClientIdentifier() uuid.UUID {
	// If set, this env var overrides the default machine-derived identifier.
	//
	// Accepts:
	//   - a UUID string (returned as-is)
	//   - any non-empty string (hashed deterministically to a UUID)
	if value := strings.TrimSpace(os.Getenv(clientIdentifierEnvVar)); value != "" {
		if parsed, err := uuid.Parse(value); err == nil {
			return parsed
		}

		hash := sha256.Sum256([]byte(value))
		return uuid.UUID(hash[:16])
	}

	// TODO(hugh): Check if the thand.io config exists and use that for an identifier.

	id, err := machineid.ID()
	if err != nil {
		// Fallback to a random ephemeral UUID if machine ID cannot be obtained
		return uuid.New()
	}

	// Hash the machine ID and convert to UUID format
	hash := sha256.Sum256([]byte(id))
	return uuid.UUID(hash[:16])
}
