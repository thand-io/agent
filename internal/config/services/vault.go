package services

import (
	vaults "github.com/thand-io/agent/internal/config/services/vault"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureVault() models.VaultImpl {

	provider := "local"
	servicesConfig := e.config.GetServicesConfig()
	if servicesConfig == nil {
		return nil
	}
	vaultConfig := servicesConfig.GetVaultConfig()

	environment := e.config.GetEnvironmentConfig()
	if vaultConfig != nil && len(vaultConfig.GetProvider()) > 0 {
		provider = vaultConfig.GetProvider()
	} else if environment != nil && len(environment.Platform) > 0 {
		provider = string(environment.Platform)
	}

	// This allows us to pass in any config values defined in the environment
	configValues := servicesConfig.GetVaultConfigWithDefaults(environment.Config)

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
