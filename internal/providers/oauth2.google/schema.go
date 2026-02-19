package googleoauth2

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Google OAuth2 provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Google OAuth2 client credentials (required)
	ClientID     string `json:"client_id" mapstructure:"client_id" validate:"required"`
	ClientSecret string `json:"client_secret" mapstructure:"client_secret" validate:"required"`

	// Requested scopes (optional, defaults to email and profile)
	Scopes []string `json:"scopes" mapstructure:"scopes"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Google OAuth2 configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Google OAuth2 config validation failed: %w", err)
	}
	return nil
}
