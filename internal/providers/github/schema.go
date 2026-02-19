package github

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for GitHub provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// GitHub organization name (required)
	Organization string `json:"organization" mapstructure:"organization" validate:"required"`

	// GitHub personal access token (required)
	Token string `json:"token" mapstructure:"token" validate:"required"`

	// Optional endpoint (defaults to https://api.github.com)
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"omitempty,url"`

	// OAuth2 fields (optional, for authorization flow)
	ClientID     string `json:"client_id" mapstructure:"client_id" validate:"omitempty"`
	ClientSecret string `json:"client_secret" mapstructure:"client_secret" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the GitHub configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("GitHub config validation failed: %w", err)
	}
	return nil
}
