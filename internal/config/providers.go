package config

import (
	"context"
	"fmt"
	"strings"

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

		for providerKey, p := range provider.Providers {

			if err := p.Validate(); err != nil {
				logrus.WithError(err).Errorln("Provider definition validation failed")
				continue
			}

			if !c.shouldIncludeProvider(providerKey, p, defs) {
				continue
			}

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

			logrus.Infoln("Provider", result.key, "supports RBAC/Identities capabilities")

			// Register provider workflows and activities with Temporal if available
			if c.IsServer() {

				if c.GetServices() != nil && c.GetServices().HasTemporal() {

					logrus.Infoln("Registering Temporal workflows/activities for provider", result.key)

					temporalService := c.GetServices().GetTemporal()

					worker := temporalService.GetWorker()

					if worker == nil {
						logrus.Errorln("Temporal client is configured but worker is nil, cannot register workflows/activities for provider", result.key)
						continue
					}

					syncWorkflowName := models.CreateTemporalProviderWorkflowName(
						providerResult.GetIdentifier(),
						models.TemporalSynchronizeWorkflowName,
					)

					logrus.WithFields(logrus.Fields{
						"workflow": syncWorkflowName,
					}).Infoln("Registering provider synchronize workflow with name", syncWorkflowName)

					// Register the provider Synchronize workflow. This updates roles, permissions,
					// resources and identities for RBAC. We register this on the provider itself since it's a core part of the provider's functionality, but we register all other workflows and activities separately to allow providers to opt out of Temporal if they want.
					worker.RegisterWorkflowWithOptions(
						models.ProviderSynchronizeWorkflow,
						workflow.RegisterOptions{
							Name:               syncWorkflowName,
							VersioningBehavior: workflow.VersioningBehaviorPinned,
						},
					)

					if providerResult.HasCapability(models.ProviderCapabilityProvisioning) {

						authWorkflowName := models.CreateTemporalProviderWorkflowName(
							providerResult.GetIdentifier(),
							models.TemporalAuthorizeRoleWorkflowName)

						logrus.WithFields(logrus.Fields{
							"workflow": authWorkflowName,
						}).Infoln("Registering provider authorize role workflow with name", authWorkflowName)

						// Register the provider-specific authorize and revoke role workflows.
						// These are closure-based: they capture the live provider instance so the
						// child workflow can call provider.AuthorizeRole / RevokeRole with a
						// full workflow.Context, allowing providers to dispatch activities,
						// use workflow.Go, etc.
						worker.RegisterWorkflowWithOptions(
							models.CreateProviderAuthorizeRoleWorkflow(c, providerResult),
							workflow.RegisterOptions{
								Name:               authWorkflowName,
								VersioningBehavior: workflow.VersioningBehaviorPinned,
							},
						)

						revokeWorkflowName := models.CreateTemporalProviderWorkflowName(
							providerResult.GetIdentifier(),
							models.TemporalRevokeRoleWorkflowName)

						logrus.WithFields(logrus.Fields{
							"workflow": revokeWorkflowName,
						}).Infoln("Registering provider revoke role workflow with name", revokeWorkflowName)

						worker.RegisterWorkflowWithOptions(
							models.CreateProviderRevokeRoleWorkflow(c, providerResult),
							workflow.RegisterOptions{
								Name:               revokeWorkflowName,
								VersioningBehavior: workflow.VersioningBehaviorPinned,
							},
						)
					}

					// Register all custom provider workflows
					workflowsRegistry := providerResult.RegisterWorkflows()
					if workflowsRegistry != nil {
						logrus.Infoln("Registering Temporal workflows for provider", result.key)
						worker.RegisterWorkflow(workflowsRegistry)
					}

					// Register default provider activities
					err := models.RegisterProviderActivities(temporalService, providerResult)
					if err != nil {
						logrus.WithError(err).Errorln("Failed to register default activities for provider:", result.key)
						continue
					}

					customActivities := providerResult.RegisterActivities()
					if customActivities != nil {
						// Now register any custom activities defined by the provider
						err = models.RegisterActivities(
							temporalService,
							providerResult.GetIdentifier(),
							customActivities,
						)
						if err != nil {
							logrus.WithError(err).Errorln("Failed to register custom activities for provider:", result.key)
							continue
						}
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
			}).Debugln("Skipping provider", name, "due to missing capability:", capability)
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
