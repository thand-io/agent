package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// VaultService provides an interface for secrets management, enabling secure storage and retrieval
// of sensitive data like API keys, passwords, tokens, and credentials. It ensures secrets are
// encrypted at rest and accessed securely by workflows and providers.
//
// Used for storing provider credentials, OAuth tokens, service account keys, encryption keys,
// and workflow secrets. Supported providers include AWS Secrets Manager, GCP Secret Manager,
// Azure Key Vault, and local file-based storage. Configure in config.yaml under services.vault.
type VaultService = internal.VaultImpl
