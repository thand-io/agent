package email_acs

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Azure Communication Services Email provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Azure Communication Services endpoint (required)
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"required,url"`

	// Default from address (required)
	From string `json:"from" mapstructure:"from" validate:"required,email"`

	// Azure credentials (optional, uses Azure credential chain)
	SubscriptionID string `json:"subscription_id" mapstructure:"subscription_id" validate:"omitempty,uuid"`
	TenantID       string `json:"tenant_id" mapstructure:"tenant_id" validate:"omitempty,uuid"`
	ClientID       string `json:"client_id" mapstructure:"client_id" validate:"omitempty,uuid"`
	ClientSecret   string `json:"client_secret" mapstructure:"client_secret" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Azure ACS Email configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Azure ACS Email config validation failed: %w", err)
	}
	return nil
}
