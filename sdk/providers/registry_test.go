package providers

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// MockProvider is a test provider implementation
type MockProvider struct {
	*models.BaseProvider
	name         string
	capabilities *models.ProviderCapabilities
	validateErr  error
}

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

func TestGetCapabilities(t *testing.T) {
	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
		Authorizer: models.NewCapability(),
	}
	mockProvider := NewMockProvider("test-caps", caps)
	providers.Register("test-caps", mockProvider, caps, &MockSchema{})

	t.Run("Get Capabilities for Registered Provider", func(t *testing.T) {
		result, err := GetCapabilities("test-caps")
		require.NoError(t, err)
		assert.NotNil(t, result)
		assert.NotNil(t, result.Identities)
		assert.NotNil(t, result.Authorizer)
	})

	t.Run("Get Capabilities Case Insensitive", func(t *testing.T) {
		result, err := GetCapabilities("TEST-CAPS")
		require.NoError(t, err)
		assert.NotNil(t, result)
	})

	t.Run("Get Capabilities for Non-existent Provider", func(t *testing.T) {
		result, err := GetCapabilities("non-existent-provider")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "provider not found")
	})

	t.Run("Get Capabilities for Provider Without Defaults", func(t *testing.T) {
		mockProviderNoCaps := NewMockProvider("no-caps", nil)
		providers.Register("no-caps", mockProviderNoCaps, nil, &MockSchema{})

		result, err := GetCapabilities("no-caps")
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "does not define default capabilities")
	})
}

func TestValidateConfig(t *testing.T) {
	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}

	t.Run("Validate Valid Config", func(t *testing.T) {
		mockProvider := NewMockProvider("validate-test", caps)
		providers.Register("validate-test", mockProvider, caps, &MockSchema{})

		config := &models.BasicConfig{
			"key": "value",
		}
		err := ValidateConfig("validate-test", config)
		assert.NoError(t, err)
	})

	t.Run("Validate Invalid Config", func(t *testing.T) {
		mockProvider := NewMockProvider("validate-error", caps)
		mockProvider.validateErr = fmt.Errorf("validation error")
		providers.Register("validate-error", mockProvider, caps, &MockSchema{})

		config := &models.BasicConfig{
			"invalid": "config",
		}
		err := ValidateConfig("validate-error", config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "validation error")
	})

	t.Run("Validate Config for Non-existent Provider", func(t *testing.T) {
		config := &models.BasicConfig{
			"key": "value",
		}
		err := ValidateConfig("non-existent", config)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "provider not found")
	})

	t.Run("Validate Nil Config", func(t *testing.T) {
		mockProvider := NewMockProvider("nil-config", caps)
		providers.Register("nil-config", mockProvider, caps, &MockSchema{})

		err := ValidateConfig("nil-config", nil)
		assert.NoError(t, err)
	})
}

func TestListProviders(t *testing.T) {
	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}

	t.Run("List Providers", func(t *testing.T) {
		// Register some test providers
		providers.Register("list-test-1", NewMockProvider("list-test-1", caps), caps, &MockSchema{})
		providers.Register("list-test-2", NewMockProvider("list-test-2", caps), caps, &MockSchema{})
		providers.Register("list-test-3", NewMockProvider("list-test-3", caps), caps, &MockSchema{})

		result := ListProviders()
		assert.NotEmpty(t, result)

		// Check that our test providers are in the list
		assert.Contains(t, result, "list-test-1")
		assert.Contains(t, result, "list-test-2")
		assert.Contains(t, result, "list-test-3")
	})

	t.Run("List Returns All Providers", func(t *testing.T) {
		result := ListProviders()
		assert.NotEmpty(t, result)

		// Should include actual registered providers like aws, gcp, azure, etc.
		// We can't assert exact count since actual providers are registered
		assert.GreaterOrEqual(t, len(result), 3)
	})
}

func TestProviderExists(t *testing.T) {
	caps := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
	}
	mockProvider := NewMockProvider("exists-test", caps)
	providers.Register("exists-test", mockProvider, caps, &MockSchema{})

	t.Run("Provider Exists", func(t *testing.T) {
		exists := ProviderExists("exists-test")
		assert.True(t, exists)
	})

	t.Run("Provider Does Not Exist", func(t *testing.T) {
		exists := ProviderExists("does-not-exist")
		assert.False(t, exists)
	})

	t.Run("Provider Exists Case Insensitive", func(t *testing.T) {
		exists := ProviderExists("EXISTS-TEST")
		assert.True(t, exists)
	})

	t.Run("Check Real Providers", func(t *testing.T) {
		// These should be registered by actual provider packages
		// At least some providers should exist
		allProviders := ListProviders()
		if len(allProviders) > 0 {
			firstProvider := allProviders[0]
			exists := ProviderExists(firstProvider)
			assert.True(t, exists)
		}
	})
}

func TestGetProviderInfo(t *testing.T) {
	caps := &models.ProviderCapabilities{
		Identities:   models.NewSynchronizableCapability(),
		Roles:        models.NewSynchronizableCapability(),
		Permissions:  models.NewSynchronizableCapability(),
		Authorizer:   models.NewCapability(),
		Notifier:     nil,
		Provisioning: nil,
	}
	mockProvider := NewMockProvider("info-test", caps)
	// Use Register() instead of Set() to properly store metadata
	providers.Register("info-test", mockProvider, caps, &MockSchema{})

	t.Run("Get Provider Info", func(t *testing.T) {
		info, err := GetProviderInfo("info-test")
		require.NoError(t, err)
		assert.NotNil(t, info)
		assert.Equal(t, "info-test", info.Name)
		assert.True(t, info.Available)
		assert.NotNil(t, info.Capabilities)
		assert.NotNil(t, info.Capabilities.Identities)
		assert.NotNil(t, info.Capabilities.Roles)
		assert.NotNil(t, info.Capabilities.Permissions)
		assert.NotNil(t, info.Capabilities.Authorizer)
		assert.Nil(t, info.Capabilities.Notifier)
		assert.Nil(t, info.Capabilities.Provisioning)
	})

	t.Run("Get Provider Info Case Insensitive", func(t *testing.T) {
		info, err := GetProviderInfo("INFO-TEST")
		require.NoError(t, err)
		// Note: Provider name is normalized to lowercase in registry
		assert.NotEmpty(t, info.Name)
	})

	t.Run("Get Provider Info Non-existent", func(t *testing.T) {
		info, err := GetProviderInfo("non-existent")
		assert.Error(t, err)
		assert.Nil(t, info)
		assert.Contains(t, err.Error(), "provider not found")
	})
}

func TestGetAllProviderInfo(t *testing.T) {
	caps1 := &models.ProviderCapabilities{
		Identities: models.NewSynchronizableCapability(),
		Authorizer: models.NewCapability(),
	}
	caps2 := &models.ProviderCapabilities{
		Notifier: models.NewCapability(),
	}
	caps3 := &models.ProviderCapabilities{
		Roles:       models.NewSynchronizableCapability(),
		Permissions: models.NewSynchronizableCapability(),
	}

	providers.Register("all-info-1", NewMockProvider("all-info-1", caps1), caps1, &MockSchema{})
	providers.Register("all-info-2", NewMockProvider("all-info-2", caps2), caps2, &MockSchema{})
	providers.Register("all-info-3", NewMockProvider("all-info-3", caps3), caps3, &MockSchema{})

	t.Run("Get All Provider Info", func(t *testing.T) {
		allInfo := GetAllProviderInfo()
		assert.NotEmpty(t, allInfo)

		// Check our test providers
		assert.Contains(t, allInfo, "all-info-1")
		assert.Contains(t, allInfo, "all-info-2")
		assert.Contains(t, allInfo, "all-info-3")

		// Verify structure
		info1 := allInfo["all-info-1"]
		assert.NotNil(t, info1)
		assert.Equal(t, "all-info-1", info1.Name)
		assert.True(t, info1.Available)
		assert.NotNil(t, info1.Capabilities)
		assert.NotNil(t, info1.Capabilities.Identities)
		assert.NotNil(t, info1.Capabilities.Authorizer)
	})

	t.Run("Get All Provider Info Contains Real Providers", func(t *testing.T) {
		allInfo := GetAllProviderInfo()
		assert.NotEmpty(t, allInfo)

		// Should have at least our test providers
		assert.GreaterOrEqual(t, len(allInfo), 3)

		// All entries should have proper structure
		for name, info := range allInfo {
			assert.NotEmpty(t, name)
			assert.NotNil(t, info)
			assert.Equal(t, name, info.Name)
			assert.True(t, info.Available)
			// Capabilities can be nil for some providers, so we don't assert it
		}
	})
}

func TestSDKRegistryIntegration(t *testing.T) {
	caps := &models.ProviderCapabilities{
		Identities:   models.NewSynchronizableCapability(),
		Roles:        models.NewSynchronizableCapability(),
		Permissions:  models.NewSynchronizableCapability(),
		Authorizer:   models.NewCapability(),
		Notifier:     models.NewCapability(),
		Provisioning: nil,
	}
	mockProvider := NewMockProvider("integration-test", caps)
	providers.Register("integration-test", mockProvider, caps, &MockSchema{})

	t.Run("Full Integration Flow", func(t *testing.T) {
		// 1. Check if provider exists
		exists := ProviderExists("integration-test")
		assert.True(t, exists)

		// 2. Get provider info
		info, err := GetProviderInfo("integration-test")
		require.NoError(t, err)
		assert.NotNil(t, info)

		// 3. Get capabilities
		capabilities, err := GetCapabilities("integration-test")
		require.NoError(t, err)
		assert.NotNil(t, capabilities.Identities)
		assert.NotNil(t, capabilities.Roles)
		assert.NotNil(t, capabilities.Authorizer)

		// 4. Validate config
		config := &models.BasicConfig{
			"key": "value",
		}
		err = ValidateConfig("integration-test", config)
		assert.NoError(t, err)

		// 5. Check in list
		allProviders := ListProviders()
		assert.Contains(t, allProviders, "integration-test")

		// 6. Check in all info
		allInfo := GetAllProviderInfo()
		assert.Contains(t, allInfo, "integration-test")
	})
}

func TestSDKRegistryWithActualProviders(t *testing.T) {
	t.Run("List Real Providers", func(t *testing.T) {
		providersList := ListProviders()
		assert.NotEmpty(t, providersList)

		// Should have common providers like aws, gcp, azure, etc.
		// We don't explicitly check for specific providers since
		// the registration depends on package imports
		t.Logf("Found %d registered providers", len(providersList))
	})

	t.Run("Get All Real Provider Info", func(t *testing.T) {
		allInfo := GetAllProviderInfo()
		assert.NotEmpty(t, allInfo)

		// Verify all have proper structure
		for name, info := range allInfo {
			assert.NotEmpty(t, name)
			assert.NotNil(t, info)
			assert.Equal(t, name, info.Name)
			assert.True(t, info.Available)
		}

		t.Logf("Found %d providers with info", len(allInfo))
	})
}
func TestGetSchema(t *testing.T) {
	t.Run("Get Schema for Non-existent Provider", func(t *testing.T) {
		schema, err := GetSchema("non-existent-provider")
		assert.Error(t, err)
		assert.Nil(t, schema)
		assert.Contains(t, err.Error(), "provider not found")
	})

	t.Run("Get Schema for Mock Provider Without Schema", func(t *testing.T) {
		caps := &models.ProviderCapabilities{
			Identities: models.NewSynchronizableCapability(),
		}
		mockProvider := NewMockProvider("no-schema-test", caps)
		providers.Register("no-schema-test", mockProvider, caps, nil)

		schema, err := GetSchema("no-schema-test")
		require.NoError(t, err)
		// Mock provider returns nil schema
		assert.Nil(t, schema)
	})

	t.Run("Get Schema for Registered Providers", func(t *testing.T) {
		providersList := ListProviders()
		assert.NotEmpty(t, providersList)

		schemasFound := 0
		providersChecked := 0

		for _, providerName := range providersList {
			// Skip mock providers
			if providerName == "no-schema-test" || providerName == "test-caps" ||
				providerName == "no-caps" || providerName == "validate-test" ||
				providerName == "validate-error" || providerName == "nil-config" ||
				providerName == "exists-test" || providerName == "info-test" ||
				providerName == "all-info-1" || providerName == "all-info-2" ||
				providerName == "all-info-3" || providerName == "integration-test" ||
				providerName == "list-test-1" || providerName == "list-test-2" ||
				providerName == "list-test-3" {
				continue
			}

			providersChecked++
			schema, err := GetSchema(providerName)
			if err == nil && schema != nil {
				schemasFound++
				t.Logf("Provider %s has schema: %T", providerName, schema)
			}
		}

		t.Logf("Found %d providers with schemas out of %d real providers checked (%d total)",
			schemasFound, providersChecked, len(providersList))

		// We expect at least some real providers to have schemas
		if providersChecked > 0 {
			assert.Greater(t, schemasFound, 0, "At least one real provider should have a schema")
		}
	})
}
