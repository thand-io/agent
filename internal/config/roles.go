// Package config handles configuration loading and role resolution for the agent.
// This file implements role inheritance, permission merging, and provider-based filtering.
package config

import (
	"fmt"
	"hash/fnv"
	"reflect"
	"slices"
	"sort"
	"strings"

	"github.com/blevesearch/bleve/v2"
	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config/environment"
	"github.com/thand-io/agent/internal/models"
)

// Hard limits for roles configuration to prevent resource exhaustion
const (
	// MaxPermissions is the maximum number of permissions (allow + deny) per role
	MaxPermissions = 500

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
	if permCount > MaxPermissions {
		return fmt.Errorf("role '%s' exceeds maximum permissions limit: %d > %d", roleKey, permCount, MaxPermissions)
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

		if err := role.Validate(); err != nil {
			logrus.WithError(err).Errorln("Role definition validation failed")
			continue
		}

		for roleKey, r := range role.Roles {
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

			r.Identifier = common.ConvertToSnakeCase(roleKey)

			if len(r.Name) == 0 {
				r.Name = roleKey
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

func (c *Config) ReloadRoleIndexes() error {

	// Create bleve index for roles
	if c.Roles.Definitions == nil {
		logrus.Debugln("No roles defined, skipping index creation")
		return nil
	}

	availableRoles := c.Roles.Definitions

	rolesMapping := bleve.NewIndexMapping()
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
			return fmt.Errorf("failed to create composite role for %s: %v", roleId, err)
		}

		if err := rolesIndex.Index(roleId, compositeRole); err != nil {
			logrus.WithError(err).Errorf("Failed to index role %s", roleId)
			return fmt.Errorf("failed to index role %s: %v", roleId, err)
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
func (c *Config) GetCompositeRole(identity *models.Identity, baseRole *models.Role) (*models.Role, error) {
	if baseRole == nil {
		return nil, fmt.Errorf("cannot resolve composite role: base role is nil")
	}
	// Pre-allocate visited map with reasonable capacity to reduce allocations
	return c.resolveCompositeRole(identity, baseRole, make(map[string]bool, 8))
}

func (c *Config) GetCompositeRoleByName(identity *models.Identity, roleName string) (*models.Role, error) {
	if len(roleName) == 0 {
		return nil, fmt.Errorf("cannot resolve composite role: role name is empty")
	}
	baseRole, err := c.GetRoleByName(roleName)
	if err != nil {
		return nil, fmt.Errorf("failed to get role '%s': %w", roleName, err)
	}
	return c.resolveCompositeRole(identity, baseRole, make(map[string]bool, 8))
}

func (c *Config) resolveCompositeRoleByName(identity *models.Identity, roleName string, visited map[string]bool) (*models.Role, error) {
	baseRole, err := c.GetRoleByName(roleName)
	if err != nil {
		return nil, err
	}
	return c.resolveCompositeRole(identity, baseRole, visited)
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
func (c *Config) resolveCompositeRole(identity *models.Identity, baseRole *models.Role, visited map[string]bool) (*models.Role, error) {

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

	log.Debugln("Resolving composite role")

	// Create composite role with provider-filtered permissions/resources/groups
	compositeRole := *baseRole
	c.filterRoleByProviders(&compositeRole)

	// Pre-allocate with expected capacity to reduce allocations
	numInherits := len(baseRole.Inherits)
	if numInherits == 0 {
		// No inheritance to process, just resolve conflicts and return
		c.resolvePermissionConflicts(&compositeRole)
		return &compositeRole, nil
	}

	// Track whether this role inherits from thand roles (not just provider roles)
	hasThandRoleInheritance := false

	// Process inherited roles
	remainingInherits := make([]string, 0, numInherits)
	for _, inheritedRoleName := range baseRole.Inherits {
		// Check if this is a provider-prefixed role
		providerName, roleName, isProviderPrefixed := c.parseProviderPrefix(inheritedRoleName)

		if isProviderPrefixed {
			// Must match one of the base role's providers
			if !slices.Contains(baseRole.Providers, providerName) {
				log.WithFields(logrus.Fields{
					"inherited_role": inheritedRoleName,
					"provider":       providerName,
				}).Debugln("Inherited role provider not in base role's providers, skipping")
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

		// Try as provider role against base role's providers
		if len(baseRole.Providers) > 0 {
			providerRole := c.GetProviderRoleWithIdentity(identity, inheritedRoleName, baseRole.Providers...)
			if providerRole != nil {
				if len(providerRole.Name) != 0 {
					remainingInherits = append(remainingInherits, providerRole.Name)
				} else if len(providerRole.ID) != 0 {
					remainingInherits = append(remainingInherits, providerRole.ID)
				}
				continue
			}
		}

		// Resolve as normal role
		inheritedRole, err := c.resolveInheritedRole(identity, inheritedRoleName, visited)
		if err != nil {
			// Skip roles not applicable to identity (expected behavior)
			if strings.Contains(err.Error(), "not applicable to identity") {
				log.WithField("inherited_role", inheritedRoleName).Debugln("Inherited role not applicable to identity, skipping")
				continue
			}
			return nil, fmt.Errorf("failed to resolve inherited role '%s' for role '%s': %w", inheritedRoleName, baseRole.Name, err)
		}

		// Merge inherited role into composite
		c.mergeRole(&compositeRole, inheritedRole)
		// Mark that we inherited from a thand role (not a provider role)
		hasThandRoleInheritance = true
	}

	compositeRole.Inherits = remainingInherits
	c.resolvePermissionConflicts(&compositeRole)

	// Mark as composite and update identifier if it inherited thand roles
	if hasThandRoleInheritance {
		compositeRole.Composite = true
		// Extract user from identity (handles nil identity gracefully)
		var user *models.User
		if identity != nil {
			user = identity.GetUser()
		}

		// Create a unique identifier hash based on role identifier and user context
		if user == nil {
			// create unknown identifier for nil user
			user = &models.User{}
		}

		userIdentity := user.GetIdentity()

		// Build the composite identifier
		versionStr := "1.0.0"
		if baseRole.Version != nil {
			versionStr = baseRole.Version.String()
		}

		// Combine all components to create a unique identifier
		composite := fmt.Sprintf("%s:%s:%s:%s", baseRole.Identifier, versionStr, baseRole.Name, userIdentity)

		// Create FNV-1a hash (non-cryptographic, fast, 6 hex chars)
		h := fnv.New32a()
		h.Write([]byte(composite))

		// Conform to snake_case for consistency
		newIdentifier := fmt.Sprintf("%s_%06x", baseRole.GetIdentifier(), h.Sum32()&0xFFFFFF)

		// Update identifier to make it unique for this composite role instance
		compositeRole.Identifier = newIdentifier

		// Log the identifier change for debugging
		log.WithFields(logrus.Fields{
			"original_identifier": baseRole.Identifier,
			"new_identifier":      compositeRole.Identifier,
		}).Debugln("Marked role as composite and updated identifier")
	}

	// Validate composite role limits after merging
	if err := validateRoleLimits(compositeRole.Name, &compositeRole); err != nil {
		return nil, fmt.Errorf("composite role exceeds limits: %w", err)
	}

	return &compositeRole, nil
}

// resolveInheritedRole handles scope checking before resolving an inherited role.
func (c *Config) resolveInheritedRole(identity *models.Identity, roleName string, visited map[string]bool) (*models.Role, error) {
	role, err := c.GetRoleByName(roleName)
	if err != nil {
		return nil, fmt.Errorf("inherited role '%s' not found: %w", roleName, err)
	}
	if !c.isRoleApplicableToIdentity(role, identity) {
		return nil, fmt.Errorf("inherited role '%s' not applicable to identity", roleName)
	}
	return c.resolveCompositeRoleByName(identity, roleName, visited)
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

		// Check user scopes (identity, email, username, ID)
		if len(scopes.Users) > 0 {
			userIdentity := user.GetIdentity()
			for _, scopeUser := range scopes.Users {
				if strings.EqualFold(scopeUser, userIdentity) ||
					strings.EqualFold(scopeUser, user.Email) ||
					strings.EqualFold(scopeUser, user.Username) ||
					strings.EqualFold(scopeUser, user.ID) {
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

// mergeRole merges an inherited role into the composite role.
// Parent (composite) takes precedence over child (inherited) in conflicts:
// - Parent Allow overrides Child Deny
// - Parent Deny overrides Child Allow
func (c *Config) mergeRole(composite *models.Role, inherited *models.Role) {
	// Filter inherited items by composite's providers
	inheritedAllowPerms := c.filterStatementsListByProvider(inherited.Permissions.Allow, composite.Providers)
	inheritedDenyPerms := c.filterStatementsListByProvider(inherited.Permissions.Deny, composite.Providers)

	// Merge permissions with conflict resolution
	c.mergePermissionsWithConflictResolution(composite, inheritedAllowPerms, inheritedDenyPerms)
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
			for _, expandedOp := range expandCondensedActions(op) {
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
func rebuildStatementsFromNormalized(normalized map[string]map[string]bool, preserved models.RoleStatements) models.RoleStatements {
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
			condensedOps := condenseActions(ops)
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
func (c *Config) mergePermissionsWithConflictResolution(composite *models.Role, childAllow, childDeny models.RoleStatements) {
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
		if strings.HasSuffix(op, ":*") {
			parentAllowWildcards[strings.TrimSuffix(op, ":*")] = true
		}
	}
	for op := range parentDenyNorm {
		if strings.HasSuffix(op, ":*") {
			parentDenyWildcards[strings.TrimSuffix(op, ":*")] = true
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
		if strings.HasSuffix(op, ":*") {
			prefix := strings.TrimSuffix(op, ":*")
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
		if strings.HasSuffix(op, ":*") {
			prefix := strings.TrimSuffix(op, ":*")
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

	// Rebuild statements from normalized form with preserved statements
	composite.Permissions.Allow = rebuildStatementsFromNormalized(finalAllowNorm, finalAllowPreserved)
	composite.Permissions.Deny = rebuildStatementsFromNormalized(finalDenyNorm, finalDenyPreserved)
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
func (c *Config) resolvePermissionConflicts(role *models.Role) {
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

	// Rebuild statements
	role.Permissions.Allow = rebuildStatementsFromNormalized(allowNorm, dedupedAllow)
	role.Permissions.Deny = rebuildStatementsFromNormalized(denyNorm, dedupedDeny)
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

// isCondensablePermission returns true if the permission can be condensed with others.
// GCP-style permissions (with dots in the action part) are not condensable.
func isCondensablePermission(permission string) bool {
	idx := strings.LastIndex(permission, ":")
	if idx == -1 {
		return false
	}
	// If last segment contains a dot, it's a GCP-style permission (not condensable)
	return !strings.Contains(permission[idx+1:], ".")
}

// expandCondensedActions expands "k8s:pods:get,list" into ["k8s:pods:get", "k8s:pods:list"].
// GCP-style permissions are returned as-is.
func expandCondensedActions(permission string) []string {
	if !isCondensablePermission(permission) {
		return []string{permission}
	}

	idx := strings.LastIndex(permission, ":")
	if idx == -1 || !strings.Contains(permission[idx+1:], ",") {
		return []string{permission}
	}

	resource := permission[:idx]
	actions := strings.Split(permission[idx+1:], ",")
	result := make([]string, 0, len(actions))

	for _, action := range actions {
		action = strings.TrimSpace(action)
		if len(action) != 0 {
			result = append(result, resource+":"+action)
		}
	}
	return result
}

// condenseActions groups permissions by resource and condenses their actions.
// Handles wildcards: "ec2:*" subsumes "ec2:DescribeInstances".
//
// Algorithm:
//  1. Separate atomic (non-condensable like GCP) from condensable permissions
//  2. Track wildcard permissions to subsume specific ones
//  3. Group condensable permissions by resource
//  4. Merge and sort actions for each resource
//  5. Filter out permissions subsumed by wildcards
func condenseActions(permissions []string) []string {
	if len(permissions) == 0 {
		return nil
	}

	// Enforce upper bound to prevent resource exhaustion
	if len(permissions) > MaxPermissions {
		logrus.Errorf("condenseActions: permissions slice length %d exceeds maximum %d; returning nil",
			len(permissions), MaxPermissions)
		return nil
	}

	// Pre-allocate with reasonable capacity
	atomic := make([]string, 0, len(permissions)/2)           // Non-condensable permissions
	byResource := make(map[string][]string, len(permissions)) // resource -> actions
	wildcards := make(map[string]bool, len(permissions)/4)    // Tracks wildcard prefixes

	for _, perm := range permissions {
		if strings.HasSuffix(perm, ":*") {
			wildcards[strings.TrimSuffix(perm, ":*")] = true
		}

		if !isCondensablePermission(perm) {
			atomic = append(atomic, perm)
			continue
		}

		idx := strings.LastIndex(perm, ":")
		resource, action := perm[:idx], perm[idx+1:]
		byResource[resource] = append(byResource[resource], action)
	}

	// Filter out items subsumed by wildcards
	result := make([]string, 0, len(atomic)+len(byResource))

	for _, perm := range atomic {
		if !isSubsumedByWildcard(perm, wildcards) {
			result = append(result, perm)
		}
	}

	for resource, actions := range byResource {
		// Check if this resource has a wildcard - if so, only output the wildcard
		if slices.Contains(actions, "*") {
			result = append(result, resource+":*")
			continue
		}

		// Check if this resource is subsumed by a DIFFERENT wildcard
		// (A wildcard shouldn't subsume itself)
		isSubsumed := false
		for prefix := range wildcards {
			// Skip if this is the same resource (self-subsumption)
			if prefix == resource {
				continue
			}
			// Check if resource is under a wildcard prefix
			if strings.HasPrefix(resource, prefix+":") {
				isSubsumed = true
				break
			}
		}

		if isSubsumed {
			continue
		}

		if len(actions) == 1 {
			result = append(result, resource+":"+actions[0])
		} else {
			sort.Strings(actions)
			result = append(result, resource+":"+strings.Join(actions, ","))
		}
	}

	sort.Strings(result)
	return result
}

// isSubsumedByWildcard checks if an item is covered by a wildcard.
func isSubsumedByWildcard(item string, wildcards map[string]bool) bool {
	for prefix := range wildcards {
		if strings.HasPrefix(item, prefix+":") && item != prefix+":*" {
			return true
		}
	}
	return false
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
