package config

import (
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
