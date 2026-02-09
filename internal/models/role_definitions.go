package models

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"
)

// RoleDefinitions represents the structure for roles YAML/JSON
// These are used for configuration. These store the full role information.
// However, when in client mode they will only store the identifier, name and description of a role. The full role information is only stored in the daemon for internal use.
// so the RoleDefinitions needs to be able to parse the RoleResponse object type
type RoleDefinitions struct {
	Version *version.Version `yaml:"version" json:"version"`
	Roles   map[string]Role  `yaml:"roles" json:"roles"`
	Meta    ResponseMeta     `json:"meta"`
}

// UnmarshalJSON converts Version to string from any type and handles both
// API response format (with roles as SearchResult array) and config file format (map)
func (h *RoleDefinitions) UnmarshalJSON(data []byte) error {
	// First, try to detect if this is a RolesResponse (array) or RoleDefinitions (map)
	var detector struct {
		Roles json.RawMessage `json:"roles"`
	}

	if err := json.Unmarshal(data, &detector); err != nil {
		return err
	}

	// Check if roles starts with '[' (array) or '{' (object/map)
	if len(detector.Roles) > 0 && detector.Roles[0] == '[' {
		// This is a RolesResponse format with roles as an array of SearchResult
		aux := &struct {
			Version any                          `json:"version"`
			Roles   []SearchResult[RoleResponse] `json:"roles"`
			Meta    ResponseMeta                 `json:"meta"`
		}{}

		if err := json.Unmarshal(data, &aux); err != nil {
			return err
		}

		parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
		if err != nil {
			return err
		}
		h.Version = parsedVersion
		h.Meta = aux.Meta

		// Convert SearchResult array to map
		h.Roles = make(map[string]Role)
		for _, searchResult := range aux.Roles {
			role := searchResult.Result
			if role.Identifier != "" {
				h.Roles[role.Identifier] = role
			}
		}

		return nil
	}

	// This is a RoleDefinitions format with roles as a map
	aux := &struct {
		Version any             `json:"version"`
		Roles   map[string]Role `json:"roles"`
		Meta    ResponseMeta    `json:"meta"`
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
	h.Meta = aux.Meta

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

	for roleKey, role := range h.Roles {

		if err := role.Validate(); err != nil {
			return fmt.Errorf("role '%s' validation failed: %w", roleKey, role.Validate())
		}

	}

	return nil
}
