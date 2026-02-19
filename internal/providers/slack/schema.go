package slack

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Slack provider
type ConfigSchema struct {
	models.BaseConfigSchema

	// Slack bot token (required)
	// Format: xoxb-...
	BotToken string `json:"bot_token" mapstructure:"bot_token" validate:"required"`

	// OAuth fields (optional, for OAuth flow)
	ClientID          string `json:"client_id" mapstructure:"client_id" validate:"omitempty"`
	ClientSecret      string `json:"client_secret" mapstructure:"client_secret" validate:"omitempty"`
	SigningSecret     string `json:"signing_secret" mapstructure:"signing_secret" validate:"omitempty"`
	VerificationToken string `json:"verification_token" mapstructure:"verification_token" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Slack configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Slack config validation failed: %w", err)
	}
	return nil
}
