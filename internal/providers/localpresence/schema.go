package localpresence

import "github.com/thand-io/agent/internal/models"

type ConfigSchema struct {
	models.BaseConfigSchema
}

func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

func (c *ConfigSchema) Validate() error {
	return nil
}
