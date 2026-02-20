package gcp

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for GCP provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// GCP Project ID (required)
	ProjectID string `json:"project_id" mapstructure:"project_id" validate:"required"`

	// API Stage (optional, defaults to GA)
	Stage string `json:"stage" mapstructure:"stage" validate:"omitempty,oneof=GA BETA ALPHA"`

	// Authentication options (one of these should be provided)
	ServiceAccountKeyPath string                               `json:"service_account_key_path" mapstructure:"service_account_key_path" validate:"omitempty"`
	ServiceAccountKey     string                               `json:"service_account_key" mapstructure:"service_account_key" validate:"omitempty" sensitive:"true"`
	Credentials           *models.GCPServiceAccountCredentials `json:"credentials" mapstructure:"credentials" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the GCP configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("GCP config validation failed: %w", err)
	}
	return nil
}
