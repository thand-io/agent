package config

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestBasicRoleComposition tests basic role composition without inheritance
func TestBasicRoleComposition(t *testing.T) {
	t.Run("simple role retrieval", func(t *testing.T) {
		roles := map[string]models.Role{
			"basic": {
				Name:        "basic",
				Description: "Basic role",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"read", "write"},
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
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "basic")
		require.NoError(t, err)
		require.NotNil(t, result)
		require.Equal(t, "basic", result.Name)
		require.NotEmpty(t, result.Permissions.Allow)
	})
}
