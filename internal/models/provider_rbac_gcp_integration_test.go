package models_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/gcp"
)

// gcpProviderPerms loads the GCP permission list via the provider's
// shared data singleton and wraps it into the SearchResult slice
// expected by ValidatePermissionsPublic.
func gcpProviderPerms(t *testing.T) []models.SearchResult[models.ProviderPermission] {
	t.Helper()
	permissions, err := gcp.GetPermissions("")
	require.NoError(t, err, "gcp.GetPermissions() failed")
	require.NotEmpty(t, permissions, "gcp provider returned no permissions")

	results := make([]models.SearchResult[models.ProviderPermission], 0, len(permissions))
	for _, p := range permissions {
		results = append(results, models.SearchResult[models.ProviderPermission]{
			Result: p,
		})
	}
	return results
}

// TestGCPWildcardExpansion verifies that GCP-style dot-separated wildcards
// like "compute.instances.*" expand against the real IAM dataset and remain
// expanded (not condensed) because GCP does not support wildcards at its API.
func TestGCPWildcardExpansion(t *testing.T) {
	perms := gcpProviderPerms(t)

	wildcards := []string{
		"compute.instances.*",
		"storage.buckets.*",
		"iam.serviceAccounts.*",
	}

	for _, wc := range wildcards {
		t.Run(wc, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{wc}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt, false)
			require.NoError(t, err, "%q should not error against real GCP data", wc)

			var resultOps []string
			for _, s := range got {
				resultOps = append(resultOps, s.Operations...)
			}

			// With supportsWildcards=false, the wildcard must NOT appear in
			// the output — only individual expanded permissions.
			assert.NotContains(t, resultOps, wc,
				"%q should be expanded, not condensed back", wc)
			assert.Greater(t, len(resultOps), 1,
				"%q should expand to multiple individual permissions", wc)
		})
	}
}

// TestGCPYamlPatternsAgainstRealDataset runs the permission patterns from
// config/roles/gcp.yaml against the real GCP IAM dataset and asserts that
// wildcards are expanded (not condensed) since GCP requires exact names.
func TestGCPYamlPatternsAgainstRealDataset(t *testing.T) {
	perms := gcpProviderPerms(t)

	// Patterns from config/roles/gcp.yaml gcp_admin role.
	patterns := []string{
		"compute.instances.*",
		"storage.buckets.*",
		"iam.serviceAccounts.*",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{pattern}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt, false)
			require.NoError(t, err, "pattern %q should not error against real GCP data", pattern)

			var resultOps []string
			for _, s := range got {
				resultOps = append(resultOps, s.Operations...)
			}

			// Wildcards must NOT survive — they should be expanded.
			assert.NotContains(t, resultOps, pattern,
				"pattern %q should be expanded, not condensed", pattern)
			assert.Greater(t, len(resultOps), 0,
				"pattern %q should expand to at least one permission", pattern)
		})
	}

	// Full expansion: the complete allow list must validate and expand
	// all wildcards into individual permissions.
	t.Run("full gcp_admin allow list expands all wildcards", func(t *testing.T) {
		allow := models.RoleStatements{{Operations: patterns}}
		got, err := models.ValidatePermissionsPublic(perms, allow, false)
		require.NoError(t, err)

		var allOps []string
		for _, s := range got {
			allOps = append(allOps, s.Operations...)
		}
		sort.Strings(allOps)
		assert.NotEmpty(t, allOps)

		// No wildcard patterns should remain.
		for _, p := range patterns {
			assert.NotContains(t, allOps, p,
				"wildcard %q should have been expanded", p)
		}
	})
}

// TestGCPExactPermissionValidation verifies that exact GCP permissions
// are validated against the real dataset and typos are rejected.
func TestGCPExactPermissionValidation(t *testing.T) {
	perms := gcpProviderPerms(t)

	// Valid exact permissions (dot-separated, no colon, no wildcard —
	// goes through the else branch).
	t.Run("exact compute.instances.get passes", func(t *testing.T) {
		stmt := models.RoleStatements{{Operations: []string{"compute.instances.get"}}}
		got, err := models.ValidatePermissionsPublic(perms, stmt, false)
		require.NoError(t, err)

		var resultOps []string
		for _, s := range got {
			resultOps = append(resultOps, s.Operations...)
		}
		assert.Equal(t, []string{"compute.instances.get"}, resultOps)
	})

	// Typo rejected
	t.Run("typo compute.instances.gett rejected", func(t *testing.T) {
		stmt := models.RoleStatements{{Operations: []string{"compute.instances.gett"}}}
		_, err := models.ValidatePermissionsPublic(perms, stmt, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was not found")
	})

	// Nonexistent service
	t.Run("nonexistent service rejected", func(t *testing.T) {
		stmt := models.RoleStatements{{Operations: []string{"fakeservice.things.read"}}}
		_, err := models.ValidatePermissionsPublic(perms, stmt, false)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was not found")
	})
}
