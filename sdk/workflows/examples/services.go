package examples

import "github.com/thand-io/agent/sdk/models"

// Services is a minimal stub implementation of models.ServicesClientImpl for testing and examples.
// It implements the services interface but returns nil for all service getters and false for all
// availability checks. This allows workflows to be tested without requiring actual service connections
// to encryption providers, vaults, schedulers, Temporal, or LLM services.
type Services struct {
}

// Initialize sets up all configured services (encryption, vault, storage, scheduler, LLM, temporal).
// In this stub implementation, it does nothing and returns nil since no actual services are configured.
func (s *Services) Initialize() error {
	return nil
}

// Shutdown gracefully closes all active service connections and releases resources.
// In this stub implementation, it does nothing and returns nil since no actual services are running.
func (s *Services) Shutdown() error {
	return nil
}

// GetEncryption returns the encryption service used for encrypting sensitive data at rest
// and in transit. Supported providers include AWS KMS, GCP KMS, Azure Key Vault, and local encryption.
// This stub returns nil.
func (s *Services) GetEncryption() models.EncryptionService {
	return nil
}

// HasEncryption checks if an encryption service is configured and available.
// This stub returns false.
func (s *Services) HasEncryption() bool {
	return false
}

// GetVault returns the vault service for storing and retrieving secrets like API keys,
// passwords, and credentials. Supported providers include AWS Secrets Manager, GCP Secret Manager,
// Azure Key Vault, and local file-based storage. This stub returns nil.
func (s *Services) GetVault() models.VaultService {
	return nil
}

// HasVault checks if a vault service is configured and available.
// This stub returns false.
func (s *Services) HasVault() bool {
	return false
}

// GetStorage returns the storage service for persisting workflow state, audit logs,
// and other application data. Supported backends include databases and cloud storage.
// This stub returns nil.
func (s *Services) GetStorage() models.StorageService {
	return nil
}

// HasStorage checks if a storage service is configured and available.
// This stub returns false.
func (s *Services) HasStorage() bool {
	return false
}

// GetScheduler returns the scheduler service for managing scheduled tasks and cron jobs.
// This handles time-based workflow triggers and periodic operations. Supported providers
// include local schedulers and cloud-based scheduling services. This stub returns nil.
func (s *Services) GetScheduler() models.SchedulerService {
	return nil
}

// HasScheduler checks if a scheduler service is configured and available.
// This stub returns false.
func (s *Services) HasScheduler() bool {
	return false
}

// GetLargeLanguageModel returns the LLM service for AI-powered features like natural language
// access requests, policy generation, and intelligent automation. Supported providers include
// OpenAI, Anthropic Claude, Google Gemini, and other LLM APIs. This stub returns nil.
func (s *Services) GetLargeLanguageModel() models.LargeLanguageModelService {
	return nil
}

// HasLargeLanguageModel checks if an LLM service is configured and available.
// This stub returns false.
func (s *Services) HasLargeLanguageModel() bool {
	return false
}

// GetTemporal returns the Temporal workflow engine service for orchestrating complex,
// long-running workflows with built-in retry logic, timeouts, and failure handling.
// This is the core workflow execution engine for Thand. Returns the in-memory temporal
// service if configured via SetupInMemoryTemporal, otherwise returns nil.
func (s *Services) GetTemporal() models.TemporalService {
	return nil
}

// HasTemporal checks if a Temporal service connection is configured and available.
// Returns true if an in-memory temporal service has been set up.
func (s *Services) HasTemporal() bool {
	return false
}
