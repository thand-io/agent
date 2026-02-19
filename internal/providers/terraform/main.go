package terraform

import (
	"fmt"

	"github.com/hashicorp/go-tfe"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const TerraformProviderName = "terraform"

// terraformProvider implements the ProviderImpl interface for Terraform
type terraformProvider struct {
	*models.BaseProvider
	client      *tfe.Client
	permissions []models.ProviderPermission
}

func (p *terraformProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		TerraformCapabilities,
	)

	terraformConfig := p.GetConfig()

	terraformToken, foundToken := terraformConfig.GetString("token")

	if !foundToken {
		return fmt.Errorf("missing required Terraform configuration: token is required")
	}

	// Initialize Terraform Cloud client
	config := &tfe.Config{
		Token: terraformToken,
	}

	client, err := tfe.NewClient(config)
	if err != nil {
		return fmt.Errorf("failed to create Terraform client: %w", err)
	}

	p.client = client

	p.permissions = []models.ProviderPermission{{
		ID:          string(tfe.AccessAdmin),
		Name:        string(tfe.AccessAdmin),
		Description: "Admin access",
	}, {
		ID:          string(tfe.AccessRead),
		Name:        string(tfe.AccessRead),
		Description: "Read access",
	}, {
		ID:          string(tfe.AccessWrite),
		Name:        string(tfe.AccessWrite),
		Description: "Write access",
	}, {
		ID:          string(tfe.AccessPlan),
		Name:        string(tfe.AccessPlan),
		Description: "Plan access",
	}, {
		ID:          string(tfe.AccessCustom),
		Name:        string(tfe.AccessCustom),
		Description: "Custom access",
	}}

	return nil
}

// ValidateConfig validates the Terraform configuration without initialization
func (p *terraformProvider) ValidateConfig(config *models.BasicConfig) error {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal Terraform config: %w", err)
	}
	return schema.Validate()
}

// GetConfigSchema returns the Terraform configuration schema
func (p *terraformProvider) GetConfigSchema() any {
	return &ConfigSchema{}
}

// Validate validates the Terraform provider configuration
func (p *terraformProvider) Validate() error {
	return p.ValidateConfig(p.GetConfig())
}

func init() {
	providers.Register(TerraformProviderName, &terraformProvider{}, TerraformCapabilities, &ConfigSchema{})
}
