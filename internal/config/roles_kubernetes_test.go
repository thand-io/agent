package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// collectOpsK8s collects all operations from statements into a single slice
func collectOpsK8s(stmts models.RoleStatements) []string {
	var result []string
	for _, stmt := range stmts {
		result = append(result, stmt.Operations...)
	}
	return result
}

// TestKubernetesRoles tests Kubernetes-specific role configurations based on config/roles/kubernetes.yaml
func TestKubernetesRoles(t *testing.T) {
	// Kubernetes role definitions based on config/roles/kubernetes.yaml
	k8sRoles := map[string]models.Role{
		"dev-pod-reader": {
			Description: "Read pods in development namespace",
			Authenticators: []string{
				"google_oauth2",
				"thand_oauth2",
			},
			Workflows: []string{
				"slack_approval",
			},
			Providers: []string{
				"kubernetes-dev",
				"kubernetes-prod",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{
						"k8s:pods:get,list,watch",
						"k8s:services:get,list",
						"k8s:events:get,list",
					},
					Targets: []string{
						"namespace:development",
					},
				}},
			},
			Enabled: true,
		},
		"dev-deployer": {
			Description: "Deploy applications in development namespace",
			Authenticators: []string{
				"google_oauth2",
				"thand_oauth2",
			},
			Workflows: []string{
				"slack_approval",
			},
			Providers: []string{
				"kubernetes-dev",
				"kubernetes-prod",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{
						"k8s:pods:get,list,watch",
						"k8s:services:get,list,create,update,patch,delete",
						"k8s:configmaps:get,list,create,update,delete",
						"k8s:secrets:get,list,create,update,delete",
						"k8s:apps/deployments:get,list,create,update,patch,delete,watch",
						"k8s:apps/replicasets:get,list",
						"k8s:events:get,list,create",
					},
					Targets: []string{
						"namespace:development",
					},
				}},
			},
			Enabled: true,
		},
		"staging-admin": {
			Description: "Full admin access to staging namespace",
			Authenticators: []string{
				"google_oauth2",
				"thand_oauth2",
			},
			Workflows: []string{
				"slack_approval",
			},
			Providers: []string{
				"kubernetes-dev",
				"kubernetes-prod",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{
						"k8s:*:*",
					},
					Targets: []string{
						"namespace:staging",
					},
				}},
			},
			Enabled: true,
		},
		"cluster-viewer": {
			Description: "Read-only access across all namespaces",
			Authenticators: []string{
				"google_oauth2",
				"thand_oauth2",
			},
			Workflows: []string{
				"slack_approval",
			},
			Providers: []string{
				"kubernetes-dev",
				"kubernetes-prod",
			},
			Permissions: models.RolePermissions{
				Allow: stmts(
					"k8s:pods:get,list,watch",
					"k8s:services:get,list,watch",
					"k8s:nodes:get,list,watch",
					"k8s:events:get,list,watch",
				),
			},
			Enabled: true,
		},
	}

	// Kubernetes providers
	k8sProviders := map[string]models.ProviderConfig{
		"kubernetes-dev": {
			Name:        "kubernetes-dev",
			Description: "Kubernetes Development Cluster",
			Provider:    "kubernetes",
		},
		"kubernetes-prod": {
			Name:        "kubernetes-prod",
			Description: "Kubernetes Production Cluster",
			Provider:    "kubernetes",
		},
	}

	t.Run("dev-pod-reader role composition", func(t *testing.T) {
		config := newTestConfig(t, k8sRoles, k8sProviders)

		identity := &models.Identity{
			ID: "dev-user",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "dev-pod-reader")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify basic properties
		assert.Equal(t, "Read pods in development namespace", result.Description)
		assert.True(t, result.Enabled)

		// Verify permissions and targets
		assert.Len(t, result.Permissions.Allow, 1)
		assert.ElementsMatch(t, []string{
			"k8s:pods:get,list,watch",
			"k8s:services:get,list",
			"k8s:events:get,list",
		}, result.Permissions.Allow[0].Operations)

		// Verify targets
		assert.ElementsMatch(t, []string{"namespace:development"}, result.Permissions.Allow[0].Targets)

		// Verify providers
		assert.ElementsMatch(t, []string{"kubernetes-dev", "kubernetes-prod"}, result.Providers)

		// Verify workflows
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)

		// Verify authenticators
		assert.ElementsMatch(t, []string{"google_oauth2", "thand_oauth2"}, result.Authenticators)
	})

	t.Run("dev-deployer role composition", func(t *testing.T) {
		config := newTestConfig(t, k8sRoles, k8sProviders)

		identity := &models.Identity{
			ID: "deployer-user",
			User: &models.User{
				Username: "deployer",
				Email:    "deployer@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "dev-deployer")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Deploy applications in development namespace", result.Description)
		// Collect all operations and targets
		var allOps, allTargets []string
		for _, stmt := range result.Permissions.Allow {
			allOps = append(allOps, stmt.Operations...)
			allTargets = append(allTargets, stmt.Targets...)
		}

		assert.ElementsMatch(t, []string{
			"k8s:pods:get,list,watch",
			"k8s:services:create,delete,get,list,patch,update",
			"k8s:configmaps:create,delete,get,list,update",
			"k8s:secrets:create,delete,get,list,update",
			"k8s:apps/deployments:create,delete,get,list,patch,update,watch",
			"k8s:apps/replicasets:get,list",
			"k8s:events:create,get,list",
		}, allOps)

		assert.ElementsMatch(t, []string{"namespace:development"}, allTargets)
	})

	t.Run("staging-admin role composition", func(t *testing.T) {
		config := newTestConfig(t, k8sRoles, k8sProviders)

		identity := &models.Identity{
			ID: "staging-admin-user",
			User: &models.User{
				Username: "stagingadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "staging-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Full admin access to staging namespace", result.Description)
		// Collect all targets
		var allTargets []string
		for _, stmt := range result.Permissions.Allow {
			allTargets = append(allTargets, stmt.Targets...)
		}
		assert.ElementsMatch(t, []string{"namespace:staging"}, allTargets)
	})
}

// TestKubernetesRoleScenarios tests realistic Kubernetes role usage scenarios
func TestKubernetesRoleScenarios(t *testing.T) {
	t.Run("progressive kubernetes permissions with inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"k8s-base": {
				Name:        "Kubernetes Base",
				Description: "Base Kubernetes permissions",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:pods:get,list",
						"k8s:services:get,list",
						"k8s:events:get,list",
					),
				},
				Enabled: true,
			},
			"k8s-developer": {
				Name:        "Kubernetes Developer",
				Description: "Developer access to Kubernetes",
				Inherits:    []string{"k8s-base"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"k8s:pods:watch",
							"k8s:configmaps:get,list,create,update",
							"k8s:apps/deployments:get,list,create,update,patch",
						},
						Targets: []string{
							"namespace:dev-*",
							"namespace:feature-*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers", "engineers"},
					},
				},
				Enabled: true,
			},
			"k8s-sre": {
				Name:        "Kubernetes SRE",
				Description: "SRE access to Kubernetes",
				Inherits:    []string{"k8s-developer"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"k8s:pods:delete",
							"k8s:services:create,update,patch,delete",
							"k8s:apps/deployments:delete,watch",
							"k8s:nodes:get,list,watch",
							"k8s:persistentvolumes:get,list,watch",
						},
						Targets: []string{
							"namespace:staging",
							"namespace:production",
						},
					}},
				},
				Scopes: models.RoleScopes{

					Allow: models.ScopeIdentities{
						Groups: []string{"sre", "ops"},
					},
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
			ID: "sre1",
			User: &models.User{
				Username: "sre1",
				Email:    "sre1@example.com",
				Groups:   []string{"sre", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "k8s-sre")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should have merged permissions from applicable inherited roles
		// Note: User is in "sre" and "engineering" groups
		// - k8s-base (no scopes, applies to all) but inheritance is blocked by k8s-developer scope mismatch
		// - k8s-developer (scoped to "developers","engineers" - user is in "engineering" not "engineers") ✗
		// - k8s-sre (scoped to "sre","ops" - user is in "sre") ✓
		// Since k8s-sre inherits through k8s-developer, and k8s-developer is not applicable,
		// the inheritance chain is broken and only k8s-sre's own permissions are included
		expectedAllowPerms := []string{
			// Only from k8s-sre (inheritance chain broken at k8s-developer)
			"k8s:pods:delete",
			"k8s:services:create,delete,patch,update", // sorted alphabetically
			"k8s:apps/deployments:delete,watch",
			"k8s:nodes:get,list,watch",
			"k8s:persistentvolumes:get,list,watch",
		}
		assert.ElementsMatch(t, expectedAllowPerms, collectOpsK8s(result.Permissions.Allow))

		// Should have merged targets from k8s-sre only (k8s-developer not in scope)
		var allTargets []string
		for _, stmt := range result.Permissions.Allow {
			allTargets = append(allTargets, stmt.Targets...)
		}
		expectedTargets := []string{
			"namespace:staging",
			"namespace:production",
		}
		assert.ElementsMatch(t, expectedTargets, allTargets)
	})

	t.Run("namespace-specific rbac with scoping", func(t *testing.T) {
		roles := map[string]models.Role{
			"team-a-dev": {
				Name:        "Team A Developer",
				Description: "Developer access for Team A namespaces",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"k8s:pods:*",
							"k8s:services:*",
							"k8s:configmaps:*",
							"k8s:secrets:get,list",
							"k8s:apps/deployments:*",
						},
						Targets: []string{
							"namespace:team-a-dev",
							"namespace:team-a-staging",
						},
					}},
					Deny: models.RoleStatements{{
						Operations: []string{"*"}, // All operations denied for these targets
						Targets: []string{
							"namespace:team-a-prod",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"team-a"},
					},
				},
				Enabled: true,
			},
			"team-b-dev": {
				Name:        "Team B Developer",
				Description: "Developer access for Team B namespaces",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"k8s:pods:get,list,create,update,delete",
							"k8s:services:get,list,create,update,delete",
							"k8s:configmaps:get,list,create,update,delete",
							"k8s:apps/deployments:get,list,create,update,patch,delete",
						},
						Targets: []string{
							"namespace:team-b-*",
						},
					}},
				},
				Scopes: models.RoleScopes{

					Allow: models.ScopeIdentities{
						Groups: []string{"team-b"},
					},
				},
				Enabled: true,
			},
			"cross-team-viewer": {
				Name:        "Cross Team Viewer",
				Description: "Read access across teams",
				Inherits:    []string{"team-a-dev", "team-b-dev"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:events:get,list,watch",
						"k8s:nodes:get,list",
					),
					Deny: stmts(
						"k8s:*:create,update,patch,delete",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"managers", "leads"},
					},
				},
				Enabled: true,
			},
		}

		config := &Config{
			Roles: RoleConfig{
				Definitions: roles,
			},
		}

		// Test team-a member access
		teamAIdentity := &models.Identity{
			ID: "team-a-dev1",
			User: &models.User{
				Username: "teamadev1",
				Email:    "teamadev1@example.com",
				Groups:   []string{"team-a", "developers"},
			},
		}

		result, err := config.GetCompositeRoleByName(teamAIdentity, "team-a-dev")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Team A Developer", result.Name)
		// Collect targets
		var allowTargets, denyTargets []string
		for _, stmt := range result.Permissions.Allow {
			allowTargets = append(allowTargets, stmt.Targets...)
		}
		for _, stmt := range result.Permissions.Deny {
			denyTargets = append(denyTargets, stmt.Targets...)
		}
		assert.ElementsMatch(t, []string{
			"namespace:team-a-dev",
			"namespace:team-a-staging",
		}, allowTargets)
		assert.ElementsMatch(t, []string{
			"namespace:team-a-prod",
		}, denyTargets)

		// Test cross-team viewer (manager accessing both teams)
		managerIdentity := &models.Identity{
			ID: "manager1",
			User: &models.User{
				Username: "manager1",
				Email:    "manager1@example.com",
				Groups:   []string{"managers", "leads"},
			},
		}

		managerResult, err := config.GetCompositeRoleByName(managerIdentity, "cross-team-viewer")
		require.NoError(t, err)
		require.NotNil(t, managerResult)

		// Manager is in "managers" and "leads" groups
		// cross-team-viewer role is scoped to "managers", "leads" so it applies
		// BUT inherited roles team-a-dev and team-b-dev are scoped to "team-a" and "team-b"
		// respectively, so they should NOT be merged since manager is not in those groups

		// Should NOT have targets from inherited roles since they don't apply to manager
		var managerAllowTargets, managerDenyTargets []string
		for _, stmt := range managerResult.Permissions.Allow {
			managerAllowTargets = append(managerAllowTargets, stmt.Targets...)
		}
		for _, stmt := range managerResult.Permissions.Deny {
			managerDenyTargets = append(managerDenyTargets, stmt.Targets...)
		}
		assert.Empty(t, managerAllowTargets)
		assert.Empty(t, managerDenyTargets)
	})

	t.Run("cluster admin with restricted permissions", func(t *testing.T) {
		roles := map[string]models.Role{
			"cluster-admin": {
				Name:        "Cluster Admin",
				Description: "Cluster-wide admin with some restrictions",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"k8s:*:*",
					),
					Deny: stmts(
						"k8s:secrets:*", // Cannot access secrets
						"k8s:certificatesigningrequests:*",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{
							"cluster-admin@example.com",
							"platform-admin@example.com",
						},
					},
				},
				Providers: []string{"kubernetes-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"kubernetes-prod": {
				Name:        "kubernetes-prod",
				Description: "Production Kubernetes Cluster",
				Provider:    "kubernetes",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "cluster-admin",
			User: &models.User{
				Username: "clusteradmin",
				Email:    "cluster-admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "cluster-admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Cluster Admin", result.Name)
		assert.ElementsMatch(t, []string{"k8s:*:*"}, collectOpsK8s(result.Permissions.Allow))
		assert.ElementsMatch(t, []string{
			"k8s:secrets:*",
			"k8s:certificatesigningrequests:*",
		}, collectOpsK8s(result.Permissions.Deny))
		assert.ElementsMatch(t, []string{"kubernetes-prod"}, result.Providers)
	})
}
