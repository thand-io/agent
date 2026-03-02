package gsuite

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for GSuite provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Path to service account key JSON file (required)
	ServiceAccountKeyPath string `json:"service_account_key_path" mapstructure:"service_account_key_path" validate:"required"`

	// GSuite domain (required)
	Domain string `json:"domain" mapstructure:"domain" validate:"required"`

	// Admin email address for impersonation (required)
	AdminEmail string `json:"admin_email" mapstructure:"admin_email" validate:"required,email"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the GSuite configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("GSuite config validation failed: %w", err)
	}
	return nil
}
