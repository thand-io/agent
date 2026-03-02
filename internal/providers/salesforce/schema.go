package salesforce

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Salesforce provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Salesforce credentials (required)
	Username string `json:"username" mapstructure:"username" validate:"required"`
	Password string `json:"password" mapstructure:"password" validate:"required" sensitive:"true"`
	Token    string `json:"token" mapstructure:"token" validate:"required" sensitive:"true"` // Security token

	// Optional endpoint (defaults to https://login.salesforce.com)
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"omitempty,url"`

	// Optional client ID (defaults to simpleforce default)
	ClientID string `json:"client_id" mapstructure:"client_id" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Salesforce configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Salesforce config validation failed: %w", err)
	}
	return nil
}
