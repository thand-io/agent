package gcpiap

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for GCP IAP provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// IAP Audience for validating incoming JWTs (required)
	// Format: /projects/PROJECT_NUMBER/apps/PROJECT_ID
	Audience string `json:"audience" mapstructure:"audience" validate:"required"`

	// OAuth2 configuration for authorization flow
	ClientID     string   `json:"client_id" mapstructure:"client_id" validate:"required"`
	ClientSecret string   `json:"client_secret" mapstructure:"client_secret" validate:"required" sensitive:"true"`
	Scopes       []string `json:"scopes" mapstructure:"scopes"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the GCP IAP configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("GCP IAP config validation failed: %w", err)
	}
	return nil
}
