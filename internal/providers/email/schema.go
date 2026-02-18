package email

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Email provider (proxy)
type ConfigSchema struct {
	models.BaseConfigSchema

	// Platform type (optional, defaults to smtp)
	// Valid values: smtp, ses, acs
	Platform string `json:"platform" mapstructure:"platform" validate:"omitempty,oneof=smtp ses acs"`

	// The actual configuration depends on the platform selected
	// and will be validated by the platform-specific provider
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Email configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Email config validation failed: %w", err)
	}
	return nil
}
