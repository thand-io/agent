package providers

import (
	"fmt"
	"reflect"
	"strings"

	"sync"

	"github.com/thand-io/agent/internal/models"
)

type ProviderMetadata struct {
	Capabilities *models.ProviderCapabilities
	Schema       any
}

var (
	registry         = make(map[string]models.Provider)
	metadataRegistry = make(map[string]*ProviderMetadata)
	registryMutex    sync.RWMutex
)

// Register adds a provider to the registry with its metadata.
func Register(
	name string,
	provider models.Provider,
	capabilities *models.ProviderCapabilities,
	schema any,
) {
	name = strings.ToLower(name)
	registryMutex.Lock()
	defer registryMutex.Unlock()
	if _, exists := registry[name]; exists {
		// Handle duplicate registration if necessary
		return
	}
	registry[name] = provider
	metadataRegistry[name] = &ProviderMetadata{
		Capabilities: capabilities,
		Schema:       schema,
	}
}

// Get returns a provider from the registry.
func Get(name string) (models.Provider, error) {
	name = strings.ToLower(name)
	registryMutex.RLock()
	defer registryMutex.RUnlock()
	provider, exists := registry[name]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return provider, nil
}

// Get returns a new instance of the provider from the registry.
func CreateInstance(name string) (models.Provider, error) {
	name = strings.ToLower(name)
	registryMutex.RLock()
	template, exists := registry[name]
	registryMutex.RUnlock()
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", name)
	}

	// Create a new instance of the same type
	providerType := reflect.TypeOf(template)
	if providerType.Kind() == reflect.Pointer {
		providerType = providerType.Elem()
	}
	newInstance := reflect.New(providerType)
	return newInstance.Interface().(models.Provider), nil
}

// List returns a list of all registered provider names
func List() []string {
	registryMutex.RLock()
	defer registryMutex.RUnlock()

	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// GetCapabilities returns the capabilities for a provider without initialization
func GetCapabilities(name string) (*models.ProviderCapabilities, error) {
	name = strings.ToLower(name)
	registryMutex.RLock()
	defer registryMutex.RUnlock()
	metadata, exists := metadataRegistry[name]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	if metadata.Capabilities == nil {
		return nil, fmt.Errorf("provider '%s' does not define default capabilities", name)
	}
	return metadata.Capabilities, nil
}

// GetSchema returns the configuration schema for a provider without initialization
func GetSchema(name string) (any, error) {
	name = strings.ToLower(name)
	registryMutex.RLock()
	defer registryMutex.RUnlock()
	metadata, exists := metadataRegistry[name]
	if !exists {
		return nil, fmt.Errorf("provider not found: %s", name)
	}
	return metadata.Schema, nil
}
