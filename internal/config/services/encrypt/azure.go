package encrypt

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azkeys"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	azureProvider "github.com/thand-io/agent/internal/providers/azure"
)

type azureEncrypt struct {
	config   *models.BasicConfig
	client   *azkeys.Client
	creds    *azureProvider.AzureConfigurationProvider
	vaultURL string
	keyName  string
}

func NewAzureEncryptionFromConfig(config *models.BasicConfig) models.EncryptionImpl {
	return &azureEncrypt{
		config: config,
	}
}

/*
Initialize() error
Shutdown() error
Encrypt(plaintext string) ([]byte, error)
Decrypt(ciphertext []byte) (string, error)
*/
func (a *azureEncrypt) Initialize() error {

	// Create Azure credentials using the provider's CreateAzureConfig function
	creds, err := azureProvider.CreateAzureConfig(a.config)
	if err != nil {
		return fmt.Errorf("failed to create Azure credential: %w", err)
	}

	a.creds = creds

	logrus.Debugln("Initializing Azure Key Vault encryption client")

	vaultURL, foundVaultURL := a.config.GetString("vault_url")
	if !foundVaultURL {
		logrus.Errorln("vault_url not found in config")
		return fmt.Errorf("vault_url not found in config")
	}
	a.vaultURL = vaultURL

	keyName, foundKeyName := a.config.GetString("key_name")
	if !foundKeyName {
		logrus.Errorln("key_name not found in config")
		return fmt.Errorf("key_name not found in config")
	}
	a.keyName = keyName

	// Create the Key Vault client for keys (not secrets)
	client, err := azkeys.NewClient(vaultURL, a.creds.Token, nil)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to create Azure Key Vault client")
		return fmt.Errorf("failed to create Azure Key Vault client: %w", err)
	}

	logrus.Debugln("Azure Key Vault encryption client created successfully for URL:", vaultURL, "and Key:", keyName)

	a.client = client

	return nil
}

func (a *azureEncrypt) Shutdown() error {

	logrus.Debugln("Shutting down Azure Key Vault encryption client")

	// Azure SDK doesn't require explicit cleanup
	return nil
}

func (a *azureEncrypt) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {

	logrus.Debugln("Encrypting data with Azure Key Vault")

	if a.client == nil {
		logrus.Errorln("Azure Key Vault client not initialized")
		return nil, fmt.Errorf("azure Key Vault client not initialized")
	}

	if len(a.keyName) == 0 {
		logrus.Errorln("Key name is not configured")
		return nil, fmt.Errorf("key name is not configured")
	}

	// Use RSA-OAEP encryption algorithm
	algorithm := azkeys.EncryptionAlgorithmRSAOAEP

	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     plaintext,
	}

	resp, err := a.client.Encrypt(
		ctx, a.keyName, azureProvider.UseLatestVersion, params, nil)
	if err != nil {
		logrus.WithError(err).Errorln("Error encrypting data with Azure Key Vault")
		return nil, fmt.Errorf("failed to encrypt data with Azure Key Vault: %w", err)
	}

	return resp.Result, nil
}

func (a *azureEncrypt) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {

	logrus.Debugln("Decrypting data with Azure Key Vault")

	if len(ciphertext) == 0 {
		return nil, fmt.Errorf("ciphertext cannot be empty")
	}

	if a.client == nil {
		logrus.Errorln("Azure Key Vault client not initialized")
		return nil, fmt.Errorf("azure Key Vault client not initialized")
	}

	// Use RSA-OAEP encryption algorithm
	algorithm := azkeys.EncryptionAlgorithmRSAOAEP

	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     ciphertext,
	}

	resp, err := a.client.Decrypt(ctx, a.keyName, azureProvider.UseLatestVersion, params, nil)
	if err != nil {
		logrus.WithError(err).Errorln("Error decrypting data with Azure Key Vault")
		return nil, fmt.Errorf("failed to decrypt data with Azure Key Vault: %w", err)
	}

	return resp.Result, nil
}
