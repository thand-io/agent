package aws

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for AWS provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// AWS Region (optional, defaults to us-east-1)
	Region string `json:"region" mapstructure:"region" validate:"omitempty"`

	// Authentication options (all optional, uses credential chain)
	AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" validate:"omitempty" sensitive:"true"`
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" validate:"omitempty" sensitive:"true"`
	SessionToken    string `json:"session_token" mapstructure:"session_token" validate:"omitempty" sensitive:"true"`
	Profile         string `json:"profile" mapstructure:"profile" validate:"omitempty"`
	SSOStartURL     string `json:"sso_start_url" mapstructure:"sso_start_url" validate:"omitempty,url"`

	// Optional endpoint override (for LocalStack, etc.)
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"omitempty,url"`

	// Disable IMDS (Instance Metadata Service)
	IMDSDisable bool `json:"imds_disable" mapstructure:"imds_disable"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the AWS configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("AWS config validation failed: %w", err)
	}
	return nil
}
