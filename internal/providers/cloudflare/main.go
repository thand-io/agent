package cloudflare

import (
	"context"
	"fmt"

	"github.com/cloudflare/cloudflare-go"
	"github.com/sirupsen/logrus"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const CloudflareProviderName = "cloudflare"

// cloudflareProvider implements the ProviderImpl interface for Cloudflare
type cloudflareProvider struct {
	*models.BaseProvider
	client    *cloudflare.API
	accountID string
}

func (p *cloudflareProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		CloudflareCapabilities,
	)

	// Get configuration
	cfConfig := p.GetConfig()

	// Create Cloudflare API client
	client, accountID, err := CreateCloudflareClient(cfConfig)
	if err != nil {
		return fmt.Errorf("failed to create Cloudflare client: %w", err)
	}

	p.client = client
	p.accountID = accountID

	// Once the roles are loaded use the roles to create the permissions reference
	// err = p.LoadPermissions()
	// if err != nil {
	// 	return fmt.Errorf("failed to load permissions: %w", err)
	// }

	logrus.WithFields(logrus.Fields{
		"provider":   CloudflareProviderName,
		"account_id": p.accountID,
	}).Info("Cloudflare provider initialized")

	return nil
}

// ValidateConfig validates the Cloudflare configuration without initialization
func (p *cloudflareProvider) ValidateConfig(config *models.BasicConfig) error {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal Cloudflare config: %w", err)
	}
	return schema.Validate()
}

// Validate validates the Cloudflare provider configuration
func (p *cloudflareProvider) Validate() error {
	return p.ValidateConfig(p.GetConfig())
}

// CreateCloudflareClient creates a Cloudflare API client from configuration
func CreateCloudflareClient(cfConfig *models.BasicConfig) (*cloudflare.API, string, error) {
	// Check for API token (recommended method)
	apiToken, foundToken := cfConfig.GetString("api_token")

	// Check for API key and email (legacy method)
	apiKey, foundKey := cfConfig.GetString("api_key")
	apiEmail, foundEmail := cfConfig.GetString("email")

	// Get account ID
	accountID, foundAccountID := cfConfig.GetString("account_id")
	if !foundAccountID {
		return nil, "", fmt.Errorf("account_id is required in Cloudflare config")
	}

	var api *cloudflare.API
	var err error

	if foundToken {
		logrus.Info("Using Cloudflare API token authentication")
		api, err = cloudflare.NewWithAPIToken(apiToken)
	} else if foundKey && foundEmail {
		logrus.Info("Using Cloudflare API key authentication")
		api, err = cloudflare.New(apiKey, apiEmail)
	} else {
		return nil, "", fmt.Errorf("either api_token or both api_key and email must be provided")
	}

	if err != nil {
		return nil, "", fmt.Errorf("failed to create Cloudflare API client: %w", err)
	}

	// Verify the client works by making a test call
	ctx := context.Background()
	_, _, err = api.Accounts(ctx, cloudflare.AccountsListParams{})
	if err != nil {
		return nil, "", fmt.Errorf("failed to verify Cloudflare credentials: %w", err)
	}

	return api, accountID, nil
}

func (p *cloudflareProvider) GetClient() *cloudflare.API {
	return p.client
}

func (p *cloudflareProvider) GetAccountID() string {
	return p.accountID
}

type CloudflareConfigurationProvider struct {
	Client    *cloudflare.API
	AccountID string
}

func init() {
	providers.Register(CloudflareProviderName, &cloudflareProvider{})
}
