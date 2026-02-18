package providers

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// GetCapabilities returns the default capabilities for a provider by name
// This queries the provider registry without initializing the provider
func GetCapabilities(providerName string) (*models.ProviderCapabilities, error) {
	provider, err := providers.Get(providerName)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	caps := provider.GetDefaultCapabilities()
	if caps == nil {
		return nil, fmt.Errorf("provider '%s' does not define default capabilities", providerName)
	}

	return caps, nil
}

// ValidateConfig validates a provider's configuration without initialization
// This uses the provider's schema validation
func ValidateConfig(providerName string, config *models.BasicConfig) error {
	provider, err := providers.Get(providerName)
	if err != nil {
		return fmt.Errorf("provider not found: %s", providerName)
	}

	return provider.ValidateConfig(config)
}

// ListProviders returns a list of all registered provider names
func ListProviders() []string {
	return providers.List()
}

// ProviderExists checks if a provider is registered
func ProviderExists(providerName string) bool {
	_, err := providers.Get(providerName)
	return err == nil
}

// GetProviderInfo returns metadata about a provider without initialization
type ProviderInfo struct {
	Name         string
	Capabilities *models.ProviderCapabilities
	Available    bool
}

// GetProviderInfo retrieves provider metadata
func GetProviderInfo(providerName string) (*ProviderInfo, error) {
	provider, err := providers.Get(providerName)
	if err != nil {
		return nil, fmt.Errorf("provider not found: %s", providerName)
	}

	return &ProviderInfo{
		Name:         providerName,
		Capabilities: provider.GetDefaultCapabilities(),
		Available:    true,
	}, nil
}

// GetAllProviderInfo returns metadata for all registered providers
func GetAllProviderInfo() map[string]*ProviderInfo {
	providerNames := providers.List()
	result := make(map[string]*ProviderInfo)

	for _, name := range providerNames {
		if info, err := GetProviderInfo(name); err == nil {
			result[name] = info
		}
	}

	return result
}
