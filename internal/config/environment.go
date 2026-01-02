package config

import (
	"github.com/thand-io/agent/internal/models"
)

func (c *Config) GetEnvironment() models.EnvironmentConfig {
	return c.Environment
}
