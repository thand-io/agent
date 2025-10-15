package config

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	// Load modules
	_ "github.com/thand-io/agent/internal/providers/aws"
	_ "github.com/thand-io/agent/internal/providers/email"
	_ "github.com/thand-io/agent/internal/providers/gcp"
	_ "github.com/thand-io/agent/internal/providers/github"
	_ "github.com/thand-io/agent/internal/providers/oauth2"
	_ "github.com/thand-io/agent/internal/providers/oauth2.google"
	_ "github.com/thand-io/agent/internal/providers/salesforce"
	_ "github.com/thand-io/agent/internal/providers/slack"
	_ "github.com/thand-io/agent/internal/providers/terraform"
)

// LoadProviders loads providers from a file or URL and maps them to their implementations
func (c *Config) LoadProviders() (map[string]models.Provider, error) {
	vaultData, err := c.loadVaultData()
	if err != nil {
		return nil, err
	}

	foundProviders, err := loadDataFromSource(
		c.Providers.Path,
		c.Providers.URL,
		vaultData,
		ProviderDefinitions{},
	)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to load providers data")
		return nil, fmt.Errorf("failed to load providers data: %w", err)
	}

	defs := c.processProviderDefinitions(foundProviders)
	return c.initializeProviders(defs)
}

// loadVaultData loads provider data from vault if configured
func (c *Config) loadVaultData() (string, error) {
	if len(c.Providers.Vault) == 0 {
		return "", nil
	}

	if !c.HasVault() {
		return "", fmt.Errorf("vault configuration is missing. Cannot load roles from vault")
	}

	logrus.Debugln("Loading providers from vault: ", c.Providers.Vault)

	data, err := c.GetVault().GetSecret(c.Providers.Vault)
	if err != nil {
		logrus.WithError(err).Errorln("Error loading providers from vault")
		return "", fmt.Errorf("failed to get secret from vault: %w", err)
	}

	logrus.Debugln("Loaded providers from vault: ", len(data), " bytes")
	return string(data), nil
}

// processProviderDefinitions processes raw provider data and returns enabled providers
func (c *Config) processProviderDefinitions(foundProviders []*ProviderDefinitions) map[string]models.Provider {
	defs := make(map[string]models.Provider)
	logrus.Debugln("Processing loaded providers: ", len(foundProviders))

	for _, provider := range foundProviders {
		for providerKey, p := range provider.Providers {
			if !c.shouldIncludeProvider(providerKey, p, defs) {
				continue
			}

			if len(p.Name) == 0 {
				p.Name = providerKey
			}

			defs[providerKey] = p
			logrus.Infoln("Found provider:", providerKey, "of type", p.Provider)
		}
	}

	return defs
}

// shouldIncludeProvider determines if a provider should be included in the final list
func (c *Config) shouldIncludeProvider(providerKey string, p models.Provider, existingDefs map[string]models.Provider) bool {
	if !p.Enabled {
		logrus.Infoln("Provider disabled:", providerKey)
		return false
	}

	if _, exists := existingDefs[providerKey]; exists {
		logrus.Warningln("Duplicate provider key found, skipping:", providerKey)
		return false
	}

	return true
}

// initResult represents the result of provider initialization
type initResult struct {
	key      string
	provider *models.Provider
	err      error
}

// initializeProviders initializes all providers in parallel using channels
func (c *Config) initializeProviders(defs map[string]models.Provider) (map[string]models.Provider, error) {
	resultChan := make(chan initResult, len(defs))

	// Start goroutines for each provider
	for providerKey, p := range defs {
		go func(providerKey string, provider models.Provider) {
			err := c.initializeSingleProvider(providerKey, &provider)
			resultChan <- initResult{
				key:      providerKey,
				provider: &provider,
				err:      err,
			}
		}(providerKey, p)
	}

	// Collect results from all goroutines
	results := make(map[string]models.Provider)
	for i := 0; i < len(defs); i++ {
		result := <-resultChan
		if result.err != nil {
			logrus.WithError(result.err).Errorln("Failed to initialize provider:", result.key)
			// Skip failed providers - don't add to results
			continue
		}
		// The provider returned from the goroutine already has the client set
		results[result.key] = *result.provider
	}

	logrus.Debugln("All providers initialized successfully")
	return results, nil
}

// initializeSingleProvider initializes a single provider
func (c *Config) initializeSingleProvider(providerKey string, p *models.Provider) error {
	impl, err := c.getProviderImplementation(providerKey, p.Provider)
	if err != nil {
		return err
	}

	if err := impl.Initialize(*p); err != nil {
		return err
	}

	p.SetClient(impl)
	return nil
} // getProviderImplementation returns the appropriate provider implementation based on config mode
func (c *Config) getProviderImplementation(providerKey string, providerName string) (models.ProviderImpl, error) {
	if c.IsServer() || c.IsAgent() {
		return providers.CreateInstance(strings.ToLower(providerName))
	}

	if c.IsClient() {
		return providers.NewRemoteProviderProxy(providerKey, c.GetLoginServerApiUrl()), nil
	}

	return nil, fmt.Errorf("unknown config mode, cannot load providers")
}
