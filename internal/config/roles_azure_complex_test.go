package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestAzureComplexInheritance exercises complex, deeply-nested Azure role
// inheritance with Azure built-in role resolution (via the embedded IAM
// dataset), scope allow/deny filtering, composite-role marking, permission
// merging, deny-permission survival, and target preservation across 1-3
// levels of thand-role nesting.
//
// Azure built-in roles used throughout:
//   - Contributor
//   - Reader
//   - Owner
//   - Storage Blob Data Owner
//   - Storage Blob Data Reader
//   - Storage Blob Data Contributor
//   - Virtual Machine Contributor
//   - Network Contributor
//   - Key Vault Administrator
//   - Key Vault Reader
//   - Monitoring Contributor
//   - Monitoring Reader
//   - Log Analytics Contributor
//   - Security Admin
//   - Security Reader
//   - SQL DB Contributor
//   - SQL Server Contributor
//   - AcrPush
//   - AcrPull
//   - User Access Administrator
//   - Data Factory Contributor
//   - DNS Zone Contributor
//   - Managed Identity Operator
func TestAzureComplexInheritance(t *testing.T) {
	// Shared Azure providers used by most subtests.
	azureProviders := map[string]models.ProviderConfig{
		"azure-prod": {
			Name:        "azure-prod",
			Description: "Azure Production",
			Provider:    "azure",
		},
		"azure-dev": {
			Name:        "azure-dev",
			Description: "Azure Development",
			Provider:    "azure",
		},
	}

	// 1. Provider-only inheritance is NOT composite (depth 1)
	t.Run("provider-only inheritance is not composite", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_infra_viewer": {
				Name:        "Azure Infra Viewer",
				Description: "Inherits only Azure built-in roles, no thand roles",
				Inherits: []string{
					"Reader",
					"Monitoring Reader",
					"Security Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/read",
						"Microsoft.Network/virtualNetworks/read",
					),
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, azureProviders)

		identity := &models.Identity{
			ID: "viewer1",
			User: &models.User{
				Username: "viewer",
				Email:    "viewer@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "azure_infra_viewer")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.False(t, result.Composite,
			"Role inheriting only Azure built-in roles should NOT be composite")

		assert.ElementsMatch(t, []string{
			"Reader",
			"Monitoring Reader",
			"Security Reader",
		}, result.Inherits,
			"Azure built-in roles should be preserved in Inherits: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/read",
			"Own permission should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Network/virtualNetworks/read",
			"Own permission should be present: got %v", allowOps)

		assert.ElementsMatch(t, []string{"azure-prod", "azure-dev"}, result.Providers)
	})

	// 2. Thand role + multiple Azure built-in roles (depth 1)
	t.Run("thand role plus multiple Azure built-in roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_base_ops": {
				Name:        "Azure Base Ops",
				Description: "Baseline Azure operational permissions",
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Authorization/roleAssignments/read",
						"Microsoft.Insights/diagnosticSettings/read",
					),
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
			"azure_senior_ops": {
				Name:        "Azure Senior Ops",
				Description: "Senior operator with thand + Azure built-in role inheritance",
				Inherits: []string{
					"azure_base_ops",
					"Virtual Machine Contributor",
					"Network Contributor",
					"Storage Blob Data Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/read",
						"Microsoft.Compute/virtualMachines/start/action",
						"Microsoft.Storage/storageAccounts/read",
					),
					Deny: stmtsAzure(
						"Microsoft.Compute/virtualMachines/delete",
						"Microsoft.Storage/storageAccounts/delete",
					),
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, azureProviders)

		identity := &models.Identity{
			ID: "ops-senior",
			User: &models.User{
				Username: "senior",
				Email:    "senior@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "azure_senior_ops")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Role inheriting from a thand role should be composite")

		assert.Len(t, result.Inherits, 3,
			"Exactly 3 Azure built-in roles should be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Virtual Machine Contributor",
			"Virtual Machine Contributor must be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Network Contributor",
			"Network Contributor must be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Storage Blob Data Contributor",
			"Storage Blob Data Contributor must be in Inherits: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// Own permissions
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/read",
			"own compute read should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/start/action",
			"own compute start should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/read",
			"own storage read should be present: got %v", allowOps)

		// Inherited from azure_base_ops
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/read",
			"base ops subscription read should be merged: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Authorization/roleAssignments/read",
			"base ops role assignments read should be merged: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Insights/diagnosticSettings/read",
			"base ops diagnostic settings read should be merged: got %v", allowOps)

		// Deny permissions survive
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Compute/virtualMachines/delete",
			"deny compute delete should survive: got %v", denyOps)
		assert.Contains(t, denyOps, "Microsoft.Storage/storageAccounts/delete",
			"deny storage delete should survive: got %v", denyOps)

		assert.ElementsMatch(t, []string{"azure-prod", "azure-dev"}, result.Providers)
		assert.Equal(t, "Azure Senior Ops", result.Name)
		assert.True(t, result.Enabled)
	})

	// 3. Two-level deep with group and domain scopes (depth 2)
	t.Run("two-level deep with group and domain scopes", func(t *testing.T) {
		roles := map[string]models.Role{
			"level0_azure_viewer": {
				Name:        "Level-0 Azure Viewer",
				Description: "Base viewer scoped to example.com domain",
				Inherits: []string{
					"Reader",
					"Key Vault Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Resources/subscriptions/resourceGroups/read",
						"Microsoft.KeyVault/vaults/read",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
			"level1_azure_developer": {
				Name:        "Level-1 Azure Developer",
				Description: "Developer inheriting viewer + VM contributor, scoped to devs group",
				Inherits: []string{
					"level0_azure_viewer",
					"Virtual Machine Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/start/action",
						"Microsoft.Compute/virtualMachines/restart/action",
						"Microsoft.Storage/storageAccounts/blobServices/containers/read",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, azureProviders)

		identity := &models.Identity{
			ID: "dev-user",
			User: &models.User{
				Username: "developer1",
				Email:    "dev@example.com",
				Groups:   []string{"developers", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "level1_azure_developer")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Two-level thand inheritance should be composite")

		assert.ElementsMatch(t, []string{
			"Reader",
			"Key Vault Reader",
			"Virtual Machine Contributor",
		}, result.Inherits,
			"Azure built-in roles from both inheritance levels should accumulate in Inherits: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// level1 own perms
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/start/action",
			"level1 own perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/restart/action",
			"level1 own perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/blobServices/containers/read",
			"level1 own perm should be present: got %v", allowOps)

		// level0 inherited perms
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/read",
			"level0 perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/resourceGroups/read",
			"level0 perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.KeyVault/vaults/read",
			"level0 perm should be present: got %v", allowOps)

		assert.ElementsMatch(t, []string{"azure-prod", "azure-dev"}, result.Providers)
	})

	// 4. Two-level scope denial skips middle role (depth 2)
	t.Run("two-level scope denial skips middle role", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_base_perms": {
				Name:        "Azure Base Perms",
				Description: "Open base permissions",
				Inherits: []string{
					"Monitoring Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Insights/metrics/read",
						"Microsoft.Insights/alertRules/read",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_restricted_layer": {
				Name:        "Azure Restricted Layer",
				Description: "High-value perms with built-in roles, deny-scoped to outsiders",
				Inherits: []string{
					"azure_base_perms",
					"Contributor",
					"Key Vault Administrator",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.KeyVault/vaults/*",
						"Microsoft.Storage/storageAccounts/*",
					),
				},
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Users: []string{"outsider@example.com"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_top_role": {
				Name:        "Azure Top Role",
				Description: "Top-level role inheriting the restricted layer",
				Inherits:    []string{"azure_restricted_layer"},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Insights/diagnosticSettings/read"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:        "azure-prod",
				Description: "Azure Production",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Outsider should be scope-denied from the restricted layer
		outsider := &models.Identity{
			ID: "outsider1",
			User: &models.User{
				Username: "outsider",
				Email:    "outsider@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(outsider, "azure_top_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectOpsAzure(result.Permissions.Allow)
		assert.ElementsMatch(t, []string{"Microsoft.Insights/diagnosticSettings/read"}, allowOps,
			"Only azure_top_role's own perms should remain when middle role is scope-denied: got %v", allowOps)

		for _, inh := range result.Inherits {
			assert.NotEqual(t, "Contributor", inh,
				"Contributor should not propagate through scope-denied chain: got %v", result.Inherits)
			assert.NotEqual(t, "Key Vault Administrator", inh,
				"Key Vault Administrator should not propagate through scope-denied chain: got %v", result.Inherits)
			assert.NotEqual(t, "Monitoring Reader", inh,
				"Monitoring Reader from base should not propagate through scope-denied chain: got %v", result.Inherits)
		}

		assert.False(t, result.Composite,
			"Role should NOT be composite when inherited role is scope-denied")

		// Allowed user should get everything
		allowed := &models.Identity{
			ID: "emp1",
			User: &models.User{
				Username: "employee",
				Email:    "employee@example.com",
			},
		}

		resultAllowed, err := config.GetCompositeRoleByName(allowed, "azure_top_role")
		require.NoError(t, err)
		require.NotNil(t, resultAllowed)

		allowedOps := collectOpsAzure(resultAllowed.Permissions.Allow)
		assert.Contains(t, allowedOps, "Microsoft.Insights/diagnosticSettings/read",
			"Allowed user should get top role perm: got %v", allowedOps)
		assert.Contains(t, allowedOps, "Microsoft.Compute/virtualMachines/*",
			"Allowed user should get restricted layer compute perms: got %v", allowedOps)
		assert.Contains(t, allowedOps, "Microsoft.KeyVault/vaults/*",
			"Allowed user should get restricted layer keyvault perms: got %v", allowedOps)
		assert.Contains(t, allowedOps, "Microsoft.Insights/metrics/read",
			"Allowed user should get base perms: got %v", allowedOps)

		assert.Contains(t, resultAllowed.Inherits, "Contributor",
			"Allowed user should get Contributor: got %v", resultAllowed.Inherits)
		assert.Contains(t, resultAllowed.Inherits, "Key Vault Administrator",
			"Allowed user should get Key Vault Administrator: got %v", resultAllowed.Inherits)
		assert.Contains(t, resultAllowed.Inherits, "Monitoring Reader",
			"Allowed user should get Monitoring Reader from base: got %v", resultAllowed.Inherits)

		assert.True(t, resultAllowed.Composite,
			"Allowed user result should be composite since thand roles were merged")
	})

	// 5. Three-level deep with mixed Azure built-in + thand roles (depth 3)
	t.Run("three-level deep mixed Azure built-in and thand roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"tier0_azure_baseline": {
				Name:        "Tier-0 Azure Baseline",
				Description: "Foundation role with Reader + Security Reader",
				Inherits: []string{
					"Reader",
					"Security Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Authorization/roleAssignments/read",
					),
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
			"tier1_azure_team": {
				Name:        "Tier-1 Azure Team",
				Description: "Team role inheriting baseline + storage roles, scoped to engineering",
				Inherits: []string{
					"tier0_azure_baseline",
					"Storage Blob Data Reader",
					"Monitoring Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"Microsoft.Storage/storageAccounts/blobServices/containers/read",
							"Microsoft.Storage/storageAccounts/read",
							"Microsoft.Compute/virtualMachines/read",
						},
						Targets: []string{
							"/subscriptions/*/resourceGroups/team-*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
			"tier2_azure_lead": {
				Name:        "Tier-2 Azure Lead",
				Description: "Team lead with Key Vault + SQL admin built-in roles",
				Inherits: []string{
					"tier1_azure_team",
					"Key Vault Administrator",
					"SQL DB Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/start/action",
						"Microsoft.Compute/virtualMachines/restart/action",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"lead@example.com"},
					},
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, azureProviders)

		identity := &models.Identity{
			ID: "lead1",
			User: &models.User{
				Username: "teamlead",
				Email:    "lead@example.com",
				Groups:   []string{"engineering", "leads"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "tier2_azure_lead")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		assert.Len(t, result.Inherits, 6,
			"All 6 Azure built-in roles from all tiers should be present: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Reader",
			"Reader from tier0 should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Security Reader",
			"Security Reader from tier0 should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Storage Blob Data Reader",
			"Storage Blob Data Reader from tier1 should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Monitoring Contributor",
			"Monitoring Contributor from tier1 should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Key Vault Administrator",
			"Key Vault Administrator from tier2: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "SQL DB Contributor",
			"SQL DB Contributor from tier2: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)
		// tier2 own perms
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/start/action",
			"tier2 perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/restart/action",
			"tier2 perm: got %v", allowOps)

		// tier0 perms should merge through chain
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/read",
			"tier0 perm should merge through chain: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Authorization/roleAssignments/read",
			"tier0 perm should merge through chain: got %v", allowOps)

		// tier1 perms should be merged
		foundStorage := false
		foundCompute := false
		for _, op := range allowOps {
			if op == "Microsoft.Storage/storageAccounts/blobServices/containers/read" || op == "Microsoft.Storage/storageAccounts/read" {
				foundStorage = true
			}
			if op == "Microsoft.Compute/virtualMachines/read" {
				foundCompute = true
			}
		}
		assert.True(t, foundStorage,
			"tier1 storage perms should be merged: got %v", allowOps)
		assert.True(t, foundCompute,
			"tier1 compute perms should be merged: got %v", allowOps)

		// Verify targets from tier1 are preserved
		var teamTargets []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if op == "Microsoft.Storage/storageAccounts/blobServices/containers/read" ||
					op == "Microsoft.Storage/storageAccounts/read" ||
					op == "Microsoft.Compute/virtualMachines/read" {
					if len(stmt.Targets) > 0 {
						teamTargets = stmt.Targets
					}
					break
				}
			}
		}
		assert.ElementsMatch(t, []string{"/subscriptions/*/resourceGroups/team-*"}, teamTargets,
			"Targets from tier1 should be preserved: got %v", teamTargets)

		assert.ElementsMatch(t, []string{"azure-prod", "azure-dev"}, result.Providers)
	})

	// 6. Three-level with deny permissions across the chain (depth 3)
	t.Run("three-level deny permissions across chain", func(t *testing.T) {
		roles := map[string]models.Role{
			"l0_azure_base": {
				Name:        "L0 Azure Base",
				Description: "Base level with specific compute and storage read perms",
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/read",
						"Microsoft.Compute/virtualMachines/instanceView/read",
						"Microsoft.Storage/storageAccounts/read",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"l1_azure_power": {
				Name:        "L1 Azure Power User",
				Description: "Power user with compute wildcard + deny delete, inherits built-in",
				Inherits: []string{
					"l0_azure_base",
					"Storage Blob Data Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.Sql/servers/databases/read",
					),
					Deny: stmtsAzure(
						"Microsoft.Compute/virtualMachines/delete",
						"Microsoft.Sql/servers/databases/delete",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"l2_azure_admin": {
				Name:        "L2 Azure Admin",
				Description: "Admin with SQL wildcard + deny critical mutations",
				Inherits: []string{
					"l1_azure_power",
					"SQL Server Contributor",
					"Key Vault Administrator",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Sql/servers/*",
						"Microsoft.KeyVault/vaults/*",
					),
					Deny: stmtsAzure(
						"Microsoft.Authorization/*/Write",
						"Microsoft.Authorization/*/Delete",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:        "azure-prod",
				Description: "Azure Production",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "azureadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "l2_azure_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// Built-in roles from l1 and l2 should be in Inherits
		assert.Contains(t, result.Inherits, "Storage Blob Data Contributor",
			"Storage Blob Data Contributor from l1 should propagate: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "SQL Server Contributor",
			"SQL Server Contributor from l2 should be present: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Key Vault Administrator",
			"Key Vault Administrator from l2 should be present: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// l2 own permissions
		assert.Contains(t, allowOps, "Microsoft.Sql/servers/*",
			"l2 sql wildcard should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.KeyVault/vaults/*",
			"l2 keyvault wildcard should be present: got %v", allowOps)

		// l1 compute wildcard should be present
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/*",
			"l1 compute wildcard should be present: got %v", allowOps)

		// l0 storage read should survive
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/read",
			"l0 storage read should survive: got %v", allowOps)

		// Deny permissions
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Authorization/*/Write",
			"l2 auth write deny should survive: got %v", denyOps)
		assert.Contains(t, denyOps, "Microsoft.Authorization/*/Delete",
			"l2 auth delete deny should survive: got %v", denyOps)

		assert.ElementsMatch(t, []string{"azure-prod"}, result.Providers)
	})

	// 7. Three-level with targets preserved across the chain (depth 3)
	t.Run("three-level targets preserved across chain", func(t *testing.T) {
		roles := map[string]models.Role{
			"t0_azure_storage_reader": {
				Name:        "T0 Azure Storage Reader",
				Description: "Storage reader scoped to data resource groups",
				Inherits: []string{
					"Storage Blob Data Reader",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"Microsoft.Storage/storageAccounts/blobServices/containers/read",
							"Microsoft.Storage/storageAccounts/read",
						},
						Targets: []string{
							"/subscriptions/*/resourceGroups/data-*",
						},
					}},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"t1_azure_storage_writer": {
				Name:        "T1 Azure Storage Writer",
				Description: "Storage writer scoped to uploads RGs, inherits reader",
				Inherits:    []string{"t0_azure_storage_reader"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"Microsoft.Storage/storageAccounts/blobServices/containers/write",
							"Microsoft.Storage/storageAccounts/blobServices/containers/delete",
						},
						Targets: []string{
							"/subscriptions/*/resourceGroups/uploads-*",
						},
					}},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"t2_azure_storage_admin": {
				Name:        "T2 Azure Storage Admin",
				Description: "Full storage admin with broad wildcard",
				Inherits:    []string{"t1_azure_storage_writer"},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Storage/storageAccounts/*"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:        "azure-prod",
				Description: "Azure Production",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "storageadmin1",
			User: &models.User{
				Username: "storageadmin",
				Email:    "storageadmin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "t2_azure_storage_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		assert.ElementsMatch(t, []string{"Storage Blob Data Reader"}, result.Inherits,
			"Storage Blob Data Reader from t0 should propagate through inheritance chain: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/*",
			"t2 storage wildcard should be present: got %v", allowOps)

		assert.ElementsMatch(t, []string{"azure-prod"}, result.Providers)
	})

	// 8. One inherited role allowed, another denied at same level (depth 1)
	t.Run("one inherited role allowed and another denied at same level", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_open_base": {
				Name:        "Azure Open Base",
				Description: "Open to all, no scope, inherits Reader",
				Inherits: []string{
					"Reader",
					"Monitoring Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Insights/metrics/read",
						"Microsoft.Insights/alertRules/read",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_restricted_admin": {
				Name:        "Azure Restricted Admin",
				Description: "Admin perms restricted to admins group, inherits Contributor",
				Inherits: []string{
					"Contributor",
					"Key Vault Administrator",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.Storage/storageAccounts/*",
						"Microsoft.KeyVault/vaults/*",
						"Microsoft.Authorization/roleAssignments/write",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_combined_role": {
				Name:        "Azure Combined",
				Description: "Inherits both open_base and restricted_admin",
				Inherits: []string{
					"azure_open_base",
					"azure_restricted_admin",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Insights/diagnosticSettings/read"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Non-admin: should get open_base perms but NOT restricted_admin perms
		nonadmin := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		resultDev, err := config.GetCompositeRoleByName(nonadmin, "azure_combined_role")
		require.NoError(t, err)
		require.NotNil(t, resultDev)

		devAllowOps := collectOpsAzure(resultDev.Permissions.Allow)

		// Should have open_base perms
		assert.Contains(t, devAllowOps, "Microsoft.Resources/subscriptions/read",
			"Non-admin should get subscription read from open_base: got %v", devAllowOps)
		assert.Contains(t, devAllowOps, "Microsoft.Insights/metrics/read",
			"Non-admin should get metrics read from open_base: got %v", devAllowOps)

		// Should have combined_role's own perms
		assert.Contains(t, devAllowOps, "Microsoft.Insights/diagnosticSettings/read",
			"Non-admin should get diagnostics perm: got %v", devAllowOps)

		// Should NOT have restricted_admin perms
		for _, op := range devAllowOps {
			if op == "Microsoft.Compute/virtualMachines/*" {
				t.Errorf("compute perms from denied azure_restricted_admin must not appear for non-admin: got %v", devAllowOps)
			}
			if op == "Microsoft.Storage/storageAccounts/*" {
				t.Errorf("storage perms from denied azure_restricted_admin must not appear for non-admin: got %v", devAllowOps)
			}
			if op == "Microsoft.Authorization/roleAssignments/write" {
				t.Errorf("auth perms from denied azure_restricted_admin must not appear for non-admin: got %v", devAllowOps)
			}
		}

		// Should have built-in roles from open_base only
		assert.Contains(t, resultDev.Inherits, "Reader",
			"Non-admin should get Reader from open_base: got %v", resultDev.Inherits)
		assert.Contains(t, resultDev.Inherits, "Monitoring Reader",
			"Non-admin should get Monitoring Reader from open_base: got %v", resultDev.Inherits)

		for _, inh := range resultDev.Inherits {
			assert.NotEqual(t, "Contributor", inh,
				"Contributor must not propagate from denied restricted_admin: got %v", resultDev.Inherits)
			assert.NotEqual(t, "Key Vault Administrator", inh,
				"Key Vault Administrator must not propagate from denied restricted_admin: got %v", resultDev.Inherits)
		}

		assert.True(t, resultDev.Composite,
			"Should be composite since azure_open_base was successfully merged")

		// Admin: should get everything
		adminUser := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
				Groups:   []string{"admins", "developers"},
			},
		}

		resultAdmin, err := config.GetCompositeRoleByName(adminUser, "azure_combined_role")
		require.NoError(t, err)
		require.NotNil(t, resultAdmin)

		adminAllowOps := collectOpsAzure(resultAdmin.Permissions.Allow)

		assert.Contains(t, adminAllowOps, "Microsoft.Insights/diagnosticSettings/read",
			"Admin should get diagnostics: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "Microsoft.Compute/virtualMachines/*",
			"Admin should get compute from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "Microsoft.Storage/storageAccounts/*",
			"Admin should get storage from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "Microsoft.KeyVault/vaults/*",
			"Admin should get keyvault from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "Microsoft.Resources/subscriptions/read",
			"Admin should get subscriptions from open_base: got %v", adminAllowOps)

		assert.Contains(t, resultAdmin.Inherits, "Reader",
			"Admin should get Reader: got %v", resultAdmin.Inherits)
		assert.Contains(t, resultAdmin.Inherits, "Monitoring Reader",
			"Admin should get Monitoring Reader: got %v", resultAdmin.Inherits)
		assert.Contains(t, resultAdmin.Inherits, "Contributor",
			"Admin should get Contributor: got %v", resultAdmin.Inherits)
		assert.Contains(t, resultAdmin.Inherits, "Key Vault Administrator",
			"Admin should get Key Vault Administrator: got %v", resultAdmin.Inherits)

		assert.True(t, resultAdmin.Composite,
			"Admin result should be composite since thand roles were merged")
	})

	// 9. Deny scope takes precedence over allow scope for same user
	t.Run("deny scope takes precedence over allow scope for same user", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_privileged": {
				Name:        "Azure Privileged",
				Description: "High-privilege role with both allow and deny scopes + Azure built-in roles",
				Inherits: []string{
					"Contributor",
					"Storage Blob Data Owner",
					"Virtual Machine Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.Storage/storageAccounts/*",
						"Microsoft.KeyVault/vaults/read",
						"Microsoft.Sql/servers/databases/*",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"intern@example.com"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_wrapper": {
				Name:        "Azure Wrapper",
				Description: "Wraps privileged role",
				Inherits:    []string{"azure_privileged"},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Insights/metrics/read"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Intern: in engineering group (allow) but explicitly denied
		intern := &models.Identity{
			ID: "intern1",
			User: &models.User{
				Username: "intern",
				Email:    "intern@example.com",
				Groups:   []string{"engineering", "interns"},
			},
		}

		resultIntern, err := config.GetCompositeRoleByName(intern, "azure_wrapper")
		require.NoError(t, err)
		require.NotNil(t, resultIntern)

		internOps := collectOpsAzure(resultIntern.Permissions.Allow)

		assert.ElementsMatch(t, []string{"Microsoft.Insights/metrics/read"}, internOps,
			"Intern (deny-scoped) should only get wrapper's own perms: got %v", internOps)

		assert.Empty(t, resultIntern.Inherits,
			"Intern should not get Azure built-in roles from denied privileged role: got %v", resultIntern.Inherits)

		assert.False(t, resultIntern.Composite,
			"Intern should not get composite when privileged role was denied")

		// Engineer: should get everything
		engineer := &models.Identity{
			ID: "eng1",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEng, err := config.GetCompositeRoleByName(engineer, "azure_wrapper")
		require.NoError(t, err)
		require.NotNil(t, resultEng)

		engOps := collectOpsAzure(resultEng.Permissions.Allow)
		assert.Contains(t, engOps, "Microsoft.Compute/virtualMachines/*",
			"Engineer should get compute perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.Storage/storageAccounts/*",
			"Engineer should get storage perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.KeyVault/vaults/read",
			"Engineer should get keyvault perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.Sql/servers/databases/*",
			"Engineer should get sql perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.Insights/metrics/read",
			"Engineer should get metrics from wrapper: got %v", engOps)

		assert.Contains(t, resultEng.Inherits, "Contributor",
			"Engineer should get Contributor: got %v", resultEng.Inherits)
		assert.Contains(t, resultEng.Inherits, "Storage Blob Data Owner",
			"Engineer should get Storage Blob Data Owner: got %v", resultEng.Inherits)
		assert.Contains(t, resultEng.Inherits, "Virtual Machine Contributor",
			"Engineer should get Virtual Machine Contributor: got %v", resultEng.Inherits)

		assert.True(t, resultEng.Composite,
			"Engineer should get composite role")
	})

	// 10. Three-level: middle role scoped by domain, bottom role scoped by group (depth 3)
	t.Run("three-level: user fails middle domain scope", func(t *testing.T) {
		roles := map[string]models.Role{
			"bottom_azure_base": {
				Name:        "Bottom Azure Base",
				Description: "Foundation with Security Admin built-in role",
				Inherits: []string{
					"Security Admin",
					"Log Analytics Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Authorization/roleAssignments/read",
						"Microsoft.OperationalInsights/workspaces/read",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering", "ops"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"middle_azure_team": {
				Name:        "Middle Azure Team",
				Description: "Team role scoped to acme.com domain",
				Inherits: []string{
					"bottom_azure_base",
					"Virtual Machine Contributor",
					"Network Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/read",
						"Microsoft.Compute/virtualMachines/start/action",
						"Microsoft.Network/virtualNetworks/read",
						"Microsoft.Storage/storageAccounts/blobServices/containers/read",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"acme.com"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"top_azure_lead": {
				Name:        "Top Azure Lead",
				Description: "Lead role, no scope restriction, inherits Contributor + DNS Zone Contributor",
				Inherits: []string{
					"middle_azure_team",
					"Contributor",
					"DNS Zone Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Sql/servers/databases/read",
						"Microsoft.Sql/servers/databases/write",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// External user (not @acme.com): should fail middle domain scope
		externalUser := &models.Identity{
			ID: "ext1",
			User: &models.User{
				Username: "external",
				Email:    "user@external.com",
				Groups:   []string{"engineering"},
			},
		}

		resultExt, err := config.GetCompositeRoleByName(externalUser, "top_azure_lead")
		require.NoError(t, err)
		require.NotNil(t, resultExt)

		extAllowOps := collectOpsAzure(resultExt.Permissions.Allow)

		// Top lead's own perms should be present
		assert.Contains(t, extAllowOps, "Microsoft.Sql/servers/databases/read",
			"External user should get top lead's SQL read: got %v", extAllowOps)
		assert.Contains(t, extAllowOps, "Microsoft.Sql/servers/databases/write",
			"External user should get top lead's SQL write: got %v", extAllowOps)

		// Middle perms should NOT be present (scope-denied)
		for _, op := range extAllowOps {
			if op == "Microsoft.Compute/virtualMachines/read" || op == "Microsoft.Compute/virtualMachines/start/action" {
				t.Errorf("compute perms from scope-denied middle should not appear for external user: got %v", extAllowOps)
				break
			}
			if op == "Microsoft.Network/virtualNetworks/read" {
				t.Errorf("network perms from scope-denied middle should not appear for external user: got %v", extAllowOps)
			}
		}

		// Bottom perms should NOT leak through denied middle
		for _, op := range extAllowOps {
			if op == "Microsoft.Resources/subscriptions/read" || op == "Microsoft.Authorization/roleAssignments/read" {
				t.Errorf("bottom perms must not leak through denied middle: got %v", extAllowOps)
				break
			}
			if op == "Microsoft.OperationalInsights/workspaces/read" {
				t.Errorf("bottom operational insights perm must not leak through denied middle: got %v", extAllowOps)
			}
		}

		// Built-in roles from denied chain should not propagate
		for _, inh := range resultExt.Inherits {
			assert.NotEqual(t, "Security Admin", inh,
				"Security Admin from bottom should not propagate through denied middle: got %v", resultExt.Inherits)
			assert.NotEqual(t, "Log Analytics Contributor", inh,
				"Log Analytics Contributor from bottom should not propagate through denied middle: got %v", resultExt.Inherits)
			assert.NotEqual(t, "Virtual Machine Contributor", inh,
				"Virtual Machine Contributor from middle should not propagate when denied: got %v", resultExt.Inherits)
			assert.NotEqual(t, "Network Contributor", inh,
				"Network Contributor from middle should not propagate when denied: got %v", resultExt.Inherits)
		}

		// Top lead's own built-in roles should remain
		assert.Contains(t, resultExt.Inherits, "Contributor",
			"Top lead's own Contributor should remain: got %v", resultExt.Inherits)
		assert.Contains(t, resultExt.Inherits, "DNS Zone Contributor",
			"Top lead's own DNS Zone Contributor should remain: got %v", resultExt.Inherits)

		assert.False(t, resultExt.Composite,
			"External user should not get composite role when middle was denied")

		// Acme user (domain match): should get everything through the chain
		acmeUser := &models.Identity{
			ID: "acme1",
			User: &models.User{
				Username: "acmedev",
				Email:    "dev@acme.com",
				Groups:   []string{"engineering"},
			},
		}

		resultAcme, err := config.GetCompositeRoleByName(acmeUser, "top_azure_lead")
		require.NoError(t, err)
		require.NotNil(t, resultAcme)

		acmeAllowOps := collectOpsAzure(resultAcme.Permissions.Allow)

		// Top lead's own perms
		assert.Contains(t, acmeAllowOps, "Microsoft.Sql/servers/databases/read",
			"acme user should get SQL read: got %v", acmeAllowOps)
		// Middle team perms
		assert.Contains(t, acmeAllowOps, "Microsoft.Compute/virtualMachines/read",
			"acme user should get compute from middle: got %v", acmeAllowOps)
		assert.Contains(t, acmeAllowOps, "Microsoft.Network/virtualNetworks/read",
			"acme user should get network from middle: got %v", acmeAllowOps)
		// Bottom base perms
		assert.Contains(t, acmeAllowOps, "Microsoft.Resources/subscriptions/read",
			"acme user should get subscriptions from bottom: got %v", acmeAllowOps)
		assert.Contains(t, acmeAllowOps, "Microsoft.OperationalInsights/workspaces/read",
			"acme user should get ops insights from bottom: got %v", acmeAllowOps)

		// All built-in roles should be present
		assert.Contains(t, resultAcme.Inherits, "Contributor",
			"acme user should get Contributor: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "DNS Zone Contributor",
			"acme user should get DNS Zone Contributor: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "Virtual Machine Contributor",
			"acme user should get Virtual Machine Contributor from middle: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "Network Contributor",
			"acme user should get Network Contributor from middle: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "Security Admin",
			"acme user should get Security Admin from bottom: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "Log Analytics Contributor",
			"acme user should get Log Analytics Contributor from bottom: got %v", resultAcme.Inherits)

		assert.True(t, resultAcme.Composite,
			"acme user should get composite role when all scopes pass")
	})
}

// TestAzureInheritanceScopeDenial validates that users outside a role's scope do
// NOT receive that role's permissions or Azure built-in role inherits when the scoped
// role appears at various depths in the inheritance chain.
func TestAzureInheritanceScopeDenial(t *testing.T) {
	azureProviders := map[string]models.ProviderConfig{
		"azure-prod": {
			Name:        "azure-prod",
			Description: "Azure Production",
			Provider:    "azure",
		},
		"azure-dev": {
			Name:        "azure-dev",
			Description: "Azure Development",
			Provider:    "azure",
		},
	}

	// 1. User outside allow-scope gets no permissions from scoped role
	t.Run("user outside allow-scope inherits nothing from scoped child", func(t *testing.T) {
		roles := map[string]models.Role{
			"scoped_azure_power": {
				Name:        "Scoped Azure Power",
				Description: "Power perms restricted to SRE group",
				Inherits: []string{
					"Contributor",
					"Storage Blob Data Owner",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.Storage/storageAccounts/*",
						"Microsoft.Sql/servers/*",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"sre-team"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"top_azure_role": {
				Name:        "Top Azure Role",
				Description: "Inherits scoped_azure_power, adds monitoring",
				Inherits:    []string{"scoped_azure_power"},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Insights/diagnosticSettings/read"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "regulardev",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "top_azure_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectOpsAzure(result.Permissions.Allow)
		assert.ElementsMatch(t, []string{"Microsoft.Insights/diagnosticSettings/read"}, allowOps,
			"No permissions from scope-denied scoped_azure_power should leak: got %v", allowOps)

		assert.Empty(t, result.Inherits,
			"Azure built-in role inherits from scope-denied role must not appear: got %v", result.Inherits)

		assert.False(t, result.Composite,
			"Should not be composite when the only inherited thand role was scope-denied")
	})

	// 2. User in deny-scope gets nothing from scoped role (depth 2)
	t.Run("user in deny-scope inherits nothing from denied child at depth 2", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_base_reader": {
				Name:        "Azure Base Reader",
				Description: "Simple read perms, no scope",
				Inherits: []string{
					"Reader",
					"Monitoring Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Insights/metrics/read",
					),
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
			"azure_sensitive_ops": {
				Name:        "Azure Sensitive Ops",
				Description: "Sensitive operations, deny-scoped to contractors",
				Inherits: []string{
					"azure_base_reader",
					"Key Vault Administrator",
					"SQL Server Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.KeyVault/vaults/*",
						"Microsoft.Sql/servers/*",
						"Microsoft.Authorization/roleAssignments/write",
					),
				},
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Groups: []string{"contractors"},
					},
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
			"azure_manager_role": {
				Name:        "Azure Manager",
				Description: "Manager inheriting sensitive_ops + own built-in roles",
				Inherits: []string{
					"azure_sensitive_ops",
					"Data Factory Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.DataFactory/factories/*"),
				},
				Providers: []string{"azure-prod", "azure-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, azureProviders)

		// Contractor: should be denied from sensitive_ops
		contractor := &models.Identity{
			ID: "contractor1",
			User: &models.User{
				Username: "contractor",
				Email:    "contractor@vendor.com",
				Groups:   []string{"contractors", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(contractor, "azure_manager_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// Manager's own perms should be present
		assert.Contains(t, allowOps, "Microsoft.DataFactory/factories/*",
			"Manager's own DataFactory perm must be present: got %v", allowOps)

		// Sensitive ops perms should NOT be present
		for _, op := range allowOps {
			assert.NotContains(t, op, "Microsoft.Compute/virtualMachines",
				"compute perms from scope-denied sensitive_ops must not appear: got %v", allowOps)
			assert.NotContains(t, op, "Microsoft.KeyVault/vaults",
				"keyvault perms from scope-denied sensitive_ops must not appear: got %v", allowOps)
			assert.NotEqual(t, "Microsoft.Sql/servers/*", op,
				"sql perms from scope-denied sensitive_ops must not appear: got %v", allowOps)
			assert.NotEqual(t, "Microsoft.Authorization/roleAssignments/write", op,
				"auth perms from scope-denied sensitive_ops must not appear: got %v", allowOps)
		}

		// Base reader perms should NOT propagate through denied chain
		for _, op := range allowOps {
			assert.NotContains(t, op, "Microsoft.Resources/subscriptions",
				"subscription perms from base_reader (via denied sensitive_ops) must not appear: got %v", allowOps)
			assert.NotContains(t, op, "Microsoft.Insights/metrics",
				"metrics perms from base_reader (via denied sensitive_ops) must not appear: got %v", allowOps)
		}

		// Built-in roles from denied chain should not propagate
		for _, inh := range result.Inherits {
			assert.NotEqual(t, "Reader", inh,
				"Reader from base_reader should not propagate through scope-denied chain: got %v", result.Inherits)
			assert.NotEqual(t, "Monitoring Reader", inh,
				"Monitoring Reader from base_reader should not propagate through scope-denied chain: got %v", result.Inherits)
			assert.NotEqual(t, "Key Vault Administrator", inh,
				"Key Vault Administrator from sensitive_ops should not propagate: got %v", result.Inherits)
			assert.NotEqual(t, "SQL Server Contributor", inh,
				"SQL Server Contributor from sensitive_ops should not propagate: got %v", result.Inherits)
		}

		// Manager's own built-in role should remain
		assert.Contains(t, result.Inherits, "Data Factory Contributor",
			"Manager's own Data Factory Contributor should remain: got %v", result.Inherits)

		assert.False(t, result.Composite,
			"Should not be composite when the inherited thand role was scope-denied")

		// Employee (not contractor): should get everything
		employee := &models.Identity{
			ID: "emp1",
			User: &models.User{
				Username: "employee",
				Email:    "employee@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEmp, err := config.GetCompositeRoleByName(employee, "azure_manager_role")
		require.NoError(t, err)
		require.NotNil(t, resultEmp)

		empAllowOps := collectOpsAzure(resultEmp.Permissions.Allow)
		assert.Contains(t, empAllowOps, "Microsoft.DataFactory/factories/*",
			"Employee should get DataFactory: got %v", empAllowOps)
		assert.Contains(t, empAllowOps, "Microsoft.Compute/virtualMachines/*",
			"Employee should get compute from sensitive_ops: got %v", empAllowOps)
		assert.Contains(t, empAllowOps, "Microsoft.KeyVault/vaults/*",
			"Employee should get keyvault from sensitive_ops: got %v", empAllowOps)

		// Employee should get all built-in roles
		assert.Contains(t, resultEmp.Inherits, "Reader",
			"Employee should get Reader from base_reader: got %v", resultEmp.Inherits)
		assert.Contains(t, resultEmp.Inherits, "Monitoring Reader",
			"Employee should get Monitoring Reader from base_reader: got %v", resultEmp.Inherits)
		assert.Contains(t, resultEmp.Inherits, "Key Vault Administrator",
			"Employee should get Key Vault Administrator: got %v", resultEmp.Inherits)
		assert.Contains(t, resultEmp.Inherits, "SQL Server Contributor",
			"Employee should get SQL Server Contributor: got %v", resultEmp.Inherits)
		assert.Contains(t, resultEmp.Inherits, "Data Factory Contributor",
			"Employee should get Data Factory Contributor: got %v", resultEmp.Inherits)

		assert.True(t, resultEmp.Composite,
			"Employee result should be composite since thand roles were merged")
	})

	// 3. Deny-scope user + allow-scope user in same group hierarchy
	t.Run("deny scope takes precedence over allow scope for same user", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_privileged_role": {
				Name:        "Azure Privileged",
				Description: "High-privilege role with both allow and deny scopes",
				Inherits: []string{
					"Owner",
					"User Access Administrator",
					"AcrPush",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.Storage/storageAccounts/*",
						"Microsoft.ContainerRegistry/registries/*",
						"Microsoft.Authorization/*",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"intern@example.com"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_wrapper_role": {
				Name:        "Azure Wrapper",
				Description: "Wraps privileged role",
				Inherits:    []string{"azure_privileged_role"},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Insights/metrics/read"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Intern: in engineering (allow) but explicitly denied
		intern := &models.Identity{
			ID: "intern1",
			User: &models.User{
				Username: "intern",
				Email:    "intern@example.com",
				Groups:   []string{"engineering", "interns"},
			},
		}

		resultIntern, err := config.GetCompositeRoleByName(intern, "azure_wrapper_role")
		require.NoError(t, err)
		require.NotNil(t, resultIntern)

		internOps := collectOpsAzure(resultIntern.Permissions.Allow)

		assert.ElementsMatch(t, []string{"Microsoft.Insights/metrics/read"}, internOps,
			"Intern (deny-scoped) should only get wrapper's own perms: got %v", internOps)

		assert.Empty(t, resultIntern.Inherits,
			"Intern should not get Azure built-in roles from denied privileged role: got %v", resultIntern.Inherits)

		assert.False(t, resultIntern.Composite,
			"Intern should not get composite when privileged role was denied")

		// Engineer: should get everything
		engineer := &models.Identity{
			ID: "eng1",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEng, err := config.GetCompositeRoleByName(engineer, "azure_wrapper_role")
		require.NoError(t, err)
		require.NotNil(t, resultEng)

		engOps := collectOpsAzure(resultEng.Permissions.Allow)
		assert.Contains(t, engOps, "Microsoft.Compute/virtualMachines/*",
			"Engineer should get compute from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.Storage/storageAccounts/*",
			"Engineer should get storage from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.ContainerRegistry/registries/*",
			"Engineer should get ACR from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.Authorization/*",
			"Engineer should get auth from privileged: got %v", engOps)
		assert.Contains(t, engOps, "Microsoft.Insights/metrics/read",
			"Engineer should get metrics from wrapper: got %v", engOps)

		assert.Contains(t, resultEng.Inherits, "Owner",
			"Engineer should get Owner: got %v", resultEng.Inherits)
		assert.Contains(t, resultEng.Inherits, "User Access Administrator",
			"Engineer should get User Access Administrator: got %v", resultEng.Inherits)
		assert.Contains(t, resultEng.Inherits, "AcrPush",
			"Engineer should get AcrPush: got %v", resultEng.Inherits)

		assert.True(t, resultEng.Composite,
			"Engineer should get composite role")
	})
}

// TestAzureMultiProviderInheritance tests role inheritance with multiple Azure
// providers and validates that built-in roles resolve across different provider
// instances.
func TestAzureMultiProviderInheritance(t *testing.T) {
	// 1. Role spanning multiple Azure providers with built-in role resolution
	t.Run("role spanning multiple Azure providers resolves built-in roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"multi_provider_base": {
				Name:        "Multi-Provider Base",
				Description: "Base role with built-in role inheritance across providers",
				Inherits: []string{
					"Reader",
					"AcrPull",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Resources/subscriptions/resourceGroups/read",
					),
				},
				Providers: []string{"azure-prod", "azure-staging", "azure-dev"},
				Enabled:   true,
			},
			"multi_provider_ops": {
				Name:        "Multi-Provider Ops",
				Description: "Ops role inheriting base + more built-in roles",
				Inherits: []string{
					"multi_provider_base",
					"Virtual Machine Contributor",
					"Storage Blob Data Contributor",
					"Managed Identity Operator",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"Microsoft.Compute/virtualMachines/start/action",
							"Microsoft.Compute/virtualMachines/restart/action",
							"Microsoft.Compute/virtualMachines/deallocate/action",
							"Microsoft.Storage/storageAccounts/blobServices/containers/*",
						},
						Targets: []string{
							"/subscriptions/*/resourceGroups/ops-*",
						},
					}},
					Deny: stmtsAzure(
						"Microsoft.Compute/virtualMachines/delete",
						"Microsoft.Authorization/*/Write",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"platform-ops", "sre-team"},
					},
				},
				Providers: []string{"azure-prod", "azure-staging"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:        "azure-prod",
				Description: "Azure Production",
				Provider:    "azure",
			},
			"azure-staging": {
				Name:        "azure-staging",
				Description: "Azure Staging",
				Provider:    "azure",
			},
			"azure-dev": {
				Name:        "azure-dev",
				Description: "Azure Development",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		sre := &models.Identity{
			ID: "sre1",
			User: &models.User{
				Username: "sreengineer",
				Email:    "sre@example.com",
				Groups:   []string{"sre-team", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(sre, "multi_provider_ops")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Should be composite since local thand role was merged")

		// All built-in roles from both levels
		assert.Contains(t, result.Inherits, "Reader",
			"Reader from base should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "AcrPull",
			"AcrPull from base should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Virtual Machine Contributor",
			"Virtual Machine Contributor from ops: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Storage Blob Data Contributor",
			"Storage Blob Data Contributor from ops: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Managed Identity Operator",
			"Managed Identity Operator from ops: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)
		// Own perms
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/start/action",
			"ops vm start should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/restart/action",
			"ops vm restart should be present: got %v", allowOps)

		// Base perms merged
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/read",
			"base subscription read should merge: got %v", allowOps)

		// Deny perms survive
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Compute/virtualMachines/delete",
			"vm delete deny should survive: got %v", denyOps)
		assert.Contains(t, denyOps, "Microsoft.Authorization/*/Write",
			"auth write deny should survive: got %v", denyOps)

		// Verify providers
		assert.ElementsMatch(t, []string{"azure-prod", "azure-staging"}, result.Providers)
	})

	// 2. Three-tier hierarchy with different provider scopes at each level
	t.Run("three-tier hierarchy with different Azure environments", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_platform_foundation": {
				Name:        "Platform Foundation",
				Description: "Foundation shared across all environments",
				Inherits: []string{
					"Reader",
					"Monitoring Reader",
					"Security Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Resources/subscriptions/resourceGroups/read",
						"Microsoft.Insights/metrics/read",
						"Microsoft.Security/*/read",
					),
				},
				Providers: []string{"azure-prod", "azure-staging", "azure-dev"},
				Enabled:   true,
			},
			"azure_deploy_engineer": {
				Name:        "Deploy Engineer",
				Description: "Deployment engineer inheriting foundation + container roles",
				Inherits: []string{
					"azure_platform_foundation",
					"AcrPush",
					"AcrPull",
					"Website Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.ContainerRegistry/registries/push/action",
						"Microsoft.ContainerRegistry/registries/pull/action",
						"Microsoft.Web/sites/*",
						"Microsoft.Web/serverFarms/read",
					),
					Deny: stmtsAzure(
						"Microsoft.Web/sites/delete",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"deploy-engineers"},
					},
				},
				Providers: []string{"azure-staging", "azure-dev"},
				Enabled:   true,
			},
			"azure_release_manager": {
				Name:        "Release Manager",
				Description: "Manager with production deploy + all lower perms",
				Inherits: []string{
					"azure_deploy_engineer",
					"Contributor",
					"Key Vault Secrets Officer",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.KeyVault/vaults/secrets/read",
						"Microsoft.KeyVault/vaults/secrets/write",
						"Microsoft.Resources/deployments/*",
					),
					Deny: stmtsAzure(
						"Microsoft.Authorization/roleAssignments/write",
						"Microsoft.Authorization/roleAssignments/delete",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{
							"releasemanager@example.com",
							"seniorops@example.com",
						},
					},
				},
				Providers: []string{"azure-prod", "azure-staging"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:        "azure-prod",
				Description: "Azure Production",
				Provider:    "azure",
			},
			"azure-staging": {
				Name:        "azure-staging",
				Description: "Azure Staging",
				Provider:    "azure",
			},
			"azure-dev": {
				Name:        "azure-dev",
				Description: "Azure Development",
				Provider:    "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Release manager with deploy-engineers group
		manager := &models.Identity{
			ID: "rm1",
			User: &models.User{
				Username: "releasemanager",
				Email:    "releasemanager@example.com",
				Groups:   []string{"deploy-engineers", "managers"},
			},
		}

		result, err := config.GetCompositeRoleByName(manager, "azure_release_manager")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// All built-in roles from all tiers should be present
		assert.Contains(t, result.Inherits, "Reader",
			"Reader from foundation: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Monitoring Reader",
			"Monitoring Reader from foundation: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Security Reader",
			"Security Reader from foundation: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "AcrPush",
			"AcrPush from deploy engineer: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "AcrPull",
			"AcrPull from deploy engineer: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Website Contributor",
			"Website Contributor from deploy engineer: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Contributor",
			"Contributor from release manager: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Key Vault Secrets Officer",
			"Key Vault Secrets Officer from release manager: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// Own perms
		assert.Contains(t, allowOps, "Microsoft.KeyVault/vaults/secrets/read",
			"keyvault secrets read: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Resources/deployments/*",
			"deployments wildcard: got %v", allowOps)

		// Deploy engineer perms
		assert.Contains(t, allowOps, "Microsoft.Web/sites/*",
			"web sites from deploy engineer: got %v", allowOps)

		// Foundation perms
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/read",
			"subscription read from foundation: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Insights/metrics/read",
			"metrics read from foundation: got %v", allowOps)

		// Deny perms from both levels
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Authorization/roleAssignments/write",
			"auth write deny: got %v", denyOps)
		assert.Contains(t, denyOps, "Microsoft.Authorization/roleAssignments/delete",
			"auth delete deny: got %v", denyOps)

		assert.ElementsMatch(t, []string{"azure-prod", "azure-staging"}, result.Providers)
	})
}

// TestAzureMixedInheritancePatterns tests complex scenarios combining multiple
// Azure built-in roles, custom thand roles, deep nesting, overlapping scopes,
// and various target constraints.
func TestAzureMixedInheritancePatterns(t *testing.T) {

	// 1. Diamond inheritance: two middle roles inheriting same base
	t.Run("diamond inheritance with shared base role", func(t *testing.T) {
		roles := map[string]models.Role{
			"diamond_base": {
				Name:        "Diamond Base",
				Description: "Shared base with Reader",
				Inherits: []string{
					"Reader",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Resources/subscriptions/read",
						"Microsoft.Resources/subscriptions/resourceGroups/read",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"diamond_left": {
				Name:        "Diamond Left",
				Description: "Left path: storage focus",
				Inherits: []string{
					"diamond_base",
					"Storage Blob Data Owner",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Storage/storageAccounts/*",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"diamond_right": {
				Name:        "Diamond Right",
				Description: "Right path: compute focus",
				Inherits: []string{
					"diamond_base",
					"Virtual Machine Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"diamond_top": {
				Name:        "Diamond Top",
				Description: "Top role merging both paths",
				Inherits: []string{
					"diamond_left",
					"diamond_right",
					"Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure("Microsoft.Network/virtualNetworks/*"),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "user@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "diamond_top")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Diamond top should be composite (inherits thand roles)")

		// Built-in roles from all paths
		assert.Contains(t, result.Inherits, "Reader",
			"Reader from base should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Storage Blob Data Owner",
			"Storage Blob Data Owner from left: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Virtual Machine Contributor",
			"Virtual Machine Contributor from right: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Contributor",
			"Contributor from top: got %v", result.Inherits)

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// Own
		assert.Contains(t, allowOps, "Microsoft.Network/virtualNetworks/*",
			"top network perm: got %v", allowOps)

		// Left path
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/*",
			"left storage perm: got %v", allowOps)

		// Right path
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/*",
			"right compute perm: got %v", allowOps)

		// Base perms (merged from one or both paths)
		assert.Contains(t, allowOps, "Microsoft.Resources/subscriptions/read",
			"base subscription read should be present: got %v", allowOps)
	})

	// 2. Complex multi-permission-set role with targets and deny
	t.Run("role with multiple permission sets and varying targets", func(t *testing.T) {
		roles := map[string]models.Role{
			"multi_stmt_base": {
				Name:        "Multi-Statement Base",
				Description: "Base with different permission sets for different resource groups",
				Inherits: []string{
					"Monitoring Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{
								"Microsoft.Compute/virtualMachines/read",
								"Microsoft.Compute/virtualMachines/start/action",
							},
							Targets: []string{
								"/subscriptions/*/resourceGroups/dev-*",
							},
						},
						{
							Operations: []string{
								"Microsoft.Storage/storageAccounts/blobServices/containers/read",
								"Microsoft.Storage/storageAccounts/blobServices/containers/write",
							},
							Targets: []string{
								"/subscriptions/*/resourceGroups/data-*",
							},
						},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"multi_stmt_elevated": {
				Name:        "Multi-Statement Elevated",
				Description: "Elevated role adding more permission sets",
				Inherits: []string{
					"multi_stmt_base",
					"Contributor",
					"Key Vault Administrator",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{
								"Microsoft.KeyVault/vaults/secrets/read",
								"Microsoft.KeyVault/vaults/secrets/write",
							},
							Targets: []string{
								"/subscriptions/*/resourceGroups/secrets-*",
							},
						},
						{
							Operations: []string{
								"Microsoft.Sql/servers/databases/read",
								"Microsoft.Sql/servers/databases/write",
							},
							Targets: []string{
								"/subscriptions/*/resourceGroups/sql-*",
							},
						},
					},
					Deny: stmtsAzure(
						"Microsoft.Sql/servers/databases/delete",
						"Microsoft.KeyVault/vaults/delete",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"platform-team"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "platformeng1",
			User: &models.User{
				Username: "platformeng",
				Email:    "platform@example.com",
				Groups:   []string{"platform-team"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "multi_stmt_elevated")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite)

		// Built-in roles from both levels
		assert.Contains(t, result.Inherits, "Monitoring Contributor",
			"Monitoring Contributor from base: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Contributor",
			"Contributor from elevated: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "Key Vault Administrator",
			"Key Vault Administrator from elevated: got %v", result.Inherits)

		// Collect all targets to ensure different targets preserved
		var allTargets []string
		for _, stmt := range result.Permissions.Allow {
			allTargets = append(allTargets, stmt.Targets...)
		}

		// Targets from base
		assert.Contains(t, allTargets, "/subscriptions/*/resourceGroups/dev-*",
			"dev target from base should be preserved: got %v", allTargets)
		assert.Contains(t, allTargets, "/subscriptions/*/resourceGroups/data-*",
			"data target from base should be preserved: got %v", allTargets)

		// Targets from elevated
		assert.Contains(t, allTargets, "/subscriptions/*/resourceGroups/secrets-*",
			"secrets target from elevated should be preserved: got %v", allTargets)
		assert.Contains(t, allTargets, "/subscriptions/*/resourceGroups/sql-*",
			"sql target from elevated should be preserved: got %v", allTargets)

		// Deny ops
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Sql/servers/databases/delete",
			"sql delete deny should survive: got %v", denyOps)
		assert.Contains(t, denyOps, "Microsoft.KeyVault/vaults/delete",
			"keyvault delete deny should survive: got %v", denyOps)
	})

	// 3. Deeply nested with diverse Azure built-in roles at every level
	t.Run("three levels with diverse built-in roles at every tier", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_leaf_infra": {
				Name:        "Azure Leaf Infrastructure",
				Description: "Bottom-tier with networking and monitoring built-in roles",
				Inherits: []string{
					"Network Contributor",
					"Monitoring Reader",
					"DNS Zone Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Network/virtualNetworks/read",
						"Microsoft.Network/networkSecurityGroups/read",
						"Microsoft.Insights/metrics/read",
						"Microsoft.Network/dnszones/read",
					),
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_mid_data": {
				Name:        "Azure Mid Data",
				Description: "Mid-tier data platform inheriting infra + data roles",
				Inherits: []string{
					"azure_leaf_infra",
					"Storage Blob Data Owner",
					"SQL DB Contributor",
					"Cosmos DB Account Reader Role",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Storage/storageAccounts/*",
						"Microsoft.Sql/servers/databases/read",
						"Microsoft.Sql/servers/databases/write",
						"Microsoft.DocumentDB/databaseAccounts/read",
					),
					Deny: stmtsAzure(
						"Microsoft.Storage/storageAccounts/delete",
						"Microsoft.Sql/servers/databases/delete",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"data-platform"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
			"azure_top_platform_admin": {
				Name:        "Azure Top Platform Admin",
				Description: "Top-tier platform admin with compute + security + keyvault",
				Inherits: []string{
					"azure_mid_data",
					"Virtual Machine Contributor",
					"Key Vault Administrator",
					"Security Admin",
					"Managed Identity Operator",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.KeyVault/vaults/*",
						"Microsoft.ManagedIdentity/userAssignedIdentities/*",
					),
					Deny: stmtsAzure(
						"Microsoft.Authorization/*/Delete",
						"Microsoft.Authorization/elevateAccess/Action",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"platformadmin@example.com"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Platform admin who is also in data-platform group
		admin := &models.Identity{
			ID: "padmin1",
			User: &models.User{
				Username: "platformadmin",
				Email:    "platformadmin@example.com",
				Groups:   []string{"data-platform", "platform-admins"},
			},
		}

		result, err := config.GetCompositeRoleByName(admin, "azure_top_platform_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// Verify ALL built-in roles from all 3 tiers
		expectedInherits := []string{
			// From leaf (tier 0)
			"Network Contributor",
			"Monitoring Reader",
			"DNS Zone Contributor",
			// From mid (tier 1)
			"Storage Blob Data Owner",
			"SQL DB Contributor",
			"Cosmos DB Account Reader Role",
			// From top (tier 2)
			"Virtual Machine Contributor",
			"Key Vault Administrator",
			"Security Admin",
			"Managed Identity Operator",
		}
		assert.Len(t, result.Inherits, len(expectedInherits),
			"All %d Azure built-in roles from all tiers should be present: got %v", len(expectedInherits), result.Inherits)
		for _, expected := range expectedInherits {
			assert.Contains(t, result.Inherits, expected,
				"%s should be in Inherits: got %v", expected, result.Inherits)
		}

		allowOps := collectOpsAzure(result.Permissions.Allow)

		// Top tier perms
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/*",
			"top compute perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.KeyVault/vaults/*",
			"top keyvault perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.ManagedIdentity/userAssignedIdentities/*",
			"top managed identity perm: got %v", allowOps)

		// Mid tier perms
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/*",
			"mid storage perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Sql/servers/databases/read",
			"mid sql read perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.DocumentDB/databaseAccounts/read",
			"mid cosmos perm: got %v", allowOps)

		// Leaf tier perms
		assert.Contains(t, allowOps, "Microsoft.Network/virtualNetworks/read",
			"leaf network perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Network/networkSecurityGroups/read",
			"leaf nsg perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Insights/metrics/read",
			"leaf metrics perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Network/dnszones/read",
			"leaf dns perm: got %v", allowOps)

		// Deny perms from mid and top
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Authorization/*/Delete",
			"top auth delete deny: got %v", denyOps)
		assert.Contains(t, denyOps, "Microsoft.Authorization/elevateAccess/Action",
			"top elevate access deny: got %v", denyOps)

		assert.ElementsMatch(t, []string{"azure-prod"}, result.Providers)
	})

	// 4. Wide inheritance: single role inheriting many Azure built-in roles
	t.Run("wide inheritance with many Azure built-in roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"azure_super_admin": {
				Name:        "Azure Super Admin",
				Description: "Super admin inheriting a wide variety of Azure built-in roles",
				Inherits: []string{
					"Owner",
					"User Access Administrator",
					"Virtual Machine Contributor",
					"Network Contributor",
					"Storage Blob Data Owner",
					"Key Vault Administrator",
					"SQL Server Contributor",
					"Monitoring Contributor",
					"Log Analytics Contributor",
					"Security Admin",
					"AcrPush",
					"Data Factory Contributor",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAzure(
						"Microsoft.Compute/virtualMachines/*",
						"Microsoft.Storage/storageAccounts/*",
						"Microsoft.Network/virtualNetworks/*",
						"Microsoft.KeyVault/vaults/*",
						"Microsoft.Sql/servers/*",
						"Microsoft.Insights/*",
						"Microsoft.ContainerRegistry/registries/*",
						"Microsoft.DataFactory/factories/*",
					),
					Deny: stmtsAzure(
						"Microsoft.Authorization/classicAdministrators/delete",
						"Microsoft.Authorization/classicAdministrators/write",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"superadmin@example.com"},
					},
				},
				Providers: []string{"azure-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"azure-prod": {
				Name:     "azure-prod",
				Provider: "azure",
			},
		}

		config := newTestConfig(t, roles, providers)

		admin := &models.Identity{
			ID: "superadmin1",
			User: &models.User{
				Username: "superadmin",
				Email:    "superadmin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(admin, "azure_super_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.False(t, result.Composite,
			"No thand role inheritance means not composite")

		expectedInherits := []string{
			"Owner",
			"User Access Administrator",
			"Virtual Machine Contributor",
			"Network Contributor",
			"Storage Blob Data Owner",
			"Key Vault Administrator",
			"SQL Server Contributor",
			"Monitoring Contributor",
			"Log Analytics Contributor",
			"Security Admin",
			"AcrPush",
			"Data Factory Contributor",
		}
		assert.Len(t, result.Inherits, len(expectedInherits),
			"All 12 Azure built-in roles should be preserved: got %v", result.Inherits)
		for _, expected := range expectedInherits {
			assert.Contains(t, result.Inherits, expected,
				"%s should be in Inherits: got %v", expected, result.Inherits)
		}

		// Own perms should be present
		allowOps := collectOpsAzure(result.Permissions.Allow)
		assert.Contains(t, allowOps, "Microsoft.Compute/virtualMachines/*",
			"compute perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.Storage/storageAccounts/*",
			"storage perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.KeyVault/vaults/*",
			"keyvault perm: got %v", allowOps)
		assert.Contains(t, allowOps, "Microsoft.DataFactory/factories/*",
			"datafactory perm: got %v", allowOps)

		// Deny should survive
		denyOps := collectOpsAzure(result.Permissions.Deny)
		assert.Contains(t, denyOps, "Microsoft.Authorization/classicAdministrators/delete",
			"classic admin delete deny: got %v", denyOps)
	})
}
