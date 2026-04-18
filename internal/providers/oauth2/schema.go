package oauth2

import (
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for generic OAuth2 provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// OAuth2 client credentials (required)
	ClientID     string `json:"client_id" mapstructure:"client_id" validate:"required"`
	ClientSecret string `json:"client_secret" mapstructure:"client_secret" validate:"required" sensitive:"true"`

	// OIDC discovery authority (optional if explicit endpoints are set)
	Authority string `json:"authority" mapstructure:"authority" validate:"omitempty,url"`

	// OAuth2 endpoints (optional if using OIDC discovery)
	AuthURL  string `json:"auth_url" mapstructure:"auth_url" validate:"omitempty,url"`
	TokenURL string `json:"token_url" mapstructure:"token_url" validate:"omitempty,url"`

	// Optional explicit userinfo endpoint override
	UserInfoURL string `json:"userinfo_url" mapstructure:"userinfo_url" validate:"omitempty,url"`

	// Requested scopes
	Scopes []string `json:"scopes" mapstructure:"scopes"`

	// Redirect URL
	RedirectURL string `json:"redirect_url" mapstructure:"redirect_url" validate:"omitempty,url"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the OAuth2 configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("OAuth2 config validation failed: %w", err)
	}

	hasAuthority := strings.TrimSpace(c.Authority) != ""
	hasAuthURL := strings.TrimSpace(c.AuthURL) != ""
	hasTokenURL := strings.TrimSpace(c.TokenURL) != ""

	if !hasAuthority && !(hasAuthURL && hasTokenURL) {
		return fmt.Errorf("OAuth2 config validation failed: either authority or both auth_url and token_url must be set")
	}

	return nil
}
