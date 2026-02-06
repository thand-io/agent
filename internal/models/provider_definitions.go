package models

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/thand-io/agent/internal/common"
)

// ProviderDefinitions represents a collection of provider configurations loaded from a file or other source.
// These definitions are used to configure the providers that the agent can interact with, including their capabilities and settings. The structure includes a version for compatibility management and a list of providers with their respective configurations.
type ProviderDefinitions struct {
	Version   *version.Version          `yaml:"version" json:"version"`
	Providers map[string]ProviderConfig `yaml:"providers" json:"providers"`
}

// UnmarshalJSON converts Version to string from any type
func (h *ProviderDefinitions) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Version   any                       `json:"version"`
		Providers map[string]ProviderConfig `json:"providers"`
	}{
		Providers: make(map[string]ProviderConfig),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Providers = aux.Providers

	return nil
}

// UnmarshalYAML converts Version to string from any type
func (h *ProviderDefinitions) UnmarshalYAML(unmarshal func(any) error) error {
	aux := &struct {
		Version   any                       `yaml:"version"`
		Providers map[string]ProviderConfig `yaml:"providers"`
	}{
		Providers: make(map[string]ProviderConfig),
	}

	if err := unmarshal(&aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Providers = aux.Providers

	return nil
}

// Validate validates all providers in the definition using struct validation tags
func (h *ProviderDefinitions) Validate() error {

	validate := common.GetValidator()

	for providerKey, provider := range h.Providers {
		if err := validate.Struct(&provider); err != nil {
			return fmt.Errorf("provider '%s' validation failed: %w", providerKey, err)
		}
	}

	return nil
}
