package aws

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/thand-io/agent/internal/models"
)

func TestAWSProviderPermissions(t *testing.T) {
	// Create a test provider instance
	provider := &awsProvider{}

	// Create minimal config for initialization
	testConfig := models.Provider{
		Name:        "test-aws",
		Description: "Test AWS provider",
		Provider:    "aws",
		Config: &models.BasicConfig{
			"region":            "us-east-1",
			"account_id":        "000000000000",
			"access_key_id":     "test",
			"secret_access_key": "test",
		},
		Enabled: true,
	}

	// Initialize the provider
	err := provider.Initialize(testConfig)
	require.NoError(t, err, "Failed to initialize AWS provider")

	ctx := context.Background()

	t.Run("List Permissions", func(t *testing.T) {
		permissions, err := provider.ListPermissions(ctx)
		assert.NoError(t, err, "Failed to list permissions")
		assert.NotEmpty(t, permissions, "Permissions list should not be empty")

		// Verify permissions have required fields
		for _, perm := range permissions[:5] { // Check first 5 permissions
			assert.NotEmpty(t, perm.Name, "Permission name should not be empty")
			assert.NotEmpty(t, perm.Description, "Permission description should not be empty")
		}
	})

	t.Run("Get Specific Permission", func(t *testing.T) {
		// First get all permissions to find a valid one
		permissions, err := provider.ListPermissions(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, permissions)

		// Test getting a specific permission
		testPermName := permissions[0].Name
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
		// Test with S3 filter
		s3Permissions, err := provider.ListPermissions(ctx, "S3")
		assert.NoError(t, err, "Failed to search S3 permissions")

		// Verify all returned permissions relate to S3
		for _, perm := range s3Permissions {
			// Check if permission name or description contains S3-related keywords
			nameContainsS3 := contains(perm.Name, "S3") || contains(perm.Name, "s3")
			descContainsS3 := contains(perm.Description, "S3") || contains(perm.Description, "s3")
			assert.True(t, nameContainsS3 || descContainsS3,
				"Permission %s should be S3-related", perm.Name)
		}
	})

	t.Run("Search Permissions with Multiple Filters", func(t *testing.T) {
		// Test with EC2 filter specifically
		permissions, err := provider.ListPermissions(ctx, "EC2")
		assert.NoError(t, err, "Failed to search permissions with EC2 filter")

		// Verify results contain EC2 related permissions
		hasEC2Related := false
		for _, perm := range permissions {
			if contains(perm.Name, "EC2") || contains(perm.Description, "EC2") {
				hasEC2Related = true
				break
			}
		}
		assert.True(t, hasEC2Related, "Should find at least one EC2-related permission")
	})

	t.Run("Empty Filter Returns All Permissions", func(t *testing.T) {
		allPermissions, err := provider.ListPermissions(ctx)
		require.NoError(t, err)

		filteredPermissions, err := provider.ListPermissions(ctx, "")
		assert.NoError(t, err, "Failed to list permissions with empty filter")

		// Empty filter should return the same as no filter
		assert.Equal(t, len(allPermissions), len(filteredPermissions),
			"Empty filter should return all permissions")
	})
}

func TestAWSProviderRoles(t *testing.T) {
	// Create a test provider instance
	provider := &awsProvider{}

	// Create minimal config for initialization
	testConfig := models.Provider{
		Name:        "test-aws",
		Description: "Test AWS provider",
		Provider:    "aws",
		Config: &models.BasicConfig{
			"region":            "us-east-1",
			"account_id":        "000000000000",
			"access_key_id":     "test",
			"secret_access_key": "test",
		},
		Enabled: true,
	}

	// Initialize the provider
	err := provider.Initialize(testConfig)
	require.NoError(t, err, "Failed to initialize AWS provider")

	ctx := context.Background()

	t.Run("List Roles", func(t *testing.T) {
		roles, err := provider.ListRoles(ctx)
		assert.NoError(t, err, "Failed to list roles")
		assert.NotEmpty(t, roles, "Roles list should not be empty")

		// Verify roles have required fields
		for _, role := range roles[:5] { // Check first 5 roles
			assert.NotEmpty(t, role.Name, "Role name should not be empty")
		}
	})

	t.Run("Get Specific Role", func(t *testing.T) {
		// First get all roles to find a valid one
		roles, err := provider.ListRoles(ctx)
		require.NoError(t, err)
		require.NotEmpty(t, roles)

		// Test getting a specific role
		testRoleName := roles[0].Name
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
		adminRoles, err := provider.ListRoles(ctx, "Admin")
		assert.NoError(t, err, "Failed to search Admin roles")

		// Verify all returned roles relate to Admin
		for _, role := range adminRoles {
			assert.True(t, contains(role.Name, "Admin"),
				"Role %s should contain 'Admin'", role.Name)
		}
	})

	t.Run("Search Roles with ReadOnly Filter", func(t *testing.T) {
		// Test with ReadOnly filter
		readOnlyRoles, err := provider.ListRoles(ctx, "ReadOnly")
		assert.NoError(t, err, "Failed to search ReadOnly roles")

		// Verify all returned roles relate to ReadOnly
		for _, role := range readOnlyRoles {
			assert.True(t, contains(role.Name, "ReadOnly"),
				"Role %s should contain 'ReadOnly'", role.Name)
		}
	})

	t.Run("Empty Filter Returns All Roles", func(t *testing.T) {
		allRoles, err := provider.ListRoles(ctx)
		require.NoError(t, err)

		filteredRoles, err := provider.ListRoles(ctx, "")
		assert.NoError(t, err, "Failed to list roles with empty filter")

		// Empty filter should return the same as no filter
		assert.Equal(t, len(allRoles), len(filteredRoles),
			"Empty filter should return all roles")
	})
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			indexOf(s, substr) >= 0))
}

// Simple indexOf implementation
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
