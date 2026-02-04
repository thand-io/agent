package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

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
					Permissions: models.Permissions{
						Allow: []string{"read"},
						Deny:  []string{"delete"},
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
				Permissions: models.Permissions{
					Allow: []string{"read"},
					Deny:  []string{"delete"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "role with single inheritance",
			roles: map[string]models.Role{
				"base": {
					Name:        "base",
					Description: "Base role",
					Permissions: models.Permissions{
						Allow: []string{"read"},
					},
					Resources: models.Resources{
						Allow: []string{"resource1"},
					},
					Enabled: true,
				},
				"extended": {
					Name:        "extended",
					Description: "Extended role",
					Inherits:    []string{"base"},
					Permissions: models.Permissions{
						Allow: []string{"write"},
						Deny:  []string{"admin"},
					},
					Resources: models.Resources{
						Allow: []string{"resource2"},
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
				Permissions: models.Permissions{
					Allow: []string{"write", "read"},
					Deny:  []string{"admin"},
				},
				Resources: models.Resources{
					Allow: []string{"resource2", "resource1"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "role with multiple inheritance",
			roles: map[string]models.Role{
				"read-role": {
					Name:        "read-role",
					Description: "Read role",
					Permissions: models.Permissions{
						Allow: []string{"read"},
					},
					Workflows: []string{"read-workflow"},
					Enabled:   true,
				},
				"write-role": {
					Name:        "write-role",
					Description: "Write role",
					Permissions: models.Permissions{
						Allow: []string{"write"},
					},
					Workflows: []string{"write-workflow"},
					Enabled:   true,
				},
				"admin": {
					Name:        "admin",
					Description: "Admin role",
					Inherits:    []string{"read-role", "write-role"},
					Permissions: models.Permissions{
						Allow: []string{"admin"},
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
				Permissions: models.Permissions{
					Allow: []string{"admin", "read", "write"},
				},
				Workflows: []string{"admin-workflow"}, // Only the role's own workflows, not inherited
				Enabled:   true,
			},
			expectError: false,
		},
		{
			name: "role with scopes - user allowed",
			roles: map[string]models.Role{
				"scoped": {
					Name:        "scoped",
					Description: "Scoped role",
					Permissions: models.Permissions{
						Allow: []string{"special"},
					},
					Scopes: &models.RoleScopes{
						Users: []string{"test@example.com"},
					},
					Enabled: true,
				},
				"public": {
					Name:        "public",
					Description: "Public role",
					Inherits:    []string{"scoped"},
					Permissions: models.Permissions{
						Allow: []string{"read"},
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
				Permissions: models.Permissions{
					Allow: []string{"read", "special"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "role with scopes - user not allowed",
			roles: map[string]models.Role{
				"scoped": {
					Name:        "scoped",
					Description: "Scoped role",
					Permissions: models.Permissions{
						Allow: []string{"special"},
					},
					Scopes: &models.RoleScopes{
						Users: []string{"other@example.com"},
					},
					Enabled: true,
				},
				"public": {
					Name:        "public",
					Description: "Public role",
					Inherits:    []string{"scoped"},
					Permissions: models.Permissions{
						Allow: []string{"read"},
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
				Permissions: models.Permissions{
					Allow: []string{"read"},
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
					Permissions: models.Permissions{
						Allow: []string{"group-action"},
					},
					Scopes: &models.RoleScopes{
						Groups: []string{"developers"},
					},
					Enabled: true,
				},
				"user-role": {
					Name:        "user-role",
					Description: "User role inheriting group role",
					Inherits:    []string{"group-role"},
					Permissions: models.Permissions{
						Allow: []string{"user-action"},
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
				Permissions: models.Permissions{
					Allow: []string{"user-action", "group-action"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "domain-scoped role inheritance - user matches domain",
			roles: map[string]models.Role{
				"base-role": {
					Name:        "base-role",
					Description: "Base role with domain scope",
					Permissions: models.Permissions{
						Allow: []string{"base-action"},
					},
					Scopes: &models.RoleScopes{
						Domains: []string{"example.com"},
					},
					Enabled: true,
				},
				"child-role": {
					Name:        "child-role",
					Description: "Child role inheriting domain-scoped base",
					Inherits:    []string{"base-role"},
					Permissions: models.Permissions{
						Allow: []string{"child-action"},
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
				Permissions: models.Permissions{
					Allow: []string{"child-action", "base-action"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "domain-scoped role inheritance - user does not match domain",
			roles: map[string]models.Role{
				"base-role": {
					Name:        "base-role",
					Description: "Base role with domain scope",
					Permissions: models.Permissions{
						Allow: []string{"base-action"},
					},
					Scopes: &models.RoleScopes{
						Domains: []string{"company.com"},
					},
					Enabled: true,
				},
				"child-role": {
					Name:        "child-role",
					Description: "Child role inheriting domain-scoped base",
					Inherits:    []string{"base-role"},
					Permissions: models.Permissions{
						Allow: []string{"child-action"},
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
				Permissions: models.Permissions{
					Allow: []string{"child-action"}, // base-action not inherited
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
					Permissions: models.Permissions{
						Allow: []string{"scoped-action"},
					},
					Scopes: &models.RoleScopes{
						Users:   []string{"other@company.com"},
						Groups:  []string{"admins"},
						Domains: []string{"example.com"},
					},
					Enabled: true,
				},
				"parent-role": {
					Name:        "parent-role",
					Description: "Parent inheriting mixed-scope role",
					Inherits:    []string{"scoped-base"},
					Permissions: models.Permissions{
						Allow: []string{"parent-action"},
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
				Permissions: models.Permissions{
					Allow: []string{"parent-action", "scoped-action"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "deep inheritance with domain scopes - middle role scoped",
			roles: map[string]models.Role{
				"grandparent": {
					Name:        "grandparent",
					Description: "Grandparent role",
					Permissions: models.Permissions{
						Allow: []string{"grandparent-action"},
					},
					Enabled: true,
				},
				"parent": {
					Name:        "parent",
					Description: "Parent with domain scope",
					Inherits:    []string{"grandparent"},
					Permissions: models.Permissions{
						Allow: []string{"parent-action"},
					},
					Scopes: &models.RoleScopes{
						Domains: []string{"example.com"},
					},
					Enabled: true,
				},
				"child": {
					Name:        "child",
					Description: "Child role",
					Inherits:    []string{"parent"},
					Permissions: models.Permissions{
						Allow: []string{"child-action"},
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
				Permissions: models.Permissions{
					Allow: []string{"child-action", "parent-action", "grandparent-action"},
				},
				Enabled: true,
			},
			expectError: false,
		},
		{
			name: "base role with group scope - user has no groups",
			roles: map[string]models.Role{
				"admin-role": {
					Name:        "admin-role",
					Description: "Admin role requiring group membership",
					Permissions: models.Permissions{
						Allow: []string{"admin:write", "admin:delete"},
					},
					Scopes: &models.RoleScopes{
						Groups: []string{"admins"},
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
					Permissions: models.Permissions{
						Allow: []string{"dev:read", "dev:write"},
					},
					Scopes: &models.RoleScopes{
						Groups: []string{"developers"},
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
				Permissions: models.Permissions{
					Allow: []string{"dev:read,write"},
				},
				Scopes: &models.RoleScopes{
					Groups: []string{"developers"},
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
					Permissions: models.Permissions{
						Allow: []string{"company:read"},
					},
					Scopes: &models.RoleScopes{
						Domains: []string{"company.com"},
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
					Permissions: models.Permissions{
						Allow: []string{"special:action"},
					},
					Scopes: &models.RoleScopes{
						Users: []string{"admin@company.com", "manager@company.com"},
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
					Permissions: models.Permissions{
						Allow: []string{"mixed:action"},
					},
					Scopes: &models.RoleScopes{
						Users:   []string{"other@company.com"},
						Groups:  []string{"admins"},
						Domains: []string{"example.com"},
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
				Permissions: models.Permissions{
					Allow: []string{"mixed:action"},
				},
				Scopes: &models.RoleScopes{
					Users:   []string{"other@company.com"},
					Groups:  []string{"admins"},
					Domains: []string{"example.com"},
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

			// Compare permissions (order doesn't matter)
			assert.ElementsMatch(t, tt.expected.Permissions.Allow, result.Permissions.Allow)
			assert.ElementsMatch(t, tt.expected.Permissions.Deny, result.Permissions.Deny)

			// Compare resources (order doesn't matter)
			assert.ElementsMatch(t, tt.expected.Resources.Allow, result.Resources.Allow)
			assert.ElementsMatch(t, tt.expected.Resources.Deny, result.Resources.Deny)

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
			Permissions: models.Permissions{
				Allow: []string{"aws:admin"},
			},
			Enabled: true,
		},
		"base": {
			Name:        "base",
			Description: "Base role",
			Inherits:    []string{"aws-prod:admin"},
			Permissions: models.Permissions{
				Allow: []string{"base:read"},
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
	assert.ElementsMatch(t, []string{"base:read"}, result.Permissions.Allow)
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
				Scopes: &models.RoleScopes{
					Users: []string{"test@example.com"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"testuser"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"other@example.com"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"john@example.com"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"johndoe"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"john_doe"}, // Name is converted to snake_case by GetIdentity()
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
				Scopes: &models.RoleScopes{
					Users: []string{"Jane Smith"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"alice@company.com", "bobsmith", "charlie@example.com"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"alice", "bob@example.com", "charlie"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"wronguser", "wrong@example.com", "Wrong Name", "wrong-id"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"developers"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"admins"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"developers"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"example.com", "company.org", "other.net"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"Example.COM"},
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
				Scopes: &models.RoleScopes{
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Users:  []string{"other@example.com"},
					Groups: []string{"developers"},
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
				Scopes: &models.RoleScopes{
					Users:  []string{"testuser@example.com"},
					Groups: []string{"admins"},
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
				Scopes: &models.RoleScopes{
					Users:  []string{"other@example.com"},
					Groups: []string{"admins"},
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
				Scopes: &models.RoleScopes{
					Users:   []string{"other@company.com"},
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Users:   []string{"testuser@example.com"},
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Groups:  []string{"developers"},
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Groups:  []string{"admins"},
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Groups:  []string{"admins"},
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Users:   []string{"other@company.com"},
					Groups:  []string{"admins"},
					Domains: []string{"example.com"},
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
				Scopes: &models.RoleScopes{
					Users:   []string{"other@company.com"},
					Groups:  []string{"developers"},
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Users:   []string{"testuser"},
					Groups:  []string{"admins"},
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Users:   []string{"other@company.com"},
					Groups:  []string{"admins"},
					Domains: []string{"company.com"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"developers"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"developers"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"Developers"},
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
				Scopes: &models.RoleScopes{
					Groups: []string{"admins", "managers"},
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
				Scopes: &models.RoleScopes{
					Users: []string{"user-id-123"},
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
				Scopes: &models.RoleScopes{},
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
				Permissions: models.Permissions{
					Allow: []string{"read", "list"},
					Deny:  []string{"write", "delete"},
				},
				Resources: models.Resources{
					Allow: []string{"bucket1"},
					Deny:  []string{"bucket2", "bucket3"},
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent Role",
				Inherits: []string{"child"},
				Permissions: models.Permissions{
					Allow: []string{"write"}, // This should override child's deny for "write"
					Deny:  []string{"read"},  // This should override child's allow for "read"
				},
				Resources: models.Resources{
					Allow: []string{"bucket2"}, // This should override child's deny for "bucket2"
					Deny:  []string{"bucket1"}, // This should override child's allow for "bucket1"
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

		// Expected: parent permissions override child permissions in conflicts
		expectedAllowPerms := []string{"write", "list"}
		expectedDenyPerms := []string{"read", "delete"}

		assert.ElementsMatch(t, expectedAllowPerms, result.Permissions.Allow)
		assert.ElementsMatch(t, expectedDenyPerms, result.Permissions.Deny)

		// Expected: parent resources override child resources in conflicts
		expectedAllowResources := []string{"bucket2"}
		expectedDenyResources := []string{"bucket1", "bucket3"}

		assert.ElementsMatch(t, expectedAllowResources, result.Resources.Allow)
		assert.ElementsMatch(t, expectedDenyResources, result.Resources.Deny)
	})

	t.Run("multi-level inheritance with conflicts", func(t *testing.T) {
		roles := map[string]models.Role{
			"grandchild": {
				Name: "Grandchild Role",
				Permissions: models.Permissions{
					Allow: []string{"read", "list"},
					Deny:  []string{"write"},
				},
				Enabled: true,
			},
			"child": {
				Name:     "Child Role",
				Inherits: []string{"grandchild"},
				Permissions: models.Permissions{
					Allow: []string{"write"}, // Overrides grandchild's deny
					Deny:  []string{"list"},  // Overrides grandchild's allow
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent Role",
				Inherits: []string{"child"},
				Permissions: models.Permissions{
					Allow: []string{"delete", "list"}, // "list" overrides child's deny
					Deny:  []string{"read"},           // Overrides grandchild's allow (inherited through child)
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

		assert.ElementsMatch(t, expectedAllowPerms, result.Permissions.Allow)
		assert.ElementsMatch(t, expectedDenyPerms, result.Permissions.Deny)
	})

	t.Run("parent deny overrides child allow", func(t *testing.T) {
		roles := map[string]models.Role{
			"permissive-child": {
				Name: "Permissive Child",
				Permissions: models.Permissions{
					Allow: []string{"read", "write", "delete"},
				},
				Enabled: true,
			},
			"restrictive-parent": {
				Name:     "Restrictive Parent",
				Inherits: []string{"permissive-child"},
				Permissions: models.Permissions{
					Deny: []string{"delete", "write"}, // Parent denies what child allows
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

		assert.ElementsMatch(t, expectedAllowPerms, result.Permissions.Allow)
		assert.ElementsMatch(t, expectedDenyPerms, result.Permissions.Deny)
	})
}
