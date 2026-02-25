// External test package so we can import internal/providers/azure without
// creating a circular dependency (azure already imports models).
package models_test

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/azure"
)

// azureProviderPerms loads the Azure permission list via the provider's
// shared data singleton (same path used at runtime) and wraps it into
// the SearchResult slice expected by validatePermissions tests.
func azureProviderPerms(t *testing.T) []models.SearchResult[models.ProviderPermission] {
	t.Helper()
	permissions, err := azure.GetPermissions()
	require.NoError(t, err, "azure.GetPermissions() failed")
	require.NotEmpty(t, permissions, "azure provider returned no permissions")

	results := make([]models.SearchResult[models.ProviderPermission], 0, len(permissions))
	for _, p := range permissions {
		results = append(results, models.SearchResult[models.ProviderPermission]{
			Result: p,
		})
	}
	return results
}

// TestAzureYamlPatternsAgainstRealDataset runs every permission pattern from
// config/roles/azure.yaml against the real Azure IAM dataset and asserts that
// each wildcard:
//  1. Does not error (proving at least one real permission matched)
//  2. Gets condensed back to the original wildcard pattern (round-trip)
func TestAzureYamlPatternsAgainstRealDataset(t *testing.T) {
	perms := azureProviderPerms(t)

	patterns := []string{
		"Microsoft.Compute/*/read",
		"Microsoft.Compute/availabilitySets/*",
		"Microsoft.Compute/proximityPlacementGroups/*",
		"Microsoft.Compute/virtualMachines/*",
		"Microsoft.Compute/disks/*",
	}

	for _, pattern := range patterns {
		t.Run(pattern, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{pattern}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt, true)
			require.NoError(t, err, "pattern %q should not error against real Azure data", pattern)

			var resultOps []string
			for _, s := range got {
				resultOps = append(resultOps, s.Operations...)
			}

			// The wildcard should be condensed back to itself.
			assert.Contains(t, resultOps, pattern,
				"pattern %q should round-trip back to itself after validation", pattern,
			)
		})
	}

	// Full round-trip: the complete allow list must validate and condense
	// back to the original wildcard patterns.
	t.Run("full azure_admin allow list round-trips", func(t *testing.T) {
		allow := models.RoleStatements{{Operations: patterns}}
		got, err := models.ValidatePermissionsPublic(perms, allow, true)
		require.NoError(t, err)

		var allOps []string
		for _, s := range got {
			allOps = append(allOps, s.Operations...)
		}
		sort.Strings(allOps)
		assert.NotEmpty(t, allOps)

		// Every original wildcard pattern should be present in the output.
		for _, p := range patterns {
			assert.Contains(t, allOps, p)
		}
	})
}
