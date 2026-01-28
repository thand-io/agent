package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestGroupsInheritance tests the inheritance of groups' allow and deny rules
// Groups define which groups a user should be added to when assuming a role
func TestGroupsInheritance(t *testing.T) {
	t.Run("simple role without inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"basic": {
				Name:        "basic",
				Description: "Basic role with groups",
				Groups: models.Groups{
					Allow: []string{"developers", "users"},
					Deny:  []string{"admins"},
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
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "basic")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should have the same groups as defined
		assert.ElementsMatch(t, []string{"developers", "users"}, result.Groups.Allow)
		assert.ElementsMatch(t, []string{"admins"}, result.Groups.Deny)
	})

	t.Run("parent allow overrides child deny", func(t *testing.T) {
		roles := map[string]models.Role{
			"child": {
				Name: "Child Role",
				Groups: models.Groups{
					Allow: []string{"users", "readers"},
					Deny:  []string{"developers", "admins"},
				},
				Enabled: true,
			},
			"parent": {
				Name:     "Parent Role",
				Inherits: []string{"child"},
				Groups: models.Groups{
					Allow: []string{"developers"}, // Parent allows what child denies
					Deny:  []string{"users"},      // Parent denies what child allows
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

		// Parent permissions override child permissions in conflicts
		expectedAllow := []string{"developers", "readers"}
		expectedDeny := []string{"users", "admins"}

		assert.ElementsMatch(t, expectedAllow, result.Groups.Allow)
		assert.ElementsMatch(t, expectedDeny, result.Groups.Deny)
	})

	t.Run("complex inheritance with groups and permissions", func(t *testing.T) {
		roles := map[string]models.Role{
			"viewer": {
				Name: "Viewer",
				Groups: models.Groups{
					Allow: []string{"viewers", "users"},
				},
				Permissions: models.Permissions{
					Allow: stmts("read"),
				},
				Enabled: true,
			},
			"editor": {
				Name:     "Editor",
				Inherits: []string{"viewer"},
				Groups: models.Groups{
					Allow: []string{"editors"},
					Deny:  []string{"viewers"}, // Editors don't need viewer group
				},
				Permissions: models.Permissions{
					Allow: stmts("write"),
				},
				Enabled: true,
			},
			"admin": {
				Name:     "Admin",
				Inherits: []string{"editor"},
				Groups: models.Groups{
					Allow: []string{"admins"},
					Deny:  []string{"users"}, // Admins don't need users group
				},
				Permissions: models.Permissions{
					Allow: stmts("admin"),
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
			ID: "admin-user",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Groups should be properly merged with parent overrides
		expectedAllowGroups := []string{"editors", "admins"}
		expectedDenyGroups := []string{"viewers", "users"}

		assert.ElementsMatch(t, expectedAllowGroups, result.Groups.Allow)
		assert.ElementsMatch(t, expectedDenyGroups, result.Groups.Deny)

		// Permissions should also be properly merged
		expectedAllowPerms := []string{"read", "write", "admin"}
		assert.ElementsMatch(t, expectedAllowPerms, result.Permissions.Allow)
	})

	t.Run("deep inheritance chain", func(t *testing.T) {
		roles := map[string]models.Role{
			"level1": {
				Name: "Level 1",
				Groups: models.Groups{
					Allow: []string{"group1", "group2"},
					Deny:  []string{"group3"},
				},
				Enabled: true,
			},
			"level2": {
				Name:     "Level 2",
				Inherits: []string{"level1"},
				Groups: models.Groups{
					Allow: []string{"group3", "group4"}, // group3 conflicts with level1 deny
					Deny:  []string{"group2"},           // group2 conflicts with level1 allow
				},
				Enabled: true,
			},
			"level3": {
				Name:     "Level 3",
				Inherits: []string{"level2"},
				Groups: models.Groups{
					Allow: []string{"group5"},
					Deny:  []string{"group4"}, // group4 conflicts with level2 allow
				},
				Enabled: true,
			},
			"level4": {
				Name:     "Level 4",
				Inherits: []string{"level3"},
				Groups: models.Groups{
					Allow: []string{"group1", "group2"}, // group2 conflicts with level2 deny
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

		result, err := config.GetCompositeRoleByName(identity, "level4")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Deep chain resolution with parent overrides:
		// - group1: allowed (level1, level4 - no conflicts)
		// - group2: allowed (level4 overrides level2's deny)
		// - group3: allowed (level2 overrides level1's deny)
		// - group4: denied (level3 overrides level2's allow)
		// - group5: allowed (level3 - no conflicts)
		expectedAllow := []string{"group1", "group2", "group3", "group5"}
		expectedDeny := []string{"group4"}

		assert.ElementsMatch(t, expectedAllow, result.Groups.Allow)
		assert.ElementsMatch(t, expectedDeny, result.Groups.Deny)
	})
}
