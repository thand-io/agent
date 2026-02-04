package config

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// statementsToStrings extracts all operations from Statements as a flat string slice
func statementsToStrings(stmts models.RoleStatements) []string {
	var result []string
	for _, stmt := range stmts {
		result = append(result, stmt.Operations...)
	}
	return result
}

// statementsContains checks if Statements contains a specific operation string
func statementsContains(stmts models.RoleStatements, op string) bool {
	for _, stmt := range stmts {
		for _, o := range stmt.Operations {
			if o == op {
				return true
			}
		}
	}
	return false
}

// TestPermissionMerging tests how permissions are merged during role inheritance,
// with specific attention to condensed actions like k8s:pods:get,list,watch
func TestPermissionMerging(t *testing.T) {
	t.Run("basic string-level merging - current behavior", func(t *testing.T) {
		roles := map[string]models.Role{
			"base-reader": {
				Name: "Base Reader",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:get,list",
						"k8s:services:get,list",
						"k8s:configmaps:get,list",
						"k8s:apps/deployments:get,list",
					),
				},
				Enabled: true,
			},
			"enhanced-dev": {
				Name:     "Enhanced Developer",
				Inherits: []string{"base-reader"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:create,update,delete",
						"k8s:services:create,update,delete",
						"k8s:configmaps:create,update,delete",
						"k8s:apps/deployments:create,update,patch,delete",
					),
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
			ID: "dev-user",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "enhanced-dev")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Updated behavior: permissions are now intelligently condensed
		expectedCondensedBehavior := []string{
			"k8s:apps/deployments:create,delete,get,list,patch,update", // merged and condensed
			"k8s:configmaps:create,delete,get,list,update",             // merged and condensed
			"k8s:pods:create,delete,get,list,update",                   // merged and condensed
			"k8s:services:create,delete,get,list,update",               // merged and condensed
		}

		// Sort both slices for comparison since map iteration order is not guaranteed
		actualAllow := statementsToStrings(result.Permissions.Allow)
		sort.Strings(actualAllow)
		sort.Strings(expectedCondensedBehavior)

		assert.ElementsMatch(t, expectedCondensedBehavior, actualAllow,
			"Current behavior: permissions are merged and intelligently condensed")
	})

	t.Run("overlapping condensed permissions - intelligent merging", func(t *testing.T) {
		roles := map[string]models.Role{
			"partial-access": {
				Name: "Partial Access",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:get,list,watch",
						"k8s:services:get",
					),
				},
				Enabled: true,
			},
			"extended-access": {
				Name:     "Extended Access",
				Inherits: []string{"partial-access"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:create,update",                 // Overlaps with get,list,watch
						"k8s:services:list,create,update,delete", // Overlaps with get
					),
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
			ID: "test-user",
			User: &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "extended-access")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Updated behavior: intelligent merging is now implemented
		expectedImprovedBehavior := []string{
			"k8s:pods:create,get,list,update,watch",      // merged and condensed
			"k8s:services:create,delete,get,list,update", // merged and condensed
		}

		actualAllow := statementsToStrings(result.Permissions.Allow)
		sort.Strings(actualAllow)
		sort.Strings(expectedImprovedBehavior)

		assert.ElementsMatch(t, expectedImprovedBehavior, actualAllow,
			"New behavior shows intelligent merging of overlapping permissions")
	})

	t.Run("deny permissions with condensed actions", func(t *testing.T) {
		roles := map[string]models.Role{
			"base-role": {
				Name: "Base Role",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:get,list,create,update,delete",
					),
					Deny: stmts(
						"k8s:pods:delete", // Should be removed from the condensed allow
					),
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
			ID: "test-user",
			User: &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "base-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Updated behavior: deny properly removes actions from condensed allow permissions
		// The "delete" action should be removed from the allow list due to the deny
		assert.True(t, statementsContains(result.Permissions.Allow, "k8s:pods:create,get,list,update"))
		assert.Empty(t, result.Permissions.Deny, "Deny permissions should be empty after conflict resolution")

		// This shows the improved behavior: deny intelligently removes
		// the 'delete' action from the condensed allow permission
	})

	t.Run("complex inheritance with condensed permissions", func(t *testing.T) {
		roles := map[string]models.Role{
			"viewer": {
				Name: "Viewer",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:get,list",
						"k8s:services:get,list",
						"k8s:events:get,list,watch",
					),
				},
				Enabled: true,
			},
			"editor": {
				Name:     "Editor",
				Inherits: []string{"viewer"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:create,update,patch",
						"k8s:services:create,update,patch",
						"k8s:configmaps:get,list,create,update,delete",
					),
				},
				Enabled: true,
			},
			"admin": {
				Name:     "Admin",
				Inherits: []string{"editor"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:delete",
						"k8s:services:delete",
						"k8s:secrets:get,list,create,update,delete",
					),
					Deny: stmts(
						"k8s:secrets:delete", // Admin can't delete secrets
					),
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

		// Updated behavior: intelligent merging across inheritance levels
		expectedImprovedBehavior := []string{
			"k8s:pods:create,delete,get,list,patch,update",     // fully merged
			"k8s:services:create,delete,get,list,patch,update", // fully merged
			"k8s:events:get,list,watch",                        // no overlaps
			"k8s:configmaps:create,delete,get,list,update",     // no overlaps
			"k8s:secrets:create,get,list,update",               // delete removed by deny
		}

		actualAllow := statementsToStrings(result.Permissions.Allow)
		sort.Strings(actualAllow)
		sort.Strings(expectedImprovedBehavior)

		assert.ElementsMatch(t, expectedImprovedBehavior, actualAllow)

		// Verify deny permissions remain to enforce security policy
		actualDeny := statementsToStrings(result.Permissions.Deny)
		assert.ElementsMatch(t, []string{"k8s:secrets:delete"}, actualDeny, "Explicit deny should remain to enforce security policy")
	})
}

// TestCondensedActionParsing tests the helper functions for parsing condensed actions
func TestCondensedActionParsing(t *testing.T) {
	t.Run("expand condensed actions", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected []string
		}{
			{
				name:     "simple condensed actions",
				input:    "k8s:pods:get,list,watch",
				expected: []string{"k8s:pods:get", "k8s:pods:list", "k8s:pods:watch"},
			},
			{
				name:     "single action",
				input:    "k8s:pods:get",
				expected: []string{"k8s:pods:get"},
			},
			{
				name:     "no actions part",
				input:    "k8s:pods",
				expected: []string{"k8s:pods"},
			},
			{
				name:     "complex resource path",
				input:    "k8s:apps/deployments:get,list,create,update,patch,delete",
				expected: []string{"k8s:apps/deployments:get", "k8s:apps/deployments:list", "k8s:apps/deployments:create", "k8s:apps/deployments:update", "k8s:apps/deployments:patch", "k8s:apps/deployments:delete"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := expandCondensedActions(tc.input)
				assert.ElementsMatch(t, tc.expected, result)
			})
		}
	})

	t.Run("condense actions", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    []string
			expected []string
		}{
			{
				name: "condense related actions",
				input: []string{
					"k8s:pods:get",
					"k8s:pods:list",
					"k8s:pods:watch",
					"k8s:services:get",
					"k8s:services:list",
				},
				expected: []string{
					"k8s:pods:get,list,watch",
					"k8s:services:get,list",
				},
			},
			{
				name: "single actions remain single",
				input: []string{
					"k8s:pods:get",
					"k8s:services:delete",
				},
				expected: []string{
					"k8s:pods:get",
					"k8s:services:delete",
				},
			},
			{
				name: "mixed condensed and single",
				input: []string{
					"k8s:pods:get",
					"k8s:pods:list",
					"k8s:services:delete",
					"k8s:configmaps:create",
				},
				expected: []string{
					"k8s:pods:get,list",
					"k8s:services:delete",
					"k8s:configmaps:create",
				},
			},
			{
				name: "wildcard overrides specific actions",
				input: []string{
					"ec2:describeInstances",
					"ec2:*",
					"ec2:startInstances",
					"rds:*",
					"rds:describeDBInstances",
					"s3:listBuckets",
					"s3:getBucketLocation",
				},
				// Wildcards subsume specific permissions for the same resource
				// s3 permissions are condensed since they have no wildcard
				expected: []string{
					"ec2:*",
					"rds:*",
					"s3:getBucketLocation,listBuckets",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := condenseActions(tc.input)
				sort.Strings(result)
				sort.Strings(tc.expected)
				assert.ElementsMatch(t, tc.expected, result)
			})
		}
	})
}

// TestIsCondensablePermission tests the helper function that determines
// whether a permission uses condensable colon-separated actions
func TestIsCondensablePermission(t *testing.T) {
	testCases := []struct {
		name       string
		permission string
		expected   bool
	}{
		// K8s-style permissions (condensable)
		{
			name:       "k8s simple action",
			permission: "k8s:pods:get",
			expected:   true,
		},
		{
			name:       "k8s with namespace path",
			permission: "k8s:apps/deployments:list",
			expected:   true,
		},
		{
			name:       "k8s condensed actions",
			permission: "k8s:pods:get,list,watch",
			expected:   true,
		},
		// GCP-style permissions (NOT condensable - last segment has dots)
		{
			name:       "gcp simple permission",
			permission: "gcp-prod:accessapproval.requests.approve",
			expected:   false,
		},
		{
			name:       "gcp compute permission",
			permission: "gcp-prod:compute.instances.start",
			expected:   false,
		},
		{
			name:       "gcp storage permission",
			permission: "gcp-prod:storage.buckets.list",
			expected:   false,
		},
		{
			name:       "gcp iam permission",
			permission: "gcp:iam.serviceAccounts.actAs",
			expected:   false,
		},
		// AWS-style permissions (condensable - no dots in action)
		{
			name:       "aws_ec2_permission",
			permission: "ec2:DescribeInstances",
			expected:   true,
		},
		{
			name:       "aws_s3_permission",
			permission: "s3:GetObject",
			expected:   true,
		},
		{
			name:       "aws_wildcard",
			permission: "ec2:*",
			expected:   true,
		},
		// Edge cases
		{
			name:       "no colon at all",
			permission: "somePermission",
			expected:   false,
		},
		{
			name:       "single_colon_only",
			permission: "resource:action",
			expected:   true, // simple action without dots is condensable
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isCondensablePermission(tc.permission)
			assert.Equal(t, tc.expected, result, "permission: %s", tc.permission)
		})
	}
}

// TestGCPStylePermissionHandling tests that GCP-style permissions are
// treated as atomic and not incorrectly condensed or expanded
func TestGCPStylePermissionHandling(t *testing.T) {
	t.Run("expand does not modify GCP permissions", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    string
			expected []string
		}{
			{
				name:     "gcp accessapproval permission",
				input:    "gcp-prod:accessapproval.requests.approve",
				expected: []string{"gcp-prod:accessapproval.requests.approve"},
			},
			{
				name:     "gcp compute permission",
				input:    "gcp-prod:compute.instances.start",
				expected: []string{"gcp-prod:compute.instances.start"},
			},
			{
				name:     "gcp storage permission",
				input:    "gcp:storage.buckets.list",
				expected: []string{"gcp:storage.buckets.list"},
			},
			{
				name:     "gcp iam permission",
				input:    "gcp:iam.serviceAccounts.actAs",
				expected: []string{"gcp:iam.serviceAccounts.actAs"},
			},
			{
				name:     "gcp permission with dots that could look like condensed",
				input:    "gcp-prod:pubsub.topics.publish",
				expected: []string{"gcp-prod:pubsub.topics.publish"},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := expandCondensedActions(tc.input)
				assert.Equal(t, tc.expected, result)
			})
		}
	})

	t.Run("condense keeps GCP permissions atomic", func(t *testing.T) {
		testCases := []struct {
			name     string
			input    []string
			expected []string
		}{
			{
				name: "multiple GCP permissions same service",
				input: []string{
					"gcp-prod:compute.instances.start",
					"gcp-prod:compute.instances.stop",
					"gcp-prod:compute.instances.list",
				},
				expected: []string{
					"gcp-prod:compute.instances.list",
					"gcp-prod:compute.instances.start",
					"gcp-prod:compute.instances.stop",
				},
			},
			{
				name: "mixed GCP and k8s permissions",
				input: []string{
					"gcp-prod:compute.instances.start",
					"gcp-prod:compute.instances.stop",
					"k8s:pods:get",
					"k8s:pods:list",
					"k8s:pods:watch",
				},
				expected: []string{
					"gcp-prod:compute.instances.start",
					"gcp-prod:compute.instances.stop",
					"k8s:pods:get,list,watch",
				},
			},
			{
				name: "GCP permissions different services",
				input: []string{
					"gcp:storage.buckets.list",
					"gcp:storage.objects.get",
					"gcp:compute.instances.list",
					"gcp:iam.serviceAccounts.actAs",
				},
				expected: []string{
					"gcp:compute.instances.list",
					"gcp:iam.serviceAccounts.actAs",
					"gcp:storage.buckets.list",
					"gcp:storage.objects.get",
				},
			},
			{
				name: "GCP wildcard subsumes GCP permissions",
				input: []string{
					"gcp-prod:*",
					"gcp-prod:compute.instances.start",
					"gcp-prod:compute.instances.stop",
					"gcp-prod:storage.buckets.list",
				},
				// gcp-prod:* subsumes all gcp-prod: permissions
				expected: []string{
					"gcp-prod:*",
				},
			},
			{
				name: "mixed wildcards subsume their respective permissions",
				input: []string{
					"ec2:*",
					"ec2:DescribeInstances",
					"s3:GetObject",
					"gcp-prod:*",
					"gcp-prod:compute.instances.list",
					"k8s:pods:get",
					"k8s:pods:list",
				},
				// ec2:* subsumes ec2:DescribeInstances, gcp-prod:* subsumes gcp-prod:compute.instances.list
				expected: []string{
					"ec2:*",
					"gcp-prod:*",
					"k8s:pods:get,list",
					"s3:GetObject",
				},
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				result := condenseActions(tc.input)
				sort.Strings(result)
				sort.Strings(tc.expected)
				assert.ElementsMatch(t, tc.expected, result)
			})
		}
	})

	t.Run("role composition preserves GCP permissions", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp-viewer": {
				Name: "GCP Viewer",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"gcp-prod:compute.instances.list",
						"gcp-prod:compute.instances.get",
						"gcp-prod:storage.buckets.list",
					),
				},
				Enabled: true,
			},
			"gcp-admin": {
				Name:     "GCP Admin",
				Inherits: []string{"gcp-viewer"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"gcp-prod:compute.instances.start",
						"gcp-prod:compute.instances.stop",
						"gcp-prod:storage.buckets.create",
					),
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
			ID: "gcp-admin-user",
			User: &models.User{
				Username: "gcpadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "gcp-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// All GCP permissions should remain as individual entries, not condensed
		expectedPermissions := []string{
			"gcp-prod:compute.instances.get",
			"gcp-prod:compute.instances.list",
			"gcp-prod:compute.instances.start",
			"gcp-prod:compute.instances.stop",
			"gcp-prod:storage.buckets.create",
			"gcp-prod:storage.buckets.list",
		}

		actualAllow := statementsToStrings(result.Permissions.Allow)
		sort.Strings(actualAllow)
		sort.Strings(expectedPermissions)

		assert.ElementsMatch(t, expectedPermissions, actualAllow,
			"GCP permissions should remain atomic and not be condensed")
	})

	t.Run("mixed provider permissions handled correctly", func(t *testing.T) {
		roles := map[string]models.Role{
			"multi-cloud": {
				Name: "Multi-Cloud Access",
				Permissions: models.RolePermissions{
					Allow: stmts(
						// GCP permissions (should stay atomic - dots in action)
						"gcp-prod:compute.instances.list",
						"gcp-prod:compute.instances.get",
						// K8s permissions (should be condensed)
						"k8s:pods:get",
						"k8s:pods:list",
						"k8s:pods:watch",
						// AWS permissions (should be condensed - no dots in action)
						"ec2:DescribeInstances",
						"ec2:StartInstances",
						"s3:GetObject",
					),
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
			ID: "multi-cloud-user",
			User: &models.User{
				Username: "multicloud",
				Email:    "user@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "multi-cloud")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify GCP permissions are atomic
		assert.True(t, statementsContains(result.Permissions.Allow, "gcp-prod:compute.instances.list"))
		assert.True(t, statementsContains(result.Permissions.Allow, "gcp-prod:compute.instances.get"))

		// Verify K8s permissions are condensed
		hasCondensedK8s := statementsContains(result.Permissions.Allow, "k8s:pods:get,list,watch")
		assert.True(t, hasCondensedK8s, "K8s permissions should be condensed: got %v", result.Permissions.Allow)

		// Verify AWS permissions are present (condensed or not based on implementation)
		hasEC2 := statementsContains(result.Permissions.Allow, "ec2:DescribeInstances,StartInstances") ||
			statementsContains(result.Permissions.Allow, "ec2:DescribeInstances") ||
			statementsContains(result.Permissions.Allow, "ec2:StartInstances")
		hasS3 := statementsContains(result.Permissions.Allow, "s3:GetObject")
		assert.True(t, hasEC2, "EC2 permissions should be present: got %v", result.Permissions.Allow)
		assert.True(t, hasS3, "S3 permissions should be present: got %v", result.Permissions.Allow)
	})
}

// Note: Helper functions expandCondensedActions and condenseActions
// are now implemented in roles.go
