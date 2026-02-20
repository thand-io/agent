package email_ses

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for AWS SES Email provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Default from address (required)
	From string `json:"from" mapstructure:"from" validate:"required,email"`

	// AWS credentials (optional, uses AWS credential chain)
	Region          string `json:"region" mapstructure:"region" validate:"omitempty"`
	AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" validate:"omitempty" sensitive:"true"`
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" validate:"omitempty" sensitive:"true"`
	SessionToken    string `json:"session_token" mapstructure:"session_token" validate:"omitempty" sensitive:"true"`
	Profile         string `json:"profile" mapstructure:"profile" validate:"omitempty"`
	Endpoint        string `json:"endpoint" mapstructure:"endpoint" validate:"omitempty,url"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the AWS SES Email configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("AWS SES Email config validation failed: %w", err)
	}
	return nil
}
