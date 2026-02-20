package azure

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Azure provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Azure Subscription ID (required)
	SubscriptionID string `json:"subscription_id" mapstructure:"subscription_id" validate:"required,uuid"`

	// Optional resource group filter
	ResourceGroup string `json:"resource_group" mapstructure:"resource_group" validate:"omitempty"`

	// Authentication options (optional, uses credential chain)
	TenantID     string `json:"tenant_id" mapstructure:"tenant_id" validate:"omitempty,uuid"`
	ClientID     string `json:"client_id" mapstructure:"client_id" validate:"omitempty,uuid"`
	ClientSecret string `json:"client_secret" mapstructure:"client_secret" validate:"omitempty" sensitive:"true"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Azure configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Azure config validation failed: %w", err)
	}
	return nil
}
