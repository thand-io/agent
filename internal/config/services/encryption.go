package services

import (
	encrypt "github.com/thand-io/agent/internal/config/services/encrypt"
	"github.com/thand-io/agent/internal/models"
)

func (e *localClient) configureEncryption() models.EncryptionImpl {

	provider := "local"
	encryptConfig := e.GetServicesConfig().GetEncryptionConfig()

	if encryptConfig != nil && len(encryptConfig.GetProvider()) > 0 {
		provider = encryptConfig.GetProvider()
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

		// Do we have our password and salt? If not try and provide a
		// better alternative than the default

		if !configValues.HasString("salt") {
			configValues.SetKeyWithValue("salt", e.GetEnvironmentConfig().GetIdentifier())
		}

		if !configValues.HasString("password") && len(e.GetSecret()) > 0 {
			configValues.SetKeyWithValue("password", e.GetSecret())
		}

		localEncrypt := encrypt.NewLocalEncryptionFromConfig(configValues)
		return localEncrypt
	}

}
