package terraform

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Terraform provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Terraform Cloud API token (required)
	Token string `json:"token" mapstructure:"token" validate:"required" sensitive:"true"`

	// Optional organization name
	Organization string `json:"organization" mapstructure:"organization" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Terraform configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Terraform config validation failed: %w", err)
	}
	return nil
}
