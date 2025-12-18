package azure

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

func TestAzureProviderPermissions(t *testing.T) {

	// Create minimal config for initialization
	testConfig := models.Provider{
		Name:        "test-azure",
		Description: "Test Azure provider",
		Provider:    "azure",
		Config: &models.BasicConfig{
			"tenant_id":       "test-tenant",
			"client_id":       "test-client",
			"client_secret":   "test-secret",
			"subscription_id": "test-subscription",
		},
		Enabled: true,
	}

	// Initialize the provider
	provider := NewMockAzureProvider()
	err := provider.Initialize("azure", testConfig)
	require.NoError(t, err, "Failed to initialize Azure provider")

	ctx := context.Background()

	t.Run("List Permissions", func(t *testing.T) {
		permissions, err := provider.ListPermissions(ctx, &models.SearchRequest{})
		assert.NoError(t, err, "Failed to list permissions")
		assert.NotEmpty(t, permissions, "Permissions list should not be empty")

		// Verify permissions have required fields
		for _, permResult := range permissions[:5] { // Check first 5 permissions
			perm := permResult.Result
			assert.NotEmpty(t, perm.Name, "Permission name should not be empty")
			assert.NotEmpty(t, perm.Description, "Permission description should not be empty")
		}
	})

	t.Run("Get Specific Permission", func(t *testing.T) {
		// First get all permissions to find a valid one
		permissions, err := provider.ListPermissions(ctx, &models.SearchRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, permissions)

		// Test getting a specific permission
		testPermName := permissions[0].Result.Name
		perm, err := provider.GetPermission(ctx, testPermName)
		assert.NoError(t, err, "Failed to get permission")
		assert.NotNil(t, perm, "Permission should not be nil")
		assert.Equal(t, testPermName, perm.Name, "Permission names should match")
		assert.NotEmpty(t, perm.Description, "Permission description should not be empty")
	})

	t.Run("Get Non-existent Permission", func(t *testing.T) {
		perm, err := provider.GetPermission(ctx, "NonExistentPermission")
		assert.Error(t, err, "Should fail for non-existent permission")
		assert.Nil(t, perm, "Permission should be nil for non-existent permission")
	})

	t.Run("Search Permissions with Filter", func(t *testing.T) {
		// Test with Compute filter
		computePermissions, err := provider.ListPermissions(ctx, &models.SearchRequest{Terms: []string{"Compute"}})
		assert.NoError(t, err, "Failed to search Compute permissions")

		// Verify all returned permissions relate to Compute
		for _, permResult := range computePermissions {
			perm := permResult.Result
			// Check if permission name or description contains Compute-related keywords
			nameContainsCompute := common.ContainsInsensitive(perm.Name, "Compute")
			descContainsCompute := common.ContainsInsensitive(perm.Description, "Compute")
			assert.True(t, nameContainsCompute || descContainsCompute,
				"Permission %s should be Compute-related", perm.Name)
		}
	})

	t.Run("Empty Filter Returns All Permissions", func(t *testing.T) {
		allPermissions, err := provider.ListPermissions(ctx, &models.SearchRequest{})
		require.NoError(t, err)

		filteredPermissions, err := provider.ListPermissions(ctx, &models.SearchRequest{})
		assert.NoError(t, err, "Failed to list permissions with empty filter")

		// Empty filter should return the same as no filter
		assert.Equal(t, len(allPermissions), len(filteredPermissions),
			"Empty filter should return all permissions")
	})
}

func TestAzureProviderRoles(t *testing.T) {

	// Create minimal config for initialization
	testConfig := models.Provider{
		Name:        "test-azure",
		Description: "Test Azure provider",
		Provider:    "azure",
		Config: &models.BasicConfig{
			"tenant_id":       "test-tenant",
			"client_id":       "test-client",
			"client_secret":   "test-secret",
			"subscription_id": "test-subscription",
		},
		Enabled: true,
	}

	// Initialize the provider
	provider := NewMockAzureProvider()
	err := provider.Initialize("azure", testConfig)
	require.NoError(t, err, "Failed to initialize Azure provider")

	ctx := context.Background()

	t.Run("List Roles", func(t *testing.T) {
		roles, err := provider.ListRoles(ctx, &models.SearchRequest{})
		assert.NoError(t, err, "Failed to list roles")
		assert.NotEmpty(t, roles, "Roles list should not be empty")

		// Verify roles have required fields
		for _, roleResult := range roles[:5] { // Check first 5 roles
			role := roleResult.Result
			assert.NotEmpty(t, role.Name, "Role name should not be empty")
		}
	})

	t.Run("Get Specific Role", func(t *testing.T) {
		// First get all roles to find a valid one
		roles, err := provider.ListRoles(ctx, &models.SearchRequest{})
		require.NoError(t, err)
		require.NotEmpty(t, roles)

		// Test getting a specific role
		testRoleName := roles[0].Result.Name
		role, err := provider.GetRole(ctx, testRoleName)
		assert.NoError(t, err, "Failed to get role")
		assert.NotNil(t, role, "Role should not be nil")
		assert.Equal(t, testRoleName, role.Name, "Role names should match")
	})

	t.Run("Get Non-existent Role", func(t *testing.T) {
		role, err := provider.GetRole(ctx, "NonExistentRole")
		assert.Error(t, err, "Should fail for non-existent role")
		assert.Nil(t, role, "Role should be nil for non-existent role")
	})

	t.Run("Search Roles with Filter", func(t *testing.T) {
		// Test with Admin filter
		adminRoles, err := provider.ListRoles(ctx, &models.SearchRequest{Terms: []string{"Admin"}})
		assert.NoError(t, err, "Failed to search Admin roles")

		// Verify all returned roles relate to Admin
		for _, roleResult := range adminRoles {
			role := roleResult.Result
			assert.True(t, common.ContainsInsensitive(role.Name, "Admin"),
				"Role %s should contain 'Admin'", role.Name)
		}
	})

	t.Run("Search Roles with Owner Filter", func(t *testing.T) {
		// Test with Owner filter
		ownerRoles, err := provider.ListRoles(ctx, &models.SearchRequest{Terms: []string{"Owner"}})
		assert.NoError(t, err, "Failed to search Owner roles")

		// Verify all returned roles relate to Owner
		for _, roleResult := range ownerRoles {
			role := roleResult.Result
			assert.True(t, common.ContainsInsensitive(role.Name, "Owner"),
				"Role %s should contain 'Owner'", role.Name)
		}
	})

	t.Run("Empty Filter Returns All Roles", func(t *testing.T) {
		allRoles, err := provider.ListRoles(ctx, &models.SearchRequest{})
		require.NoError(t, err)

		filteredRoles, err := provider.ListRoles(ctx, &models.SearchRequest{})
		assert.NoError(t, err, "Failed to list roles with empty filter")

		// Empty filter should return the same as no filter
		assert.Equal(t, len(allRoles), len(filteredRoles),
			"Empty filter should return all roles")
	})
}
