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

	// Optional GCP Organization ID used for organization-scoped custom roles
	OrganizationID string `json:"organization_id" mapstructure:"organization_id" validate:"omitempty"`

	// API Stage (optional, defaults to GA)
	Stage string `json:"stage" mapstructure:"stage" validate:"omitempty,oneof=GA BETA ALPHA"`

	// Authentication options (exactly one of these should be provided, or none for ADC)
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

	// Enforce that at most one explicit authentication method is specified
	authCount := 0
	if len(c.ServiceAccountKeyPath) > 0 {
		authCount++
	}
	if len(c.ServiceAccountKey) > 0 {
		authCount++
	}
	if c.Credentials != nil {
		authCount++
	}
	if authCount > 1 {
		return fmt.Errorf("GCP config validation failed: only one of service_account_key_path, service_account_key, or credentials may be specified; found %d", authCount)
	}

	return nil
}
