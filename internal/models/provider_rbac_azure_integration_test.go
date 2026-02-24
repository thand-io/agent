// External test package so we can import internal/providers/azure without
// creating a circular dependency (azure already imports models).
package models_test

import (
	"sort"
	"strings"
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
// config/roles/azure.yaml against the real Azure IAM dataset (loaded via the
// same provider shared-data singleton used at runtime) and asserts that each
// wildcard expands to at least one real permission.
func TestAzureYamlPatternsAgainstRealDataset(t *testing.T) {
	perms := azureProviderPerms(t)

	patterns := []struct {
		pattern  string
		mustHave []string // spot-checks that must appear in the expansion
	}{
		{
			pattern:  "Microsoft.Compute/*/read",
			mustHave: []string{"Microsoft.Compute/availabilitySets/read", "Microsoft.Compute/virtualMachines/read", "Microsoft.Compute/disks/read"},
		},
		{
			pattern:  "Microsoft.Compute/availabilitySets/*",
			mustHave: []string{"Microsoft.Compute/availabilitySets/read", "Microsoft.Compute/availabilitySets/write", "Microsoft.Compute/availabilitySets/delete"},
		},
		{
			pattern:  "Microsoft.Compute/proximityPlacementGroups/*",
			mustHave: []string{"Microsoft.Compute/proximityPlacementGroups/read"},
		},
		{
			pattern:  "Microsoft.Compute/virtualMachines/*",
			mustHave: []string{"Microsoft.Compute/virtualMachines/read", "Microsoft.Compute/virtualMachines/write", "Microsoft.Compute/virtualMachines/delete"},
		},
		{
			pattern:  "Microsoft.Compute/disks/*",
			mustHave: []string{"Microsoft.Compute/disks/read", "Microsoft.Compute/disks/write"},
		},
	}

	for _, tt := range patterns {
		t.Run(tt.pattern, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{tt.pattern}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt)
			require.NoError(t, err, "pattern %q should not error against real Azure data", tt.pattern)

			var expanded []string
			for _, s := range got {
				expanded = append(expanded, s.Operations...)
			}
			sort.Strings(expanded)

			assert.NotEmpty(t, expanded, "pattern %q matched no real Azure permissions", tt.pattern)

			for _, perm := range expanded {
				assert.True(t,
					strings.HasPrefix(perm, "Microsoft.Compute/"),
					"unexpected permission %q returned for pattern %q", perm, tt.pattern,
				)
			}

			for _, want := range tt.mustHave {
				assert.Contains(t, expanded, want,
					"pattern %q should have expanded to include %q", tt.pattern, want,
				)
			}
		})
	}

	// Full round-trip: the complete azure_admin allow list must expand
	// without error against real data.
	t.Run("full azure_admin allow list validates without error", func(t *testing.T) {
		allPatterns := make([]string, 0, len(patterns))
		for _, p := range patterns {
			allPatterns = append(allPatterns, p.pattern)
		}

		allow := models.RoleStatements{{Operations: allPatterns}}
		got, err := models.ValidatePermissionsPublic(perms, allow)
		require.NoError(t, err)

		var allOps []string
		for _, s := range got {
			allOps = append(allOps, s.Operations...)
		}
		sort.Strings(allOps)
		assert.NotEmpty(t, allOps)

		// Spot-checks across all patterns
		checks := []string{
			"Microsoft.Compute/availabilitySets/read",
			"Microsoft.Compute/availabilitySets/write",
			"Microsoft.Compute/availabilitySets/delete",
			"Microsoft.Compute/virtualMachines/read",
			"Microsoft.Compute/virtualMachines/write",
			"Microsoft.Compute/virtualMachines/delete",
			"Microsoft.Compute/disks/read",
			"Microsoft.Compute/disks/write",
			"Microsoft.Compute/proximityPlacementGroups/read",
		}
		for _, want := range checks {
			assert.Contains(t, allOps, want)
		}
	})
}
