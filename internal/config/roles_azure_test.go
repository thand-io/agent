package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// stmts converts a []string to models.Statements for test convenience
func stmtsAzure(ops ...string) models.Statements {
	if len(ops) == 0 {
		return nil
	}
	return models.Statements{{Operations: ops}}
}

// TestAzureRoles tests Azure-specific role configurations based on config/roles/azure.yaml
func TestAzureRoles(t *testing.T) {
	// Azure role definitions based on config/roles/azure.yaml
	azureRoles := map[string]models.Role{
		"azure_admin": {
			Name:        "Azure Admin",
			Description: "Full access to all resources and capabilities.",
			Workflows: []string{
				"slack_approval",
			},
			Inherits: []string{
				"custom_storage_admin",
			},
			Permissions: models.Permissions{
				Allow: stmts(
					"Microsoft.Compute/*/read",
					"Microsoft.Compute/availabilitySets/*",
					"Microsoft.Compute/proximityPlacementGroups/*",
					"Microsoft.Compute/virtualMachines/*",
					"Microsoft.Compute/disks/*",
				),
			},
			Resources: models.Resources{
				Allow: []string{
					"azure:*",
				},
			},
			Providers: []string{
				"azure-prod",
			},
			Enabled: true,
		},
		"custom_storage_admin": {
			Name:        "Custom Storage Admin",
			Description: "Custom role for storage administration",
			Permissions: models.Permissions{
				Allow: stmts(
					"Microsoft.Storage/storageAccounts/blobServices/containers/*",
					"Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action",
				),
			},
			Enabled: true,
		},
	}

	// Azure providers
	azureProviders := map[string]models.ProviderConfig{
		"azure-prod": {
			Name:        "azure-prod",
			Description: "Azure Production Environment",
			Provider:    "azure",
		},
	}

	t.Run("azure_admin role composition", func(t *testing.T) {
		config := newTestConfig(t, azureRoles, azureProviders)

		identity := &models.Identity{
			ID: "azure-admin-user",
			User: &models.User{
				Username: "azureadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "azure_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify basic properties
		assert.Equal(t, "Azure Admin", result.Name)
		assert.Equal(t, "Full access to all resources and capabilities.", result.Description)
		assert.True(t, result.Enabled)

		// Should have merged permissions from both azure_admin and inherited "Azure Container Storage Owner"
		// Since "Azure Container Storage Owner" is defined in the test roles (not a real Azure built-in role),
		// it should be resolved and merged
		expectedPermissions := []string{
			"Microsoft.Compute/*/read",
			"Microsoft.Compute/availabilitySets/*",
			"Microsoft.Compute/proximityPlacementGroups/*",
			"Microsoft.Compute/virtualMachines/*",
			"Microsoft.Compute/disks/*",
			"Microsoft.Storage/storageAccounts/blobServices/containers/*",
			"Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action",
		}
		assert.ElementsMatch(t, expectedPermissions, result.Permissions.Allow)

		// The inherited role should be removed from Inherits list after being merged
		assert.Empty(t, result.Inherits)

		// Verify resources - azure:* becomes * since azure matches allowed providers
		assert.ElementsMatch(t, []string{"*"}, result.Resources.Allow)

		// Verify providers
		assert.ElementsMatch(t, []string{"azure-prod"}, result.Providers)

		// Verify workflows
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)
	})
}

// TestAzureRoleScenarios tests realistic Azure role usage scenarios
func TestAzureRoleScenarios(t *testing.T) {
	t.Run("azure developer with resource group scoping", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_developer": {
				Name:        "Azure Developer",
				Description: "Developer access to Azure resources",
				Permissions: models.Permissions{
					Allow: stmts(
						"Microsoft.Compute/virtualMachines/read",
						"Microsoft.Compute/virtualMachines/start/action",
						"Microsoft.Compute/virtualMachines/restart/action",
						"Microsoft.Storage/storageAccounts/blobServices/containers/read",
						"Microsoft.Storage/storageAccounts/blobServices/containers/write",
						"Microsoft.Web/sites/*",
					),
				},
				Resources: models.Resources{
					Allow: []string{
						"/subscriptions/*/resourceGroups/dev-*",
						"/subscriptions/*/resourceGroups/staging-*",
					},
					Deny: []string{
						"/subscriptions/*/resourceGroups/prod-*",
					},
				},
				Scopes: &models.RoleScopes{
					Groups: []string{"developers", "engineers"},
				},
				Providers: []string{"azure-dev", "azure-staging"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-dev": {
				Name:        "azure-dev",
				Description: "Azure Development Environment",
				Provider:    "azure",
			},
			"azure-staging": {
				Name:        "azure-staging",
				Description: "Azure Staging Environment",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "developer1",
				Email:    "dev1@example.com",
				Groups:   []string{"developers", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "azure_developer")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Azure Developer", result.Name)
		assert.ElementsMatch(t, []string{
			"Microsoft.Compute/virtualMachines/read",
			"Microsoft.Compute/virtualMachines/start/action",
			"Microsoft.Compute/virtualMachines/restart/action",
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
			"Microsoft.Storage/storageAccounts/blobServices/containers/write",
			"Microsoft.Web/sites/*",
		}, result.Permissions.Allow)

		assert.ElementsMatch(t, []string{
			"/subscriptions/*/resourceGroups/dev-*",
			"/subscriptions/*/resourceGroups/staging-*",
		}, result.Resources.Allow)

		assert.ElementsMatch(t, []string{
			"/subscriptions/*/resourceGroups/prod-*",
		}, result.Resources.Deny)

		assert.ElementsMatch(t, []string{"azure-dev", "azure-staging"}, result.Providers)
	})

	t.Run("azure rbac inheritance pattern", func(t *testing.T) {
		roles := map[string]models.Role{
			"custom_reader": {
				Name:        "Custom Reader",
				Description: "Custom read-only role",
				Permissions: models.Permissions{
					Allow: stmts(
						"*/read",
					),
				},
				Enabled: true,
			},
			"custom_blob_reader": {
				Name:        "Custom Blob Reader",
				Description: "Custom storage blob reading role",
				Permissions: models.Permissions{
					Allow: stmts(
						"Microsoft.Storage/storageAccounts/blobServices/containers/read",
						"Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action",
						"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read",
					),
				},
				Enabled: true,
			},
			"custom_analyst": {
				Name:        "Data Analyst",
				Description: "Custom role for data analysts",
				Inherits: []string{
					"custom_reader",
					"custom_blob_reader",
				},
				Permissions: models.Permissions{
					Allow: stmts(
						"Microsoft.DataFactory/datafactories/read",
						"Microsoft.DataFactory/factories/read",
					),
				},
				Scopes: &models.RoleScopes{
					Groups: []string{"analysts", "data-team"},
				},
				Providers: []string{"azure-analytics"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-analytics": {
				Name:        "azure-analytics",
				Description: "Azure Analytics Environment",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "analyst1",
			User: &models.User{
				Username: "analyst1",
				Email:    "analyst1@example.com",
				Groups:   []string{"analysts", "data-team"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "custom_analyst")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should have merged permissions from all inherited roles
		// Since "Reader" and "Storage Blob Data Reader" are defined in the test roles,
		// they should be resolved and merged
		expectedAllowPerms := []string{
			// from custom_analyst
			"Microsoft.DataFactory/datafactories/read",
			"Microsoft.DataFactory/factories/read",
			// from Reader
			"*/read",
			// from Storage Blob Data Reader
			"Microsoft.Storage/storageAccounts/blobServices/containers/read",
			"Microsoft.Storage/storageAccounts/blobServices/generateUserDelegationKey/action",
			"Microsoft.Storage/storageAccounts/blobServices/containers/blobs/read",
		}
		assert.ElementsMatch(t, expectedAllowPerms, result.Permissions.Allow)
		assert.ElementsMatch(t, []string{"azure-analytics"}, result.Providers)

		// The inherited roles should be removed from Inherits list after being merged
		assert.Empty(t, result.Inherits)
	})

	t.Run("azure subscription scoped role", func(t *testing.T) {
		roles := map[string]models.Role{
			"subscription_admin": {
				Name:        "Subscription Admin",
				Description: "Admin access to specific Azure subscriptions",
				Permissions: models.Permissions{
					Allow: stmts(
						"*",
					),
					Deny: stmts(
						"Microsoft.Authorization/*/Delete",
						"Microsoft.Authorization/*/Write",
						"Microsoft.Authorization/elevateAccess/Action",
					),
				},
				Resources: models.Resources{
					Allow: []string{
						"/subscriptions/12345678-1234-1234-1234-123456789abc",
						"/subscriptions/87654321-4321-4321-4321-cba987654321",
					},
				},
				Scopes: &models.RoleScopes{
					Users: []string{
						"subscription-admin@example.com",
					},
				},
				Providers: []string{"azure-subscriptions"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-subscriptions": {
				Name:        "azure-subscriptions",
				Description: "Azure Subscription Management",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "sub-admin",
			User: &models.User{
				Username: "subadmin",
				Email:    "subscription-admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "subscription_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Subscription Admin", result.Name)
		assert.ElementsMatch(t, []string{"*"}, result.Permissions.Allow)
		assert.ElementsMatch(t, []string{
			"Microsoft.Authorization/*/Delete",
			"Microsoft.Authorization/*/Write",
			"Microsoft.Authorization/elevateAccess/Action",
		}, result.Permissions.Deny)

		assert.ElementsMatch(t, []string{
			"/subscriptions/12345678-1234-1234-1234-123456789abc",
			"/subscriptions/87654321-4321-4321-4321-cba987654321",
		}, result.Resources.Allow)

		assert.ElementsMatch(t, []string{"azure-subscriptions"}, result.Providers)
	})
}
