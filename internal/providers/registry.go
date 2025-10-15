package providers

import (
	"fmt"
	"reflect"
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

// Get returns a new instance of the provider from the registry.
func CreateInstance(name string) (models.ProviderImpl, error) {
	name = strings.ToLower(name)
	template, exists := registry[name]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", name)
	}

	// Create a new instance of the same type
	providerType := reflect.TypeOf(template)
	if providerType.Kind() == reflect.Pointer {
		providerType = providerType.Elem()
	}
	newInstance := reflect.New(providerType)
	return newInstance.Interface().(models.ProviderImpl), nil
}
