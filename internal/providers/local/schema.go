package local

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

type ConfigSchema struct {
	models.BaseConfigSchema

	DeniedUsernames  []string `json:"denied_usernames" mapstructure:"denied_usernames" validate:"omitempty,dive,min=1"`
	AllowedUIDRanges []string `json:"allowed_uid_ranges" mapstructure:"allowed_uid_ranges" validate:"omitempty,dive,min=1"`
	SudoersDir       string   `json:"sudoers_dir" mapstructure:"sudoers_dir" validate:"omitempty"`
	LeaseDir         string   `json:"lease_dir" mapstructure:"lease_dir" validate:"omitempty"`
	SudoPath         string   `json:"sudo_path" mapstructure:"sudo_path" validate:"omitempty"`
	VisudoPath       string   `json:"visudo_path" mapstructure:"visudo_path" validate:"omitempty"`
}

func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("local config validation failed: %w", err)
	}
	return nil
}
