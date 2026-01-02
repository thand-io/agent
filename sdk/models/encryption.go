// Package models provides public SDK types for the thand agent.
package models

import internal "github.com/thand-io/agent/internal/models"

// EncryptionService defines the encryption implementation interface.
// It provides methods for encrypting and decrypting sensitive data
// used by the agent for secure credential storage and transmission.
type EncryptionService = internal.EncryptionImpl
