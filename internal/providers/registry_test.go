package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thand-io/agent/internal/models"
)

// MockProvider is a test provider implementation
type MockProvider struct {
	*models.BaseProvider
	name         string
	capabilities *models.ProviderCapabilities
	validateErr  error
}

func NewMockProvider(name string, caps *models.ProviderCapabilities) *MockProvider {
	config := models.ProviderConfig{
		Name:     name,
		Provider: name,
		Config:   &models.BasicConfig{},
	}
	return &MockProvider{
		BaseProvider: models.NewBaseProvider(name, config, caps),
		name:         name,
		capabilities: caps,
	}
}

func (m *MockProvider) Initialize(providerType string, config models.ProviderConfig) error {
	m.BaseProvider.Initialize(providerType, config)
	return nil
}

// MockSchema is a test schema implementation
type MockSchema struct {
	data map[string]string
}

func (s *MockSchema) Unmarshal(config *models.BasicConfig) error {
	if config == nil {
		return nil
	}
	s.data = make(map[string]string)
	for k, v := range *config {
		if str, ok := v.(string); ok {
			s.data[k] = str
		}
	}
	return nil
}

func (s *MockSchema) Validate() error {
	if _, ok := s.data["invalid"]; ok {
		return fmt.Errorf("validation error: 'invalid' key is not allowed")
	}
	return nil
}

func TestRegisterProvider(t *testing.T) {
	// Create a test registry (save and restore original)
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()
	registry = make(map[string]models.Provider)

	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}
	mockProvider := NewMockProvider("test-provider", caps)

	t.Run("Register New Provider", func(t *testing.T) {
		Register("test-provider", mockProvider, caps, &MockSchema{})

		provider, err := Get("test-provider")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("Register Provider Case Insensitive", func(t *testing.T) {
		Register("CaseSensitive", mockProvider, caps, &MockSchema{})

		// Should be retrievable with lowercase
		provider, err := Get("casesensitive")
		require.NoError(t, err)
		assert.NotNil(t, provider)

		// Should also work with original case
		provider, err = Get("CaseSensitive")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("Register Duplicate Provider", func(t *testing.T) {
		Register("duplicate", mockProvider, caps, &MockSchema{})
		Register("duplicate", mockProvider, caps, &MockSchema{}) // Should not panic

		provider, err := Get("duplicate")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})
}

func TestGetProvider(t *testing.T) {
	// Create a test registry
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()
	registry = make(map[string]models.Provider)

	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}
	mockProvider := NewMockProvider("existing-provider", caps)
	Register("existing-provider", mockProvider, caps, &MockSchema{})

	t.Run("Get Existing Provider", func(t *testing.T) {
		provider, err := Get("existing-provider")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("Get Non-existent Provider", func(t *testing.T) {
		provider, err := Get("non-existent")
		assert.Error(t, err)
		assert.Nil(t, provider)
		assert.Contains(t, err.Error(), "provider not found")
	})

	t.Run("Get Provider Case Insensitive", func(t *testing.T) {
		provider, err := Get("EXISTING-PROVIDER")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})
}

func TestCreateInstance(t *testing.T) {
	// Create a test registry
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()
	registry = make(map[string]models.Provider)

	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}
	mockProvider := NewMockProvider("template", caps)
	Register("template", mockProvider, caps, &MockSchema{})

	t.Run("Create Instance from Template", func(t *testing.T) {
		instance, err := CreateInstance("template")
		require.NoError(t, err)
		assert.NotNil(t, instance)

		// Verify it's a new instance, not the same reference
		assert.NotEqual(t, mockProvider, instance)
	})

	t.Run("Create Instance Case Insensitive", func(t *testing.T) {
		instance, err := CreateInstance("TEMPLATE")
		require.NoError(t, err)
		assert.NotNil(t, instance)
	})

	t.Run("Create Instance Non-existent Provider", func(t *testing.T) {
		instance, err := CreateInstance("non-existent")
		assert.Error(t, err)
		assert.Nil(t, instance)
		assert.Contains(t, err.Error(), "provider not found")
	})
}

func TestListProviders(t *testing.T) {
	// Create a test registry
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()
	registry = make(map[string]models.Provider)

	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}

	t.Run("List Empty Registry", func(t *testing.T) {
		providers := List()
		assert.Empty(t, providers)
	})

	t.Run("List Multiple Providers", func(t *testing.T) {
		Register("provider1", NewMockProvider("provider1", caps), caps, &MockSchema{})
		Register("provider2", NewMockProvider("provider2", caps), caps, &MockSchema{})
		Register("provider3", NewMockProvider("provider3", caps), caps, &MockSchema{})

		providers := List()
		assert.Len(t, providers, 3)
		assert.Contains(t, providers, "provider1")
		assert.Contains(t, providers, "provider2")
		assert.Contains(t, providers, "provider3")
	})
}

func TestRegistryConcurrency(t *testing.T) {
	// Create a test registry
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()
	registry = make(map[string]models.Provider)

	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}

	t.Run("Concurrent Register and Get", func(t *testing.T) {
		done := make(chan bool)

		// Register providers concurrently
		for i := 0; i < 10; i++ {
			go func(idx int) {
				provider := NewMockProvider("concurrent", caps)
				Register("concurrent", provider, caps, &MockSchema{})
				done <- true
			}(i)
		}

		// Get providers concurrently
		for i := 0; i < 10; i++ {
			go func() {
				_, _ = Get("concurrent")
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 20; i++ {
			<-done
		}

		// Verify provider was registered
		provider, err := Get("concurrent")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("Concurrent List", func(t *testing.T) {
		Register("list-test-1", NewMockProvider("list-test-1", caps), caps, &MockSchema{})
		Register("list-test-2", NewMockProvider("list-test-2", caps), caps, &MockSchema{})

		done := make(chan bool)

		// List concurrently
		for i := 0; i < 10; i++ {
			go func() {
				providers := List()
				assert.NotEmpty(t, providers)
				done <- true
			}()
		}

		// Wait for all goroutines
		for i := 0; i < 10; i++ {
			<-done
		}
	})
}
