package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// containsAnyWildcard returns true if any operation string contains a bare
// GCP-style dot-separated wildcard (".*").
func containsAnyWildcard(ops []string) bool {
	for _, op := range ops {
		if len(op) > 2 && op[len(op)-2:] == ".*" {
			return true
		}
	}
	return false
}

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

// =============================================================================
// GCP BigQuery Admin – wildcard expansion + multi-level inheritance
// =============================================================================

// TestGCPBigQueryAdminWildcardExpansion mirrors the production request payload
// for the gcp_bigquery_admin role.  It verifies that:
//
//   - When resolved *without* a provider, dot-wildcard patterns like
//     "bigquery.datasets.*" are preserved as-is (no provider to drive expansion).
//   - When resolved *with* a GCP provider, every dot-wildcard is expanded to
//     individual concrete IAM permissions, none of the wildcard strings survive
//     in the final allow list, and the permissions inherited through a three-level
//     chain are all present.
//   - Parent-wins conflict resolution holds: a child role's explicit Deny for one
//     of the bigquery.tables.* permissions does NOT remove it from the final allow
//     list because the parent uses the wildcard "bigquery.tables.*" which takes
//     precedence over the child's concrete deny.
//
// Role hierarchy used in this test:
//
//	gcp_bigquery_admin  (top – wildcards + concrete + inherits two child roles)
//	├─ gcp_data_analyst (mid – concrete bigquery read + inherits leaf)
//	│   │   Deny: bigquery.tables.delete  ← overridden by parent wildcard allow
//	│   └─ gcp_storage_reader (leaf – concrete storage read perms)
//	└─ gcp_iam_viewer   (leaf – concrete IAM view perms)
func TestGCPBigQueryAdminWildcardExpansion(t *testing.T) {
	// --- role definitions ---------------------------------------------------

	roles := map[string]models.Role{
		// Leaf: read-only access to GCS objects and buckets.
		"gcp_storage_reader": {
			Name:        "GCP Storage Reader",
			Description: "Read-only access to Cloud Storage.",
			Permissions: models.RolePermissions{
				Allow: stmts(
					"storage.buckets.get",
					"storage.buckets.list",
					"storage.objects.get",
					"storage.objects.list",
				),
			},
			Enabled: true,
		},

		// Leaf: read-only IAM visibility.
		"gcp_iam_viewer": {
			Name:        "GCP IAM Viewer",
			Description: "View IAM policies and service accounts.",
			Permissions: models.RolePermissions{
				Allow: stmts(
					"iam.serviceAccounts.get",
					"iam.serviceAccounts.list",
					"resourcemanager.projects.get",
					"resourcemanager.projects.getIamPolicy",
				),
			},
			Enabled: true,
		},

		// Mid-tier: analyst inherits the storage reader and adds BigQuery read perms.
		// It explicitly Denies "bigquery.tables.delete" – the parent role's
		// "bigquery.tables.*" wildcard allow is expected to expand that perm into
		// the allow list, but note that dot-wildcard subsumption is processed at
		// provider-expansion time rather than conflict-resolution time, so the
		// child's deny for bigquery.tables.delete survives into the final deny list.
		"gcp_data_analyst": {
			Name:        "GCP Data Analyst",
			Description: "Read BigQuery data and Cloud Storage objects.",
			Inherits:    []string{"gcp_storage_reader"},
			Permissions: models.RolePermissions{
				Allow: stmts(
					"bigquery.datasets.get",
					// bigquery.datasets.list is intentionally omitted: the GCP IAM
					// dataset does not expose it as a directly-grantable permission
					// in custom roles, so it would fail provider validation.
					"bigquery.tables.getData",
					"bigquery.tables.get",
					"bigquery.tables.list",
					"bigquery.jobs.get",
					"bigquery.jobs.list",
				),
				// This deny propagates through to the final composite role.
				// bigquery.tables.delete also appears in the Allow list after
				// bigquery.tables.* is expanded by the GCP provider, so the
				// permission ends up in both Allow and Deny (conflict left to
				// the cloud API to resolve with its own precedence rules).
				Deny: stmts("bigquery.tables.delete"),
			},
			Enabled: true,
		},

		// Top-level admin: exactly the role from the production payload.
		// Uses dot-wildcards for all bigquery sub-services, plus concrete
		// compute/storage fallback perms.
		"gcp_bigquery_admin": {
			Name:        "Gcp BigQuery Admin",
			Description: "Full access to BigQuery resources for data analytics.",
			Identifier:  "gcp_bigquery_admin",
			Providers:   []string{"gcp-prod", "gcp-dev"},
			Workflows:   []string{"slack_approval"},
			Inherits: []string{
				"gcp_data_analyst",
				"gcp_iam_viewer",
			},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{
						"bigquery.datasets.*",
						"bigquery.jobs.*",
						"bigquery.tables.*",
						"compute.instances.get",
						"compute.instances.list",
						"storage.buckets.list",
						"storage.objects.get",
						"storage.objects.list",
					},
				}},
			},
			Enabled: true,
		},
	}

	// --- providers ----------------------------------------------------------

	gcpProviders := map[string]models.ProviderConfig{
		"gcp-prod": {
			Name:        "GCP Production",
			Description: "GCP Production Environment",
			Provider:    "gcp",
		},
		"gcp-dev": {
			Name:        "GCP Development",
			Description: "GCP Development Environment",
			Provider:    "gcp",
		},
	}

	// --- identity (mirrors the production payload) --------------------------

	identity := &models.Identity{
		ID: "c682c264-c0c1-706f-be7a-c9eda38c45bc",
		User: &models.User{
			Username: "hugh@thand.io",
			Email:    "hugh@thand.io",
			Groups:   []string{"Everyone"},
		},
	}

	// =========================================================================
	// Sub-test 1: without provider – wildcards are preserved
	// =========================================================================
	t.Run("without provider – dot-wildcards are kept as-is", func(t *testing.T) {
		cfg := newTestConfig(t, roles, gcpProviders)

		result, err := cfg.GetCompositeRoleByName(identity, "gcp_bigquery_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)

		// Without a provider argument no expansion happens; the three wildcard
		// patterns from the role definition must still be present.
		assert.Contains(t, ops, "bigquery.datasets.*", "wildcard should survive without a provider")
		assert.Contains(t, ops, "bigquery.jobs.*", "wildcard should survive without a provider")
		assert.Contains(t, ops, "bigquery.tables.*", "wildcard should survive without a provider")

		// But individual permissions from inherited roles should still be merged in.
		assert.Contains(t, ops, "iam.serviceAccounts.get", "inherited IAM perm should be present")
		assert.Contains(t, ops, "storage.objects.get", "inherited storage perm should be present")
	})

	// =========================================================================
	// Sub-test 2: with GCP provider – wildcards expand, none survive
	// =========================================================================
	t.Run("with GCP provider – dot-wildcards are fully expanded", func(t *testing.T) {
		cfg := newTestConfig(t, roles, gcpProviders)

		gcpProdProvider := cfg.providerInstances["gcp-prod"]
		require.NotNil(t, gcpProdProvider, "gcp-prod provider instance must be initialised")

		baseRole := roles["gcp_bigquery_admin"]
		result, err := cfg.GetCompositeRoleForIdentity(identity, &baseRole, gcpProdProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)

		// No dot-wildcards (".*") should survive after expansion.
		assert.False(t, containsAnyWildcard(allowOps),
			"no dot-wildcards should remain after GCP provider expansion: %v", allowOps)

		// ── bigquery.datasets.* expansion ──────────────────────────────────
		// bigquery.datasets.list is intentionally excluded: it is NOT present
		// in the GCP IAM dataset as a grantable permission for custom roles.
		for _, expected := range []string{
			"bigquery.datasets.create",
			"bigquery.datasets.delete",
			"bigquery.datasets.get",
			"bigquery.datasets.update",
		} {
			assert.Contains(t, allowOps, expected,
				"bigquery.datasets.* should expand to include %q", expected)
		}

		// ── bigquery.jobs.* expansion ───────────────────────────────────────
		for _, expected := range []string{
			"bigquery.jobs.create",
			"bigquery.jobs.get",
			"bigquery.jobs.list",
		} {
			assert.Contains(t, allowOps, expected,
				"bigquery.jobs.* should expand to include %q", expected)
		}

		// ── bigquery.tables.* expansion ────────────────────────────────────
		for _, expected := range []string{
			"bigquery.tables.create",
			"bigquery.tables.delete",
			"bigquery.tables.get",
			"bigquery.tables.list",
			"bigquery.tables.update",
		} {
			assert.Contains(t, allowOps, expected,
				"bigquery.tables.* should expand to include %q", expected)
		}

		// ── concrete perms from the role itself ─────────────────────────────
		for _, expected := range []string{
			"compute.instances.get",
			"compute.instances.list",
			"storage.buckets.list",
			"storage.objects.get",
			"storage.objects.list",
		} {
			assert.Contains(t, allowOps, expected,
				"concrete permission from gcp_bigquery_admin should be present: %q", expected)
		}

		// ── perms inherited via gcp_data_analyst → gcp_storage_reader ───────
		for _, expected := range []string{
			"storage.buckets.get",
			"storage.objects.get",
			"storage.objects.list",
		} {
			assert.Contains(t, allowOps, expected,
				"inherited storage perm should be present: %q", expected)
		}

		// ── perms inherited directly from gcp_iam_viewer ────────────────────
		for _, expected := range []string{
			"iam.serviceAccounts.get",
			"iam.serviceAccounts.list",
			"resourcemanager.projects.get",
			"resourcemanager.projects.getIamPolicy",
		} {
			assert.Contains(t, allowOps, expected,
				"inherited IAM perm should be present: %q", expected)
		}

		// ── deny list ──────────────────────────────────────────────────────
		// bigquery.tables.delete from gcp_data_analyst's Deny propagates to the
		// final composite. The parent's bigquery.tables.* also expands to add
		// bigquery.tables.delete into Allow, so it ends up in both lists.
		// Dot-wildcard subsumption in conflict resolution is not performed
		// (only colon-wildcards are handled there); that decision is left to
		// the cloud provider's own IAM precedence rules.
		denyOps := collectAllOps(result.Permissions.Deny)
		assert.Contains(t, denyOps, "bigquery.tables.delete",
			"child deny for bigquery.tables.delete should propagate to final composite")
	})

	// =========================================================================
	// Sub-test 3: with both GCP providers – same expansion, both providers
	// =========================================================================
	t.Run("with both GCP providers – expansion happens for SupportsWildcards=false providers", func(t *testing.T) {
		cfg := newTestConfig(t, roles, gcpProviders)

		gcpProd := cfg.providerInstances["gcp-prod"]
		gcpDev := cfg.providerInstances["gcp-dev"]
		require.NotNil(t, gcpProd)
		require.NotNil(t, gcpDev)

		baseRole := roles["gcp_bigquery_admin"]
		result, err := cfg.GetCompositeRoleForIdentity(identity, &baseRole, gcpProd, gcpDev)
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)

		assert.False(t, containsAnyWildcard(allowOps),
			"no dot-wildcards should remain when both GCP providers are passed")
		assert.Contains(t, allowOps, "bigquery.datasets.create")
		assert.Contains(t, allowOps, "bigquery.tables.delete")
		assert.Contains(t, allowOps, "iam.serviceAccounts.list")
	})
}

// =============================================================================
// GCP parent-deny overrides wildcard child allow – no re-condensation
// =============================================================================

// TestGCPParentDenyOverridesWildcardChildAllow verifies the interaction between:
//   - A child role that uses dot-wildcard Allow statements (e.g. "storage.objects.*")
//   - A parent role that inherits the child but adds explicit Deny entries
//
// Core invariants being tested:
//  1. Parent Deny overrides Child Allow (parent-wins conflict resolution).
//  2. After expansion with a GCP provider, no wildcard patterns remain in the
//     output (SupportsWildcards=false ⇒ CondenseActions is skipped).
//  3. Only the specifically denied permissions are absent from the allow list;
//     all other expanded permissions from the wildcard are still present.
func TestGCPParentDenyOverridesWildcardChildAllow(t *testing.T) {
	gcpProviders := map[string]models.ProviderConfig{
		"gcp-prod": {
			Name:     "GCP Production",
			Provider: "gcp",
		},
	}

	identity := &models.Identity{
		ID:   "hugh-test",
		User: &models.User{Username: "hugh@thand.io", Email: "hugh@thand.io"},
	}

	// ─── Scenario 1 ──────────────────────────────────────────────────────────
	// Child:  storage.objects.*  (wildcard allow)
	// Parent: specific deny for create + delete
	// Expected: all storage.objects.* perms expand and survive except create/delete
	t.Run("storage objects wildcard child – parent denies create and delete", func(t *testing.T) {
		roles := map[string]models.Role{
			// Child: broad wildcard allow
			"gcp_storage_full": {
				Name:    "GCP Storage Full",
				Enabled: true,
				Permissions: models.RolePermissions{
					Allow: stmts("storage.objects.*"),
				},
			},
			// Parent: inherits child, then narrows via deny
			"gcp_storage_readonly": {
				Name:     "GCP Storage Read-Only",
				Enabled:  true,
				Inherits: []string{"gcp_storage_full"},
				Permissions: models.RolePermissions{
					Deny: stmts(
						"storage.objects.create",
						"storage.objects.delete",
					),
				},
			},
		}

		cfg := newTestConfig(t, roles, gcpProviders)
		gcpProvider := cfg.providerInstances["gcp-prod"]
		require.NotNil(t, gcpProvider)

		base := roles["gcp_storage_readonly"]
		result, err := cfg.GetCompositeRoleForIdentity(identity, &base, gcpProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)
		denyOps := collectAllOps(result.Permissions.Deny)

		// No wildcard patterns survive after GCP expansion.
		assert.False(t, containsAnyWildcard(allowOps),
			"GCP provider must expand wildcards; none should remain in allow")

		// Parent deny wins: these must NOT appear in allow.
		assert.NotContains(t, allowOps, "storage.objects.create",
			"parent deny for storage.objects.create must remove it from allow")
		assert.NotContains(t, allowOps, "storage.objects.delete",
			"parent deny for storage.objects.delete must remove it from allow")

		// All other expansions of storage.objects.* must be present.
		for _, expected := range []string{
			"storage.objects.get",
			"storage.objects.list",
		} {
			assert.Contains(t, allowOps, expected,
				"non-denied expanded permission should remain in allow: %q", expected)
		}

		// The parent's deny entries propagate to the final deny list.
		// The corresponding permissions are removed from allow (parent-wins), but
		// the deny statements themselves survive so downstream enforcement is clear.
		assert.Contains(t, denyOps, "storage.objects.create",
			"parent deny for storage.objects.create must appear in final deny list")
		assert.Contains(t, denyOps, "storage.objects.delete",
			"parent deny for storage.objects.delete must appear in final deny list")
	})

	// ─── Scenario 2 ──────────────────────────────────────────────────────────
	// Child:  bigquery.tables.* + bigquery.jobs.*  (two wildcard services)
	// Parent: denies all mutating table operations, leaving only read access
	// Expected: bigquery.jobs.* fully expanded, bigquery.tables.* expanded
	//           with the write ops absent, no wildcards anywhere.
	t.Run("bigquery multi-wildcard child – parent denies table write operations", func(t *testing.T) {
		roles := map[string]models.Role{
			// Child: full BigQuery access via wildcards
			"gcp_bigquery_full": {
				Name:    "GCP BigQuery Full",
				Enabled: true,
				Permissions: models.RolePermissions{
					Allow: stmts(
						"bigquery.tables.*",
						"bigquery.jobs.*",
					),
				},
			},
			// Parent: analyst view – deny all table mutation
			"gcp_bigquery_analyst": {
				Name:     "GCP BigQuery Analyst",
				Enabled:  true,
				Inherits: []string{"gcp_bigquery_full"},
				Permissions: models.RolePermissions{
					Deny: stmts(
						"bigquery.tables.create",
						"bigquery.tables.delete",
						"bigquery.tables.update",
						"bigquery.tables.updateData",
						"bigquery.tables.export",
					),
				},
			},
		}

		cfg := newTestConfig(t, roles, gcpProviders)
		gcpProvider := cfg.providerInstances["gcp-prod"]
		require.NotNil(t, gcpProvider)

		base := roles["gcp_bigquery_analyst"]
		result, err := cfg.GetCompositeRoleForIdentity(identity, &base, gcpProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)
		denyOps := collectAllOps(result.Permissions.Deny)

		// No wildcards in final output.
		assert.False(t, containsAnyWildcard(allowOps),
			"GCP provider must expand all wildcards in allow list")

		// Denied write ops must NOT appear in allow.
		for _, denied := range []string{
			"bigquery.tables.create",
			"bigquery.tables.delete",
			"bigquery.tables.update",
			"bigquery.tables.updateData",
			"bigquery.tables.export",
		} {
			assert.NotContains(t, allowOps, denied,
				"parent deny must prevent %q from appearing in allow", denied)
		}

		// Read ops from bigquery.tables.* must survive.
		for _, expected := range []string{
			"bigquery.tables.get",
			"bigquery.tables.getData",
			"bigquery.tables.list",
		} {
			assert.Contains(t, allowOps, expected,
				"non-denied table read permission must be present: %q", expected)
		}

		// All bigquery.jobs.* expansions must be present (no deny for jobs).
		for _, expected := range []string{
			"bigquery.jobs.create",
			"bigquery.jobs.get",
			"bigquery.jobs.list",
		} {
			assert.Contains(t, allowOps, expected,
				"undented job permission must be present: %q", expected)
		}

		// The parent's deny entries must appear in the final deny list.
		for _, denied := range []string{
			"bigquery.tables.create",
			"bigquery.tables.delete",
			"bigquery.tables.update",
			"bigquery.tables.updateData",
			"bigquery.tables.export",
		} {
			assert.Contains(t, denyOps, denied,
				"parent deny entry must appear in final deny list: %q", denied)
		}
	})

	// ─── Scenario 3 ──────────────────────────────────────────────────────────
	// Three-level chain:
	//   gcp_compute_leaf   (wildcard allow: compute.instances.*)
	//   gcp_compute_viewer (inherits leaf, adds specific concrete allows)
	//   gcp_compute_ops    (inherits viewer, denies start/stop/delete/reset)
	//
	// The deny at the top level must propagate through the chain, the wildcard
	// from the leaf must be fully expanded, and CondenseActions must never
	// re-condense the result.
	t.Run("three-level chain – leaf wildcard, mid adds perms, top denies mutations", func(t *testing.T) {
		roles := map[string]models.Role{
			// Leaf: full compute instance access via wildcard
			"gcp_compute_leaf": {
				Name:    "GCP Compute Leaf",
				Enabled: true,
				Permissions: models.RolePermissions{
					Allow: stmts("compute.instances.*"),
				},
			},
			// Mid: inherits leaf, adds explicit storage/iam perms + concrete compute perms
			"gcp_compute_viewer": {
				Name:     "GCP Compute Viewer",
				Enabled:  true,
				Inherits: []string{"gcp_compute_leaf"},
				Permissions: models.RolePermissions{
					Allow: stmts(
						"storage.buckets.list",
						"storage.buckets.get",
						"iam.serviceAccounts.list",
					),
				},
			},
			// Top: denies destructive compute instance operations
			"gcp_compute_ops": {
				Name:     "GCP Compute Ops",
				Enabled:  true,
				Inherits: []string{"gcp_compute_viewer"},
				Permissions: models.RolePermissions{
					Deny: stmts(
						"compute.instances.delete",
						"compute.instances.start",
						"compute.instances.stop",
						"compute.instances.reset",
					),
				},
			},
		}

		cfg := newTestConfig(t, roles, gcpProviders)
		gcpProvider := cfg.providerInstances["gcp-prod"]
		require.NotNil(t, gcpProvider)

		base := roles["gcp_compute_ops"]
		result, err := cfg.GetCompositeRoleForIdentity(identity, &base, gcpProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectAllOps(result.Permissions.Allow)
		denyOps := collectAllOps(result.Permissions.Deny)

		// No wildcards anywhere – CondenseActions must be skipped for GCP.
		assert.False(t, containsAnyWildcard(allowOps),
			"no dot-wildcards should remain in allow list after GCP expansion")

		// Top-level denied operations must not appear in allow.
		for _, denied := range []string{
			"compute.instances.delete",
			"compute.instances.start",
			"compute.instances.stop",
			"compute.instances.reset",
		} {
			assert.NotContains(t, allowOps, denied,
				"top-level deny must remove %q from allow", denied)
		}

		// Non-denied compute.instances.* expansions must be present.
		for _, expected := range []string{
			"compute.instances.get",
			"compute.instances.list",
			"compute.instances.setMetadata",
		} {
			assert.Contains(t, allowOps, expected,
				"non-denied compute permission must survive expansion: %q", expected)
		}

		// Mid-tier explicit allows must be present.
		for _, expected := range []string{
			"storage.buckets.list",
			"storage.buckets.get",
			"iam.serviceAccounts.list",
		} {
			assert.Contains(t, allowOps, expected,
				"mid-tier concrete allow must be present: %q", expected)
		}

		// The top-level deny entries must appear in the final deny list.
		for _, denied := range []string{
			"compute.instances.delete",
			"compute.instances.start",
			"compute.instances.stop",
			"compute.instances.reset",
		} {
			assert.Contains(t, denyOps, denied,
				"top-level deny entry must appear in final deny list: %q", denied)
		}
	})
}
