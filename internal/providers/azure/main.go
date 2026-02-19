package azure

import (
	_ "embed"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/authorization/armauthorization"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/resources/armsubscriptions"
	"github.com/sirupsen/logrus"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const AzureProviderName = "azure"

var UseLatestVersion = ""

type azureProvider struct {
	*models.BaseProvider

	cred                *AzureConfigurationProvider
	authClient          *armauthorization.RoleAssignmentsClient
	roleDefClient       *armauthorization.RoleDefinitionsClient
	subscriptionsClient *armsubscriptions.Client
	subscriptionID      string
	resourceGroupName   string
}

func (p *azureProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	// Set the provider to the base provider
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		AzureCapabilities,
	)

	config := p.GetConfig()

	// Get subscription ID from config
	subscriptionID, ok := config.GetString("subscription_id")
	if !ok {
		return fmt.Errorf("subscription_id not found in config")
	}
	p.subscriptionID = subscriptionID

	// Get resource group name from config (optional, can use subscription scope)
	if rgName, ok := config.GetString("resource_group"); ok {
		p.resourceGroupName = rgName
	}

	// Initialize Azure credentials using CreateAzureConfig
	azureCreds, err := CreateAzureConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	p.cred = azureCreds

	// Initialize Azure clients
	p.authClient, err = armauthorization.NewRoleAssignmentsClient(subscriptionID, p.cred.Token, nil)
	if err != nil {
		return fmt.Errorf("failed to create role assignments client: %w", err)
	}

	p.roleDefClient, err = armauthorization.NewRoleDefinitionsClient(p.cred.Token, nil)
	if err != nil {
		return fmt.Errorf("failed to create role definitions client: %w", err)
	}

	p.subscriptionsClient, err = armsubscriptions.NewClient(p.cred.Token, nil)
	if err != nil {
		return fmt.Errorf("failed to create subscriptions client: %w", err)
	}

	return nil
}

func init() {
	providers.Register(AzureProviderName, &azureProvider{}, AzureCapabilities, &ConfigSchema{})
}

// CreateAzureConfig creates Azure credentials based on the provided configuration
func CreateAzureConfig(azureConfig *models.BasicConfig) (*AzureConfigurationProvider, error) {
	var cred azcore.TokenCredential
	var err error

	// Check if client credentials are provided
	if clientID, hasClientID := azureConfig.GetString("client_id"); hasClientID {
		if clientSecret, hasClientSecret := azureConfig.GetString("client_secret"); hasClientSecret {
			if tenantID, hasTenantID := azureConfig.GetString("tenant_id"); hasTenantID {
				logrus.Info("Using Azure client credentials authentication")
				cred, err = azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
			} else {
				return nil, fmt.Errorf("tenant_id required when using client credentials")
			}
		} else {
			return nil, fmt.Errorf("client_secret required when using client_id")
		}
	} else {
		// Use default credential chain (managed identity, environment variables, etc.)
		logrus.Info("Using Azure default credential chain")
		cred, err = azidentity.NewDefaultAzureCredential(nil)
	}

	if err != nil {
		logrus.WithError(err).Errorln("Failed to create Azure credentials")
		return nil, fmt.Errorf("failed to create Azure credentials: %w", err)
	}

	return &AzureConfigurationProvider{
		Token: cred,
	}, nil
}

type AzureConfigurationProvider struct {
	ProjectID string
	Token     azcore.TokenCredential
}
