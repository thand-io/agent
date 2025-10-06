package providers

import (
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
)

var registry = make(map[string]models.ProviderImpl)

// Register adds a provider to the registry.
func Register(name string, provider models.ProviderImpl) {
	name = strings.ToLower(name)
	if _, exists := registry[name]; exists {
		// Handle duplicate registration if necessary
		return
	}
	registry[name] = provider
}

// Get returns a provider from the registry.
func Get(name string) (models.ProviderImpl, error) {
	name = strings.ToLower(name)
	provider, exists := registry[name]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return provider, nil
}
