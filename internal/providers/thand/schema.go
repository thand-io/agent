package thand

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Thand provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Thand authentication endpoint (optional, defaults to https://auth.thand.io)
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"omitempty,url"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Thand configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Thand config validation failed: %w", err)
	}
	return nil
}
