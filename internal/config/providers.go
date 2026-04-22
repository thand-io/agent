package config

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/environment"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	providerSdk "github.com/thand-io/agent/sdk/providers"
	"go.temporal.io/sdk/workflow"

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

var providerBindingsMu sync.Mutex

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
		// Prepend providers defined directly in config so they take
		// priority over externally loaded or default providers.
		logrus.Debugln("Adding providers defined directly in config: ", providersLen)

		defaultVersion := version.Must(version.NewVersion("1.0.0"))

		inlineProviders := make([]*models.ProviderDefinitions, 0, providersLen)
		for providerKey, provider := range c.Providers.Definitions {
			inlineProviders = append(inlineProviders, &models.ProviderDefinitions{
				Version: defaultVersion,
				Providers: map[string]models.ProviderConfig{
					providerKey: provider,
				},
			})
		}
		foundProviders = append(inlineProviders, foundProviders...)
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

		for providerKey, p := range provider.Providers {

			if err := p.Validate(); err != nil {
				logrus.WithError(err).Errorln("Provider definition validation failed")
				continue
			}

			if !c.shouldIncludeProvider(providerKey, p, defs) {
				continue
			}

			// Identifier is not set here as its a provider config.
			// The identifier is set during provider initialization when we
			// have the full provider config with all defaults applied.
			// This allows providers to be referenced by their key in the config,
			// but also allows the provider implementation to override the identifier if needed.

			defs[providerKey] = p

			logrus.WithFields(logrus.Fields{
				"capabiltiies": p.Capabilities,
			}).Infoln("Found provider:", providerKey, "of type", p.Provider)
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
		logrus.Warningln("Provider disabled (not marked as enabled):", providerKey)
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

		providerResult := result.provider

		// Check for capabilities for RBAC and Identities
		if providerResult.HasAnyCapability(
			models.ProviderCapabilityIdentities,
			models.ProviderCapabilityUsers,
			models.ProviderCapabilityGroups,
			models.ProviderCapabilityResources,
			models.ProviderCapabilityRoles,
			models.ProviderCapabilityPermissions,
			models.ProviderCapabilityTenants,
		) {

			logrus.Infoln("Provider", result.key, "supports synchronization or provisioning capabilities")

			if err := c.registerProviderTemporalBindings(providerResult); err != nil {
				logrus.WithError(err).Errorln("Failed to register Temporal bindings for provider:", result.key)
				continue
			}

			if c.IsServer() {
				logrus.Infoln("Synchronizing provider", result.key)
				c.synchronizeProvider(result.provider)
			} else {
				providerResult.SetReady()
			}
		} else {
			// Provider doesn't have RBAC/Identity capabilities, no sync needed
			result.provider.SetReady()
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

func (c *Config) registerProviderTemporalBindings(providerResult models.Provider) error {
	if providerResult == nil {
		return fmt.Errorf("provider is nil")
	}
	if c.GetServices() == nil || !c.GetServices().HasTemporal() {
		logrus.WithFields(logrus.Fields{
			"provider": providerResult.GetIdentifier(),
			"mode":     c.GetMode(),
		}).Info("Skipping provider Temporal registration because Temporal is unavailable")
		return nil
	}

	providerBindingsMu.Lock()
	defer providerBindingsMu.Unlock()

	if c.providerBindings == nil {
		c.providerBindings = map[string]struct{}{}
	}
	if _, exists := c.providerBindings[providerResult.GetIdentifier()]; exists {
		return nil
	}

	// Provider bindings should stay on operational workers and never leak onto
	// the shared device-registry queue.
	temporalService := c.getOperationalTemporalService()
	worker := temporalService.GetWorker()
	if worker == nil {
		return fmt.Errorf("temporal client is configured but worker is nil")
	}

	syncWorkflowName := models.CreateTemporalProviderWorkflowName(
		providerResult.GetIdentifier(),
		models.TemporalSynchronizeWorkflowName,
	)

	worker.RegisterWorkflowWithOptions(
		models.CreateProviderSynchronizeWorkflow(providerResult),
		workflow.RegisterOptions{
			Name:               syncWorkflowName,
			VersioningBehavior: workflow.VersioningBehaviorPinned,
		},
	)

	if providerResult.HasCapability(models.ProviderCapabilityProvisioning) {
		authWorkflowName := models.CreateTemporalProviderWorkflowName(
			providerResult.GetIdentifier(),
			models.TemporalAuthorizeRoleWorkflowName,
		)
		worker.RegisterWorkflowWithOptions(
			models.CreateProviderAuthorizeRoleWorkflow(c, providerResult),
			workflow.RegisterOptions{
				Name:               authWorkflowName,
				VersioningBehavior: workflow.VersioningBehaviorPinned,
			},
		)

		revokeWorkflowName := models.CreateTemporalProviderWorkflowName(
			providerResult.GetIdentifier(),
			models.TemporalRevokeRoleWorkflowName,
		)
		worker.RegisterWorkflowWithOptions(
			models.CreateProviderRevokeRoleWorkflow(c, providerResult),
			workflow.RegisterOptions{
				Name:               revokeWorkflowName,
				VersioningBehavior: workflow.VersioningBehaviorPinned,
			},
		)
	}

	if workflowsRegistry := providerResult.RegisterWorkflows(); workflowsRegistry != nil {
		worker.RegisterWorkflow(workflowsRegistry)
	}

	if err := models.RegisterProviderActivities(temporalService, providerResult); err != nil {
		return err
	}

	if customActivities := providerResult.RegisterActivities(); customActivities != nil {
		if err := models.RegisterActivities(temporalService, providerResult.GetIdentifier(), customActivities); err != nil {
			return err
		}
	}

	c.providerBindings[providerResult.GetIdentifier()] = struct{}{}
	return nil
}

func (c *Config) EnsureProviderTemporalBindings() error {
	c.mu.RLock()
	providers := make([]models.Provider, 0, len(c.providerInstances))
	for _, provider := range c.providerInstances {
		providers = append(providers, provider)
	}
	c.mu.RUnlock()

	for _, provider := range providers {
		if !provider.HasAnyCapability(
			models.ProviderCapabilityIdentities,
			models.ProviderCapabilityUsers,
			models.ProviderCapabilityGroups,
			models.ProviderCapabilityResources,
			models.ProviderCapabilityRoles,
			models.ProviderCapabilityPermissions,
			models.ProviderCapabilityTenants,
		) {
			continue
		}
		if err := c.registerProviderTemporalBindings(provider); err != nil {
			return err
		}
	}

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

	// Client mode proxies provider operations to the login server and should not
	// require the full provider config locally. Synced provider metadata from
	// /providers intentionally omits sensitive config such as client secrets, but
	// any non-sensitive config that is present should still be resolved first.
	if c.IsClient() {
		if err := impl.Initialize(providerKey, *p); err != nil {
			return nil, err
		}
		return impl, nil
	}

	err = providerSdk.ValidateConfig(p.Provider, p.Config)

	if err != nil {
		return nil, fmt.Errorf("provider config validation failed for provider %s: %w", providerKey, err)
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

func (c *Config) GetProviders() ProviderDefinitionsConfig {
	return c.Providers
}

func (c *Config) GetProviderDefinitions() map[string]models.ProviderConfig {
	return c.Providers.Definitions
}

func (c *Config) GetProvider(providerName string) (string, models.Provider, error) {

	// Get the first provider by provider name
	c.mu.RLock()
	defer c.mu.RUnlock()

	for foundName, provider := range c.providerInstances {
		if strings.Compare(provider.GetProvider(), providerName) == 0 {
			return foundName, provider, nil
		}
	}

	return "", nil, fmt.Errorf("provider not found: %s", providerName)
}

func (c *Config) GetProviderByName(name string) (models.Provider, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if provider, exists := c.providerInstances[name]; exists {
		return provider, nil
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

func (c *Config) HasProvider(name string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	_, exists := c.providerInstances[name]
	return exists
}

func (c *Config) AddProvider(name string, provider models.Provider) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.providerInstances == nil {
		c.providerInstances = make(map[string]models.Provider)
	}

	c.providerInstances[name] = provider
}

func (c *Config) GetProvidersByCapability(capability ...models.ProviderCapability) map[string]models.Provider {
	return c.GetProvidersByCapabilityWithUser(nil, capability...)
}

func (c *Config) GetProvidersByCapabilityWithUser(user *models.User, capability ...models.ProviderCapability) map[string]models.Provider {

	providers := make(map[string]models.Provider)

	c.mu.RLock()
	defer c.mu.RUnlock()

	for name, provider := range c.providerInstances {
		// Skip providers that don't have a client initialized

		if !provider.HasPermission(user) {
			logrus.Debugln("Skipping provider", name, "due to missing permissions for user")
			continue
		}

		if len(capability) != 0 && !provider.HasAnyCapability(capability...) {
			logrus.WithFields(logrus.Fields{
				"capabilities": provider.GetCapabilities(),
			}).Traceln("Skipping provider", name, "due to missing capability:", capability)
			continue
		}

		providers[name] = provider
	}
	return providers
}

func (p *ProviderDefinitionsConfig) GetProviderByName(name string) (*models.ProviderConfig, error) {
	if provider, exists := p.Definitions[name]; exists {
		return &provider, nil
	}
	return nil, fmt.Errorf("provider not found: %s", name)
}

func (r *Config) GetProviderRole(roleName string, providers ...string) *models.ProviderRole {
	return r.GetProviderRoleWithIdentity(nil, roleName, providers...)
}

func (r *Config) GetProviderRoleWithIdentity(identity *models.Identity, roleName string, providers ...string) *models.ProviderRole {

	ctx := context.TODO()

	for _, providerName := range providers {

		p, err := r.GetProviderByName(providerName)

		if err != nil || p == nil {
			continue
		}

		// Check provider-level permissions
		// If identity is nil, pass nil user to HasPermission which handles it appropriately
		var user *models.User

		if identity != nil {
			user = identity.GetUser()
		}

		if !p.HasPermission(user) {
			continue
		}

		providerRole, err := p.GetRole(ctx, roleName)

		if err != nil {
			continue
		}

		if providerRole != nil {
			return providerRole
		}
	}

	return nil
}

func (r *Config) GetProviderPermission(permissionName string, providers ...string) *models.ProviderPermission {

	ctx := context.TODO()

	for _, providerName := range providers {

		p, err := r.GetProviderByName(providerName)

		if err != nil || p == nil {
			continue
		}

		providerPermission, err := p.GetPermission(ctx, permissionName)

		if err != nil {
			continue
		}

		if providerPermission != nil {
			return providerPermission
		}
	}

	return nil
}
