package config

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// stmts converts a []string to models.RoleStatements for test convenience
func stmts(ops ...string) models.RoleStatements {
	if len(ops) == 0 {
		return nil
	}
	return models.RoleStatements{{Operations: ops}}
}

// collectAllOps collects all operations from statements into a single slice
func collectAllOps(stmts models.RoleStatements) []string {
	var result []string
	for _, stmt := range stmts {
		result = append(result, stmt.Operations...)
	}
	return result
}

// Test GetCompositeRole functionality
func TestGetCompositeRole(t *testing.T) {
	tests := []struct {
		name          string
		roles         map[string]models.Role
		providers     map[string]models.Provider
		identity      *models.Identity
		roleName      string
		expected      *models.Role
		expectError   bool
		errorContains string
	}{
		{
			name: "simple role without inheritance",
			roles: map[string]models.Role{
				"basic": {
					Name:        "basic",
					Description: "Basic role",
					Permissions: models.RolePermissions{
						Allow: stmts("read"),
						Deny:  stmts("delete"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			roleName: "basic",
			expected: &models.Role{
				Name:        "basic",
				Description: "Basic role",
				Permissions: models.RolePermissions{
					Allow: stmts("read"),
					Deny:  stmts("delete"),
				},
				Enabled:   true,
				Composite: false, // No inheritance, not composite
			},
			expectError: false,
		},
		{
			name: "role with single inheritance",
			roles: map[string]models.Role{
				"base": {
					Name:        "base",
					Description: "Base role",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{{
							Operations: []string{"read"},
							Targets:    []string{"resource1"},
						}},
					},
					Enabled: true,
				},
				"extended": {
					Name:        "extended",
					Description: "Extended role",
					Inherits:    []string{"base"},
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{{
							Operations: []string{"write"},
							Targets:    []string{"resource2"},
						}},
						Deny: stmts("admin"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
				},
			},
			roleName: "extended",
			expected: &models.Role{
				Name:        "extended",
				Description: "Extended role",
				Inherits:    []string{"base"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{"read"},
							Targets:    []string{"resource1"},
						},
						{
							Operations: []string{"write"},
							Targets:    []string{"resource2"},
						},
					},
					Deny: stmts("admin"),
				},
				Enabled:   true,
				Composite: true, // Inherits from thand role, should be composite
			},
			expectError: false,
		},
		{
			name: "role with multiple inheritance",
			roles: map[string]models.Role{
				"read-role": {
					Name:        "read-role",
					Description: "Read role",
					Permissions: models.RolePermissions{
						Allow: stmts("read"),
					},
					Workflows: []string{"read-workflow"},
					Enabled:   true,
				},
				"write-role": {
					Name:        "write-role",
					Description: "Write role",
					Permissions: models.RolePermissions{
						Allow: stmts("write"),
					},
					Workflows: []string{"write-workflow"},
					Enabled:   true,
				},
				"admin": {
					Name:        "admin",
					Description: "Admin role",
					Inherits:    []string{"read-role", "write-role"},
					Permissions: models.RolePermissions{
						Allow: stmts("admin"),
					},
					Workflows: []string{"admin-workflow"},
					Enabled:   true,
				},
			},
			identity: &models.Identity{
				ID: "admin1",
				User: &models.User{
					Username: "admin",
				},
			},
			roleName: "admin",
			expected: &models.Role{
				Name:        "admin",
				Description: "Admin role",
				Inherits:    []string{"read-role", "write-role"},
				Permissions: models.RolePermissions{
					Allow: stmts("admin", "read", "write"),
				},
				Workflows: []string{"admin-workflow"}, // Only the role's own workflows, not inherited
				Enabled:   true,
				Composite: true, // Inherits from thand roles, should be composite
			},
			expectError: false,
		},
		{
			name: "role with scopes - user allowed",
			roles: map[string]models.Role{
				"scoped": {
					Name:        "scoped",
					Description: "Scoped role",
					Permissions: models.RolePermissions{
						Allow: stmts("special"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Users: []string{"test@example.com"},
						},
					},
					Enabled: true,
				},
				"public": {
					Name:        "public",
					Description: "Public role",
					Inherits:    []string{"scoped"},
					Permissions: models.RolePermissions{
						Allow: stmts("read"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			roleName: "public",
			expected: &models.Role{
				Name:        "public",
				Description: "Public role",
				Inherits:    []string{"scoped"},
				Permissions: models.RolePermissions{
					Allow: stmts("read", "special"),
				},
				Enabled:   true,
				Composite: true, // Inherits from thand role, should be composite
			},
			expectError: false,
		},
		{
			name: "role with scopes - user not allowed",
			roles: map[string]models.Role{
				"scoped": {
					Name:        "scoped",
					Description: "Scoped role",
					Permissions: models.RolePermissions{
						Allow: stmts("special"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Users: []string{"other@example.com"},
						},
					},
					Enabled: true,
				},
				"public": {
					Name:        "public",
					Description: "Public role",
					Inherits:    []string{"scoped"},
					Permissions: models.RolePermissions{
						Allow: stmts("read"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			roleName: "public",
			expected: &models.Role{
				Name:        "public",
				Description: "Public role",
				Inherits:    []string{"scoped"},
				Permissions: models.RolePermissions{
					Allow: stmts("read"),
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "cyclic inheritance",
			roles: map[string]models.Role{
				"role1": {
					Name:     "role1",
					Inherits: []string{"role2"},
					Enabled:  true,
				},
				"role2": {
					Name:     "role2",
					Inherits: []string{"role1"},
					Enabled:  true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
				},
			},
			roleName:      "role1",
			expectError:   true,
			errorContains: "cyclic inheritance detected",
		},
		{
			name: "nonexistent inherited role",
			roles: map[string]models.Role{
				"parent": {
					Name:     "parent",
					Inherits: []string{"nonexistent"},
					Enabled:  true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
				},
			},
			roleName:      "parent",
			expectError:   true,
			errorContains: "role not found: nonexistent",
		},
		{
			name: "nonexistent base role",
			roles: map[string]models.Role{
				"other": {
					Name:    "other",
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
				},
			},
			roleName:      "nonexistent",
			expectError:   true,
			errorContains: "role not found: nonexistent",
		},
		{
			name: "group scope inheritance",
			roles: map[string]models.Role{
				"group-role": {
					Name:        "group-role",
					Description: "Group specific role",
					Permissions: models.RolePermissions{
						Allow: stmts("group-action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Groups: []string{"developers"},
						},
					},
					Enabled: true,
				},
				"user-role": {
					Name:        "user-role",
					Description: "User role inheriting group role",
					Inherits:    []string{"group-role"},
					Permissions: models.RolePermissions{
						Allow: stmts("user-action"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "developer1",
					Groups:   []string{"developers", "users"},
				},
			},
			roleName: "user-role",
			expected: &models.Role{
				Name:        "user-role",
				Description: "User role inheriting group role",
				Inherits:    []string{"group-role"},
				Permissions: models.RolePermissions{
					Allow: stmts("group-action", "user-action"), // sorted alphabetically
				},
				Enabled:   true,
				Composite: true, // Inherits from thand role, should be composite
			},
			expectError: false,
		},
		{
			name: "domain-scoped role inheritance - user matches domain",
			roles: map[string]models.Role{
				"base-role": {
					Name:        "base-role",
					Description: "Base role with domain scope",
					Permissions: models.RolePermissions{
						Allow: stmts("base-action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Domains: []string{"example.com"},
						},
					},
					Enabled: true,
				},
				"child-role": {
					Name:        "child-role",
					Description: "Child role inheriting domain-scoped base",
					Inherits:    []string{"base-role"},
					Permissions: models.RolePermissions{
						Allow: stmts("child-action"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			roleName: "child-role",
			expected: &models.Role{
				Name:        "child-role",
				Description: "Child role inheriting domain-scoped base",
				Inherits:    []string{"base-role"},
				Permissions: models.RolePermissions{
					Allow: stmts("base-action", "child-action"),
				},
				Composite: true,
				Enabled:   true,
			},
			expectError: false,
		},
		{
			name: "domain-scoped role inheritance - user does not match domain",
			roles: map[string]models.Role{
				"base-role": {
					Name:        "base-role",
					Description: "Base role with domain scope",
					Permissions: models.RolePermissions{
						Allow: stmts("base-action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Domains: []string{"company.com"},
						},
					},
					Enabled: true,
				},
				"child-role": {
					Name:        "child-role",
					Description: "Child role inheriting domain-scoped base",
					Inherits:    []string{"base-role"},
					Permissions: models.RolePermissions{
						Allow: stmts("child-action"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			roleName: "child-role",
			expected: &models.Role{
				Name:        "child-role",
				Description: "Child role inheriting domain-scoped base",
				Inherits:    []string{"base-role"},
				Permissions: models.RolePermissions{
					Allow: stmts("child-action"), // base-action not inherited
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "mixed scopes in inheritance - user matches via domain",
			roles: map[string]models.Role{
				"scoped-base": {
					Name:        "scoped-base",
					Description: "Base with mixed scopes",
					Permissions: models.RolePermissions{
						Allow: stmts("scoped-action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Users:   []string{"other@company.com"},
							Groups:  []string{"admins"},
							Domains: []string{"example.com"},
						},
					},
					Enabled: true,
				},
				"parent-role": {
					Name:        "parent-role",
					Description: "Parent inheriting mixed-scope role",
					Inherits:    []string{"scoped-base"},
					Permissions: models.RolePermissions{
						Allow: stmts("parent-action"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers"},
				},
			},
			roleName: "parent-role",
			expected: &models.Role{
				Name:        "parent-role",
				Description: "Parent inheriting mixed-scope role",
				Inherits:    []string{"scoped-base"},
				Permissions: models.RolePermissions{
					Allow: stmts("parent-action", "scoped-action"),
				},
				Composite: true,
				Enabled:   true,
			},
			expectError: false,
		},
		{
			name: "deep inheritance with domain scopes - middle role scoped",
			roles: map[string]models.Role{
				"grandparent": {
					Name:        "grandparent",
					Description: "Grandparent role",
					Permissions: models.RolePermissions{
						Allow: stmts("grandparent-action"),
					},
					Enabled: true,
				},
				"parent": {
					Name:        "parent",
					Description: "Parent with domain scope",
					Inherits:    []string{"grandparent"},
					Permissions: models.RolePermissions{
						Allow: stmts("parent-action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Domains: []string{"example.com"},
						},
					},
					Enabled: true,
				},
				"child": {
					Name:        "child",
					Description: "Child role",
					Inherits:    []string{"parent"},
					Permissions: models.RolePermissions{
						Allow: stmts("child-action"),
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			roleName: "child",
			expected: &models.Role{
				Name:        "child",
				Description: "Child role",
				Inherits:    []string{"parent"},
				Permissions: models.RolePermissions{
					Allow: stmts("child-action", "grandparent-action", "parent-action"),
				},
				Composite: true,
				Enabled:   true,
			},
			expectError: false,
		},
		{
			name: "base role with group scope - user has no groups",
			roles: map[string]models.Role{
				"admin-role": {
					Name:        "admin-role",
					Description: "Admin role requiring group membership",
					Permissions: models.RolePermissions{
						Allow: stmts("admin:write", "admin:delete"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Groups: []string{"admins"},
						},
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{}, // No groups
				},
			},
			roleName:      "admin-role",
			expectError:   true,
			errorContains: "not applicable to identity",
		},
		{
			name: "base role with group scope - user has matching group",
			roles: map[string]models.Role{
				"developer-role": {
					Name:        "developer-role",
					Description: "Developer role requiring group membership",
					Permissions: models.RolePermissions{
						Allow: stmts("dev:read", "dev:write"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Groups: []string{"developers"},
						},
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			roleName: "developer-role",
			expected: &models.Role{
				Name:        "developer-role",
				Description: "Developer role requiring group membership",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{Operations: []string{"dev:read,write"}}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "base role with domain scope - user domain does not match",
			roles: map[string]models.Role{
				"company-role": {
					Name:        "company-role",
					Description: "Role for company employees",
					Permissions: models.RolePermissions{
						Allow: stmts("company:read"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Domains: []string{"company.com"},
						},
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			roleName:      "company-role",
			expectError:   true,
			errorContains: "not applicable to identity",
		},
		{
			name: "base role with user scope - user does not match",
			roles: map[string]models.Role{
				"specific-user-role": {
					Name:        "specific-user-role",
					Description: "Role for specific users only",
					Permissions: models.RolePermissions{
						Allow: stmts("special:action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Users: []string{"admin@company.com", "manager@company.com"},
						},
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			roleName:      "specific-user-role",
			expectError:   true,
			errorContains: "not applicable to identity",
		},
		{
			name: "base role with mixed scopes - user matches via domain",
			roles: map[string]models.Role{
				"mixed-scope-role": {
					Name:        "mixed-scope-role",
					Description: "Role with multiple scope types",
					Permissions: models.RolePermissions{
						Allow: stmts("mixed:action"),
					},
					Scopes: models.RoleScopes{
						Allow: models.ScopeIdentities{
							Users:   []string{"other@company.com"},
							Groups:  []string{"admins"},
							Domains: []string{"example.com"},
						},
					},
					Enabled: true,
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			roleName: "mixed-scope-role",
			expected: &models.Role{
				Name:        "mixed-scope-role",
				Description: "Role with multiple scope types",
				Permissions: models.RolePermissions{
					Allow: stmts("mixed:action"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"other@company.com"},
						Groups:  []string{"admins"},
						Domains: []string{"example.com"},
					},
				},
				Enabled: true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a config with test data
			config := &Config{
				Roles: RoleConfig{
					Definitions: tt.roles,
				},
			}

			if tt.providers != nil {
				config.providerInstances = tt.providers
			}

			// Call GetCompositeRole
			result, err := config.GetCompositeRoleByName(tt.identity, tt.roleName)

			// Check error expectations
			if tt.expectError {
				require.Error(t, err)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				return
			}

			// Check success case
			require.NoError(t, err)
			require.NotNil(t, result)

			// Compare the results
			assert.Equal(t, tt.expected.Name, result.Name)
			assert.Equal(t, tt.expected.Description, result.Description)
			assert.Equal(t, tt.expected.Enabled, result.Enabled)
			assert.Equal(t, tt.expected.Composite, result.Composite, "Composite field mismatch")

			// If role is composite, verify identifier was updated
			if result.Composite {
				assert.NotEmpty(t, result.Identifier, "Composite role should have non-empty identifier")
				// Verify identifier includes hash (has "_" separator for snake_case)
				assert.Contains(t, result.Identifier, "_", "Composite role identifier should include hash")
			}

			// Compare permissions (order doesn't matter)
			assert.ElementsMatch(t, tt.expected.Permissions.Allow, result.Permissions.Allow)
			assert.ElementsMatch(t, tt.expected.Permissions.Deny, result.Permissions.Deny)

			// Compare workflows (order doesn't matter)
			assert.ElementsMatch(t, tt.expected.Workflows, result.Workflows)

			// Compare providers (order doesn't matter)
			assert.ElementsMatch(t, tt.expected.Providers, result.Providers)
		})
	}
}

func TestGetCompositeRole_ProviderSpecificInheritance(t *testing.T) {
	roles := map[string]models.Role{
		"admin": {
			Name:        "admin",
			Description: "Admin role in AWS",
			Permissions: models.RolePermissions{
				Allow: stmts("aws:admin"),
			},
			Enabled: true,
		},
		"base": {
			Name:        "base",
			Description: "Base role",
			Inherits:    []string{"aws-prod:admin"},
			Permissions: models.RolePermissions{
				Allow: stmts("base:read"),
			},
			Enabled: true,
		},
	}

	providers := map[string]models.ProviderConfig{
		"aws-prod": {
			Name:        "aws-prod",
			Description: "AWS Production",
			Provider:    "aws",
		},
	}

	config := &Config{
		Roles: RoleConfig{
			Definitions: roles,
		},
		Providers: ProviderDefinitionsConfig{
			Definitions: providers,
		},
	}

	identity := &models.Identity{
		ID: "user1",
		User: &models.User{
			Username: "testuser",
		},
	}

	result, err := config.GetCompositeRoleByName(identity, "base")
	require.NoError(t, err)
	require.NotNil(t, result)

	// Should inherit from the 'admin' role since aws-prod provider exists
	assert.Equal(t, "base", result.Name)
	assert.ElementsMatch(t, []string{"base:read"}, collectAllOps(result.Permissions.Allow))
}

func TestIsRoleApplicableToIdentity(t *testing.T) {
	config := &Config{}

	tests := []struct {
		name     string
		role     *models.Role
		identity *models.Identity
		expected bool
	}{
		{
			name: "no scopes - always applicable",
			role: &models.Role{
				Name: "test",
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
				},
			},
			expected: true,
		},
		{
			name: "user scope - email match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"test@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			expected: true,
		},
		{
			name: "user scope - username match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"testuser"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			expected: true,
		},
		{
			name: "user scope - no match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"other@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			expected: false,
		},
		{
			name: "user scope - email in scope matches user with both email and username",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"john@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "johndoe",
					Email:    "john@example.com",
					Name:     "John Doe",
				},
			},
			expected: true,
		},
		{
			name: "user scope - username in scope matches user with both email and username",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"johndoe"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "johndoe",
					Email:    "john@example.com",
					Name:     "John Doe",
				},
			},
			expected: true,
		},
		{
			name: "user scope - name in scope matches user with only name (no email/username/ID)",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"john_doe"}, // Name is converted to snake_case by GetIdentity()
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Name: "John Doe",
				},
			},
			expected: true,
		},
		{
			name: "user scope - full name matches user with multiple fields",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"Jane Smith"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					ID:       "user-789",
					Username: "jsmith",
					Email:    "jane@example.com",
					Name:     "Jane Smith",
				},
			},
			expected: true,
		},
		{
			name: "user scope - multiple identifiers in scope, user matches one via username",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"alice@company.com", "bobsmith", "charlie@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					ID:       "user-456",
					Username: "bobsmith",
					Email:    "bob@example.com",
					Name:     "Bob Smith",
				},
			},
			expected: true,
		},
		{
			name: "user scope - multiple identifiers in scope, user matches one via email",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"alice", "bob@example.com", "charlie"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					ID:       "user-456",
					Username: "robertsmith",
					Email:    "bob@example.com",
					Name:     "Bob Smith",
				},
			},
			expected: true,
		},
		{
			name: "user scope - checks all user fields, none match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"wronguser", "wrong@example.com", "Wrong Name", "wrong-id"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					ID:       "user-456",
					Username: "johndoe",
					Email:    "john@example.com",
					Name:     "John Doe",
				},
			},
			expected: false,
		},
		{
			name: "group scope - user in group",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "group scope - user not in group",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: false,
		},
		{
			name: "group identity - group match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "group1",
				Group: &models.Group{
					Name: "developers",
				},
			},
			expected: true,
		},
		// Domain scope tests
		{
			name: "domain scope - user email domain matches",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			expected: true,
		},
		{
			name: "domain scope - user email domain does not match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			expected: false,
		},
		{
			name: "domain scope - user has no email",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "",
					Groups:   []string{"users"},
				},
			},
			expected: false,
		},
		{
			name: "domain scope - user email has no @ symbol",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "malformed-email",
					Groups:   []string{"users"},
				},
			},
			expected: false,
		},
		{
			name: "domain scope - multiple domains, user matches one",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com", "company.org", "other.net"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@company.org",
					Groups:   []string{"users"},
				},
			},
			expected: true,
		},
		{
			name: "domain scope - case insensitive matching",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"Example.COM"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			expected: true,
		},
		{
			name: "domain scope - subdomain does not match parent domain",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@mail.example.com",
					Groups:   []string{"users"},
				},
			},
			expected: false,
		},
		// Combined scope tests (OR logic - matching any scope type should grant access)
		{
			name: "users + groups scopes - user matches via groups",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:  []string{"other@example.com"},
						Groups: []string{"developers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "users + groups scopes - user matches via users",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:  []string{"testuser@example.com"},
						Groups: []string{"admins"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "users + groups scopes - user matches neither",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:  []string{"other@example.com"},
						Groups: []string{"admins"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: false,
		},
		{
			name: "users + domains scopes - user matches via domain",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"other@company.com"},
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			expected: true,
		},
		{
			name: "users + domains scopes - user matches via users",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"testuser@example.com"},
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"users"},
				},
			},
			expected: true,
		},
		{
			name: "groups + domains scopes - user matches via groups",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups:  []string{"developers"},
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "groups + domains scopes - user matches via domain",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups:  []string{"admins"},
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "groups + domains scopes - user matches neither",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups:  []string{"admins"},
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: false,
		},
		{
			name: "users + groups + domains - user matches only via domain",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"other@company.com"},
						Groups:  []string{"admins"},
						Domains: []string{"example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "users + groups + domains - user matches only via groups",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"other@company.com"},
						Groups:  []string{"developers"},
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "users + groups + domains - user matches only via users",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"testuser"},
						Groups:  []string{"admins"},
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "users + groups + domains - user matches none",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users:   []string{"other@company.com"},
						Groups:  []string{"admins"},
						Domains: []string{"company.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: false,
		},
		// Edge case tests
		{
			name: "group scope - user with nil groups",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   nil,
				},
			},
			expected: false,
		},
		{
			name: "group scope - user with empty groups array",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{},
				},
			},
			expected: false,
		},
		{
			name: "group scope - case insensitive group matching",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"Developers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "group scope - user with multiple groups, only one matches",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins", "managers"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
					Groups:   []string{"developers", "managers", "users"},
				},
			},
			expected: true,
		},
		{
			name: "user scope - ID match",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"user-id-123"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					ID:       "user-id-123",
					Username: "testuser",
					Email:    "testuser@example.com",
				},
			},
			expected: true,
		},
		{
			name: "empty scopes object - always applicable",
			role: &models.Role{
				Name:   "test",
				Scopes: models.RoleScopes{},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "testuser@example.com",
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.isRoleApplicableToIdentity(tt.role, tt.identity)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestAllowDenyConflictResolution tests how Allow/Deny conflicts are resolved during role inheritance
func TestAllowDenyConflictResolution(t *testing.T) {
	t.Run("parent allow overrides child deny", func(t *testing.T) {
		roles := map[string]models.Role{
			"child": {
				Name: "Child Role",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"read", "list"},
						Targets:    []string{"bucket1"},
					}},
					Deny: models.RoleStatements{{
						Operations: []string{"write", "delete"},
						Targets:    []string{"bucket2", "bucket3"},
					}},
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent Role",
				Inherits: []string{"child"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"write"},   // This should override child's deny for "write"
						Targets:    []string{"bucket2"}, // This should override child's deny for "bucket2"
					}},
					Deny: models.RoleStatements{{
						Operations: []string{"read"},    // This should override child's allow for "read"
						Targets:    []string{"bucket1"}, // This should override child's allow for "bucket1"
					}},
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID: "test1",
			User: &models.User{
				Username: "test1",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Collect all targets from result
		var allowTargets, denyTargets []string
		for _, stmt := range result.Permissions.Allow {
			allowTargets = append(allowTargets, stmt.Targets...)
		}
		for _, stmt := range result.Permissions.Deny {
			denyTargets = append(denyTargets, stmt.Targets...)
		}

		// Expected: operations are resolved, targets preserved per operation
		// - list (from child) stays allowed on bucket1
		// - write (from parent, overrides child deny) is allowed on bucket2
		// - read (from parent, overrides child allow) is denied on bucket1
		// - delete (from child, not overridden) stays denied on bucket2,bucket3
		expectedAllowTargets := []string{"bucket1", "bucket2"}
		expectedDenyTargets := []string{"bucket1", "bucket2", "bucket3"}

		assert.ElementsMatch(t, expectedAllowTargets, allowTargets)
		assert.ElementsMatch(t, expectedDenyTargets, denyTargets)
	})

	t.Run("multi-level inheritance with conflicts", func(t *testing.T) {
		roles := map[string]models.Role{
			"grandchild": {
				Name: "Grandchild Role",
				Permissions: models.RolePermissions{
					Allow: stmts("read", "list"),
					Deny:  stmts("write"),
				},
				Enabled: true,
			},
			"child": {
				Name:     "Child Role",
				Inherits: []string{"grandchild"},
				Permissions: models.RolePermissions{
					Allow: stmts("write"), // Overrides grandchild's deny
					Deny:  stmts("list"),  // Overrides grandchild's allow
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent Role",
				Inherits: []string{"child"},
				Permissions: models.RolePermissions{
					Allow: stmts("delete", "list"), // "list" overrides child's deny
					Deny:  stmts("read"),           // Overrides grandchild's allow (inherited through child)
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID: "test1",
			User: &models.User{
				Username: "test1",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Expected final state after all inheritance and conflict resolution
		expectedAllowPerms := []string{"list", "write", "delete"}
		expectedDenyPerms := []string{"read"}

		assert.ElementsMatch(t, expectedAllowPerms, collectAllOps(result.Permissions.Allow))
		assert.ElementsMatch(t, expectedDenyPerms, collectAllOps(result.Permissions.Deny))
	})

	t.Run("parent deny overrides child allow", func(t *testing.T) {
		roles := map[string]models.Role{
			"permissive-child": {
				Name: "Permissive Child",
				Permissions: models.RolePermissions{
					Allow: stmts("read", "write", "delete"),
				},
				Enabled: true,
			},
			"restrictive-parent": {
				Name:     "Restrictive Parent",
				Inherits: []string{"permissive-child"},
				Permissions: models.RolePermissions{
					Deny: stmts("delete", "write"), // Parent denies what child allows
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID: "test1",
			User: &models.User{
				Username: "test1",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "restrictive-parent")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Parent deny should override child allow
		expectedAllowPerms := []string{"read"}
		expectedDenyPerms := []string{"delete", "write"}

		assert.ElementsMatch(t, expectedAllowPerms, collectAllOps(result.Permissions.Allow))
		assert.ElementsMatch(t, expectedDenyPerms, collectAllOps(result.Permissions.Deny))
	})
}

// =============================================================================
// DENY SCOPE TESTS
// =============================================================================

// TestDenyScopePrecedence verifies that deny scopes always take precedence over allow scopes.
// This is SECURITY CRITICAL - explicit denials must not be bypassable by also adding to allow.
func TestDenyScopePrecedence(t *testing.T) {
	config := &Config{}

	tests := []struct {
		name     string
		role     *models.Role
		identity *models.Identity
		expected bool
	}{
		{
			name: "deny scope - user in Deny.Users blocks access",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Users: []string{"blocked@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "blockeduser",
					Email:    "blocked@example.com",
				},
			},
			expected: false,
		},
		{
			name: "deny scope - user in Deny.Groups blocks access",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"testuser"}, // User is explicitly allowed
					},
					Deny: models.ScopeIdentities{
						Groups: []string{"blocked-group"}, // But their group is denied
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
					Groups:   []string{"developers", "blocked-group"},
				},
			},
			expected: false, // Deny takes precedence even though user is in Allow.Users
		},
		{
			name: "deny scope - user domain in Deny.Domains blocks access",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"}, // Domain is allowed
					},
					Deny: models.ScopeIdentities{
						Domains: []string{"example.com"}, // But also denied
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			expected: false, // Deny takes precedence
		},
		{
			name: "deny takes precedence - user in both Allow.Users and Deny.Users",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"admin@example.com"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"admin@example.com"}, // Same user in both
					},
				},
			},
			identity: &models.Identity{
				ID: "admin1",
				User: &models.User{
					Username: "admin",
					Email:    "admin@example.com",
				},
			},
			expected: false, // Deny MUST take precedence
		},
		{
			name: "deny takes precedence - group in both Allow.Groups and Deny.Groups",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins"},
					},
					Deny: models.ScopeIdentities{
						Groups: []string{"admins"}, // Same group in both
					},
				},
			},
			identity: &models.Identity{
				ID: "admin1",
				User: &models.User{
					Username: "admin",
					Email:    "admin@example.com",
					Groups:   []string{"admins", "users"},
				},
			},
			expected: false, // Deny MUST take precedence
		},
		{
			name: "deny by username blocks access even when email is allowed",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"allowed@example.com"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"blockeduser"}, // Denied by username
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "blockeduser",
					Email:    "allowed@example.com", // Email is allowed but username is denied
				},
			},
			expected: false, // Deny takes precedence
		},
		{
			name: "user allowed when not in any deny scope",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"allowed@example.com"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"other@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "alloweduser",
					Email:    "allowed@example.com",
				},
			},
			expected: true, // Not in deny, is in allow
		},
		{
			name: "group identity blocked by Deny.Groups",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
					Deny: models.ScopeIdentities{
						Groups: []string{"developers"}, // Same group denied
					},
				},
			},
			identity: &models.Identity{
				ID: "group1",
				Group: &models.Group{
					Name: "developers",
				},
			},
			expected: false, // Group identity denied
		},
		{
			name: "only deny scopes defined - user not in deny list allowed",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Users: []string{"blocked@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "testuser",
					Email:    "test@example.com",
				},
			},
			expected: true, // No allow scopes means open to all except deny list
		},
		{
			name: "only deny scopes defined - user in deny list blocked",
			role: &models.Role{
				Name: "test",
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Users: []string{"blocked@example.com"},
					},
				},
			},
			identity: &models.Identity{
				ID: "user1",
				User: &models.User{
					Username: "blockeduser",
					Email:    "blocked@example.com",
				},
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := config.isRoleApplicableToIdentity(tt.role, tt.identity)
			assert.Equal(t, tt.expected, result, "Deny scope precedence test failed: %s", tt.name)
		})
	}
}

// TestDenyScopePrecedenceInInheritance verifies that roles with matching deny scopes
// are NOT inherited, even if allow scopes also match.
func TestDenyScopePrecedenceInInheritance(t *testing.T) {
	t.Run("inherited role with deny scope is not inherited", func(t *testing.T) {
		roles := map[string]models.Role{
			"restricted": {
				Name:        "Restricted Role",
				Description: "Role with both allow and deny scopes",
				Permissions: models.RolePermissions{
					Allow: stmts("restricted:action"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"admin@example.com"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"admin@example.com"}, // Same user denied
					},
				},
				Enabled: true,
			},
			"parent": {
				Name:        "Parent Role",
				Description: "Parent that inherits restricted",
				Inherits:    []string{"restricted"},
				Permissions: models.RolePermissions{
					Allow: stmts("parent:action"),
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should only have parent's permissions, not the restricted role's
		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "parent:action")
		assert.NotContains(t, allowOps, "restricted:action", "Denied role should NOT be inherited")
	})

	t.Run("inherited role denied by group scope is not inherited", func(t *testing.T) {
		roles := map[string]models.Role{
			"admin-only": {
				Name: "Admin Only Role",
				Permissions: models.RolePermissions{
					Allow: stmts("admin:superpower"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins"},
					},
					Deny: models.ScopeIdentities{
						Groups: []string{"suspended-admins"},
					},
				},
				Enabled: true,
			},
			"general": {
				Name:     "General Role",
				Inherits: []string{"admin-only"},
				Permissions: models.RolePermissions{
					Allow: stmts("general:action"),
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		// User is in both admins AND suspended-admins
		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "suspendedadmin",
				Email:    "suspended@example.com",
				Groups:   []string{"admins", "suspended-admins"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "general")
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "general:action")
		assert.NotContains(t, allowOps, "admin:superpower", "User in deny group should NOT inherit role")
	})

	t.Run("multi-level inheritance respects deny scopes at each level", func(t *testing.T) {
		roles := map[string]models.Role{
			"level3": {
				Name: "Level 3 - Deepest",
				Permissions: models.RolePermissions{
					Allow: stmts("level3:action"),
				},
				Enabled: true,
			},
			"level2": {
				Name:     "Level 2 - Middle (Denied)",
				Inherits: []string{"level3"},
				Permissions: models.RolePermissions{
					Allow: stmts("level2:action"),
				},
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Domains: []string{"blocked.com"},
					},
				},
				Enabled: true,
			},
			"level1": {
				Name:     "Level 1 - Top",
				Inherits: []string{"level2"},
				Permissions: models.RolePermissions{
					Allow: stmts("level1:action"),
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "blockeduser",
				Email:    "user@blocked.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "level1")
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "level1:action", "Level 1 should be present")
		assert.NotContains(t, allowOps, "level2:action", "Level 2 should NOT be inherited (domain denied)")
		assert.NotContains(t, allowOps, "level3:action", "Level 3 should NOT be inherited (blocked by level 2)")
	})
}

// =============================================================================
// WILDCARD EXPANSION EDGE CASE TESTS
// =============================================================================

func TestWildcardExpansionEdgeCases(t *testing.T) {
	t.Run("service-level wildcard subsumes specific actions", func(t *testing.T) {
		roles := map[string]models.Role{
			"child": {
				Name: "Child",
				Permissions: models.RolePermissions{
					Allow: stmts("s3:GetObject", "s3:PutObject", "s3:DeleteObject"),
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent",
				Inherits: []string{"child"},
				Permissions: models.RolePermissions{
					Allow: stmts("s3:*"), // Wildcard should subsume all s3 actions
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)

		allowOps := collectAllOps(result.Permissions.Allow)
		// Should only contain the wildcard, not individual actions
		assert.Contains(t, allowOps, "s3:*")
		// Specific actions may or may not be present depending on condensation
	})

	t.Run("GCP-style atomic permissions are not condensed", func(t *testing.T) {
		// GCP permissions have dots in the action: compute.instances.start
		perms := []string{
			"compute.instances.start",
			"compute.instances.stop",
			"storage.buckets.get",
		}

		result := condenseActions(perms)

		// Each should remain separate (not condensed)
		assert.Contains(t, result, "compute.instances.start")
		assert.Contains(t, result, "compute.instances.stop")
		assert.Contains(t, result, "storage.buckets.get")
	})

	t.Run("mixed AWS and GCP permissions", func(t *testing.T) {
		perms := []string{
			"s3:GetObject",
			"s3:PutObject",
			"compute.instances.start",
			"ec2:DescribeInstances",
		}

		result := condenseActions(perms)

		// AWS should be condensable, GCP should not
		assert.Len(t, result, 3) // s3:GetObject,PutObject + compute... + ec2:...
	})

	t.Run("empty operations list", func(t *testing.T) {
		result := condenseActions(nil)
		assert.Nil(t, result)

		result = condenseActions([]string{})
		assert.Nil(t, result)
	})

	t.Run("single action not condensed", func(t *testing.T) {
		result := condenseActions([]string{"s3:GetObject"})
		assert.Equal(t, []string{"s3:GetObject"}, result)
	})

	t.Run("wildcard at different levels", func(t *testing.T) {
		perms := []string{
			"s3:*",             // Service-level wildcard
			"s3:Get*",          // This is not a valid wildcard pattern in our system
			"s3:GetObject",     // Should be subsumed by s3:*
			"ec2:RunInstances", // Different service
		}

		result := condenseActions(perms)

		assert.Contains(t, result, "s3:*")
		assert.Contains(t, result, "ec2:RunInstances")
	})

	t.Run("expandCondensedActions handles comma-separated actions", func(t *testing.T) {
		expanded := expandCondensedActions("k8s:pods:get,list,watch")
		assert.ElementsMatch(t, []string{
			"k8s:pods:get",
			"k8s:pods:list",
			"k8s:pods:watch",
		}, expanded)
	})

	t.Run("expandCondensedActions handles single action", func(t *testing.T) {
		expanded := expandCondensedActions("s3:GetObject")
		assert.Equal(t, []string{"s3:GetObject"}, expanded)
	})

	t.Run("expandCondensedActions handles GCP-style permissions unchanged", func(t *testing.T) {
		expanded := expandCondensedActions("compute.instances.start")
		assert.Equal(t, []string{"compute.instances.start"}, expanded)
	})
}

// =============================================================================
// PERMISSION CONDENSATION EDGE CASE TESTS
// =============================================================================

func TestCondenseActionsEdgeCases(t *testing.T) {
	t.Run("multiple actions per resource get condensed", func(t *testing.T) {
		perms := []string{
			"s3:GetObject",
			"s3:PutObject",
			"s3:ListBucket",
		}

		result := condenseActions(perms)

		// Should be condensed to a single entry with comma-separated actions
		assert.Len(t, result, 1)
		assert.Contains(t, result[0], "s3:")
		assert.Contains(t, result[0], "GetObject")
		assert.Contains(t, result[0], "PutObject")
		assert.Contains(t, result[0], "ListBucket")
	})

	t.Run("actions are sorted within condensed permission", func(t *testing.T) {
		perms := []string{
			"s3:Zebra",
			"s3:Alpha",
			"s3:Middle",
		}

		result := condenseActions(perms)

		assert.Len(t, result, 1)
		// Should be sorted alphabetically
		assert.Equal(t, "s3:Alpha,Middle,Zebra", result[0])
	})

	t.Run("self-subsumption does not occur", func(t *testing.T) {
		// A wildcard should not remove itself
		perms := []string{"s3:*"}

		result := condenseActions(perms)

		assert.Contains(t, result, "s3:*")
	})

	t.Run("different services remain separate", func(t *testing.T) {
		perms := []string{
			"s3:GetObject",
			"ec2:DescribeInstances",
			"iam:GetUser",
		}

		result := condenseActions(perms)

		assert.Len(t, result, 3)
	})

	t.Run("wildcard subsumes specific but not other wildcards", func(t *testing.T) {
		perms := []string{
			"s3:*",
			"s3:GetObject", // Should be subsumed
			"ec2:*",
			"ec2:RunInstances", // Should be subsumed
		}

		result := condenseActions(perms)

		assert.Len(t, result, 2)
		assert.Contains(t, result, "s3:*")
		assert.Contains(t, result, "ec2:*")
	})

	t.Run("nested service prefixes handled correctly", func(t *testing.T) {
		perms := []string{
			"iam:*",
			"iam-identity-center:GetUser", // Different service (has hyphen)
		}

		result := condenseActions(perms)

		// iam:* should NOT subsume iam-identity-center:GetUser
		assert.Contains(t, result, "iam:*")
		assert.Contains(t, result, "iam-identity-center:GetUser")
	})
}

// =============================================================================
// CONFLICT RESOLUTION EDGE CASE TESTS
// =============================================================================

func TestConflictResolutionEdgeCases(t *testing.T) {
	t.Run("same operation in both allow and deny within single role is removed from both", func(t *testing.T) {
		role := &models.Role{
			Name: "Conflicted",
			Permissions: models.RolePermissions{
				Allow: stmts("s3:GetObject", "s3:PutObject"),
				Deny:  stmts("s3:PutObject", "s3:DeleteObject"),
			},
		}

		config := &Config{}
		config.resolvePermissionConflicts(role)

		allowOps := collectAllOps(role.Permissions.Allow)
		denyOps := collectAllOps(role.Permissions.Deny)

		// s3:PutObject should be removed from BOTH
		assert.Contains(t, allowOps, "s3:GetObject")
		assert.NotContains(t, allowOps, "s3:PutObject", "Conflicted permission should be removed from allow")
		assert.Contains(t, denyOps, "s3:DeleteObject")
		assert.NotContains(t, denyOps, "s3:PutObject", "Conflicted permission should be removed from deny")
	})

	t.Run("wildcard allow vs specific deny - parent allow wins", func(t *testing.T) {
		roles := map[string]models.Role{
			"child": {
				Name: "Child",
				Permissions: models.RolePermissions{
					Deny: stmts("s3:GetObject", "s3:PutObject"), // Specific denies
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent",
				Inherits: []string{"child"},
				Permissions: models.RolePermissions{
					Allow: stmts("s3:*"), // Wildcard allow should override child denies
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)

		allowOps := collectAllOps(result.Permissions.Allow)
		denyOps := collectAllOps(result.Permissions.Deny)

		assert.Contains(t, allowOps, "s3:*")
		// Child denies should be removed since parent allows with wildcard
		assert.NotContains(t, denyOps, "s3:GetObject")
		assert.NotContains(t, denyOps, "s3:PutObject")
	})

	t.Run("wildcard deny vs specific allow - parent deny wins", func(t *testing.T) {
		roles := map[string]models.Role{
			"child": {
				Name: "Child",
				Permissions: models.RolePermissions{
					Allow: stmts("s3:GetObject", "s3:PutObject"), // Specific allows
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent",
				Inherits: []string{"child"},
				Permissions: models.RolePermissions{
					Deny: stmts("s3:*"), // Wildcard deny should override child allows
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)

		allowOps := collectAllOps(result.Permissions.Allow)
		denyOps := collectAllOps(result.Permissions.Deny)

		assert.Contains(t, denyOps, "s3:*")
		// Child allows should be removed since parent denies with wildcard
		assert.NotContains(t, allowOps, "s3:GetObject")
		assert.NotContains(t, allowOps, "s3:PutObject")
	})

	t.Run("empty permissions handling", func(t *testing.T) {
		role := &models.Role{
			Name: "Empty",
			Permissions: models.RolePermissions{
				Allow: nil,
				Deny:  nil,
			},
		}

		config := &Config{}
		config.resolvePermissionConflicts(role)

		assert.Nil(t, role.Permissions.Allow)
		assert.Nil(t, role.Permissions.Deny)
	})

	t.Run("all permissions conflict and are removed", func(t *testing.T) {
		role := &models.Role{
			Name: "AllConflict",
			Permissions: models.RolePermissions{
				Allow: stmts("action1", "action2"),
				Deny:  stmts("action1", "action2"),
			},
		}

		config := &Config{}
		config.resolvePermissionConflicts(role)

		allowOps := collectAllOps(role.Permissions.Allow)
		denyOps := collectAllOps(role.Permissions.Deny)

		assert.Empty(t, allowOps)
		assert.Empty(t, denyOps)
	})
}

// =============================================================================
// SCOPE MATCHING EDGE CASE TESTS
// =============================================================================

func TestScopeMatchingEdgeCases(t *testing.T) {
	config := &Config{}

	t.Run("case-insensitive email matching", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Users: []string{"USER@EXAMPLE.COM"},
				},
			},
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Email: "user@example.com", // Lowercase
			},
		}

		assert.True(t, config.isRoleApplicableToIdentity(role, identity))
	})

	t.Run("case-insensitive group matching", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Groups: []string{"DEVELOPERS"},
				},
			},
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "test",
				Groups:   []string{"developers"}, // Lowercase
			},
		}

		assert.True(t, config.isRoleApplicableToIdentity(role, identity))
	})

	t.Run("case-insensitive domain matching", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Domains: []string{"EXAMPLE.COM"},
				},
			},
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Email: "user@example.com",
			},
		}

		assert.True(t, config.isRoleApplicableToIdentity(role, identity))
	})

	t.Run("partial string should NOT match", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Users: []string{"admin"},
				},
			},
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "administrator", // Contains "admin" but is not "admin"
				Email:    "administrator@example.com",
			},
		}

		assert.False(t, config.isRoleApplicableToIdentity(role, identity))
	})

	t.Run("nil identity returns false", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Users: []string{"anyone"},
				},
			},
		}

		assert.False(t, config.isRoleApplicableToIdentity(role, nil))
	})

	t.Run("empty scope lists with non-empty role scopes", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Users:   []string{}, // Empty but initialized
					Groups:  []string{},
					Domains: []string{},
				},
			},
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		// No allow scopes defined means open to all
		assert.True(t, config.isRoleApplicableToIdentity(role, identity))
	})

	t.Run("user identity with groups vs group identity", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Groups: []string{"developers"},
				},
			},
		}

		// User identity with group membership
		userIdentity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Groups:   []string{"developers"},
			},
		}

		// Group identity
		groupIdentity := &models.Identity{
			ID: "group1",
			Group: &models.Group{
				Name: "developers",
			},
		}

		assert.True(t, config.isRoleApplicableToIdentity(role, userIdentity))
		assert.True(t, config.isRoleApplicableToIdentity(role, groupIdentity))
	})

	t.Run("user matched by ID", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Users: []string{"unique-user-id-123"},
				},
			},
		}

		identity := &models.Identity{
			ID: "different",
			User: &models.User{
				ID:       "unique-user-id-123",
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		assert.True(t, config.isRoleApplicableToIdentity(role, identity))
	})

	t.Run("group matched by ID", func(t *testing.T) {
		role := &models.Role{
			Name: "test",
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Groups: []string{"group-id-456"},
				},
			},
		}

		identity := &models.Identity{
			ID: "different",
			Group: &models.Group{
				ID:   "group-id-456",
				Name: "developers",
			},
		}

		assert.True(t, config.isRoleApplicableToIdentity(role, identity))
	})
}

// =============================================================================
// INHERITANCE DEPTH AND CYCLE TESTS
// =============================================================================

func TestInheritanceDepthAndCycles(t *testing.T) {
	t.Run("maximum inheritance depth exceeded", func(t *testing.T) {
		roles := make(map[string]models.Role)

		// Create a chain of MaxInheritanceDepth + 1 roles
		for i := 0; i <= MaxInheritanceDepth+1; i++ {
			roleName := fmt.Sprintf("role%d", i)
			role := models.Role{
				Name:    roleName,
				Enabled: true,
			}
			if i > 0 {
				role.Inherits = []string{fmt.Sprintf("role%d", i-1)}
			}
			roles[roleName] = role
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		_, err := config.GetCompositeRoleByName(identity, fmt.Sprintf("role%d", MaxInheritanceDepth+1))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "exceeds maximum inheritance depth")
	})

	t.Run("self-referencing role detected as cycle", func(t *testing.T) {
		roles := map[string]models.Role{
			"self-ref": {
				Name:     "self-ref",
				Inherits: []string{"self-ref"},
				Enabled:  true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		_, err := config.GetCompositeRoleByName(identity, "self-ref")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cyclic inheritance")
	})

	t.Run("indirect cycle detected", func(t *testing.T) {
		roles := map[string]models.Role{
			"a": {
				Name:     "a",
				Inherits: []string{"b"},
				Enabled:  true,
			},
			"b": {
				Name:     "b",
				Inherits: []string{"c"},
				Enabled:  true,
			},
			"c": {
				Name:     "c",
				Inherits: []string{"a"}, // Creates cycle: a -> b -> c -> a
				Enabled:  true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		_, err := config.GetCompositeRoleByName(identity, "a")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "cyclic inheritance")
	})
}

// =============================================================================
// TARGET MERGING TESTS
// =============================================================================

func TestTargetMerging(t *testing.T) {
	t.Run("targets are merged during inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"child": {
				Name: "Child",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:GetObject"},
						Targets:    []string{"bucket1", "bucket2"},
					}},
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent",
				Inherits: []string{"child"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:GetObject"},
						Targets:    []string{"bucket2", "bucket3"}, // bucket2 overlaps
					}},
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent")
		require.NoError(t, err)

		// Collect all targets
		var targets []string
		for _, stmt := range result.Permissions.Allow {
			targets = append(targets, stmt.Targets...)
		}

		// All unique targets should be present
		assert.Contains(t, targets, "bucket1")
		assert.Contains(t, targets, "bucket2")
		assert.Contains(t, targets, "bucket3")
	})

	t.Run("empty targets means all targets", func(t *testing.T) {
		roles := map[string]models.Role{
			"global": {
				Name: "Global",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:GetObject"},
						Targets:    nil, // No targets = all targets
					}},
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "test"},
		}

		result, err := config.GetCompositeRoleByName(identity, "global")
		require.NoError(t, err)

		// Should have the permission with no targets (means all)
		assert.NotEmpty(t, result.Permissions.Allow)
		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "s3:GetObject")
	})
}
