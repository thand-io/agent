package models

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/thand-io/agent/internal/common"
)

// RoleDefinitions represents the structure for roles YAML/JSON
// These are used for configuration.
type RoleDefinitions struct {
	Version *version.Version `yaml:"version" json:"version"`
	Roles   map[string]Role  `yaml:"roles" json:"roles"`
}

// UnmarshalJSON converts Version to string from any type and handles both
// API response format (with roles as SearchResult array) and config file format (map)
func (h *RoleDefinitions) UnmarshalJSON(data []byte) error {
	// First, try to unmarshal to detect the structure
	aux := &struct {
		Version any             `json:"version"`
		Roles   map[string]Role `json:"roles"`
	}{
		Roles: make(map[string]Role),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}
	h.Version = parsedVersion
	h.Roles = aux.Roles

	return nil
}

// UnmarshalYAML converts Version to string from any type
func (h *RoleDefinitions) UnmarshalYAML(unmarshal func(any) error) error {
	aux := &struct {
		Version any             `yaml:"version"`
		Roles   map[string]Role `yaml:"roles"`
	}{
		Roles: make(map[string]Role),
	}

	if err := unmarshal(&aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Roles = aux.Roles

	return nil
}

// Validate validates all roles in the definition using struct validation tags
func (h *RoleDefinitions) Validate() error {
	validate := common.GetValidator()

	const (
		MaxInherits  = 50
		MaxProviders = 5
		MaxWorkflows = 5
	)

	for roleKey, role := range h.Roles {
		// Validate struct tags
		if err := validate.Struct(&role); err != nil {
			return fmt.Errorf("role '%s' validation failed: %w", roleKey, err)
		}

		// Additional business logic validations
		if len(role.Inherits) > MaxInherits {
			return fmt.Errorf("role '%s' exceeds maximum inherits limit (%d > %d)", roleKey, len(role.Inherits), MaxInherits)
		}
		if len(role.Providers) > MaxProviders {
			return fmt.Errorf("role '%s' exceeds maximum providers limit (%d > %d)", roleKey, len(role.Providers), MaxProviders)
		}
		if len(role.Workflows) > MaxWorkflows {
			return fmt.Errorf("role '%s' exceeds maximum workflows limit (%d > %d)", roleKey, len(role.Workflows), MaxWorkflows)
		}

	}

	return nil
}
