package providers

import (
	"fmt"
	"reflect"
	"strings"

	"sync"

	"github.com/thand-io/agent/internal/models"
)

var (
	registry      = make(map[string]models.Provider)
	registryMutex sync.RWMutex
)

// Register adds a provider to the registry.
func Register(name string, provider models.Provider) {
	name = strings.ToLower(name)
	registryMutex.Lock()
	defer registryMutex.Unlock()
	if _, exists := registry[name]; exists {
		// Handle duplicate registration if necessary
		return
	}
	registry[name] = provider
}

// Set replaces a provider in the registry (useful for testing)
func Set(name string, provider models.Provider) {
	name = strings.ToLower(name)
	registryMutex.Lock()
	defer registryMutex.Unlock()
	registry[name] = provider
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
