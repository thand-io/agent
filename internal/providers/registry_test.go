package providers

import (
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

func (m *MockProvider) ValidateConfig(config *models.BasicConfig) error {
	if m.validateErr != nil {
		return m.validateErr
	}
	return nil
}

func (m *MockProvider) GetDefaultCapabilities() *models.ProviderCapabilities {
	return m.capabilities
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
		Register("test-provider", mockProvider, caps, &struct{}{})

		provider, err := Get("test-provider")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("Register Provider Case Insensitive", func(t *testing.T) {
		Register("CaseSensitive", mockProvider, caps, &struct{}{})

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
		Register("duplicate", mockProvider, caps, &struct{}{})
		Register("duplicate", mockProvider, caps, &struct{}{}) // Should not panic

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
	Register("existing-provider", mockProvider, caps, &struct{}{})

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

func TestSetProvider(t *testing.T) {
	// Create a test registry
	originalRegistry := registry
	defer func() {
		registry = originalRegistry
	}()
	registry = make(map[string]models.Provider)

	caps1 := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}
	caps2 := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
		Authorizer: models.NewCapability(),
	}

	mockProvider1 := NewMockProvider("test", caps1)
	mockProvider2 := NewMockProvider("test", caps2)

	t.Run("Set New Provider", func(t *testing.T) {
		Set("new-provider", mockProvider1)

		provider, err := Get("new-provider")
		require.NoError(t, err)
		assert.NotNil(t, provider)
	})

	t.Run("Replace Existing Provider", func(t *testing.T) {
		Set("replaceable", mockProvider1)

		provider, err := Get("replaceable")
		require.NoError(t, err)
		assert.Nil(t, provider.GetDefaultCapabilities().Authorizer)

		// Replace with different provider
		Set("replaceable", mockProvider2)

		provider, err = Get("replaceable")
		require.NoError(t, err)
		assert.NotNil(t, provider.GetDefaultCapabilities().Authorizer)
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
	Register("template", mockProvider, caps, &struct{}{})

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
		Register("provider1", NewMockProvider("provider1", caps), caps, &struct{}{})
		Register("provider2", NewMockProvider("provider2", caps), caps, &struct{}{})
		Register("provider3", NewMockProvider("provider3", caps), caps, &struct{}{})

		providers := List()
		assert.Len(t, providers, 3)
		assert.Contains(t, providers, "provider1")
		assert.Contains(t, providers, "provider2")
		assert.Contains(t, providers, "provider3")
	})
}

func TestValidateProviderConfigIntegration(t *testing.T) {
	// Create a test registry
	originalRegistry := registry
	originalValidator := models.ValidateProviderConfig
	defer func() {
		registry = originalRegistry
		models.ValidateProviderConfig = originalValidator
	}()
	registry = make(map[string]models.Provider)

	// Re-initialize the validator
	models.ValidateProviderConfig = func(providerName string, config *models.BasicConfig) error {
		if config == nil {
			return nil
		}

		provider, err := Get(providerName)
		if err != nil {
			return nil
		}

		return provider.ValidateConfig(config)
	}

	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}
	mockProvider := NewMockProvider("validator-test", caps)
	Register("validator-test", mockProvider, caps, &struct{}{})

	t.Run("Validate with Registered Provider", func(t *testing.T) {
		config := &models.BasicConfig{
			"key": "value",
		}
		err := models.ValidateProviderConfig("validator-test", config)
		assert.NoError(t, err)
	})

	t.Run("Validate with Unregistered Provider", func(t *testing.T) {
		config := &models.BasicConfig{
			"key": "value",
		}
		// Should not error for unregistered provider (graceful fallback)
		err := models.ValidateProviderConfig("unregistered", config)
		assert.NoError(t, err)
	})

	t.Run("Validate with Nil Config", func(t *testing.T) {
		err := models.ValidateProviderConfig("validator-test", nil)
		assert.NoError(t, err)
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
				Register("concurrent", provider, caps, &struct{}{})
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
		Register("list-test-1", NewMockProvider("list-test-1", caps), caps, &struct{}{})
		Register("list-test-2", NewMockProvider("list-test-2", caps), caps, &struct{}{})

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
