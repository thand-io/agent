package models

import "maps"

// THAND_SERVICES_VAULT_PROVIDER=aws|gcp|azure|local
type ServiceConfig struct {
	Provider string       `mapstructure:"provider" default:"local"` // aws|gcp|azure|local
	Config   *BasicConfig `mapstructure:",remain"`
}

func (e *ServiceConfig) GetProvider() string {
	if e == nil || len(e.Provider) == 0 {
		return "local"
	}
	return e.Provider
}

type ServicesConfig struct {

	// Encryption - used for encrypting sensitive data
	Encryption *ServiceConfig `mapstructure:"encryption"`

	// Vault - used for storing sensitive data
	Vault *ServiceConfig `mapstructure:"vault"`

	// Scheduler - used for scheduling tasks
	Scheduler *ServiceConfig `mapstructure:"scheduler"`

	// LLM - used for large language model interactions
	LargeLanguageModel *LargeLanguageModelConfig `mapstructure:"llm"`

	// Temporal - used for workflow processing and orchestration
	Temporal *TemporalConfig `mapstructure:"temporal"`
}

func (e *ServicesConfig) GetEncryptionConfig() *ServiceConfig {
	return e.Encryption
}

// getConfigWithDefaults is a generic helper that merges defaults with a service config.
// If there are conflicts, the values in the service config take precedence.
func (e *ServicesConfig) getConfigWithDefaults(serviceConfig *ServiceConfig, defaults *BasicConfig) *BasicConfig {
	// Start with defaults
	result := &BasicConfig{}
	if defaults != nil {
		maps.Copy((*result), *defaults)
	}

	// Merge service config values, overriding defaults
	if serviceConfig != nil && serviceConfig.Config != nil {
		if *result == nil {
			*result = make(BasicConfig)
		}
		maps.Copy((*result), *serviceConfig.Config)
	}

	return result
}

// GetEncryptionConfigWithDefaults provides a new BasicConfig that merges the provided defaults
// with any config values set in the ServicesConfig Encryption config.
// If there are conflicts, the values in the ServicesConfig take precedence.
func (e *ServicesConfig) GetEncryptionConfigWithDefaults(defaults *BasicConfig) *BasicConfig {
	return e.getConfigWithDefaults(e.Encryption, defaults)
}

// GetVaultConfigWithDefaults provides a new BasicConfig that merges the provided defaults
// with any config values set in the ServicesConfig Vault config.
// If there are conflicts, the values in the ServicesConfig take precedence.
func (e *ServicesConfig) GetVaultConfigWithDefaults(defaults *BasicConfig) *BasicConfig {
	return e.getConfigWithDefaults(e.Vault, defaults)
}

// GetSchedulerConfigWithDefaults provides a new BasicConfig that merges the provided defaults
// with any config values set in the ServicesConfig Scheduler config.
// If there are conflicts, the values in the ServicesConfig take precedence.
func (e *ServicesConfig) GetSchedulerConfigWithDefaults(defaults *BasicConfig) *BasicConfig {
	return e.getConfigWithDefaults(e.Scheduler, defaults)
}

func (e *ServicesConfig) GetVaultConfig() *ServiceConfig {
	return e.Vault
}

func (e *ServicesConfig) GetSchedulerConfig() *ServiceConfig {
	return e.Scheduler
}

func (e *ServicesConfig) GetLLMConfig() *LargeLanguageModelConfig {
	return e.LargeLanguageModel
}

func (e *ServicesConfig) GetTemporalConfig() *TemporalConfig {
	return e.Temporal
}

type ServicesClientImpl interface {
	Initialize() error
	Shutdown() error

	GetEncryption() EncryptionImpl
	HasEncryption() bool

	GetVault() VaultImpl
	HasVault() bool

	GetStorage() StorageImpl
	HasStorage() bool

	GetScheduler() SchedulerImpl
	HasScheduler() bool

	GetLargeLanguageModel() LargeLanguageModelImpl
	HasLargeLanguageModel() bool

	GetTemporal() TemporalImpl
	HasTemporal() bool
}
