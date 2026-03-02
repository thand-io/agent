package services

import (
	encrypt "github.com/thand-io/agent/internal/config/services/encrypt"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureEncryption() models.EncryptionImpl {

	provider := "local"
	encryptConfig := e.config.GetServicesConfig().GetEncryptionConfig()

	if encryptConfig != nil && len(encryptConfig.GetProvider()) > 0 {
		provider = encryptConfig.GetProvider()
	} else if e.config.GetEnvironmentConfig() != nil && len(e.config.GetEnvironmentConfig().Platform) > 0 {
		provider = string(e.config.GetEnvironmentConfig().Platform)
	}

	// This allows us to pass in any config values defined in the environment
	configValues := e.config.GetServicesConfig().GetEncryptionConfigWithDefaults(e.config.GetEnvironmentConfig().Config)

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

		// Do we have our password and salt? If not try and provide a
		// better alternative than the default

		if !configValues.HasString("salt") {
			configValues.SetKeyWithValue("salt", e.config.GetEnvironmentConfig().GetIdentifier())
		}

		if !configValues.HasString("password") && len(e.config.GetSecret()) > 0 {
			configValues.SetKeyWithValue("password", e.config.GetSecret())
		}

		localEncrypt := encrypt.NewLocalEncryptionFromConfig(configValues)
		return localEncrypt
	}

}
