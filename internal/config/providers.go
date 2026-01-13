package config

import (
	"errors"
	"fmt"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/environment"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	// Load modules
	_ "github.com/thand-io/agent/internal/providers/aws"
	_ "github.com/thand-io/agent/internal/providers/cloudflare"
	_ "github.com/thand-io/agent/internal/providers/email"
	_ "github.com/thand-io/agent/internal/providers/gcp"
	_ "github.com/thand-io/agent/internal/providers/gcp.iap"
	_ "github.com/thand-io/agent/internal/providers/github"
	_ "github.com/thand-io/agent/internal/providers/kubernetes"
	_ "github.com/thand-io/agent/internal/providers/oauth2"
	_ "github.com/thand-io/agent/internal/providers/oauth2.google"
	_ "github.com/thand-io/agent/internal/providers/okta"
	_ "github.com/thand-io/agent/internal/providers/salesforce"
	_ "github.com/thand-io/agent/internal/providers/saml"
	_ "github.com/thand-io/agent/internal/providers/slack"
	_ "github.com/thand-io/agent/internal/providers/terraform"
	_ "github.com/thand-io/agent/internal/providers/thand"
)

// LoadProviders loads providers from a file or URL and maps them to their implementations
func (c *Config) LoadProviders() (map[string]models.ProviderConfig, error) {

	vaultData, err := c.loadProviderVaultData()

	if err != nil {
		return nil, err
	}

	foundProviders := []*models.ProviderDefinitions{}

	if len(vaultData) > 0 || len(c.Providers.Path) > 0 || c.Providers.URL != nil {

		importedProviders, err := loadDataFromSource(
			c.Providers.Path,
			c.Providers.URL,
			vaultData,
			models.ProviderDefinitions{},
		)

		if err != nil {
			logrus.WithError(err).Errorln("Failed to load providers data")
			return nil, fmt.Errorf("failed to load providers data: %w", err)
		}

		foundProviders = importedProviders

	}

	if len(foundProviders) == 0 {
		logrus.Warningln("No providers found from any source, loading defaults")
		foundProviders, err = environment.GetDefaultProviders(c.Environment.Platform)
		if err != nil {
			return nil, fmt.Errorf("failed to load default providers: %w", err)
		}
		logrus.Infoln("Loaded default providers:", len(foundProviders))
	}

	return c.ApplyProviders(foundProviders)
}

func (c *Config) ApplyProviders(foundProviders []*models.ProviderDefinitions) (map[string]models.ProviderConfig, error) {

	providersLen := len(c.Providers.Definitions)
	if providersLen > 0 {
		// Add providers defined directly in config
		logrus.Debugln("Adding providers defined directly in config: ", providersLen)

		defaultVersion := version.Must(version.NewVersion("1.0.0"))

		for providerKey, provider := range c.Providers.Definitions {
			foundProviders = append(foundProviders, &models.ProviderDefinitions{
				Version: defaultVersion,
				Providers: map[string]models.ProviderConfig{
					providerKey: provider,
				},
			})
		}
	}

	return c.processProviderDefinitions(foundProviders), nil

}

// loadProviderVaultData loads provider data from vault if configured
func (c *Config) loadProviderVaultData() (string, error) {

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
func (c *Config) processProviderDefinitions(foundProviders []*models.ProviderDefinitions) map[string]models.ProviderConfig {
	defs := make(map[string]models.ProviderConfig)
	logrus.Debugln("Processing loaded providers: ", len(foundProviders))

	for _, provider := range foundProviders {

		if err := provider.Validate(); err != nil {
			logrus.WithError(err).Errorln("Provider definition validation failed")
			continue
		}

		for providerKey, p := range provider.Providers {

			if !c.shouldIncludeProvider(providerKey, p, defs) {
				continue
			}

			defs[providerKey] = p

			logrus.Infoln("Found provider:", providerKey, "of type", p.Provider)
		}
	}

	return defs
}

// shouldIncludeProvider determines if a provider should be included in the final list
func (c *Config) shouldIncludeProvider(
	providerKey string,
	p models.ProviderConfig,
	existingDefs map[string]models.ProviderConfig,
) bool {

	if !p.Enabled {
		logrus.Infoln("Provider disabled (not marked as enabled):", providerKey)
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
	provider models.Provider
	err      error
}

// InitializeProviders initializes all providers in parallel using channels
func (c *Config) InitializeProviders() error {

	defs := c.GetProviders().Definitions

	logrus.Debugln("Initializing providers: ", len(defs))

	resultChan := make(chan initResult, len(defs))

	// Start goroutines for each provider
	for providerKey, p := range defs {
		go func(providerKey string, provider models.ProviderConfig) {
			impl, err := c.initializeSingleProvider(providerKey, &provider)
			resultChan <- initResult{
				key:      providerKey,
				provider: impl,
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

		if result.provider == nil {
			logrus.Errorln("Provider client is nil after initialization:", result.key)
			// Skip providers with nil client
			continue
		}

		// Check for capabilities for RBAC and Identities
		if result.provider.HasAnyCapability(
			models.ProviderCapabilityIdentities,
			models.ProviderCapabilityUsers,
			models.ProviderCapabilityGroups,
			models.ProviderCapabilityResources,
			models.ProviderCapabilityRoles,
			models.ProviderCapabilityPermissions,
			models.ProviderCapabilityTenants,
		) {

			logrus.Infoln("Provider", result.key, "supports RBAC/Identities capabilities")

			// Register provider workflows and activities with Temporal if available
			if c.IsServer() {

				if c.GetServices() != nil && c.GetServices().HasTemporal() {

					logrus.Infoln("Registering Temporal workflows/activities for provider", result.key)

					temporalService := c.GetServices().GetTemporal()

					// Register all provider workflows and activities
					err := result.provider.RegisterWorkflows(temporalService)
					if err != nil && !errors.Is(err, models.ErrNotImplemented) {
						logrus.WithError(err).Errorln("Failed to register workflows for provider:", result.key)
						continue
					}

					err = result.provider.RegisterActivities(temporalService)
					if err != nil && !errors.Is(err, models.ErrNotImplemented) {
						logrus.WithError(err).Errorln("Failed to register activities for provider:", result.key)
						continue
					}

				}

				logrus.Infoln("Synchronizing provider", result.key)
				c.synchronizeProvider(result.provider)

			} else {
				logrus.Infoln("Skipping Temporal registration for provider", result.key, "in non-server mode")
			}
		}

		// The provider returned from the goroutine already has the client set
		results[result.key] = result.provider

	}

	c.mu.Lock()
	c.providerInstances = results
	c.mu.Unlock()

	logrus.Debugln("All providers initialized successfully")
	return nil
}

// initializeSingleProvider initializes a single provider
func (c *Config) initializeSingleProvider(providerKey string, p *models.ProviderConfig) (models.Provider, error) {

	impl, err := c.getProviderImplementation(providerKey, p.Provider)

	if err != nil {
		return nil, err
	}

	// Before we initialize, we need to check if any of the provider's
	// config has any environment variable references and resolve them
	err = p.ResolveConfig(
		map[string]any{},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to resolve environment variables for provider %s: %w", providerKey, err)
	}

	if err := impl.Initialize(providerKey, *p); err != nil {
		return nil, err
	}

	return impl, nil
}

// getProviderImplementation returns the appropriate provider implementation based on config mode
func (c *Config) getProviderImplementation(providerKey string, providerName string) (models.Provider, error) {

	if c.IsServer() || c.IsAgent() {
		return providers.CreateInstance(strings.ToLower(providerName))
	}

	// If we're a client, return a remote proxy. This will let us forward
	// requests to the server for provider operations
	if c.IsClient() {
		return providers.NewRemoteProviderProxy(
			providerKey,
			c.DiscoverLoginServerApiUrl(
				c.GetLoginServerUrl(),
			),
		), nil
	}

	return nil, fmt.Errorf("unknown config mode, cannot load providers")
}
