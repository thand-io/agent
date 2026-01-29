package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestGCPRoles tests GCP-specific role configurations based on config/roles/gcp.yaml
func TestGCPRoles(t *testing.T) {
	// GCP role definitions based on config/roles/gcp.yaml
	gcpRoles := map[string]models.Role{
		"gcp_admin": {
			Name:        "Gcp Admin",
			Description: "Full access to all resources and capabilities.",
			Workflows: []string{
				"slack_approval",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{
						"compute.instances.*",
						"storage.buckets.*",
						"iam.serviceAccounts.*",
					},
					Targets: []string{
						"gcp:*",
					},
				}},
			},
			Providers: []string{
				"gcp-prod",
			},
			Enabled: true,
		},
	}

	// GCP providers
	gcpProviders := map[string]models.ProviderConfig{
		"gcp-prod": {
			Name:        "gcp-prod",
			Description: "GCP Production Environment",
			Provider:    "gcp",
		},
	}

	t.Run("gcp_admin role composition", func(t *testing.T) {
		config := newTestConfig(t, gcpRoles, gcpProviders)

		identity := &models.Identity{
			ID: "gcp-admin-user",
			User: &models.User{
				Username: "gcpadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "gcp_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify basic properties
		assert.Equal(t, "Gcp Admin", result.Name)
		assert.Equal(t, "Full access to all resources and capabilities.", result.Description)
		assert.True(t, result.Enabled)

		// Verify permissions and targets
		assert.Len(t, result.Permissions.Allow, 1)
		assert.ElementsMatch(t, []string{
			"compute.instances.*",
			"storage.buckets.*",
			"iam.serviceAccounts.*",
		}, result.Permissions.Allow[0].Operations)

		// Verify targets - gcp:* becomes * since gcp matches allowed providers
		assert.ElementsMatch(t, []string{"*"}, result.Permissions.Allow[0].Targets)

		// Verify providers
		assert.ElementsMatch(t, []string{"gcp-prod"}, result.Providers)

		// Verify workflows
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)
	})
}

// TestGCPRoleScenarios tests realistic GCP role usage scenarios
func TestGCPRoleScenarios(t *testing.T) {
	t.Run("gcp developer role with project-specific access", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_developer": {
				Name:        "GCP Developer",
				Description: "Developer access to GCP resources",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"compute.instances.get",
							"compute.instances.list",
							"storage.objects.get",
							"storage.objects.list",
							"storage.objects.create",
							"cloudsql.instances.connect",
						},
						Targets: []string{
							"projects/dev-project-*",
							"projects/staging-project-*",
						},
					}},
					Deny: models.RoleStatements{{
						Operations: []string{"*"}, // All operations denied for these targets
						Targets: []string{
							"projects/prod-project-*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers", "engineers"},
					},
				},
				Providers: []string{"gcp-dev", "gcp-staging"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-dev": {
				Name:        "gcp-dev",
				Description: "GCP Development Environment",
				Provider:    "gcp",
			},
			"gcp-staging": {
				Name:        "gcp-staging",
				Description: "GCP Staging Environment",
				Provider:    "gcp",
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

		result, err := config.GetCompositeRoleByName(identity, "gcp_developer")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "GCP Developer", result.Name)

		// Collect all operations and targets
		var allowOps, allowTargets, denyTargets []string
		for _, stmt := range result.Permissions.Allow {
			allowOps = append(allowOps, stmt.Operations...)
			allowTargets = append(allowTargets, stmt.Targets...)
		}
		for _, stmt := range result.Permissions.Deny {
			denyTargets = append(denyTargets, stmt.Targets...)
		}

		assert.ElementsMatch(t, []string{
			"compute.instances.get",
			"compute.instances.list",
			"storage.objects.get",
			"storage.objects.list",
			"storage.objects.create",
			"cloudsql.instances.connect",
		}, allowOps)

		assert.ElementsMatch(t, []string{
			"projects/dev-project-*",
			"projects/staging-project-*",
		}, allowTargets)

		assert.ElementsMatch(t, []string{
			"projects/prod-project-*",
		}, denyTargets)

		assert.ElementsMatch(t, []string{"gcp-dev", "gcp-staging"}, result.Providers)
	})

	t.Run("gcp sre role with inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_base": {
				Name:        "GCP Base",
				Description: "Base GCP permissions",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"resourcemanager.projects.get",
						"iam.serviceAccounts.list",
					),
				},
				Enabled: true,
			},
			"gcp_monitoring": {
				Name:        "GCP Monitoring",
				Description: "GCP monitoring permissions",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"monitoring.*",
						"logging.logEntries.list",
						"logging.logEntries.create",
					),
				},
				Enabled: true,
			},
			"gcp_sre": {
				Name:        "GCP SRE",
				Description: "Site Reliability Engineer access",
				Inherits: []string{
					"gcp_base",
					"gcp_monitoring",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"compute.instances.start",
							"compute.instances.stop",
							"compute.instances.reset",
							"storage.buckets.list",
							"cloudsql.instances.restart",
						},
						Targets: []string{
							"projects/prod-*",
							"projects/staging-*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"sre", "ops"},
					},
				},
				Providers: []string{"gcp-prod", "gcp-staging"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:        "gcp-prod",
				Description: "GCP Production Environment",
				Provider:    "gcp",
			},
			"gcp-staging": {
				Name:        "gcp-staging",
				Description: "GCP Staging Environment",
				Provider:    "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "sre1",
			User: &models.User{
				Username: "sre1",
				Email:    "sre1@example.com",
				Groups:   []string{"sre", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "gcp_sre")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Collect all targets from allow statements
		var allowTargets []string
		for _, stmt := range result.Permissions.Allow {
			allowTargets = append(allowTargets, stmt.Targets...)
		}

		assert.ElementsMatch(t, []string{
			"projects/prod-*",
			"projects/staging-*",
		}, allowTargets)

		assert.ElementsMatch(t, []string{"gcp-prod", "gcp-staging"}, result.Providers)
	})

	t.Run("gcp multi-project role", func(t *testing.T) {
		roles := map[string]models.Role{
			"project_a_access": {
				Name:        "Project A Access",
				Description: "Access to project A resources",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"compute.instances.*"},
						Targets:    []string{"projects/project-a/*"},
					}},
				},
				Enabled: true,
			},
			"project_b_access": {
				Name:        "Project B Access",
				Description: "Access to project B resources",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"storage.buckets.*"},
						Targets:    []string{"projects/project-b/*"},
					}},
				},
				Enabled: true,
			},
			"multi_project_admin": {
				Name:        "Multi Project Admin",
				Description: "Admin access across multiple projects",
				Inherits: []string{
					"project_a_access",
					"project_b_access",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"iam.serviceAccounts.*",
					),
				},
				Enabled: true,
			},
		}

		config := newTestConfig(t, roles, nil)

		identity := &models.Identity{
			ID: "multi-admin",
			User: &models.User{
				Username: "multiadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "multi_project_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Collect all targets from allow statements
		var allowTargets []string
		for _, stmt := range result.Permissions.Allow {
			allowTargets = append(allowTargets, stmt.Targets...)
		}

		// Should have merged targets from inherited roles
		expectedTargets := []string{
			"projects/project-a/*",
			"projects/project-b/*",
		}
		assert.ElementsMatch(t, expectedTargets, allowTargets)
	})
}
