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

// TestGCPWildcardRoundTrip verifies that GCP-style dot-separated wildcards
// like "compute.instances.*" expand against the real IAM dataset and then
// condense back to the original wildcard pattern.
func TestGCPWildcardRoundTrip(t *testing.T) {
	perms := gcpProviderPerms(t)

	wildcards := []string{
		"compute.instances.*",
		"storage.buckets.*",
		"iam.serviceAccounts.*",
	}

	for _, wc := range wildcards {
		t.Run(wc, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{wc}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt)
			require.NoError(t, err, "%q should not error against real GCP data", wc)

			var resultOps []string
			for _, s := range got {
				resultOps = append(resultOps, s.Operations...)
			}

			// The wildcard should condense back to exactly one entry: itself.
			assert.Equal(t, []string{wc}, resultOps,
				"%q should round-trip to itself, not %d individual permissions", wc, len(resultOps),
			)
		})
	}
}

// TestGCPYamlPatternsAgainstRealDataset runs the permission patterns from
// config/roles/gcp.yaml against the real GCP IAM dataset and asserts that
// each wildcard round-trips and the full allow list validates cleanly.
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
			got, err := models.ValidatePermissionsPublic(perms, stmt)
			require.NoError(t, err, "pattern %q should not error against real GCP data", pattern)

			var resultOps []string
			for _, s := range got {
				resultOps = append(resultOps, s.Operations...)
			}

			assert.Contains(t, resultOps, pattern,
				"pattern %q should round-trip back to itself after validation", pattern,
			)
		})
	}

	// Full round-trip: the complete allow list must validate and condense
	// back to the original wildcard patterns.
	t.Run("full gcp_admin allow list round-trips", func(t *testing.T) {
		allow := models.RoleStatements{{Operations: patterns}}
		got, err := models.ValidatePermissionsPublic(perms, allow)
		require.NoError(t, err)

		var allOps []string
		for _, s := range got {
			allOps = append(allOps, s.Operations...)
		}
		sort.Strings(allOps)
		assert.NotEmpty(t, allOps)

		for _, p := range patterns {
			assert.Contains(t, allOps, p)
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
		got, err := models.ValidatePermissionsPublic(perms, stmt)
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
		_, err := models.ValidatePermissionsPublic(perms, stmt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was not found")
	})

	// Nonexistent service
	t.Run("nonexistent service rejected", func(t *testing.T) {
		stmt := models.RoleStatements{{Operations: []string{"fakeservice.things.read"}}}
		_, err := models.ValidatePermissionsPublic(perms, stmt)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "was not found")
	})
}
