package kubernetes

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// ConfigSchema defines the configuration structure for Kubernetes provider
// Kubernetes provider doesn't require configuration - it uses in-cluster config
// or the default kubeconfig from ~/.kube/config
type ConfigSchema struct {
	models.BaseConfigSchema

	// Optional kubeconfig path (if not provided, uses default)
	Kubeconfig string `json:"kubeconfig" mapstructure:"kubeconfig" validate:"omitempty"`

	// Optional context name (if not provided, uses current context)
	Context string `json:"context" mapstructure:"context" validate:"omitempty"`
}

// Unmarshal converts BasicConfig to ConfigSchema
func (c *ConfigSchema) Unmarshal(config *models.BasicConfig) error {
	return c.BaseConfigSchema.Unmarshal(config, c)
}

// Validate validates the Kubernetes configuration
func (c *ConfigSchema) Validate() error {
	validate := common.GetValidator()
	if err := validate.Struct(c); err != nil {
		return fmt.Errorf("Kubernetes config validation failed: %w", err)
	}
	return nil
}
