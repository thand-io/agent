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

// EncryptKey implements KEKProvider interface - encrypts a DEK with Azure Key Vault
func (a *azureEncrypt) EncryptKey(ctx context.Context, dek []byte) ([]byte, error) {
	if a.client == nil {
		return nil, fmt.Errorf("azure Key Vault client not initialized")
	}

	if len(a.keyName) == 0 {
		return nil, fmt.Errorf("key name is not configured")
	}

	// Validate input
	if len(dek) == 0 {
		return nil, fmt.Errorf("DEK cannot be empty")
	}

	// RSA-OAEP size limits: 2048-bit key can encrypt ~190 bytes, 4096-bit key ~446 bytes
	// Standard DEK is 32 bytes (AES-256), so this is well within limits
	// However, we enforce a reasonable maximum to prevent issues
	if len(dek) > 256 {
		return nil, fmt.Errorf("DEK size %d exceeds maximum allowed size of 256 bytes", len(dek))
	}

	logrus.Debugln("Encrypting DEK with Azure Key Vault")

	// Use RSA-OAEP encryption algorithm
	algorithm := azkeys.EncryptionAlgorithmRSAOAEP

	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     dek,
	}

	// NOTE: Using UseLatestVersion means key rotation can break decryption of old data
	// TODO: Store the key version (from resp.KeyID) with the encrypted DEK for proper version tracking
	resp, err := a.client.Encrypt(
		ctx, a.keyName, azureProvider.UseLatestVersion, params, nil)
	if err != nil {
		logrus.WithFields(logrus.Fields{
			"key_name": a.keyName,
		}).WithError(err).Errorln("Failed to encrypt DEK with Azure Key Vault")
		return nil, fmt.Errorf("failed to encrypt DEK with Azure Key Vault: %w", err)
	}

	return resp.Result, nil
}

func (a *azureEncrypt) Encrypt(ctx context.Context, plaintext []byte) ([]byte, error) {
	logrus.Debugln("Encrypting data with Azure Key Vault using envelope encryption")

	// Use envelope encryption with Azure Key Vault as KEK provider
	return EnvelopeEncrypt(ctx, plaintext, a)
}

// DecryptKey implements KEKProvider interface - decrypts a DEK with Azure Key Vault
func (a *azureEncrypt) DecryptKey(ctx context.Context, encryptedDEK []byte) ([]byte, error) {
	if a.client == nil {
		return nil, fmt.Errorf("azure Key Vault client not initialized")
	}

	// Validate input
	if len(encryptedDEK) == 0 {
		return nil, fmt.Errorf("encrypted DEK cannot be empty")
	}

	// Use RSA-OAEP encryption algorithm
	algorithm := azkeys.EncryptionAlgorithmRSAOAEP

	params := azkeys.KeyOperationParameters{
		Algorithm: &algorithm,
		Value:     encryptedDEK,
	}

	// NOTE: Using UseLatestVersion for decryption - should use the version that encrypted the data
	// TODO: Extract and use the key version from the encryption operation
	resp, err := a.client.Decrypt(ctx, a.keyName, azureProvider.UseLatestVersion, params, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt DEK with Azure Key Vault: %w", err)
	}

	return resp.Result, nil
}

func (a *azureEncrypt) Decrypt(ctx context.Context, ciphertext []byte) ([]byte, error) {
	logrus.Debugln("Decrypting data with Azure Key Vault using envelope encryption")

	// Use envelope decryption with Azure Key Vault as KEK provider
	return EnvelopeDecrypt(ctx, ciphertext, a)
}
