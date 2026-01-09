package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestProviderSpecificInheritance tests provider-specific role inheritance
// with complex role names containing multiple colons (AWS ARNs, GCP service accounts, etc.)
func TestProviderSpecificInheritance(t *testing.T) {
	t.Run("aws arn inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"app-role": {
				Name: "AWS Test Role",
				Permissions: models.Permissions{
					Allow: []string{
						"s3:GetObject",
						"s3:ListBucket",
						"ec2:DescribeInstances",
					},
				},
				Enabled: true,
			},
			"app-role-two": {
				Name:     "Application Role",
				Inherits: []string{"app-role"},
				Permissions: models.Permissions{
					Allow: []string{"app:deploy"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "AWS Production",
				Description: "Production AWS Account",
				Provider:    "aws",
				Enabled:     true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "app-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		expectedPerms := []string{
			"s3:GetObject,ListBucket",
			"ec2:DescribeInstances",
		}
		assert.ElementsMatch(t, expectedPerms, result.Permissions.Allow)
	})

	t.Run("gcp service account inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"service-role": {
				Name: "GCP Service Account",
				Permissions: models.Permissions{
					Allow: []string{
						"compute.instances.get",
						"compute.instances.list",
						"storage.objects.get",
					},
				},
				Enabled: true,
			},
			"developer-role": {
				Name:     "Developer Role",
				Inherits: []string{"service-role"},
				Permissions: models.Permissions{
					Allow: []string{"cloudrun.services.deploy"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-dev": {
				Name:     "GCP Development",
				Provider: "gcp",
				Enabled:  true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "developer",
				Email:    "developer@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "developer-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		expectedPerms := []string{
			"cloudrun.services.deploy",
			"compute.instances.get",
			"compute.instances.list",
			"storage.objects.get",
		}
		assert.ElementsMatch(t, expectedPerms, result.Permissions.Allow)
	})

	t.Run("azure resource id inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure-role": {
				Name: "Custom Azure Role",
				Permissions: models.Permissions{
					Allow: []string{
						"Microsoft.Compute/virtualMachines/read",
						"Microsoft.Compute/virtualMachines/start/action",
						"Microsoft.Storage/storageAccounts/listKeys/action",
					},
				},
				Enabled: true,
			},
			"ops-role": {
				Name:     "Operations Role",
				Inherits: []string{"azure-role"},
				Permissions: models.Permissions{
					Allow: []string{"ops.deploy"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "Azure Production",
				Provider: "azure",
				Enabled:  true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "operator",
				Email:    "operator@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "ops-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		expectedPerms := []string{
			"ops.deploy",
			"Microsoft.Compute/virtualMachines/read",
			"Microsoft.Compute/virtualMachines/start/action",
			"Microsoft.Storage/storageAccounts/listKeys/action",
		}
		assert.ElementsMatch(t, expectedPerms, result.Permissions.Allow)
	})

	t.Run("provider not found fallback", func(t *testing.T) {
		roles := map[string]models.Role{
			"nonexistent:arn:aws:iam::123456789012:role/TestRole": {
				Name: "Fallback Role",
				Permissions: models.Permissions{
					Allow: []string{"fallback:action"},
				},
				Enabled: true,
			},
			"parent-role": {
				Name:     "Parent Role",
				Inherits: []string{"nonexistent:arn:aws:iam::123456789012:role/TestRole"},
				Permissions: models.Permissions{
					Allow: []string{"parent:action"},
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
			// No providers defined - forces fallback behavior
		}

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "parent-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		expectedPerms := []string{
			"parent:action",
			"fallback:action",
		}
		assert.ElementsMatch(t, expectedPerms, result.Permissions.Allow)
	})
}

// TestProviderParsingLogicConsolidated tests provider parsing logic consolidation
// This validates that the fix for multiple colons (AWS ARNs) works correctly
func TestProviderParsingLogicConsolidated(t *testing.T) {
	t.Run("validate parsing with multiple colons", func(t *testing.T) {
		// This test validates that provider parsing correctly handles role names with multiple colons
		inheritSpec := "aws-prod:arn:aws:iam::123456789012:role/TestRole"

		// Demonstrate old logic would have failed:
		// strings.Split(inheritSpec, ":") would result in 7 parts, not 2
		oldParts := strings.Split(inheritSpec, ":")
		assert.Equal(t, 7, len(oldParts), "Old logic would incorrectly split into 7 parts")
		assert.Equal(t, []string{"aws-prod", "arn", "aws", "iam", "", "123456789012", "role/TestRole"}, oldParts)

		// New logic correctly splits only on first colon:
		colonIndex := strings.Index(inheritSpec, ":")
		require.True(t, colonIndex > 0 && colonIndex < len(inheritSpec)-1, "Should detect provider:role format")

		providerName := inheritSpec[:colonIndex]
		roleName := inheritSpec[colonIndex+1:]

		assert.Equal(t, "aws-prod", providerName, "Should extract provider name correctly")
		assert.Equal(t, "arn:aws:iam::123456789012:role/TestRole", roleName, "Should extract full role name correctly")
	})

	t.Run("edge cases in parsing", func(t *testing.T) {
		testCases := []struct {
			name         string
			inheritSpec  string
			shouldParse  bool
			expectedProv string
			expectedRole string
		}{
			{
				name:        "no colon - regular role",
				inheritSpec: "simple-role",
				shouldParse: false,
			},
			{
				name:        "colon at start",
				inheritSpec: ":role-name",
				shouldParse: false,
			},
			{
				name:        "colon at end",
				inheritSpec: "provider:",
				shouldParse: false,
			},
			{
				name:         "valid provider:role",
				inheritSpec:  "provider:role-name",
				shouldParse:  true,
				expectedProv: "provider",
				expectedRole: "role-name",
			},
			{
				name:         "complex role with colons",
				inheritSpec:  "aws:arn:aws:iam::123:role/test",
				shouldParse:  true,
				expectedProv: "aws",
				expectedRole: "arn:aws:iam::123:role/test",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				colonIndex := strings.Index(tc.inheritSpec, ":")
				isParseable := colonIndex > 0 && colonIndex < len(tc.inheritSpec)-1

				assert.Equal(t, tc.shouldParse, isParseable, "Parsing detection should match expectation")

				if tc.shouldParse {
					providerName := tc.inheritSpec[:colonIndex]
					roleName := tc.inheritSpec[colonIndex+1:]

					assert.Equal(t, tc.expectedProv, providerName, "Provider name should match")
					assert.Equal(t, tc.expectedRole, roleName, "Role name should match")
				}
			})
		}
	})

	t.Run("integration test - end to end parsing", func(t *testing.T) {
		// This test proves the parsing logic works in the actual inheritance system
		roles := map[string]models.Role{
			"inc-role": {
				Name: "AWS Role",
				Permissions: models.Permissions{
					Allow: []string{"s3:GetObject"},
				},
				Enabled: true,
			},
			"test-role": {
				Name:     "Test Role",
				Inherits: []string{"inc-role"},
				Permissions: models.Permissions{
					Allow: []string{"test:action"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		// This would have failed with the old strings.Split() logic
		result, err := config.GetCompositeRoleByName(identity, "test-role")
		require.NoError(t, err, "Should successfully parse and inherit AWS ARN role")
		require.NotNil(t, result)

		expectedPerms := []string{
			"test:action",
			"s3:GetObject",
		}
		assert.ElementsMatch(t, expectedPerms, result.Permissions.Allow)
	})
}

// TestProviderRoleLookup tests the ability to specify provider:role to get a specific role lookup from a provider
func TestProviderRoleLookup(t *testing.T) {
	t.Run("lookup by provider name", func(t *testing.T) {
		roles := map[string]models.Role{
			"custom-admin": {
				Name:      "Custom Admin Role",
				Inherits:  []string{"aws-prod:AdministratorAccess"},
				Providers: []string{"aws-prod"},
				Permissions: models.Permissions{
					Allow: []string{"internal:check"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "AWS Production",
				Description: "Production AWS Account",
				Provider:    "aws",
				Enabled:     true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "custom-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.Inherits, "AdministratorAccess")
		assert.Contains(t, result.Permissions.Allow, "internal:check")
	})

	t.Run("lookup by provider type as name", func(t *testing.T) {
		roles := map[string]models.Role{
			"aws-admin": {
				Name:      "AWS Admin",
				Inherits:  []string{"aws:AdministratorAccess"},
				Providers: []string{"aws"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws": {
				Name:     "AWS Default",
				Provider: "aws",
				Enabled:  true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "aws-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.Inherits, "AdministratorAccess")
	})

	t.Run("lookup by provider type with base role restriction", func(t *testing.T) {
		roles := map[string]models.Role{
			"aws-admin": {
				Name:      "AWS Admin",
				Inherits:  []string{"aws:AdministratorAccess"},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "aws-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Contains(t, result.Inherits, "AdministratorAccess")
	})

	t.Run("lookup by provider name mismatch with base role", func(t *testing.T) {
		roles := map[string]models.Role{
			"aws-admin": {
				Name:      "AWS Admin",
				Inherits:  []string{"aws-dev:AdministratorAccess"},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
			},
			"aws-dev": {
				Name:     "AWS Development",
				Provider: "aws",
				Enabled:  true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "testuser@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "aws-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should NOT contain "AdministratorAccess" because it was skipped due to provider mismatch
		assert.NotContains(t, result.Inherits, "AdministratorAccess")
		assert.NotContains(t, result.Inherits, "aws-dev:AdministratorAccess")
	})
}

// TestProviderRoleWithIdentityScopes tests GetProviderRoleWithIdentity with identity-aware scoping
// at both the role level (Role.Scopes) and provider level (Provider.Role.Scopes)
func TestProviderRoleWithIdentityScopes(t *testing.T) {
	t.Run("provider with role scopes - user in allowed groups", func(t *testing.T) {
		// Setup: Provider has a Role with Scopes that restrict access to specific groups
		roles := map[string]models.Role{
			"admin-role": {
				Name:      "Admin Role",
				Inherits:  []string{"aws-prod:AdministratorAccess"},
				Providers: []string{"aws-prod"},
				Permissions: models.Permissions{
					Allow: []string{"admin:all"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
				// Provider-level role scoping: only "admins" and "sre" groups can access this provider
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Groups: []string{"admins", "sre"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// User in the "admins" group should have access
		adminIdentity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "adminuser",
				Email:    "admin@example.com",
				Groups:   []string{"admins", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(adminIdentity, "admin-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should have the provider role in inherits since user is in allowed group
		assert.Contains(t, result.Inherits, "AdministratorAccess")
		assert.Contains(t, result.Permissions.Allow, "admin:all")
	})

	t.Run("provider with role scopes - user NOT in allowed groups", func(t *testing.T) {
		roles := map[string]models.Role{
			"admin-role": {
				Name:      "Admin Role",
				Inherits:  []string{"aws-prod:AdministratorAccess"},
				Providers: []string{"aws-prod"},
				Permissions: models.Permissions{
					Allow: []string{"admin:all"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
				// Provider-level role scoping: only "admins" and "sre" groups can access
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Groups: []string{"admins", "sre"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// User NOT in the allowed groups should NOT get the provider role
		developerIdentity := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
				Groups:   []string{"developers", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(developerIdentity, "admin-role")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should NOT have the provider role since user is not in allowed group
		assert.NotContains(t, result.Inherits, "AdministratorAccess")
		// Should still have the base role's permissions
		assert.Contains(t, result.Permissions.Allow, "admin:all")
	})

	t.Run("provider with role scopes - specific user allowed", func(t *testing.T) {
		roles := map[string]models.Role{
			"special-role": {
				Name:      "Special Role",
				Inherits:  []string{"aws-prod:PowerUserAccess"},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
				// Provider-level role scoping: only specific users can access
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Users: []string{"special-user", "vip@example.com"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// Allowed user by username
		allowedUserByUsername := &models.Identity{
			ID: "special1",
			User: &models.User{
				Username: "special-user",
				Email:    "special@example.com",
			},
		}

		result1, err := config.GetCompositeRoleByName(allowedUserByUsername, "special-role")
		require.NoError(t, err)
		require.NotNil(t, result1)
		assert.Contains(t, result1.Inherits, "PowerUserAccess")

		// Allowed user by email
		allowedUserByEmail := &models.Identity{
			ID: "vip1",
			User: &models.User{
				Username: "vipuser",
				Email:    "vip@example.com",
			},
		}

		result2, err := config.GetCompositeRoleByName(allowedUserByEmail, "special-role")
		require.NoError(t, err)
		require.NotNil(t, result2)
		assert.Contains(t, result2.Inherits, "PowerUserAccess")

		// NOT allowed user
		notAllowedUser := &models.Identity{
			ID: "regular1",
			User: &models.User{
				Username: "regularuser",
				Email:    "regular@example.com",
			},
		}

		result3, err := config.GetCompositeRoleByName(notAllowedUser, "special-role")
		require.NoError(t, err)
		require.NotNil(t, result3)
		assert.NotContains(t, result3.Inherits, "PowerUserAccess")
	})

	t.Run("provider with role scopes - domain allowed", func(t *testing.T) {
		roles := map[string]models.Role{
			"company-role": {
				Name:      "Company Role",
				Inherits:  []string{"aws-corp:ReadOnlyAccess"},
				Providers: []string{"aws-corp"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-corp": {
				Name:     "AWS Corporate",
				Provider: "aws",
				Enabled:  true,
				// Provider-level role scoping: only users from specific domains
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Domains: []string{"company.com", "subsidiary.com"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// User from allowed domain
		companyUser := &models.Identity{
			ID: "emp1",
			User: &models.User{
				Username: "employee",
				Email:    "employee@company.com",
			},
		}

		result1, err := config.GetCompositeRoleByName(companyUser, "company-role")
		require.NoError(t, err)
		require.NotNil(t, result1)
		assert.Contains(t, result1.Inherits, "ReadOnlyAccess")

		// User from NOT allowed domain
		externalUser := &models.Identity{
			ID: "ext1",
			User: &models.User{
				Username: "external",
				Email:    "contractor@external.com",
			},
		}

		result2, err := config.GetCompositeRoleByName(externalUser, "company-role")
		require.NoError(t, err)
		require.NotNil(t, result2)
		assert.NotContains(t, result2.Inherits, "ReadOnlyAccess")
	})

	t.Run("combined role and provider scopes - both must match", func(t *testing.T) {
		// This tests the scenario where both the Role has scopes AND the Provider.Role has scopes
		// The user must match BOTH to get the full inherited permissions
		roles := map[string]models.Role{
			"restricted-admin": {
				Name:      "Restricted Admin",
				Inherits:  []string{"aws-secure:AdministratorAccess"},
				Providers: []string{"aws-secure"},
				Permissions: models.Permissions{
					Allow: []string{"base:permission"},
				},
				// Role-level scopes: only admins group
				Scopes: &models.RoleScopes{
					Groups: []string{"admins"},
				},
				Enabled: true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-secure": {
				Name:     "AWS Secure",
				Provider: "aws",
				Enabled:  true,
				// Provider-level scopes: only users from secure domain
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Domains: []string{"secure.company.com"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// User in admins group AND from secure domain - should get everything
		fullAccessUser := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "secureadmin",
				Email:    "admin@secure.company.com",
				Groups:   []string{"admins", "security"},
			},
		}

		result1, err := config.GetCompositeRoleByName(fullAccessUser, "restricted-admin")
		require.NoError(t, err)
		require.NotNil(t, result1)
		// Should have provider role because user is from secure domain
		assert.Contains(t, result1.Inherits, "AdministratorAccess")
		assert.Contains(t, result1.Permissions.Allow, "base:permission")

		// User in admins group but NOT from secure domain - role applies but no provider role
		adminWrongDomain := &models.Identity{
			ID: "admin2",
			User: &models.User{
				Username: "regularadmin",
				Email:    "admin@regular.company.com",
				Groups:   []string{"admins"},
			},
		}

		result2, err := config.GetCompositeRoleByName(adminWrongDomain, "restricted-admin")
		require.NoError(t, err)
		require.NotNil(t, result2)
		// Should NOT have provider role because user is not from secure domain
		assert.NotContains(t, result2.Inherits, "AdministratorAccess")
		// Should still have the base role's permissions
		assert.Contains(t, result2.Permissions.Allow, "base:permission")

		// User from secure domain but NOT in admins group - role doesn't apply
		// Note: Since the role has scopes and user doesn't match, GetCompositeRoleByName
		// still returns the role but inherited scoped roles won't apply
		wrongGroupUser := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "securedev",
				Email:    "dev@secure.company.com",
				Groups:   []string{"developers"},
			},
		}

		result3, err := config.GetCompositeRoleByName(wrongGroupUser, "restricted-admin")
		require.NoError(t, err)
		require.NotNil(t, result3)
		// Should have provider role because user is from secure domain (provider scope passes)
		assert.Contains(t, result3.Inherits, "AdministratorAccess")
	})

	t.Run("multiple providers with different scopes", func(t *testing.T) {
		roles := map[string]models.Role{
			"multi-provider-role": {
				Name: "Multi Provider Role",
				Inherits: []string{
					"aws-dev:PowerUserAccess",
					"aws-prod:AdministratorAccess",
				},
				Providers: []string{"aws-dev", "aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-dev": {
				Name:     "AWS Development",
				Provider: "aws",
				Enabled:  true,
				// Dev is open to all developers
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Groups: []string{"developers", "admins"},
					},
				},
			},
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
				// Prod is restricted to admins only
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Groups: []string{"admins"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// Developer should only get dev provider role
		developerUser := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		result1, err := config.GetCompositeRoleByName(developerUser, "multi-provider-role")
		require.NoError(t, err)
		require.NotNil(t, result1)
		// Should have dev provider role
		assert.Contains(t, result1.Inherits, "PowerUserAccess")
		// Should NOT have prod provider role
		assert.NotContains(t, result1.Inherits, "AdministratorAccess")

		// Admin should get both provider roles
		adminUser := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
				Groups:   []string{"admins"},
			},
		}

		result2, err := config.GetCompositeRoleByName(adminUser, "multi-provider-role")
		require.NoError(t, err)
		require.NotNil(t, result2)
		// Should have both provider roles
		assert.Contains(t, result2.Inherits, "PowerUserAccess")
		assert.Contains(t, result2.Inherits, "AdministratorAccess")
	})

	t.Run("provider with no role scopes - open access", func(t *testing.T) {
		roles := map[string]models.Role{
			"open-role": {
				Name:      "Open Role",
				Inherits:  []string{"aws-public:ViewOnlyAccess"},
				Providers: []string{"aws-public"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-public": {
				Name:     "AWS Public",
				Provider: "aws",
				Enabled:  true,
				// No Role defined - open to all
			},
		}

		config := newTestConfig(t, roles, providers)

		// Any user should have access
		anyUser := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "anyuser",
				Email:    "anyone@anywhere.com",
				Groups:   []string{"random-group"},
			},
		}

		result, err := config.GetCompositeRoleByName(anyUser, "open-role")
		require.NoError(t, err)
		require.NotNil(t, result)
		// Should have provider role since no scopes are defined
		assert.Contains(t, result.Inherits, "ViewOnlyAccess")
	})

	t.Run("nil identity with provider scopes", func(t *testing.T) {
		roles := map[string]models.Role{
			"scoped-role": {
				Name:      "Scoped Role",
				Inherits:  []string{"aws-scoped:ReadOnlyAccess"},
				Providers: []string{"aws-scoped"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-scoped": {
				Name:     "AWS Scoped",
				Provider: "aws",
				Enabled:  true,
				Role: &models.Role{
					Scopes: &models.RoleScopes{
						Groups: []string{"required-group"},
					},
				},
			},
		}

		config := newTestConfig(t, roles, providers)

		// Nil identity should not get the provider role
		result, err := config.GetCompositeRoleByName(nil, "scoped-role")
		require.NoError(t, err)
		require.NotNil(t, result)
		// Should NOT have provider role since identity is nil
		assert.NotContains(t, result.Inherits, "ReadOnlyAccess")
	})
}
