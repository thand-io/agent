package models

import internal "github.com/thand-io/agent/internal/models"

// Identity is an alias for the internal Identity type.
// It represents either a user or a group in the system, serving as a unified
// abstraction for access control subjects.
// See internal/models.Identity for full documentation.
type Identity = internal.Identity

// IdentitiesResponse represents a response containing multiple identities.
type IdentitiesResponse = internal.IdentitiesResponse
