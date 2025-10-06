package services

import (
	encrypt "github.com/thand-io/agent/internal/config/services/encrypt"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureEncryption() models.EncryptionImpl {

	provider := "local"
	encryptConfig := e.GetServicesConfig().GetEncryptionConfig()

	if e.config.Encryption != nil && len(e.config.Encryption.Provider) > 0 {
		provider = encryptConfig.Provider
	} else if e.environment != nil && len(e.environment.Platform) > 0 {
		provider = string(e.environment.Platform)
	}

	// This allows us to pass in any config values defined in the environment
	configValues := e.config.GetEncryptionConfigWithDefaults(e.GetEnvironmentConfig().Config)

	switch provider {
	case string(models.AWS):
		// AWS Encryption
		awsEncrypt := encrypt.NewAwsEncryptionFromConfig(configValues)
		return awsEncrypt
	case string(models.GCP):
		// GCP Encryption
		gcpEncrypt := encrypt.NewGcpEncryptionFromConfig(configValues)
		return gcpEncrypt
	case string(models.Azure):
		// Azure Encryption
		azureEncrypt := encrypt.NewAzureEncryptionFromConfig(configValues)
		return azureEncrypt
	case string(models.Local):
		fallthrough
	default:
		localEncrypt := encrypt.NewLocalEncryptionFromConfig(configValues)
		return localEncrypt
	}

}
