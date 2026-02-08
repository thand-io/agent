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
	Meta      ResponseMeta              `json:"meta"`
}

// UnmarshalJSON converts Version to string from any type and handles both
// API response format (with providers as SearchResult array) and config file format (map)
func (h *ProviderDefinitions) UnmarshalJSON(data []byte) error {
	// First, try to detect if this is a ProvidersResponse (array) or ProviderDefinitions (map)
	var detector struct {
		Providers json.RawMessage `json:"providers"`
	}

	if err := json.Unmarshal(data, &detector); err != nil {
		return err
	}

	// Check if providers starts with '[' (array) or '{' (object/map)
	if len(detector.Providers) > 0 && detector.Providers[0] == '[' {
		// This is a ProvidersResponse format with providers as an array of SearchResult
		aux := &struct {
			Version   any                              `json:"version"`
			Providers []SearchResult[ProviderResponse] `json:"providers"`
			Meta      ResponseMeta                     `json:"meta"`
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
		h.Providers = make(map[string]ProviderConfig)
		for _, searchResult := range aux.Providers {
			providerResp := searchResult.Result
			if providerResp.Identifier != "" {
				// Create a ProviderConfig from ProviderResponse
				provider := ProviderConfig{
					Version:      providerResp.Version,
					Name:         providerResp.Name,
					Description:  providerResp.Description,
					Provider:     providerResp.Provider,
					Capabilities: providerResp.Capabilities,
					Enabled:      providerResp.Enabled,
					// Note: Config and Role fields are not populated from response
				}
				h.Providers[providerResp.Identifier] = provider
			}
		}

		return nil
	}

	// This is a ProviderDefinitions format with providers as a map
	aux := &struct {
		Version   any                       `json:"version"`
		Providers map[string]ProviderConfig `json:"providers"`
		Meta      ResponseMeta              `json:"meta"`
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
	h.Meta = aux.Meta

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
