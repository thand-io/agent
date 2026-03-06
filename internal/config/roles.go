// Package config handles configuration loading and role resolution for the agent.
// This file implements role inheritance, permission merging, and provider-based filtering.
package config

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/environment"
	"github.com/thand-io/agent/internal/models"
)

// Hard limits for roles configuration to prevent resource exhaustion
const (
	// MaxResources is the maximum number of resources (allow + deny) per role
	MaxResources = 100

	// MaxGroups is the maximum number of groups (allow + deny) per role
	MaxGroups = 50

	// MaxScopes is the maximum number of scopes (users + groups + domains) per role
	MaxScopes = 50

	// MaxInherits is the maximum number of roles that can be inherited
	MaxInherits = 50

	// MaxProviders is the maximum number of providers per role
	MaxProviders = 5

	// MaxWorkflows is the maximum number of workflows per role
	MaxWorkflows = 5

	// MaxInheritanceDepth is the maximum depth of role inheritance chain
	MaxInheritanceDepth = 10
)

// validateRoleLimits validates that a role does not exceed configured limits.
// Returns an error describing the first limit violation found.
func validateRoleLimits(roleKey string, role *models.Role) error {
	// Check permissions limit
	permCount := len(role.Permissions.Allow) + len(role.Permissions.Deny)
	if permCount > models.MaxPermissions {
		return fmt.Errorf("role '%s' exceeds maximum permissions limit: %d > %d", roleKey, permCount, models.MaxPermissions)
	}

	// Check scopes limit
	if !role.Scopes.IsEmpty() {
		scopeCount := len(role.Scopes.Allow.Users) + len(role.Scopes.Allow.Groups) + len(role.Scopes.Allow.Domains)
		if scopeCount > MaxScopes {
			return fmt.Errorf("role '%s' exceeds maximum scopes limit: %d > %d", roleKey, scopeCount, MaxScopes)
		}
	}

	// Check inherits limit
	if len(role.Inherits) > MaxInherits {
		return fmt.Errorf("role '%s' exceeds maximum inherits limit: %d > %d", roleKey, len(role.Inherits), MaxInherits)
	}

	// Check providers limit
	if len(role.Providers) > MaxProviders {
		return fmt.Errorf("role '%s' exceeds maximum providers limit: %d > %d", roleKey, len(role.Providers), MaxProviders)
	}

	// Check workflows limit
	if len(role.Workflows) > MaxWorkflows {
		return fmt.Errorf("role '%s' exceeds maximum workflows limit: %d > %d", roleKey, len(role.Workflows), MaxWorkflows)
	}

	return nil
}

// LoadRoles loads roles from a file or URL
func (c *Config) LoadRoles() (map[string]models.Role, error) {
	vaultData, err := c.loadRolesVaultData()
	if err != nil {
		return nil, err
	}

	foundRoles := []*models.RoleDefinitions{}

	if len(vaultData) > 0 || len(c.Roles.Path) > 0 || c.Roles.URL != nil {
		importedRoles, err := loadDataFromSource(
			c.Roles.Path,
			c.Roles.URL,
			vaultData,
			models.RoleDefinitions{},
		)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to load roles data")
			return nil, fmt.Errorf("failed to load roles data: %w", err)
		}
		foundRoles = importedRoles
	}

	if len(foundRoles) == 0 {
		logrus.Warningln("No roles found from any source, loading defaults")
		foundRoles, err = environment.GetDefaultRoles(c.Environment.Platform)
		if err != nil {
			return nil, fmt.Errorf("failed to load default roles: %w", err)
		}
		logrus.Infoln("Loaded default roles:", len(foundRoles))
	}

	return c.ApplyRoles(foundRoles)
}

func (c *Config) ApplyRoles(foundRoles []*models.RoleDefinitions) (map[string]models.Role, error) {

	// Add roles defined directly in config
	c.mu.RLock()
	rolesLen := len(c.Roles.Definitions)
	if rolesLen > 0 {
		logrus.Debugln("Adding roles defined directly in config: ", rolesLen)
		defaultVersion := version.Must(version.NewVersion("1.0"))

		for roleKey, role := range c.Roles.Definitions {
			foundRoles = append(foundRoles, &models.RoleDefinitions{
				Version: defaultVersion,
				Roles:   map[string]models.Role{roleKey: role},
			})
		}
	}
	c.mu.RUnlock()

	defs := make(map[string]models.Role)
	logrus.Debugln("Processing loaded roles: ", len(foundRoles))

	for _, role := range foundRoles {

		for roleKey, r := range role.Roles {

			r.Identifier = roleKey

			if err := r.Validate(); err != nil {
				logrus.WithError(err).Errorln("Role definition validation failed")
				continue
			}

			if len(r.Name) == 0 {
				r.Name = roleKey
			}

			if !r.Enabled {
				logrus.Infoln("Role disabled:", roleKey)
				continue
			}

			if _, exists := defs[roleKey]; exists {
				logrus.Warningln("Duplicate role key found, skipping:", roleKey)
				continue
			}

			if r.Version == nil {
				r.Version = role.Version
			}

			// Validate role limits
			if err := validateRoleLimits(roleKey, &r); err != nil {
				logrus.WithError(err).Warnln("Role exceeds limits, skipping:", roleKey)
				continue
			}
			defs[roleKey] = r
		}
	}

	return defs, nil
}

// ListRoles searches for roles using the Bleve index. If no search request is provided,
// returns all roles. Access control is applied by the caller.
func (rc *RoleConfig) ListRoles(
	ctx context.Context,
	searchRequest *models.SearchRequest,
) ([]models.SearchResult[models.Role], error) {
	rc.mu.RLock()
	rolesIndex := rc.rolesIndex
	definitions := rc.Definitions
	rc.mu.RUnlock()

	if definitions == nil {
		return nil, fmt.Errorf("no roles defined")
	}

	// Convert map to slice for searching
	roles := make([]models.Role, 0, len(definitions))
	for roleId, role := range definitions {
		// Set the identifier so we can match search results
		role.Identifier = roleId
		roles = append(roles, role)
	}

	// If no search request, return all roles
	if searchRequest == nil || searchRequest.IsEmpty() {
		return models.ReturnSearchResults(roles), nil
	}

	// Check if search index is ready
	if rolesIndex != nil {
		// Use Bleve search
		return models.BleveListSearch(ctx, rolesIndex, func(a *search.DocumentMatch, b models.Role) bool {
			// Compare the document ID from Bleve with the role identifier
			return a.ID == b.Identifier
		}, roles, searchRequest)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []models.Role
	filterText := strings.ToLower(strings.Join(searchRequest.Terms, " "))
	limit := searchRequest.GetLimit()

	for _, role := range roles {
		if strings.Contains(strings.ToLower(role.Name), filterText) ||
			strings.Contains(strings.ToLower(role.Description), filterText) {
			filtered = append(filtered, role)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return models.ReturnSearchResults(filtered), nil
}

const providerReadinessTimeout = 5 * time.Minute

// awaitProviderReadiness blocks until every registered provider signals ready
// or the timeout expires. Uses each provider's Ready() channel so there is no
// polling — we get signalled the moment each provider finishes synchronising.
func (c *Config) awaitProviderReadiness() {
	c.mu.RLock()
	providers := make([]models.Provider, 0, len(c.providerInstances))
	for _, p := range c.providerInstances {
		providers = append(providers, p)
	}
	c.mu.RUnlock()

	if len(providers) == 0 {
		return
	}

	logrus.Infof("Waiting for %d provider(s) to become ready", len(providers))

	deadline := time.After(providerReadinessTimeout)
	for _, p := range providers {
		select {
		case <-p.Ready():
			// provider is ready
		case <-deadline:
			logrus.WithField("provider", p.GetIdentifier()).
				Error("Timed out waiting for provider readiness, role indexes may be incomplete")
			return
		}
	}

	logrus.Info("All providers ready, proceeding with role index build")
}

// anyProviderNotReady returns true if any of the named providers exist and
// have not yet completed their initial synchronization.
func (c *Config) anyProviderNotReady(providerNames []string) bool {
	for _, name := range providerNames {
		p, err := c.GetProviderByName(name)
		if err != nil || p == nil {
			continue
		}
		if !p.IsReady() {
			return true
		}
	}
	return false
}

func (c *Config) ReloadRoleIndexes() error {

	// Never compute indexs if we're not in server mode.
	// The CLI makes requests to the server to get role information so we don't need to compute indexes in the CLI. This also saves resources and prevents potential issues with concurrent access to the index from multiple CLI instances.
	if c.IsClient() {
		logrus.Debugln("Not running in server mode, skipping role index reload")
		return nil
	}

	// Block until all providers have finished loading their roles/permissions
	// so that inherited provider roles (ARNs, Azure built-ins, etc.) resolve.
	c.awaitProviderReadiness()

	// Create bleve index for roles
	if c.Roles.Definitions == nil {
		logrus.Debugln("No roles defined, skipping index creation")
		return nil
	}

	availableRoles := c.Roles.Definitions

	// Create index mapping with standard analyzer
	rolesMapping := bleve.NewIndexMapping()

	// Use standard analyzer which tokenizes on whitespace and most punctuation
	// We pre-tokenize at index time to create all necessary search terms
	rolesMapping.DefaultAnalyzer = "standard"

	rolesIndex, err := bleve.NewMemOnly(rolesMapping)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to create roles index")
		return fmt.Errorf("failed to create roles index: %w", err)
	}

	// Index roles
	for roleId, role := range availableRoles {

		// Before indexing the role, we first need to create a composite role
		// This will ensure that any inherited permissions are included in the indexing
		// This is important as a user might search for a permission that is only
		// included via inheritance.

		// No use provided so we can get the entire role. HOWEVER, if
		// a role is scoped then we won't resolve the entire role.
		// Just what is available to ALL users.
		compositeRole, err := c.GetCompositeRole(nil, &role)

		if err != nil {
			logrus.WithError(err).Errorf("Failed to create composite role for %s", roleId)
			continue
		}

		// Create a searchable document that flattens nested arrays for better indexing
		// Bleve needs flat string fields to properly index and search nested structures
		// Lowercase everything for case-insensitive search
		searchDoc := map[string]any{
			"name":           strings.ToLower(compositeRole.Name),
			"description":    strings.ToLower(compositeRole.Description),
			"identifier":     strings.ToLower(compositeRole.Identifier),
			"inherits":       strings.ToLower(strings.Join(role.Inherits, " ")), // Use original role's inherits for searchability
			"authenticators": strings.ToLower(strings.Join(compositeRole.Authenticators, " ")),
			"workflows":      strings.ToLower(strings.Join(compositeRole.Workflows, " ")),
			"providers":      strings.ToLower(strings.Join(compositeRole.Providers, " ")),
		}

		// Flatten operations from nested permissions structure
		var allOperations []string
		for _, stmt := range compositeRole.Permissions.Allow {
			allOperations = append(allOperations, stmt.Operations...)
		}
		for _, stmt := range compositeRole.Permissions.Deny {
			allOperations = append(allOperations, stmt.Operations...)
		}

		// Create searchable tokens from operations for better matching
		// This handles cases like "ec2:*", "ec2:describeInstances", "compute.instances.list"
		operationTokens := make([]string, 0, len(allOperations)*3)
		for _, op := range allOperations {
			// Add the original operation (lowercased)
			opLower := strings.ToLower(op)
			operationTokens = append(operationTokens, opLower)

			// For operations with colons, add both prefix and suffix parts
			// "ec2:describeInstances" -> also index "ec2" and "describeinstances"
			// "s3:GetObject" -> also index "s3" and "getobject"
			if colonIdx := strings.Index(opLower, ":"); colonIdx > 0 {
				operationTokens = append(operationTokens, opLower[:colonIdx]) // prefix before colon
				if colonIdx+1 < len(opLower) {
					suffix := opLower[colonIdx+1:]
					if suffix != "*" { // don't index the literal asterisk
						operationTokens = append(operationTokens, suffix) // part after colon
					}
				}
			}

			// For dot-separated operations, add intermediate parts too
			// "compute.instances.list" -> also index "compute", "compute.instances", "instances", "list"
			if strings.Contains(opLower, ".") {
				parts := strings.Split(opLower, ".")
				for i, part := range parts {
					operationTokens = append(operationTokens, part) // each part individually
					if i > 0 {
						// also add progressive combinations: "compute.instances"
						operationTokens = append(operationTokens, strings.Join(parts[:i+1], "."))
					}
				}
			}
		}
		searchDoc["operations"] = strings.Join(operationTokens, " ")

		if err := rolesIndex.Index(roleId, searchDoc); err != nil {
			logrus.WithError(err).Errorf("Failed to index role %s", roleId)
		}
	}

	c.mu.Lock()
	c.Roles.rolesIndex = rolesIndex
	c.mu.Unlock()

	return nil

}

func (c *Config) loadRolesVaultData() (string, error) {
	if len(c.Roles.Vault) == 0 {
		return "", nil
	}
	if !c.HasVault() {
		return "", fmt.Errorf("vault configuration is missing. Cannot load roles from vault")
	}

	logrus.Debugln("Loading roles from vault: ", c.Roles.Vault)
	data, err := c.GetVault().GetSecret(c.Roles.Vault)
	if err != nil {
		logrus.WithError(err).Errorln("Error loading roles from vault")
		return "", fmt.Errorf("failed to get secret from vault: %w", err)
	}

	logrus.Debugln("Loaded roles from vault: ", len(data), " bytes")
	return string(data), nil
}

func (c *Config) GetRoleByName(name string) (*models.Role, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.Roles.GetRoleByName(name)
}

func (c *Config) GetCompositeRoleByName(
	identity *models.Identity,
	roleName string,
) (*models.CompositeRole, error) {
	if len(roleName) == 0 {
		return nil, fmt.Errorf("cannot resolve composite role: role name is empty")
	}
	baseRole, err := c.GetRoleByName(roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role '%s': %w", roleName, err)
	}
	return c.GetCompositeRole(identity, baseRole)
}

func (c *Config) GetCompositeRole(
	identity *models.Identity,
	baseRole *models.Role,
	providers ...models.Provider,
) (*models.CompositeRole, error) {

	derivedRole, err := c.resolveCompositeRole(
		identity,
		baseRole,
		make(map[string]bool, 8),
		providers...,
	)
	if err != nil {
		return nil, err
	}

	return derivedRole, nil
}

// GetCompositeRole evaluates a role and resolves all inherited roles into a single composite role.
// Provider-prefixed items (inherits, permissions, resources, groups) are filtered to only include
// those matching the role's configured providers.
//
// The resolution process:
//  1. Detects cyclic inheritance to prevent infinite loops
//  2. Filters permissions/resources/groups by the role's allowed providers
//  3. Recursively resolves inherited roles (both local and provider roles)
//  4. Merges permissions with conflict resolution (parent overrides child)
//  5. Condenses permissions back to efficient format
//
// Returns an error if:
//   - baseRole is nil
//   - Cyclic inheritance is detected
//   - An inherited role cannot be resolved
func (c *Config) GetCompositeRoleForWorkflow(
	identity *models.Identity,
	baseRole *models.Role,
	workflowID string,
	providers ...models.Provider,
) (*models.CompositeRole, error) {

	if workflowID == "" {
		return nil, fmt.Errorf("cannot resolve composite role: workflow ID is empty")
	}

	if baseRole == nil {
		return nil, fmt.Errorf("cannot resolve composite role: workflow role is nil")
	}

	// Combine all components to create a unique identifier
	roleIdentifier := models.CompositeRoleWorkflowIdentifier(workflowID, baseRole, identity)

	derivedRole, err := c.getCompositeRoleForIdentity(
		roleIdentifier, identity, baseRole, providers...)

	if err != nil {
		logrus.WithError(err).Errorf("Failed to resolve composite role for workflow '%s'", workflowID)
		return nil, err
	}

	return derivedRole, nil
}

func (c *Config) GetCompositeRoleForIdentity(
	identity *models.Identity,
	baseRole *models.Role,
	providers ...models.Provider,
) (*models.CompositeRole, error) {

	if baseRole == nil {
		return nil, fmt.Errorf("cannot resolve composite role: base role is nil")
	}

	// Set an ephemeral identifier for this composite role (not persisted, just for caching/indexing)
	roleIdentifier := models.CompositeRoleWorkflowIdentifier("", baseRole, identity)

	derivedRole, err := c.getCompositeRoleForIdentity(
		roleIdentifier, identity, baseRole, providers...)

	if err != nil {
		logrus.WithError(err).Errorf("Failed to resolve composite role for identity '%s'", identity.GetMappableIdentifier())
		return nil, err
	}

	return derivedRole, nil
}

func (c *Config) getCompositeRoleForIdentity(
	roleIdentifier string,
	identity *models.Identity,
	baseRole *models.Role,
	providers ...models.Provider,
) (*models.CompositeRole, error) {

	// Pre-allocate visited map with reasonable capacity to reduce allocations
	resolvedRole, err := c.GetCompositeRole(identity, baseRole, providers...)

	if err != nil {
		return nil, fmt.Errorf("failed to resolve composite role for '%s': %w", baseRole.Name, err)
	}

	// Update identifier to make it unique for this composite role instance
	resolvedRole.SetUniqueIdentifierFromString(roleIdentifier)

	// Log the identifier change for debugging
	logrus.WithFields(logrus.Fields{
		"role":                baseRole.Name,
		"original_identifier": baseRole.Identifier,
		"new_identifier":      resolvedRole.GetUniqueIdentifier(),
	}).Debugln("Marked role as composite and updated identifier")

	return resolvedRole, nil

}

func (c *Config) resolveCompositeRoleByName(
	identity *models.Identity,
	roleName string,
	visited map[string]bool,
	providers ...models.Provider,
) (*models.CompositeRole, error) {
	baseRole, err := c.GetRoleByName(roleName)
	if err != nil {
		return nil, err
	}
	return c.resolveCompositeRole(identity, baseRole, visited, providers...)
}

// resolveCompositeRole recursively resolves a role and its inheritance chain.
// It uses a visited map to detect cycles and prevent infinite recursion.
//
// The algorithm ensures parent roles take precedence over child roles in conflicts:
//   - Parent Allow overrides Child Deny (for the same operation)
//   - Parent Deny overrides Child Allow (for the same operation)
//
// This "parent wins" model allows administrators to define base roles with specific
// permissions that cannot be overridden by inherited roles.
// resolveCompositeRole is the internal implementation. It works on models.Role
// throughout and wraps the result into a CompositeRole at the very end.
func (c *Config) resolveCompositeRole(
	identity *models.Identity,
	baseRole *models.Role,
	visited map[string]bool,
	providers ...models.Provider,
) (*models.CompositeRole, error) {

	if len(baseRole.Name) == 0 {
		return nil, fmt.Errorf("cannot resolve role with empty name")
	}

	log := logrus.WithField("role", baseRole.Name)

	if visited[baseRole.Name] {
		log.WithField("visited_chain", mapKeys(visited)).Debugln("Cyclic inheritance detected, breaking cycle")
		return nil, fmt.Errorf("cyclic inheritance detected in role: %s", baseRole.Name)
	}

	// Check inheritance depth limit
	if len(visited) >= MaxInheritanceDepth {
		log.WithFields(logrus.Fields{
			"depth":     len(visited),
			"max_depth": MaxInheritanceDepth,
		}).Debugln("Maximum inheritance depth exceeded")
		return nil, fmt.Errorf("role '%s' exceeds maximum inheritance depth: %d", baseRole.Name, MaxInheritanceDepth)
	}

	visited[baseRole.Name] = true
	defer delete(visited, baseRole.Name)

	// Check if the base role is applicable to the identity
	if identity != nil && !c.isRoleApplicableToIdentity(baseRole, identity) {
		return nil, fmt.Errorf("role '%s' not applicable to identity", baseRole.Name)
	}

	var identityInfo string
	if identity != nil {
		identityInfo = identity.GetMappableIdentifier()
	}

	logrus.WithFields(logrus.Fields{
		"role":      baseRole.Name,
		"providers": providers,
		"identity":  identityInfo,
	}).Debugln("Resolving composite role for role: ", baseRole.Identifier)

	// Build a provider filter from the explicitly-passed providers argument.
	// When providers are supplied, only provider-prefixed items matching one
	// of these providers are kept; non-prefixed items are always included.
	// When no providers are passed the role's own Providers list is used
	// (preserving the original behaviour).
	explicitProviderNames := providerIdentifiers(providers...)

	// Create composite role with provider-filtered permissions/resources/groups
	compositeRole := *baseRole
	if len(explicitProviderNames) > 0 {
		c.filterRoleByProviderNames(&compositeRole, explicitProviderNames)
	} else {
		c.filterRoleByProviders(&compositeRole)
	}

	// Pre-allocate with expected capacity to reduce allocations
	numInherits := len(baseRole.Inherits)
	if numInherits == 0 {
		// No inheritance to process, just resolve conflicts and return
		c.resolvePermissionConflicts(&compositeRole, providers...)
		c.expandPermissionsForProviders(&compositeRole, providers)
		return &models.CompositeRole{Role: compositeRole, Providers: baseRole.Providers}, nil
	}

	// Track whether this role inherits from thand roles (not just provider roles)
	hasThandRoleInheritance := false

	// Process inherited roles
	remainingInherits := make([]string, 0, numInherits)
	for _, inheritedRoleName := range baseRole.Inherits {
		// Check if this is a provider-prefixed role
		providerName, roleName, isProviderPrefixed := c.parseProviderPrefix(inheritedRoleName)

		if isProviderPrefixed {
			// Determine which provider list to check against.
			// If explicit providers were passed, use those; otherwise fall back
			// to the base role's own Providers list.
			allowedForInherit := baseRole.Providers
			if len(explicitProviderNames) > 0 {
				allowedForInherit = explicitProviderNames
			}

			// Must match one of the allowed providers
			if !slices.Contains(allowedForInherit, providerName) {
				log.WithFields(logrus.Fields{
					"inherited_role": inheritedRoleName,
					"provider":       providerName,
				}).Debugln("Inherited role provider not in allowed providers, skipping")
				continue
			}

			// Try to get as provider role
			providerRole := c.GetProviderRoleWithIdentity(identity, roleName, providerName)
			if providerRole != nil {
				if len(providerRole.Name) != 0 {
					remainingInherits = append(remainingInherits, providerRole.Name)
				} else if len(providerRole.ID) != 0 {
					remainingInherits = append(remainingInherits, providerRole.ID)
				}
				continue
			}
			// Provider role not found, skip
			log.WithFields(logrus.Fields{
				"provider": providerName,
				"role":     roleName,
			}).Debugln("Provider role not found, skipping")
			continue
		}

		// Try as provider role against allowed providers
		providersForLookup := baseRole.Providers
		if len(explicitProviderNames) > 0 {
			providersForLookup = explicitProviderNames
		}
		if len(providersForLookup) > 0 {
			providerRole := c.GetProviderRoleWithIdentity(identity, inheritedRoleName, providersForLookup...)
			if providerRole != nil {
				if len(providerRole.Name) != 0 {
					remainingInherits = append(remainingInherits, providerRole.Name)
				} else if len(providerRole.ID) != 0 {
					remainingInherits = append(remainingInherits, providerRole.ID)
				}
				continue
			}

			// Provider role not found. If any target provider is still
			// syncing, skip gracefully instead of falling through to the
			// thand role registry (which will never contain provider roles
			// like ARNs or Azure built-in names).
			if c.anyProviderNotReady(providersForLookup) {
				log.WithFields(logrus.Fields{
					"inherited_role": inheritedRoleName,
					"providers":      providersForLookup,
				}).Warn("Skipping inherited role — provider(s) still syncing")
				continue
			}
		}

		// Resolve as normal role
		inheritedRole, err := c.resolveInheritedRole(identity, inheritedRoleName, visited, providers...)
		if err != nil {
			// Skip roles not applicable to identity (expected behavior)
			if strings.Contains(err.Error(), "not applicable to identity") {
				log.WithField("inherited_role", inheritedRoleName).Debugln("Inherited role not applicable to identity, skipping")
				continue
			}
			return nil, fmt.Errorf("failed to resolve inherited role '%s' for role '%s': %w", inheritedRoleName, baseRole.Name, err)
		}

		// Merge inherited role into composite
		c.mergeRolePermissions(&compositeRole, inheritedRole, providers...)
		// Bubble up provider-role inherits (e.g. ARN managed policies) from
		// the resolved child so they propagate through the full chain.
		if len(inheritedRole.Inherits) > 0 {
			remainingInherits = append(remainingInherits, inheritedRole.Inherits...)
		}
		// Mark that we inherited from a thand role (not a provider role)
		hasThandRoleInheritance = true
	}

	compositeRole.Inherits = remainingInherits
	c.resolvePermissionConflicts(&compositeRole, providers...)
	c.expandPermissionsForProviders(&compositeRole, providers)

	// Validate composite role limits after merging
	if err := validateRoleLimits(compositeRole.Name, &compositeRole); err != nil {
		return nil, fmt.Errorf("composite role exceeds limits: %w", err)
	}

	return &models.CompositeRole{
		Role:      compositeRole,
		Providers: baseRole.Providers,
		Composite: hasThandRoleInheritance,
	}, nil
}

// providersSupportsWildcards returns true when every provider in the list supports
// wildcard permissions in its API (e.g. AWS, Azure). Returns true when the list is
// empty (no provider restriction). Returns false as soon as any provider reports
// SupportsWildcards=false (e.g. GCP, Okta), so that CondenseActions is skipped and
// individually-expanded permissions are kept in their concrete form.
func providersSupportsWildcards(providers []models.Provider) bool {
	for _, p := range providers {
		caps := p.GetCapabilities()
		if caps != nil && caps.Permissions != nil && !caps.Permissions.SupportsWildcards {
			return false
		}
	}
	return true
}

// expandPermissionsForProviders expands wildcard permissions in a role for each
// provider that does not support wildcard patterns in its API (e.g. GCP, Okta).
// Providers where SupportsWildcards is true (e.g. AWS, Azure) are skipped — their
// APIs accept wildcard patterns natively and no expansion is required.
func (c *Config) expandPermissionsForProviders(role *models.Role, providers []models.Provider) {
	for _, provider := range providers {
		caps := provider.GetCapabilities()
		if caps == nil || caps.Permissions == nil || caps.Permissions.SupportsWildcards {
			continue
		}
		models.ExpandWildcardPermissionsForProvider(provider, role)
	}
}

// resolveInheritedRole handles scope checking before resolving an inherited role.
// It returns *models.Role (the embedded Role from CompositeRole) so that it can
// be passed directly to mergeRole.
func (c *Config) resolveInheritedRole(identity *models.Identity, roleName string, visited map[string]bool, providers ...models.Provider) (*models.Role, error) {
	role, err := c.GetRoleByName(roleName)
	if err != nil {
		return nil, fmt.Errorf("inherited role '%s' not found: %w", roleName, err)
	}
	if !c.isRoleApplicableToIdentity(role, identity) {
		return nil, fmt.Errorf("inherited role '%s' not applicable to identity", roleName)
	}
	composite, err := c.resolveCompositeRoleByName(identity, roleName, visited, providers...)
	if err != nil {
		return nil, err
	}
	return &composite.Role, nil
}

// isRoleApplicableToIdentity checks if a role's scopes allow it to be applied to the identity.
//
// Scope evaluation order (SECURITY CRITICAL):
//  1. If role has no scopes defined → open to all (return true)
//  2. If identity is nil → cannot match any scope (return false)
//  3. Check DENY scopes first → if identity matches any deny scope (return false)
//  4. Check ALLOW scopes → if identity matches any allow scope (return true)
//  5. No match found → deny by default (return false)
//
// Deny always takes precedence over Allow. This is critical for security as it ensures
// explicit denials cannot be bypassed by also adding the identity to an allow list.
//
// Matching is case-insensitive for all scope types (users, groups, domains).
func (c *Config) isRoleApplicableToIdentity(role *models.Role, identity *models.Identity) bool {
	log := logrus.WithFields(logrus.Fields{
		"role": role.Name,
	})

	// No scopes means open to all
	if role.Scopes.IsEmpty() {
		log.Debugln("Role has no scopes defined, applicable to all identities")
		return true
	}

	// Nil identity cannot match any scopes
	if identity == nil {
		log.Debugln("Identity is nil, cannot match any scopes")
		return false
	}

	// Add identity context for logging
	log = log.WithField("identity", identity.GetId())

	// SECURITY: Check DENY scopes first - deny always takes precedence over allow
	if c.identityMatchesScopeIdentities(identity, &role.Scopes.Deny) {
		log.Debugln("Identity matched DENY scope, blocking role access")
		return false
	}

	// Check if there are any allow scopes defined
	hasAnyAllowScope := len(role.Scopes.Allow.Users) > 0 ||
		len(role.Scopes.Allow.Groups) > 0 ||
		len(role.Scopes.Allow.Domains) > 0

	// If no allow scopes are defined (but deny scopes exist and didn't match), allow access
	if !hasAnyAllowScope {
		log.Debugln("No allow scopes defined and identity not in deny list, allowing access")
		return true
	}

	// Check ALLOW scopes
	if c.identityMatchesScopeIdentities(identity, &role.Scopes.Allow) {
		log.Debugln("Identity matched ALLOW scope, granting role access")
		return true
	}

	// Scopes defined but no match found - default deny
	log.Debugln("Identity did not match any allow scope, denying role access")
	return false
}

// identityMatchesScopeIdentities checks if an identity matches any of the scope identities.
// This is a helper function used by isRoleApplicableToIdentity for both allow and deny checks.
//
// Matching rules:
//   - For user identities: matches against Users list (by identity, email, username, or ID),
//     Groups list (by user's group memberships), and Domains list (by user's domain)
//   - For group identities: matches against Groups list (by group name or ID)
//
// All string comparisons are case-insensitive.
func (c *Config) identityMatchesScopeIdentities(identity *models.Identity, scopes *models.ScopeIdentities) bool {
	if scopes == nil || scopes.IsEmpty() {
		return false
	}

	// Check user-related scopes
	if identity.IsUser() {
		user := identity.GetUser()
		if user == nil {
			return false
		}

		// Check user scopes (identity, email, username, ID, name)
		if len(scopes.Users) > 0 {
			userIdentity := user.GetIdentity()
			for _, allowed := range scopes.Users {
				if strings.EqualFold(allowed, userIdentity) ||
					strings.EqualFold(allowed, user.Email) ||
					strings.EqualFold(allowed, user.Username) ||
					strings.EqualFold(allowed, user.ID) ||
					strings.EqualFold(allowed, user.Name) {
					return true
				}
			}
		}

		// Check if user belongs to any scoped groups
		if len(scopes.Groups) > 0 {
			userGroups := user.GetGroups()
			for _, userGroup := range userGroups {
				for _, scopeGroup := range scopes.Groups {
					if strings.EqualFold(scopeGroup, userGroup) {
						return true
					}
				}
			}
		}

		// Check domain scopes
		if len(scopes.Domains) > 0 {
			userDomain := user.GetDomain()
			for _, scopeDomain := range scopes.Domains {
				if strings.EqualFold(scopeDomain, userDomain) {
					return true
				}
			}
		}
	}

	// Check group scopes for group identities
	if identity.IsGroup() && len(scopes.Groups) > 0 {
		group := identity.GetGroup()
		if group != nil {
			groupName := group.GetName()
			groupID := group.GetID()
			for _, scopeGroup := range scopes.Groups {
				if strings.EqualFold(scopeGroup, groupName) || strings.EqualFold(scopeGroup, groupID) {
					return true
				}
			}
		}
	}

	return false
}

// filterRoleByProviders filters all provider-prefixed items in a role to only include
// those matching the role's configured providers.
func (c *Config) filterRoleByProviders(role *models.Role) {
	role.Permissions.Allow = c.filterStatementsListByProvider(role.Permissions.Allow, role.Providers)
	role.Permissions.Deny = c.filterStatementsListByProvider(role.Permissions.Deny, role.Providers)
}

// filterRoleByProviderNames filters all provider-prefixed items in a role using
// the given explicit provider name list instead of the role's own Providers field.
// This is used when callers pass specific models.Provider objects to restrict
// evaluation to only those providers.
func (c *Config) filterRoleByProviderNames(role *models.Role, providerNames []string) {
	role.Permissions.Allow = c.filterStatementsListByProvider(role.Permissions.Allow, providerNames)
	role.Permissions.Deny = c.filterStatementsListByProvider(role.Permissions.Deny, providerNames)
}

// mergeRolePermissions merges an inherited role into the composite role.
// Parent (composite) takes precedence over child (inherited) in conflicts:
// - Parent Allow overrides Child Deny
// - Parent Deny overrides Child Allow
func (c *Config) mergeRolePermissions(composite *models.Role, inherited *models.Role, providers ...models.Provider) {
	// Filter inherited items by composite's providers
	inheritedAllowPerms := c.filterStatementsListByProvider(inherited.Permissions.Allow, composite.Providers)
	inheritedDenyPerms := c.filterStatementsListByProvider(inherited.Permissions.Deny, composite.Providers)

	// Merge permissions with conflict resolution
	c.mergePermissionsWithConflictResolution(composite, inheritedAllowPerms, inheritedDenyPerms, providers...)
}

// normalizeStatements expands all statement operations and creates a map by operation.
// Returns a map where key is operation and value is the set of targets associated with that operation.
// normalizeStatements separates statements into two groups:
// 1. Normalized map (for statements WITHOUT conditions) - can be merged and deduplicated
// 2. Preserved statements (for statements WITH conditions) - kept as complete units
// This ensures conditions are not lost during merge operations.
func normalizeStatements(stmts models.RoleStatements) (map[string]map[string]bool, models.RoleStatements) {
	result := make(map[string]map[string]bool)
	preservedStmts := make(models.RoleStatements, 0)

	for _, stmt := range stmts {
		// Statements WITH conditions are preserved as-is
		if len(stmt.Conditions) > 0 {
			preservedStmts = append(preservedStmts, stmt)
			continue
		}

		// Statements WITHOUT conditions are normalized (existing logic)
		for _, op := range stmt.Operations {
			for _, expandedOp := range models.ExpandCondensedActions(op) {
				if result[expandedOp] == nil {
					result[expandedOp] = make(map[string]bool)
				}
				// Add all targets for this operation
				if len(stmt.Targets) == 0 {
					// No targets means "all targets" - use empty string as marker
					result[expandedOp][""] = true
				} else {
					for _, target := range stmt.Targets {
						result[expandedOp][target] = true
					}
				}
			}
		}
	}

	return result, preservedStmts
}

// deduplicatePreservedStatements removes conflicts between allow and deny statements.
// If a statement appears in both, the allow is removed and deny is kept (deny wins).
// Statements are compared by checking equality of operations, targets, and conditions.
func deduplicatePreservedStatements(allow, deny models.RoleStatements) (models.RoleStatements, models.RoleStatements) {
	// Track conflicting statement indices in allow
	conflictingAllowIndices := make(map[int]bool)

	// Find conflicts: check each allow statement against all deny statements
	for allowIdx, allowStmt := range allow {
		for _, denyStmt := range deny {
			if statementsEqual(allowStmt, denyStmt) {
				conflictingAllowIndices[allowIdx] = true
				break // No need to check further deny statements for this allow
			}
		}
	}

	// Filter out conflicting statements from allow
	filteredAllow := make(models.RoleStatements, 0, len(allow)-len(conflictingAllowIndices))
	for i, stmt := range allow {
		if !conflictingAllowIndices[i] {
			filteredAllow = append(filteredAllow, stmt)
		}
	}

	// Keep all deny statements (deny wins in conflicts)
	return filteredAllow, deny
}

// statementsEqual checks if two statements are equal by comparing their operations, targets, and conditions.
// String slices are compared in sorted order to ensure consistent comparison.
func statementsEqual(a, b models.Statement) bool {
	// Compare operations (sorted)
	if !stringSlicesEqual(a.Operations, b.Operations) {
		return false
	}

	// Compare targets (sorted)
	if !stringSlicesEqual(a.Targets, b.Targets) {
		return false
	}

	// Normalize nil vs empty for conditions comparison
	aConditions := a.Conditions
	bConditions := b.Conditions
	if len(aConditions) == 0 {
		aConditions = nil
	}
	if len(bConditions) == 0 {
		bConditions = nil
	}

	return reflect.DeepEqual(aConditions, bConditions)
}

// stringSlicesEqual compares two string slices for equality after sorting.
// Returns true if both slices contain the same elements in any order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	if len(a) == 0 {
		return true
	}

	// Create sorted copies to avoid mutating original slices
	aSorted := make([]string, len(a))
	bSorted := make([]string, len(b))
	copy(aSorted, a)
	copy(bSorted, b)
	sort.Strings(aSorted)
	sort.Strings(bSorted)

	for i := range aSorted {
		if aSorted[i] != bSorted[i] {
			return false
		}
	}

	return true
}

// rebuildStatementsFromNormalized converts the normalized operation->targets map back to statements.
// Groups operations by their target sets for more efficient statement representation.
// rebuildStatementsFromNormalized reconstructs Statement objects from normalized data
// and appends any preserved statements (those with conditions) at the end.
func rebuildStatementsFromNormalized(normalized map[string]map[string]bool, preserved models.RoleStatements, supportsWildcards bool) models.RoleStatements {
	// Return nil if both inputs are empty
	if len(normalized) == 0 && len(preserved) == 0 {
		return nil
	}

	result := make(models.RoleStatements, 0)

	if len(normalized) > 0 {
		// Group operations by their target key (sorted targets joined)
		targetGroupToOps := make(map[string][]string)
		targetGroupToTargets := make(map[string][]string)

		for op, targets := range normalized {
			targetList := mapKeys(targets)
			sort.Strings(targetList)
			targetKey := strings.Join(targetList, "|")

			if _, exists := targetGroupToOps[targetKey]; !exists {
				targetGroupToOps[targetKey] = []string{}
				targetGroupToTargets[targetKey] = targetList
			}
			targetGroupToOps[targetKey] = append(targetGroupToOps[targetKey], op)
		}

		// Build statements, one per unique target set
		for targetKey, ops := range targetGroupToOps {
			var condensedOps []string
			if supportsWildcards {
				condensedOps = models.CondenseActions(ops)
			} else {
				sort.Strings(ops)
				condensedOps = ops
			}
			targets := targetGroupToTargets[targetKey]

			// Filter out empty string marker (means "all targets")
			var finalTargets []string
			for _, t := range targets {
				if t != "" {
					finalTargets = append(finalTargets, t)
				}
			}

			result = append(result, models.Statement{
				Operations: condensedOps,
				Targets:    finalTargets,
			})
		}
	}

	// Append preserved conditioned statements at the end
	result = append(result, preserved...)

	return result
}

// mergePermissionsWithConflictResolution merges permissions with proper conflict resolution.
// Parent Allow overrides Child Deny, Parent Deny overrides Child Allow.
// This function preserves both operations AND targets during merging.
func (c *Config) mergePermissionsWithConflictResolution(composite *models.Role, childAllow, childDeny models.RoleStatements, providers ...models.Provider) {
	// Separate normalized and preserved statements
	parentAllowNorm, parentAllowPreserved := normalizeStatements(composite.Permissions.Allow)
	parentDenyNorm, parentDenyPreserved := normalizeStatements(composite.Permissions.Deny)
	childAllowNorm, childAllowPreserved := normalizeStatements(childAllow)
	childDenyNorm, childDenyPreserved := normalizeStatements(childDeny)

	finalAllowNorm := make(map[string]map[string]bool)
	finalDenyNorm := make(map[string]map[string]bool)

	// Collect parent wildcards for subsumption checking
	parentAllowWildcards := make(map[string]bool)
	parentDenyWildcards := make(map[string]bool)
	for op := range parentAllowNorm {
		if before, ok := strings.CutSuffix(op, ":*"); ok {
			parentAllowWildcards[before] = true
		}
	}
	for op := range parentDenyNorm {
		if before, ok := strings.CutSuffix(op, ":*"); ok {
			parentDenyWildcards[before] = true
		}
	}

	// Start with child permissions, but skip operations subsumed by parent wildcards
	for op, targets := range childAllowNorm {
		// Skip if subsumed by parent allow wildcard
		if isOperationSubsumedByWildcard(op, parentAllowWildcards) {
			continue
		}
		if finalAllowNorm[op] == nil {
			finalAllowNorm[op] = make(map[string]bool)
		}
		for target := range targets {
			finalAllowNorm[op][target] = true
		}
	}
	for op, targets := range childDenyNorm {
		// Skip if subsumed by parent deny wildcard
		if isOperationSubsumedByWildcard(op, parentDenyWildcards) {
			continue
		}
		if finalDenyNorm[op] == nil {
			finalDenyNorm[op] = make(map[string]bool)
		}
		for target := range targets {
			finalDenyNorm[op][target] = true
		}
	}

	// Parent Allow overrides Child Deny (for matching operations)
	for op, targets := range parentAllowNorm {
		delete(finalDenyNorm, op) // Remove from deny if child denied it
		// Also remove child denies subsumed by this parent allow wildcard
		if before, ok := strings.CutSuffix(op, ":*"); ok {
			prefix := before
			for childOp := range finalDenyNorm {
				if strings.HasPrefix(childOp, prefix+":") && childOp != op {
					delete(finalDenyNorm, childOp)
				}
			}
		}
		if finalAllowNorm[op] == nil {
			finalAllowNorm[op] = make(map[string]bool)
		}
		// Merge parent targets into allow
		for target := range targets {
			finalAllowNorm[op][target] = true
		}
	}

	// Parent Deny overrides Child Allow (for matching operations)
	for op, targets := range parentDenyNorm {
		delete(finalAllowNorm, op) // Remove from allow if child allowed it
		// Also remove child allows subsumed by this parent deny wildcard
		if before, ok := strings.CutSuffix(op, ":*"); ok {
			prefix := before
			for childOp := range finalAllowNorm {
				if strings.HasPrefix(childOp, prefix+":") && childOp != op {
					delete(finalAllowNorm, childOp)
				}
			}
		}
		if finalDenyNorm[op] == nil {
			finalDenyNorm[op] = make(map[string]bool)
		}
		// Merge parent targets into deny
		for target := range targets {
			finalDenyNorm[op][target] = true
		}
	}

	// Merge preserved statements: parent wins (parent conditions override child)
	finalAllowPreserved := append(childAllowPreserved, parentAllowPreserved...)
	finalDenyPreserved := append(childDenyPreserved, parentDenyPreserved...)

	// Rebuild statements from normalized form with preserved statements.
	// Skip CondenseActions for providers that do not support wildcards so that
	// individually expanded permissions are not re-condensed back into patterns.
	supportsWildcards := providersSupportsWildcards(providers)
	composite.Permissions.Allow = rebuildStatementsFromNormalized(finalAllowNorm, finalAllowPreserved, supportsWildcards)
	composite.Permissions.Deny = rebuildStatementsFromNormalized(finalDenyNorm, finalDenyPreserved, supportsWildcards)
}

// isOperationSubsumedByWildcard checks if an operation is covered by a wildcard
func isOperationSubsumedByWildcard(op string, wildcards map[string]bool) bool {
	for prefix := range wildcards {
		if strings.HasPrefix(op, prefix+":") && op != prefix+":*" {
			return true
		}
	}
	return false
}

// filterStatementsListByProvider filters statements based on allowed providers
func (c *Config) filterStatementsListByProvider(stmts models.RoleStatements, allowedProviders []string) models.RoleStatements {
	if len(stmts) == 0 {
		return nil
	}
	if len(allowedProviders) == 0 {
		return stmts
	}

	result := make(models.RoleStatements, 0, len(stmts))
	for _, stmt := range stmts {
		// Filter operations by provider
		filteredOps := c.filterByProvider(stmt.Operations, allowedProviders)
		if len(filteredOps) == 0 {
			continue // Skip statements with no matching operations
		}

		// Filter targets by provider
		filteredTargets := c.filterByProvider(stmt.Targets, allowedProviders)

		result = append(result, models.Statement{
			Operations: filteredOps,
			Targets:    filteredTargets,
			Conditions: stmt.Conditions,
		})
	}
	return result
}

// resolvePermissionConflicts resolves Allow/Deny conflicts within a SINGLE role.
//
// IMPORTANT: This differs from inheritance conflict resolution (see mergePermissionsWithConflictResolution).
//
// Conflict Resolution Rules:
//   - When the same operation appears in BOTH Allow AND Deny within a single role,
//     it is removed from BOTH lists (effective neutral/no-op for that operation).
//   - This "remove from both" behavior prevents ambiguous permission states.
//   - Targets are NOT considered during conflict detection - only operations are compared.
//
// Rationale:
//   - A role author who explicitly allows AND denies the same operation has created
//     a logical contradiction. Removing from both is the safest resolution.
//   - This differs from INHERITANCE where parent permissions take precedence over child.
//
// Example:
//
//	Allow: ["s3:GetObject", "s3:PutObject"]  +  Deny: ["s3:PutObject", "s3:DeleteObject"]
//	Result: Allow: ["s3:GetObject"]  +  Deny: ["s3:DeleteObject"]
//	(s3:PutObject removed from both due to conflict)
func (c *Config) resolvePermissionConflicts(role *models.Role, providers ...models.Provider) {
	// Separate normalized and preserved statements
	allowNorm, allowPreserved := normalizeStatements(role.Permissions.Allow)
	denyNorm, denyPreserved := normalizeStatements(role.Permissions.Deny)

	// Remove conflicts in normalized statements: deny wins (remove operation from both allow and deny)
	for op := range denyNorm {
		if _, exists := allowNorm[op]; exists {
			delete(allowNorm, op)
			delete(denyNorm, op)
		}
	}

	// Preserved statements don't conflict with normalized ones (conditions make them distinct)
	// They could conflict with each other if identical - remove both
	dedupedAllow, dedupedDeny := deduplicatePreservedStatements(allowPreserved, denyPreserved)

	// Rebuild statements, skipping CondenseActions for providers that do not support wildcards.
	supportsWildcards := providersSupportsWildcards(providers)
	role.Permissions.Allow = rebuildStatementsFromNormalized(allowNorm, dedupedAllow, supportsWildcards)
	role.Permissions.Deny = rebuildStatementsFromNormalized(denyNorm, dedupedDeny, supportsWildcards)
}

// parseProviderPrefix checks if a spec has a provider prefix (e.g., "gcp-prod:permission").
// Returns the provider name, the remainder, and whether it matched a known provider.
// Checks both exact provider names and provider engine types.
// Used for inheritance resolution where engine type matching is desired.
// Note: This function assumes the caller already holds c.mu.RLock()
func (c *Config) parseProviderPrefix(spec string) (providerName, remainder string, isProvider bool) {
	colonIdx := strings.Index(spec, ":")
	if colonIdx <= 0 || colonIdx >= len(spec)-1 {
		return "", spec, false
	}

	prefix := spec[:colonIdx]
	suffix := spec[colonIdx+1:]

	// Check if prefix is a known provider by exact name (direct map access)
	if _, exists := c.Providers.Definitions[prefix]; exists {
		return prefix, suffix, true
	}
	// Check if prefix is a provider engine type
	for foundName, provider := range c.Providers.Definitions {
		if strings.Compare(provider.Provider, prefix) == 0 {
			return foundName, suffix, true
		}
	}

	return "", spec, false
}

// filterByProvider filters items to only include those without a provider prefix,
// or those with a provider prefix matching one of the allowed providers.
// When an item has a matching provider prefix, the prefix is stripped from the result.
//
// Behavior:
//   - Items without provider prefix: included as-is
//   - Items with matching provider prefix: included with prefix stripped
//   - Items with non-matching provider prefix: excluded
func (c *Config) filterByProvider(items []string, allowedProviders []string) []string {
	if len(items) == 0 {
		return nil
	}
	if len(allowedProviders) == 0 {
		return items
	}

	// Build a set of allowed providers for O(1) lookup
	allowedSet := make(map[string]struct{}, len(allowedProviders))
	for _, p := range allowedProviders {
		allowedSet[p] = struct{}{}
	}

	result := make([]string, 0, len(items))
	for _, item := range items {
		providerName, remainder, isProvider := c.parseProviderPrefix(item)
		if !isProvider {
			// No provider prefix - include as-is
			result = append(result, item)
		} else if _, ok := allowedSet[providerName]; ok {
			// Has matching provider prefix - include with prefix stripped
			result = append(result, remainder)
		}
		// else: has provider prefix but doesn't match - exclude
	}
	return result
}

// Convenience aliases for moved functions. The canonical implementations
// now live in the models package (condense.go) so they can be shared by
// both the config layer (role composition) and the provider RBAC layer
// (permission validation).
var (
	condenseActions         = models.CondenseActions
	expandCondensedActions  = models.ExpandCondensedActions
	isCondensablePermission = models.IsCondensablePermission
)

// providerIdentifiers builds a deduplicated list of provider names from the
// given Provider objects. For each provider it includes both the unique
// identifier (GetIdentifier, e.g. "aws-prod") and the engine type
// (GetProvider, e.g. "aws") so that parseProviderPrefix matching works
// correctly against both forms.
// Returns nil when no providers are supplied (caller should treat this as
// "no provider filter — evaluate everything").
func providerIdentifiers(providers ...models.Provider) []string {
	if len(providers) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(providers)*2)
	result := make([]string, 0, len(providers)*2)
	for _, p := range providers {
		if p == nil {
			continue
		}
		for _, id := range []string{p.GetIdentifier(), p.GetProvider()} {
			if len(id) == 0 {
				continue
			}
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				result = append(result, id)
			}
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

// mapKeys returns the keys of a map as a sorted slice.
// Returns nil for empty or nil maps.
func mapKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	result := make([]string, 0, len(m))
	for k := range m {
		result = append(result, k)
	}
	sort.Strings(result)
	return result
}

func (r *RoleConfig) GetRoleByName(name string) (*models.Role, error) {
	if role, exists := r.Definitions[name]; exists {
		// Ensure the role has a name (use the key if not set)
		if len(role.Name) == 0 {
			role.Name = name
		}
		return &role, nil
	}
	return nil, fmt.Errorf("role not found: %s", name)
}
