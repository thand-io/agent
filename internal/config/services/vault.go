package services

import (
	vaults "github.com/thand-io/agent/internal/config/services/vault"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureVault() models.VaultImpl {

	provider := "local"
	vaultConfig := e.GetServicesConfig().GetVaultConfig()

	if e.config.Vault != nil && len(e.config.Vault.Provider) > 0 {
		provider = vaultConfig.GetProvider()
	} else if e.environment != nil && len(e.environment.Platform) > 0 {
		provider = string(e.environment.Platform)
	}

	// This allows us to pass in any config values defined in the environment
	configValues := e.config.GetVaultConfigWithDefaults(e.GetEnvironmentConfig().Config)

	switch provider {
	case string(models.AWS):
		// AWS Vault - KMS
		awsVault := vaults.NewAwsVaultFromConfig(configValues)
		return awsVault
	case string(models.GCP):
		// GCP Vault - KMS
		gcpVault := vaults.NewGcpVaultFromConfig(configValues)
		return gcpVault
	case string(models.Azure):
		// Azure Vault - KMS
		azureVault := vaults.NewAzureVaultFromConfig(configValues)
		return azureVault
	case string(models.Local):
		fallthrough
	default:
		localVault := vaults.NewLocalVaultFromConfig(configValues)
		return localVault
	}

}
