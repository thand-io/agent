package cloudflare

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Cloudflare provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Cloudflare account ID (required)
	AccountID string `json:"account_id" mapstructure:"account_id" validate:"required"`

	// Authentication: either API token (recommended) OR API key + email
	APIToken string `json:"api_token" mapstructure:"api_token" validate:"omitempty"`
	APIKey   string `json:"api_key" mapstructure:"api_key" validate:"omitempty"`
	Email    string `json:"email" mapstructure:"email" validate:"omitempty,email"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Cloudflare configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Cloudflare config validation failed: %w", err)
	}

	// Custom validation: ensure either api_token OR (api_key + email) is provided
	hasToken := len(c.APIToken) > 0
	hasKeyAndEmail := len(c.APIKey) > 0 && len(c.Email) > 0

	if !hasToken && !hasKeyAndEmail {
		return fmt.Errorf("Cloudflare config validation failed: either api_token or both api_key and email must be provided")
	}

	return nil
}
