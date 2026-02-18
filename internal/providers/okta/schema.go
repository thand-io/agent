package okta

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Okta provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Okta organization URL (required)
	// Example: https://dev-123456.okta.com
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"required,url"`

	// Okta API token (required)
	Token string `json:"token" mapstructure:"token" validate:"required"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Okta configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Okta config validation failed: %w", err)
	}
	return nil
}
