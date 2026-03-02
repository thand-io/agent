package email_smtp

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for SMTP Email provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// SMTP server configuration (required)
	Host string `json:"host" mapstructure:"host" validate:"required"`
	Port int    `json:"port" mapstructure:"port" validate:"required,min=1,max=65535"`

	// SMTP authentication (required)
	User string `json:"user" mapstructure:"user" validate:"required"`
	Pass string `json:"pass" mapstructure:"pass" validate:"required" sensitive:"true"`

	// Default from address (required)
	From string `json:"from" mapstructure:"from" validate:"required,email"`

	// TLS configuration (optional)
	TLSSkipVerify bool `json:"tls_skip_verify" mapstructure:"tls_skip_verify"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the SMTP Email configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("SMTP Email config validation failed: %w", err)
	}
	return nil
}
