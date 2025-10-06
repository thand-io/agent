package vault

import (
	"context"
	"fmt"
	"time"

	"github.com/Azure/azure-sdk-for-go/sdk/security/keyvault/azsecrets"
	"github.com/aws/smithy-go/ptr"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	azureProvider "github.com/thand-io/agent/internal/providers/azure"
)

type azureVault struct {
	config   *models.BasicConfig
	client   *azsecrets.Client
	creds    *azureProvider.AzureConfigurationProvider
	vaultURL string
}

func NewAzureVaultFromConfig(config *models.BasicConfig) models.VaultImpl {
	return &azureVault{
		config: config,
	}
}

func (a *azureVault) Initialize() error {

	// Create Azure credentials using the provider's CreateAzureConfig function
	creds, err := azureProvider.CreateAzureConfig(a.config)
	if err != nil {
		return fmt.Errorf("failed to create Azure credential: %w", err)
	}

	a.creds = creds

	logrus.Debugln("Initializing Azure Key Vault client")

	vaultURL, foundVaultURL := a.config.GetString("vault_url")
	if !foundVaultURL {
		logrus.Errorln("vault_url not found in config")
		return fmt.Errorf("vault_url not found in config")
	}
	a.vaultURL = vaultURL

	// Create the Key Vault client
	client, err := azsecrets.NewClient(vaultURL, a.creds.Token, nil)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to create Azure Key Vault client")
		return fmt.Errorf("failed to create Azure Key Vault client: %w", err)
	}

	a.client = client

	logrus.Debugln("Azure Key Vault client created successfully for URL:", vaultURL)

	// Test the connection by attempting to get a dummy secret (which may fail but confirms auth)
	// This is optional - we could remove this test if preferred
	// _, _ = a.client.GetSecret(ctx, "test-connection", "", nil)

	return nil
}

func (a *azureVault) Shutdown() error {

	logrus.Debugln("Shutting down Azure Key Vault client")

	// Azure SDK doesn't require explicit cleanup
	return nil
}

func (a *azureVault) GetSecret(key string) ([]byte, error) {

	logrus.Debugln("Getting secret from Azure Key Vault:", key)

	if a.client == nil {
		return nil, fmt.Errorf("Azure Key Vault client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	resp, err := a.client.GetSecret(ctx, key, azureProvider.UseLatestVersion, nil)
	if err != nil {
		logrus.WithError(err).Errorf("Error retrieving secret '%s' from Azure Key Vault", key)
		return nil, fmt.Errorf("failed to get secret '%s' from Azure Key Vault: %w", key, err)
	}

	if resp.Value == nil {
		logrus.WithError(err).Errorf("Secret '%s' has no value in Azure Key Vault", key)
		return nil, fmt.Errorf("secret '%s' has no value", key)
	}

	// TODO/NOTE: Depending on the secret type the response may need to
	// be decoded from base64 or another format. Here we assume it's plain text.

	return []byte(*resp.Value), nil
}

func (a *azureVault) StoreSecret(key string, value []byte) error {

	logrus.Debugln("Storing secret in Azure Key Vault:", key)

	if a.client == nil {
		return fmt.Errorf("Azure Key Vault client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	params := azsecrets.SetSecretParameters{
		Value: ptr.String(string(value)),
	}

	resp, err := a.client.SetSecret(ctx, key, params, nil)

	if err != nil {
		logrus.WithError(err).Errorf("Error storing secret '%s' in Azure Key Vault", key)
		return fmt.Errorf("failed to store secret '%s' in Azure Key Vault: %w", key, err)
	}

	logrus.Debugln("Secret stored successfully with ID:", *resp.ID)

	return nil
}
