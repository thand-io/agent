package saml

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for SAML provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// IdP metadata URL (required)
	IDPMetadataURL string `json:"idp_metadata_url" mapstructure:"idp_metadata_url" validate:"required,url"`

	// Service Provider entity ID (required)
	EntityID string `json:"entity_id" mapstructure:"entity_id" validate:"required,url"`

	// Root URL for the service provider (required)
	RootURL string `json:"root_url" mapstructure:"root_url" validate:"required,url"`

	// Certificate and key for SAML signing (optional, but required if sign_requests is true)
	// Can be provided either as file paths or as inline content
	CertFile string `json:"cert_file" mapstructure:"cert_file" validate:"omitempty"`
	Cert     string `json:"cert" mapstructure:"cert" validate:"omitempty"`
	KeyFile  string `json:"key_file" mapstructure:"key_file" validate:"omitempty"`
	Key      string `json:"key" mapstructure:"key" validate:"omitempty"`

	// Whether to sign authentication requests (optional, defaults to false)
	SignRequests bool `json:"sign_requests" mapstructure:"sign_requests"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the SAML configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("SAML config validation failed: %w", err)
	}

	// Custom validation: if sign_requests is true, ensure we have cert and key
	if c.SignRequests {
		hasCert := (len(c.Cert) > 0 || len(c.CertFile) > 0)
		hasKey := (len(c.Key) > 0 || len(c.KeyFile) > 0)

		if !hasCert || !hasKey {
			return fmt.Errorf("SAML config validation failed: sign_requests is enabled but cert/key are missing")
		}
	}

	return nil
}
