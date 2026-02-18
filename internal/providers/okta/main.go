package okta

import (
	"context"
	"fmt"

	"github.com/okta/okta-sdk-golang/v2/okta"
	"github.com/sirupsen/logrus"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const OktaProviderName = "okta"

// oktaProvider implements the ProviderImpl interface for Okta
type oktaProvider struct {
	*models.BaseProvider

	client   *okta.Client
	orgUrl   string
	apiToken string
}

func (p *oktaProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		OktaCapabilities,
	)

	// Get Okta configuration
	oktaConfig := p.GetConfig()

	// Create Okta client
	client, err := CreateOktaClient(oktaConfig)
	if err != nil {
		return fmt.Errorf("failed to create Okta client: %w", err)
	}

	p.client = client

	// Store configuration values
	orgUrl, found := oktaConfig.GetString("endpoint")
	if !found {
		return fmt.Errorf("endpoint is required for Okta provider")
	}
	p.orgUrl = orgUrl

	apiToken, found := oktaConfig.GetString("token")
	if !found {
		return fmt.Errorf("token is required for Okta provider")
	}
	p.apiToken = apiToken

	logrus.WithField("org_url", p.orgUrl).Info("Initialized Okta provider")
	return nil
}

// ValidateConfig validates the Okta configuration without initialization
func (p *oktaProvider) ValidateConfig(config *models.BasicConfig) error {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal Okta config: %w", err)
	}
	return schema.Validate()
}

// Validate validates the Okta provider configuration
func (p *oktaProvider) Validate() error {
	return p.ValidateConfig(p.GetConfig())
}

// CreateOktaClient creates and configures an Okta API client
func CreateOktaClient(oktaConfig *models.BasicConfig) (*okta.Client, error) {
	ctx := context.Background()

	// Get required configuration
	orgUrl, foundOrgUrl := oktaConfig.GetString("endpoint")
	apiToken, foundApiToken := oktaConfig.GetString("token")

	if !foundOrgUrl {
		return nil, fmt.Errorf("endpoint is required for Okta provider")
	}

	if !foundApiToken {
		return nil, fmt.Errorf("token is required for Okta provider")
	}

	// Configure Okta client
	_, client, err := okta.NewClient(
		ctx,
		okta.WithOrgUrl(orgUrl),
		okta.WithToken(apiToken),
		okta.WithCache(true),
	)

	if err != nil {
		return nil, fmt.Errorf("failed to initialize Okta client: %w", err)
	}

	logrus.WithField("org_url", orgUrl).Info("Created Okta client")
	return client, nil
}

// GetClient returns the Okta API client
func (p *oktaProvider) GetClient() *okta.Client {
	return p.client
}

// GetOrgUrl returns the Okta organization URL
func (p *oktaProvider) GetOrgUrl() string {
	return p.orgUrl
}

// GetApiToken returns the Okta API token (use with caution)
func (p *oktaProvider) GetApiToken() string {
	return p.apiToken
}

func init() {
	providers.Register(OktaProviderName, &oktaProvider{})
}
