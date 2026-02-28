package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestGCPComplexInheritance exercises complex, deeply-nested GCP role
// inheritance with GCP pre-defined role resolution (via the embedded IAM
// dataset), scope allow/deny filtering, composite-role marking, wildcard
// expansion (dot-wildcards for SupportsWildcards=false), deny-permission
// survival, and target preservation across 1-3 levels of thand-role nesting.
//
// GCP pre-defined roles used throughout:
//   - roles/compute.instanceAdmin.v1
//   - roles/storage.objectAdmin
//   - roles/bigquery.admin
//   - roles/iam.securityAdmin
//   - roles/compute.networkAdmin
//   - roles/viewer
func TestGCPComplexInheritance(t *testing.T) {
	// Shared GCP providers used by most subtests.
	gcpProviders := map[string]models.ProviderConfig{
		"gcp-prod": {
			Name:        "gcp-prod",
			Description: "GCP Production",
			Provider:    "gcp",
		},
		"gcp-dev": {
			Name:        "gcp-dev",
			Description: "GCP Development",
			Provider:    "gcp",
		},
	}

	// ---------------------------------------------------------------
	// 1. Provider-only inheritance is NOT composite (depth 1)
	// ---------------------------------------------------------------
	t.Run("provider-only inheritance is not composite", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_infra_viewer": {
				Name:        "GCP Infra Viewer",
				Description: "Inherits only GCP pre-defined roles, no thand roles",
				Inherits: []string{
					"roles/compute.instanceAdmin.v1",
					"roles/storage.objectAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"monitoring.timeSeries.list",
						"logging.logEntries.list",
					),
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, gcpProviders)

		identity := &models.Identity{
			ID: "viewer1",
			User: &models.User{
				Username: "viewer",
				Email:    "viewer@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "gcp_infra_viewer")
		require.NoError(t, err)
		require.NotNil(t, result)

		// No thand-role inheritance → Composite must be false
		assert.False(t, result.Composite,
			"Role inheriting only GCP pre-defined roles should NOT be composite")

		// GCP pre-defined role names should be resolved and kept in Inherits
		assert.ElementsMatch(t, []string{
			"roles/compute.instanceAdmin.v1",
			"roles/storage.objectAdmin",
		}, result.Inherits,
			"GCP pre-defined roles should be preserved in Inherits: got %v", result.Inherits)

		// Own permissions should be preserved
		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "monitoring.timeSeries.list",
			"Own permission should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "logging.logEntries.list",
			"Own permission should be present: got %v", allowOps)

		assert.ElementsMatch(t, []string{"gcp-prod", "gcp-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 2. Thand role + multiple GCP pre-defined roles (depth 1)
	// ---------------------------------------------------------------
	t.Run("thand role plus multiple GCP pre-defined roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_base_ops": {
				Name:        "GCP Base Ops",
				Description: "Baseline GCP operational permissions",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"resourcemanager.projects.get",
						"iam.serviceAccounts.list",
						"logging.logEntries.list",
					),
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
			"gcp_senior_ops": {
				Name:        "GCP Senior Ops",
				Description: "Senior operator with thand + GCP pre-defined role inheritance",
				Inherits: []string{
					"gcp_base_ops",                   // thand role → merged
					"roles/compute.instanceAdmin.v1", // GCP pre-defined → kept
					"roles/storage.objectAdmin",      // GCP pre-defined → kept
					"roles/bigquery.admin",           // GCP pre-defined → kept
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.get",
						"compute.instances.list",
						"storage.buckets.list",
					),
					Deny: stmts(
						"compute.instances.delete",
						"storage.objects.delete",
					),
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, gcpProviders)

		identity := &models.Identity{
			ID: "ops-senior",
			User: &models.User{
				Username: "senior",
				Email:    "senior@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "gcp_senior_ops")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Inherited from thand role gcp_base_ops → composite
		assert.True(t, result.Composite,
			"Role inheriting from a thand role should be composite")

		// All three GCP pre-defined roles should be in Inherits,
		// gcp_base_ops was a thand role so it's merged and removed.
		assert.Len(t, result.Inherits, 3,
			"Exactly 3 GCP pre-defined roles should be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/compute.instanceAdmin.v1",
			"compute.instanceAdmin.v1 must be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/storage.objectAdmin",
			"storage.objectAdmin must be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/bigquery.admin",
			"bigquery.admin must be in Inherits: got %v", result.Inherits)

		// Validate merged allow permissions
		allowOps := collectAllOps(result.Permissions.Allow)

		// gcp_senior_ops own permissions must be present
		assert.Contains(t, allowOps, "compute.instances.get",
			"own compute.instances.get should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "compute.instances.list",
			"own compute.instances.list should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "storage.buckets.list",
			"own storage.buckets.list should be present: got %v", allowOps)

		// gcp_base_ops permissions must be merged in
		assert.Contains(t, allowOps, "resourcemanager.projects.get",
			"base ops resourcemanager.projects.get should be merged: got %v", allowOps)
		assert.Contains(t, allowOps, "iam.serviceAccounts.list",
			"base ops iam.serviceAccounts.list should be merged: got %v", allowOps)
		assert.Contains(t, allowOps, "logging.logEntries.list",
			"base ops logging.logEntries.list should be merged: got %v", allowOps)

		// Deny permissions should survive
		denyOps := collectAllOps(result.Permissions.Deny)
		assert.Contains(t, denyOps, "compute.instances.delete",
			"deny compute.instances.delete should survive: got %v", denyOps)
		assert.Contains(t, denyOps, "storage.objects.delete",
			"deny storage.objects.delete should survive: got %v", denyOps)

		// Providers preserved
		assert.ElementsMatch(t, []string{"gcp-prod", "gcp-dev"}, result.Providers)

		// Metadata
		assert.Equal(t, "GCP Senior Ops", result.Name)
		assert.True(t, result.Enabled)
	})

	// ---------------------------------------------------------------
	// 3. Two-level deep with group and domain scopes (depth 2)
	// ---------------------------------------------------------------
	t.Run("two-level deep with group and domain scopes", func(t *testing.T) {
		roles := map[string]models.Role{
			"level0_gcp_viewer": {
				Name:        "Level-0 GCP Viewer",
				Description: "Base viewer scoped to example.com domain",
				Inherits: []string{
					"roles/viewer",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"resourcemanager.projects.get",
						"resourcemanager.projects.list",
						"storage.buckets.list",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
			"level1_gcp_developer": {
				Name:        "Level-1 GCP Developer",
				Description: "Developer inheriting viewer + compute pre-defined role, scoped to devs group",
				Inherits: []string{
					"level0_gcp_viewer",              // thand (depth 2)
					"roles/compute.instanceAdmin.v1", // GCP pre-defined
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.start",
						"compute.instances.stop",
						"storage.objects.create",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, gcpProviders)

		// Identity that satisfies BOTH scopes: domain example.com + group developers
		identity := &models.Identity{
			ID: "dev-user",
			User: &models.User{
				Username: "developer1",
				Email:    "dev@example.com",
				Groups:   []string{"developers", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "level1_gcp_developer")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Inherited from thand role level0_gcp_viewer → composite
		assert.True(t, result.Composite,
			"Two-level thand inheritance should be composite")

		// GCP pre-defined roles from BOTH levels should be in Inherits
		assert.ElementsMatch(t, []string{
			"roles/viewer",
			"roles/compute.instanceAdmin.v1",
		}, result.Inherits,
			"GCP roles from both inheritance levels should accumulate in Inherits: got %v", result.Inherits)

		// Merged allow permissions from both levels
		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "compute.instances.start",
			"level1 own perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "compute.instances.stop",
			"level1 own perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "storage.objects.create",
			"level1 own perm should be present: got %v", allowOps)

		// level0 permissions should be merged in
		assert.Contains(t, allowOps, "resourcemanager.projects.get",
			"level0 perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "resourcemanager.projects.list",
			"level0 perm should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "storage.buckets.list",
			"level0 perm should be present: got %v", allowOps)

		assert.ElementsMatch(t, []string{"gcp-prod", "gcp-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 4. Two-level scope denial skips middle role (depth 2)
	// ---------------------------------------------------------------
	t.Run("two-level scope denial skips middle role", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_base_perms": {
				Name:        "GCP Base Perms",
				Description: "Open base permissions",
				Inherits: []string{
					"roles/viewer",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"logging.logEntries.list",
						"monitoring.timeSeries.list",
					),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"gcp_restricted_layer": {
				Name:        "GCP Restricted Layer",
				Description: "High-value perms with pre-defined roles, deny-scoped to outsiders",
				Inherits: []string{
					"gcp_base_perms",
					"roles/compute.instanceAdmin.v1",
					"roles/storage.objectAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.get",
						"compute.instances.list",
						"storage.objects.get",
						"storage.objects.list",
					),
				},
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Users: []string{"outsider@example.com"},
					},
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"gcp_top_role": {
				Name:        "GCP Top Role",
				Description: "Top-level role inheriting the restricted layer",
				Inherits:    []string{"gcp_restricted_layer"},
				Permissions: models.RolePermissions{
					Allow: stmts("monitoring.dashboards.list"),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:        "gcp-prod",
				Description: "GCP Production",
				Provider:    "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Identity that is DENIED by gcp_restricted_layer's scope
		outsider := &models.Identity{
			ID: "outsider1",
			User: &models.User{
				Username: "outsider",
				Email:    "outsider@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(outsider, "gcp_top_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		// gcp_restricted_layer was skipped (scope deny), so its perms and
		// gcp_base_perms perms (transitive) should NOT be present
		allowOps := collectAllOps(result.Permissions.Allow)
		assert.ElementsMatch(t, []string{"monitoring.dashboards.list"}, allowOps,
			"Only gcp_top_role's own perms should remain when middle role is scope-denied: got %v", allowOps)

		// GCP pre-defined roles from the denied chain must NOT leak
		for _, inh := range result.Inherits {
			assert.NotEqual(t, "roles/compute.instanceAdmin.v1", inh,
				"compute.instanceAdmin.v1 should not propagate through scope-denied chain: got %v", result.Inherits)
			assert.NotEqual(t, "roles/storage.objectAdmin", inh,
				"storage.objectAdmin should not propagate through scope-denied chain: got %v", result.Inherits)
			assert.NotEqual(t, "roles/viewer", inh,
				"roles/viewer from base should not propagate through scope-denied chain: got %v", result.Inherits)
		}

		// No thand role was successfully merged → not composite
		assert.False(t, result.Composite,
			"Role should NOT be composite when inherited role is scope-denied")

		// ─── Now test with an allowed user ───
		allowed := &models.Identity{
			ID: "emp1",
			User: &models.User{
				Username: "employee",
				Email:    "employee@example.com",
			},
		}

		resultAllowed, err := config.GetCompositeRoleByName(allowed, "gcp_top_role")
		require.NoError(t, err)
		require.NotNil(t, resultAllowed)

		// Allowed user should get everything merged
		allowedOps := collectAllOps(resultAllowed.Permissions.Allow)
		assert.Contains(t, allowedOps, "monitoring.dashboards.list",
			"Allowed user should get top role perm: got %v", allowedOps)
		assert.Contains(t, allowedOps, "compute.instances.get",
			"Allowed user should get restricted layer perms: got %v", allowedOps)
		assert.Contains(t, allowedOps, "logging.logEntries.list",
			"Allowed user should get base perms: got %v", allowedOps)

		// GCP pre-defined roles from all levels should propagate for allowed user
		assert.Contains(t, resultAllowed.Inherits, "roles/compute.instanceAdmin.v1",
			"Allowed user should get compute.instanceAdmin.v1: got %v", resultAllowed.Inherits)
		assert.Contains(t, resultAllowed.Inherits, "roles/storage.objectAdmin",
			"Allowed user should get storage.objectAdmin: got %v", resultAllowed.Inherits)
		assert.Contains(t, resultAllowed.Inherits, "roles/viewer",
			"Allowed user should get roles/viewer from base: got %v", resultAllowed.Inherits)

		assert.True(t, resultAllowed.Composite,
			"Allowed user result should be composite since thand roles were merged")
	})

	// ---------------------------------------------------------------
	// 5. Three-level deep with mixed provider + thand roles (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level deep mixed provider and thand roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"tier0_gcp_baseline": {
				Name:        "Tier-0 GCP Baseline",
				Description: "Foundation role with roles/viewer",
				Inherits: []string{
					"roles/viewer",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"resourcemanager.projects.get",
						"resourcemanager.projects.list",
					),
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
			"tier1_gcp_team": {
				Name:        "Tier-1 GCP Team",
				Description: "Team role inheriting baseline, scoped to engineering",
				Inherits: []string{
					"tier0_gcp_baseline",        // thand (→ depth 3)
					"roles/storage.objectAdmin", // GCP pre-defined
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"compute.instances.get",
							"compute.instances.list",
							"storage.objects.get",
						},
						Targets: []string{
							"projects/team-project-*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
			"tier2_gcp_lead": {
				Name:        "Tier-2 GCP Lead",
				Description: "Team lead with iam.securityAdmin and bigquery.admin",
				Inherits: []string{
					"tier1_gcp_team",          // thand (→ tier0 → depth 3)
					"roles/iam.securityAdmin", // GCP pre-defined
					"roles/bigquery.admin",    // GCP pre-defined
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.start",
						"compute.instances.stop",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"lead@example.com"},
					},
				},
				Providers: []string{"gcp-prod", "gcp-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, gcpProviders)

		// Identity passes ALL three scope gates
		identity := &models.Identity{
			ID: "lead1",
			User: &models.User{
				Username: "teamlead",
				Email:    "lead@example.com",
				Groups:   []string{"engineering", "leads"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "tier2_gcp_lead")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand inheritance → composite
		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// GCP pre-defined roles accumulated from all tiers:
		// tier0: roles/viewer
		// tier1: roles/storage.objectAdmin
		// tier2: roles/iam.securityAdmin, roles/bigquery.admin
		assert.Len(t, result.Inherits, 4,
			"All 4 GCP pre-defined roles from all tiers should be present: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/viewer",
			"roles/viewer from tier0 should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/storage.objectAdmin",
			"roles/storage.objectAdmin from tier1 should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/iam.securityAdmin",
			"roles/iam.securityAdmin from tier2: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/bigquery.admin",
			"roles/bigquery.admin from tier2: got %v", result.Inherits)

		// Verify merged permissions from all three tiers
		allowOps := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "compute.instances.start",
			"tier2 perm: got %v", allowOps)
		assert.Contains(t, allowOps, "compute.instances.stop",
			"tier2 perm: got %v", allowOps)

		// tier0 permissions should be merged through the chain
		assert.Contains(t, allowOps, "resourcemanager.projects.get",
			"tier0 perm should merge through chain: got %v", allowOps)
		assert.Contains(t, allowOps, "resourcemanager.projects.list",
			"tier0 perm should merge through chain: got %v", allowOps)

		// tier1 permissions should be merged
		foundCompute := false
		foundStorage := false
		for _, op := range allowOps {
			if op == "compute.instances.get" || op == "compute.instances.list" {
				foundCompute = true
			}
			if op == "storage.objects.get" {
				foundStorage = true
			}
		}
		assert.True(t, foundCompute,
			"tier1 compute perms should be merged: got %v", allowOps)
		assert.True(t, foundStorage,
			"tier1 storage perms should be merged: got %v", allowOps)

		// Verify the tier1 targets are preserved through the merge
		var computeTargets []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if op == "compute.instances.get" || op == "compute.instances.list" || op == "storage.objects.get" {
					if len(stmt.Targets) > 0 {
						computeTargets = stmt.Targets
					}
					break
				}
			}
		}
		assert.ElementsMatch(t, []string{"projects/team-project-*"}, computeTargets,
			"Targets from tier1 should be preserved: got %v", computeTargets)

		assert.ElementsMatch(t, []string{"gcp-prod", "gcp-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 6. Three-level with deny + wildcard expansion via GCP provider (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level deny permissions and wildcard expansion with GCP provider", func(t *testing.T) {
		roles := map[string]models.Role{
			"l0_gcp_base": {
				Name:        "L0 GCP Base",
				Description: "Base level with specific compute and storage read perms",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.get",
						"compute.instances.list",
						"storage.buckets.get",
					),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"l1_gcp_power": {
				Name:        "L1 GCP Power User",
				Description: "Power user with compute wildcard, deny compute.instances.delete, inherits pre-defined role",
				Inherits: []string{
					"l0_gcp_base",
					"roles/storage.objectAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts("compute.instances.*"),
					Deny:  stmts("compute.instances.delete"),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"l2_gcp_admin": {
				Name:        "L2 GCP Admin",
				Description: "Admin with storage wildcard + deny mutations, plus compute.networkAdmin",
				Inherits: []string{
					"l1_gcp_power",
					"roles/compute.networkAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts("storage.objects.*"),
					Deny: stmts(
						"storage.objects.delete",
						"compute.instances.stop",
					),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:        "gcp-prod",
				Description: "GCP Production",
				Provider:    "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "gcpadmin",
				Email:    "admin@example.com",
			},
		}

		// ─── Sub-test: without provider (wildcards preserved) ───
		t.Run("without provider – wildcards preserved", func(t *testing.T) {
			result, err := config.GetCompositeRoleByName(identity, "l2_gcp_admin")
			require.NoError(t, err)
			require.NotNil(t, result)

			allowOps := collectAllOps(result.Permissions.Allow)

			// Wildcards should survive without a provider
			assert.Contains(t, allowOps, "storage.objects.*",
				"storage.objects.* wildcard should survive without provider: got %v", allowOps)

			// Inherited from thand roles → composite
			assert.True(t, result.Composite,
				"Three-level thand inheritance should be composite")

			// GCP pre-defined roles from all levels
			assert.Contains(t, result.Inherits, "roles/storage.objectAdmin",
				"roles/storage.objectAdmin from l1 should propagate: got %v", result.Inherits)
			assert.Contains(t, result.Inherits, "roles/compute.networkAdmin",
				"roles/compute.networkAdmin from l2 should be present: got %v", result.Inherits)
		})

		// ─── Sub-test: with GCP provider (wildcards expanded, conflicts resolved) ───
		t.Run("with GCP provider – wildcards expanded and conflicts resolved", func(t *testing.T) {
			gcpProdProvider := config.providerInstances["gcp-prod"]
			require.NotNil(t, gcpProdProvider, "gcp-prod provider instance must be initialised")

			baseRole := roles["l2_gcp_admin"]
			result, err := config.GetCompositeRoleForIdentity(identity, &baseRole, gcpProdProvider)
			require.NoError(t, err)
			require.NotNil(t, result)

			allowOps := collectAllOps(result.Permissions.Allow)
			denyOps := collectAllOps(result.Permissions.Deny)

			// No dot-wildcards should remain after GCP expansion
			assert.False(t, containsAnyWildcard(allowOps),
				"no dot-wildcards should remain after GCP provider expansion: %v", allowOps)

			// ── storage.objects.* expansion + deny interaction ──
			// l2's own allow has storage.objects.* and l2's own deny has
			// storage.objects.delete. During conflict resolution, the literal
			// "storage.objects.*" does not match "storage.objects.delete", so
			// both survive. Then GCP expansion turns storage.objects.* into
			// all concrete perms including storage.objects.delete. The result
			// is that storage.objects.delete appears in BOTH allow and deny —
			// the conflict is left to the cloud API (same pattern as
			// bigquery.tables.delete in TestGCPBigQueryAdminWildcardExpansion).
			assert.Contains(t, allowOps, "storage.objects.delete",
				"storage.objects.delete should appear in allow after expansion (conflict left to cloud API)")

			// ── compute.instances.* was expanded during l1's own resolution ──
			// Because l1_gcp_power had compute.instances.* in its own allow,
			// it was expanded to all individual perms during l1's resolution.
			// When l2 merges l1's resolved result:
			//  - compute.instances.stop: in parent deny, in child's expanded
			//    allow → parent deny removes it → NOT in final allow.
			//  - compute.instances.delete: in child's expanded allow AND in
			//    child's deny (from l1) → resolvePermissionConflicts removes
			//    it from BOTH → NOT in final allow, NOT in final deny.
			assert.NotContains(t, allowOps, "compute.instances.stop",
				"parent deny should remove compute.instances.stop from child's expanded allow")
			assert.NotContains(t, allowOps, "compute.instances.delete",
				"compute.instances.delete should be removed from both allow and deny by conflict resolution")

			// Non-denied storage.objects.* expansions must be present
			for _, expected := range []string{
				"storage.objects.get",
				"storage.objects.list",
				"storage.objects.create",
			} {
				assert.Contains(t, allowOps, expected,
					"non-denied expanded storage permission should be present: %q", expected)
			}

			// Non-denied compute.instances.* expansions must be present
			// (these survived: not in any deny list after resolution)
			for _, expected := range []string{
				"compute.instances.get",
				"compute.instances.list",
				"compute.instances.start",
			} {
				assert.Contains(t, allowOps, expected,
					"non-denied compute permission should survive: %q", expected)
			}

			// ── concrete perms from l0 ──
			assert.Contains(t, allowOps, "storage.buckets.get",
				"l0 concrete storage.buckets.get should be present: got %v", allowOps)

			// ── deny list ──
			// storage.objects.delete: in deny (parent's explicit deny) AND in
			// allow (from expansion) — conflict left to cloud API.
			assert.Contains(t, denyOps, "storage.objects.delete",
				"storage.objects.delete should be in deny list: got %v", denyOps)
			// compute.instances.stop: in deny (removed from allow by parent deny).
			assert.Contains(t, denyOps, "compute.instances.stop",
				"compute.instances.stop should be in deny list: got %v", denyOps)
			// compute.instances.delete: removed from BOTH allow and deny by
			// resolvePermissionConflicts (it appeared in l1's expanded allow
			// and l1's own deny — a self-contradiction within the merged role).
			assert.NotContains(t, denyOps, "compute.instances.delete",
				"compute.instances.delete should be removed from both allow and deny")

			// GCP pre-defined roles should be present
			assert.Contains(t, result.Inherits, "roles/storage.objectAdmin",
				"roles/storage.objectAdmin should be in Inherits: got %v", result.Inherits)
			assert.Contains(t, result.Inherits, "roles/compute.networkAdmin",
				"roles/compute.networkAdmin should be in Inherits: got %v", result.Inherits)
		})
	})

	// ---------------------------------------------------------------
	// 7. One inherited role allowed, another denied at same level (depth 1)
	// ---------------------------------------------------------------
	t.Run("one inherited role allowed and another denied at same level", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_open_base": {
				Name:        "GCP Open Base",
				Description: "Open to all, no scope, inherits storage.objectAdmin",
				Inherits: []string{
					"roles/storage.objectAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"storage.objects.get",
						"storage.objects.list",
						"storage.buckets.list",
					),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"gcp_restricted_admin": {
				Name:        "GCP Restricted Admin",
				Description: "Admin perms restricted to admins group, inherits compute.instanceAdmin.v1",
				Inherits: []string{
					"roles/compute.instanceAdmin.v1",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.get",
						"compute.instances.list",
						"compute.instances.start",
						"compute.instances.stop",
						"iam.serviceAccounts.get",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins"},
					},
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"gcp_combined_role": {
				Name:        "GCP Combined",
				Description: "Inherits both open_base and restricted_admin",
				Inherits: []string{
					"gcp_open_base",
					"gcp_restricted_admin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts("monitoring.dashboards.list"),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:     "gcp-prod",
				Provider: "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		// ── Non-admin: gcp_open_base merges, gcp_restricted_admin skipped ──
		nonadmin := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		resultDev, err := config.GetCompositeRoleByName(nonadmin, "gcp_combined_role")
		require.NoError(t, err)
		require.NotNil(t, resultDev)

		devAllowOps := collectAllOps(resultDev.Permissions.Allow)

		// gcp_open_base's storage permissions should be merged
		assert.Contains(t, devAllowOps, "storage.objects.get",
			"Non-admin should get storage perms from open_base: got %v", devAllowOps)
		assert.Contains(t, devAllowOps, "storage.buckets.list",
			"Non-admin should get storage perms from open_base: got %v", devAllowOps)

		// monitoring from gcp_combined_role itself
		assert.Contains(t, devAllowOps, "monitoring.dashboards.list",
			"Non-admin should get monitoring perm: got %v", devAllowOps)

		// gcp_restricted_admin's compute/iam perms must NOT be present
		for _, op := range devAllowOps {
			if op == "compute.instances.get" || op == "compute.instances.list" ||
				op == "compute.instances.start" || op == "compute.instances.stop" {
				t.Errorf("compute perms from denied gcp_restricted_admin must not appear for non-admin: got %v", devAllowOps)
				break
			}
			if op == "iam.serviceAccounts.get" {
				t.Errorf("iam perms from denied gcp_restricted_admin must not appear for non-admin: got %v", devAllowOps)
			}
		}

		// storage.objectAdmin from open_base should propagate
		assert.Contains(t, resultDev.Inherits, "roles/storage.objectAdmin",
			"Non-admin should get storage.objectAdmin from open_base: got %v", resultDev.Inherits)

		// compute.instanceAdmin.v1 from restricted_admin must NOT propagate
		for _, inh := range resultDev.Inherits {
			assert.NotEqual(t, "roles/compute.instanceAdmin.v1", inh,
				"compute.instanceAdmin.v1 must not propagate from denied restricted_admin: got %v", resultDev.Inherits)
		}

		// Still composite — gcp_open_base WAS merged
		assert.True(t, resultDev.Composite,
			"Should be composite since gcp_open_base was successfully merged")

		// ── Admin user: BOTH should merge ──
		adminUser := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
				Groups:   []string{"admins", "developers"},
			},
		}

		resultAdmin, err := config.GetCompositeRoleByName(adminUser, "gcp_combined_role")
		require.NoError(t, err)
		require.NotNil(t, resultAdmin)

		adminAllowOps := collectAllOps(resultAdmin.Permissions.Allow)

		// Admin should get everything from both roles
		assert.Contains(t, adminAllowOps, "monitoring.dashboards.list",
			"Admin should get monitoring: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "compute.instances.get",
			"Admin should get compute from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "compute.instances.start",
			"Admin should get compute from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "storage.objects.get",
			"Admin should get storage from open_base: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "iam.serviceAccounts.get",
			"Admin should get iam from restricted_admin: got %v", adminAllowOps)

		// Both GCP pre-defined roles should be in Inherits
		assert.Contains(t, resultAdmin.Inherits, "roles/storage.objectAdmin",
			"Admin should get storage.objectAdmin: got %v", resultAdmin.Inherits)
		assert.Contains(t, resultAdmin.Inherits, "roles/compute.instanceAdmin.v1",
			"Admin should get compute.instanceAdmin.v1: got %v", resultAdmin.Inherits)

		assert.True(t, resultAdmin.Composite,
			"Admin result should be composite since thand roles were merged")
	})

	// ---------------------------------------------------------------
	// 8. Deny scope takes precedence over allow scope for same user
	// ---------------------------------------------------------------
	t.Run("deny scope takes precedence over allow scope for same user", func(t *testing.T) {
		roles := map[string]models.Role{
			"gcp_privileged": {
				Name:        "GCP Privileged",
				Description: "High-privilege role with both allow and deny scopes + GCP pre-defined roles",
				Inherits: []string{
					"roles/bigquery.admin",
					"roles/compute.instanceAdmin.v1",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.get",
						"compute.instances.list",
						"compute.instances.start",
						"bigquery.datasets.get",
						"bigquery.tables.get",
						"storage.objects.get",
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
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"gcp_wrapper": {
				Name:        "GCP Wrapper",
				Description: "Wraps privileged role",
				Inherits:    []string{"gcp_privileged"},
				Permissions: models.RolePermissions{
					Allow: stmts("monitoring.timeSeries.list"),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:     "gcp-prod",
				Provider: "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Intern: in engineering group (matches allow) BUT explicitly in deny list
		// Deny should take precedence
		intern := &models.Identity{
			ID: "intern1",
			User: &models.User{
				Username: "intern",
				Email:    "intern@example.com",
				Groups:   []string{"engineering", "interns"},
			},
		}

		resultIntern, err := config.GetCompositeRoleByName(intern, "gcp_wrapper")
		require.NoError(t, err)
		require.NotNil(t, resultIntern)

		internOps := collectAllOps(resultIntern.Permissions.Allow)

		// Only wrapper's own perms
		assert.ElementsMatch(t, []string{"monitoring.timeSeries.list"}, internOps,
			"Intern (deny-scoped) should only get wrapper's own perms: got %v", internOps)

		// GCP pre-defined roles must NOT propagate
		assert.Empty(t, resultIntern.Inherits,
			"Intern should not get GCP pre-defined roles from denied privileged role: got %v", resultIntern.Inherits)

		assert.False(t, resultIntern.Composite,
			"Intern should not get composite when privileged role was denied")

		// Regular engineer: in engineering group, NOT in deny list → gets everything
		engineer := &models.Identity{
			ID: "eng1",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEng, err := config.GetCompositeRoleByName(engineer, "gcp_wrapper")
		require.NoError(t, err)
		require.NotNil(t, resultEng)

		engOps := collectAllOps(resultEng.Permissions.Allow)
		assert.Contains(t, engOps, "compute.instances.get",
			"Engineer should get compute perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "bigquery.datasets.get",
			"Engineer should get bigquery perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "storage.objects.get",
			"Engineer should get storage perms from privileged: got %v", engOps)
		assert.Contains(t, engOps, "monitoring.timeSeries.list",
			"Engineer should get monitoring from wrapper: got %v", engOps)

		assert.Contains(t, resultEng.Inherits, "roles/bigquery.admin",
			"Engineer should get bigquery.admin: got %v", resultEng.Inherits)
		assert.Contains(t, resultEng.Inherits, "roles/compute.instanceAdmin.v1",
			"Engineer should get compute.instanceAdmin.v1: got %v", resultEng.Inherits)

		assert.True(t, resultEng.Composite,
			"Engineer should get composite role")
	})

	// ---------------------------------------------------------------
	// 9. Three-level: middle role scoped by domain, bottom role scoped
	//    by group — user fails middle scope (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level: user fails middle domain scope", func(t *testing.T) {
		roles := map[string]models.Role{
			"bottom_gcp_base": {
				Name:        "Bottom GCP Base",
				Description: "Foundation with IAM pre-defined role",
				Inherits: []string{
					"roles/iam.securityAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"resourcemanager.projects.get",
						"iam.serviceAccounts.list",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering", "ops"},
					},
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"middle_gcp_team": {
				Name:        "Middle GCP Team",
				Description: "Team role scoped to acme.com domain",
				Inherits: []string{
					"bottom_gcp_base",
					"roles/storage.objectAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"storage.objects.get",
						"storage.objects.create",
						"compute.instances.list",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"acme.com"},
					},
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
			"top_gcp_lead": {
				Name:        "Top GCP Lead",
				Description: "Lead role, no scope restriction, inherits bigquery.admin",
				Inherits: []string{
					"middle_gcp_team",
					"roles/bigquery.admin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"bigquery.datasets.get",
						"bigquery.tables.list",
					),
				},
				Providers: []string{"gcp-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:     "gcp-prod",
				Provider: "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		// ── User from external.com (fails middle_gcp_team's acme.com domain scope) ──
		externalUser := &models.Identity{
			ID: "ext1",
			User: &models.User{
				Username: "external",
				Email:    "user@external.com",
				Groups:   []string{"engineering"},
			},
		}

		resultExt, err := config.GetCompositeRoleByName(externalUser, "top_gcp_lead")
		require.NoError(t, err)
		require.NotNil(t, resultExt)

		extAllowOps := collectAllOps(resultExt.Permissions.Allow)

		// Only top_gcp_lead's own perms should remain
		assert.Contains(t, extAllowOps, "bigquery.datasets.get",
			"External user should get top lead's bigquery perm: got %v", extAllowOps)
		assert.Contains(t, extAllowOps, "bigquery.tables.list",
			"External user should get top lead's bigquery perm: got %v", extAllowOps)

		// middle_gcp_team's storage/compute permissions should NOT be present
		for _, op := range extAllowOps {
			if op == "storage.objects.get" || op == "storage.objects.create" {
				t.Errorf("storage perms from scope-denied middle should not appear for external user: got %v", extAllowOps)
				break
			}
			if op == "compute.instances.list" {
				t.Errorf("compute perms from scope-denied middle should not appear for external user: got %v", extAllowOps)
			}
		}

		// bottom_gcp_base's permissions should NOT propagate (transitive through denied middle)
		for _, op := range extAllowOps {
			if op == "resourcemanager.projects.get" || op == "iam.serviceAccounts.list" {
				t.Errorf("bottom perms must not leak through denied middle: got %v", extAllowOps)
				break
			}
		}

		// iam.securityAdmin from bottom must NOT propagate
		for _, inh := range resultExt.Inherits {
			assert.NotEqual(t, "roles/iam.securityAdmin", inh,
				"iam.securityAdmin from bottom should not propagate through denied middle: got %v", resultExt.Inherits)
			assert.NotEqual(t, "roles/storage.objectAdmin", inh,
				"storage.objectAdmin from middle should not propagate when denied: got %v", resultExt.Inherits)
		}

		// top_gcp_lead's own bigquery.admin should still be there
		assert.Contains(t, resultExt.Inherits, "roles/bigquery.admin",
			"Top lead's own bigquery.admin should remain: got %v", resultExt.Inherits)

		// Not composite — middle_gcp_team was denied, so no thand role merged
		assert.False(t, resultExt.Composite,
			"External user should not get composite role when middle was denied")

		// ── User from acme.com + engineering group (passes ALL scopes) ──
		acmeUser := &models.Identity{
			ID: "acme1",
			User: &models.User{
				Username: "acmedev",
				Email:    "dev@acme.com",
				Groups:   []string{"engineering"},
			},
		}

		resultAcme, err := config.GetCompositeRoleByName(acmeUser, "top_gcp_lead")
		require.NoError(t, err)
		require.NotNil(t, resultAcme)

		acmeAllowOps := collectAllOps(resultAcme.Permissions.Allow)

		// Should get everything merged from all three levels
		assert.Contains(t, acmeAllowOps, "bigquery.datasets.get",
			"acme user should get bigquery perms: got %v", acmeAllowOps)
		assert.Contains(t, acmeAllowOps, "storage.objects.get",
			"acme user should get storage from middle: got %v", acmeAllowOps)
		assert.Contains(t, acmeAllowOps, "compute.instances.list",
			"acme user should get compute from middle: got %v", acmeAllowOps)
		assert.Contains(t, acmeAllowOps, "resourcemanager.projects.get",
			"acme user should get resourcemanager from bottom: got %v", acmeAllowOps)
		assert.Contains(t, acmeAllowOps, "iam.serviceAccounts.list",
			"acme user should get iam from bottom: got %v", acmeAllowOps)

		// GCP pre-defined roles from all levels should propagate
		assert.Contains(t, resultAcme.Inherits, "roles/bigquery.admin",
			"acme user should get bigquery.admin: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "roles/storage.objectAdmin",
			"acme user should get storage.objectAdmin from middle: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "roles/iam.securityAdmin",
			"acme user should get iam.securityAdmin from bottom: got %v", resultAcme.Inherits)

		assert.True(t, resultAcme.Composite,
			"acme user should get composite role when all scopes pass")
	})

	// ---------------------------------------------------------------
	// 10. Three-level with wildcard expansion + pre-defined roles
	//     at every tier, resolved with GCP provider (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level full expansion with pre-defined roles at every tier", func(t *testing.T) {
		roles := map[string]models.Role{
			// Leaf: storage wildcard + concrete compute perms
			"gcp_leaf_storage": {
				Name:    "GCP Leaf Storage",
				Enabled: true,
				Inherits: []string{
					"roles/viewer",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"storage.buckets.*",
						"compute.instances.get",
					),
				},
			},
			// Mid: inherits leaf, adds bigquery wildcard + deny for dataset delete
			"gcp_mid_analyst": {
				Name:    "GCP Mid Analyst",
				Enabled: true,
				Inherits: []string{
					"gcp_leaf_storage",
					"roles/storage.objectAdmin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"bigquery.datasets.*",
						"bigquery.tables.get",
						"bigquery.tables.list",
					),
					Deny: stmts("bigquery.datasets.delete"),
				},
			},
			// Top: inherits mid, adds compute wildcard + denies mutations
			"gcp_top_platform": {
				Name:    "GCP Top Platform",
				Enabled: true,
				Inherits: []string{
					"gcp_mid_analyst",
					"roles/compute.instanceAdmin.v1",
					"roles/bigquery.admin",
				},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"compute.instances.*",
						"iam.serviceAccounts.get",
						"iam.serviceAccounts.list",
					),
					Deny: stmts(
						"compute.instances.delete",
						"storage.buckets.delete",
					),
				},
				Providers: []string{"gcp-prod"},
			},
		}

		providers := map[string]models.ProviderConfig{
			"gcp-prod": {
				Name:     "gcp-prod",
				Provider: "gcp",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID:   "platform-eng",
			User: &models.User{Username: "platformeng", Email: "eng@example.com"},
		}

		gcpProvider := config.providerInstances["gcp-prod"]
		require.NotNil(t, gcpProvider, "gcp-prod must be initialised")

		baseRole := roles["gcp_top_platform"]
		result, err := config.GetCompositeRoleForIdentity(identity, &baseRole, gcpProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)
		denyOps := collectAllOps(result.Permissions.Deny)

		// ── No wildcards survive after GCP expansion ──
		assert.False(t, containsAnyWildcard(allowOps),
			"no dot-wildcards should remain after GCP expansion: %v", allowOps)

		// ── compute.instances.* expansion (from top) ──
		// top's own allow has compute.instances.* and top's own deny has
		// compute.instances.delete. During conflict resolution, the literal
		// "compute.instances.*" doesn't match "compute.instances.delete", so
		// both survive. Then GCP expansion turns compute.instances.* into all
		// concrete perms including compute.instances.delete. The result is that
		// compute.instances.delete appears in BOTH allow and deny — the conflict
		// is left to the cloud API (same pattern as bigquery.tables.delete in
		// TestGCPBigQueryAdminWildcardExpansion).
		assert.Contains(t, allowOps, "compute.instances.delete",
			"compute.instances.delete should appear in allow after expansion (conflict left to cloud API)")
		// Non-denied compute perms should be present
		for _, expected := range []string{
			"compute.instances.get",
			"compute.instances.list",
			"compute.instances.start",
			"compute.instances.setMetadata",
		} {
			assert.Contains(t, allowOps, expected,
				"non-denied compute perm should be present: %q", expected)
		}

		// ── storage.buckets.* expansion (from leaf) ──
		// The leaf's storage.buckets.* was expanded during the leaf's own
		// resolution (including storage.buckets.delete). When top merges
		// the mid's resolved result (which contains the leaf's expanded
		// perms), top's parent deny for storage.buckets.delete removes it
		// from the child's expanded allow. So storage.buckets.delete is
		// NOT in the final allow — it only appears in the deny list.
		assert.NotContains(t, allowOps, "storage.buckets.delete",
			"parent deny should remove storage.buckets.delete from child's expanded allow")
		for _, expected := range []string{
			"storage.buckets.get",
			"storage.buckets.list",
			"storage.buckets.create",
		} {
			assert.Contains(t, allowOps, expected,
				"non-denied storage bucket perm should be present: %q", expected)
		}

		// ── bigquery.datasets.* expansion (from mid) ──
		// mid's bigquery.datasets.* was expanded during mid's own resolution
		// (including bigquery.datasets.delete). Mid's own deny also had
		// bigquery.datasets.delete. Both propagate to the top-level merge.
		// resolvePermissionConflicts detects the same op in both allow and
		// deny → removes from BOTH. So bigquery.datasets.delete appears in
		// neither the final allow nor the final deny.
		assert.NotContains(t, allowOps, "bigquery.datasets.delete",
			"bigquery.datasets.delete should be removed from allow by conflict resolution")
		for _, expected := range []string{
			"bigquery.datasets.get",
			"bigquery.datasets.create",
			"bigquery.datasets.update",
		} {
			assert.Contains(t, allowOps, expected,
				"non-denied bigquery dataset perm should be present: %q", expected)
		}

		// ── Deny list ──
		// compute.instances.delete: in deny AND in allow (from top's own
		// wildcard expansion) — conflict left to cloud API.
		assert.Contains(t, denyOps, "compute.instances.delete",
			"compute.instances.delete should be in deny: got %v", denyOps)
		// storage.buckets.delete: in deny, removed from allow by parent deny.
		assert.Contains(t, denyOps, "storage.buckets.delete",
			"storage.buckets.delete should be in deny: got %v", denyOps)
		// bigquery.datasets.delete: removed from BOTH by conflict resolution.
		assert.NotContains(t, denyOps, "bigquery.datasets.delete",
			"bigquery.datasets.delete should be removed from both allow and deny by conflict resolution")

		// ── Concrete perms from mid should survive ──
		assert.Contains(t, allowOps, "bigquery.tables.get",
			"mid concrete bigquery.tables.get should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "bigquery.tables.list",
			"mid concrete bigquery.tables.list should be present: got %v", allowOps)

		// ── Concrete perms from top level ──
		assert.Contains(t, allowOps, "iam.serviceAccounts.get",
			"top iam.serviceAccounts.get should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "iam.serviceAccounts.list",
			"top iam.serviceAccounts.list should be present: got %v", allowOps)

		// ── GCP pre-defined roles from all three tiers ──
		assert.Contains(t, result.Inherits, "roles/viewer",
			"roles/viewer from leaf should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/storage.objectAdmin",
			"roles/storage.objectAdmin from mid should bubble up: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/compute.instanceAdmin.v1",
			"roles/compute.instanceAdmin.v1 from top: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "roles/bigquery.admin",
			"roles/bigquery.admin from top: got %v", result.Inherits)

		// Composite since thand roles were merged
		assert.True(t, result.Composite,
			"Three-level thand inheritance with GCP provider expansion should be composite")
	})
}
