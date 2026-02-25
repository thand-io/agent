// External test package so we can import internal/providers/aws without
// creating a circular dependency (aws already imports models).
package models_test

import (
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/aws"
)

// awsProviderPerms loads the AWS permission list via the provider's shared
// data singleton (same path used at runtime) and wraps it into the
// SearchResult slice expected by ValidatePermissionsPublic.
func awsProviderPerms(t *testing.T) []models.SearchResult[models.ProviderPermission] {
	t.Helper()
	permissions, err := aws.GetPermissions()
	require.NoError(t, err, "aws.GetPermissions() failed")
	require.NotEmpty(t, permissions, "aws provider returned no permissions")

	results := make([]models.SearchResult[models.ProviderPermission], 0, len(permissions))
	for _, p := range permissions {
		results = append(results, models.SearchResult[models.ProviderPermission]{
			Result: p,
		})
	}
	return results
}

// TestAWSYamlPatternsAgainstRealDataset runs every wildcard pattern from
// config/roles/aws.yaml against the real AWS IAM dataset and asserts that
// each pattern expands to at least one real permission.
func TestAWSYamlPatternsAgainstRealDataset(t *testing.T) {
	perms := awsProviderPerms(t)

	patterns := []struct {
		pattern  string
		prefix   string   // expected service prefix on all results
		mustHave []string // spot-checks that must appear in the expansion
	}{
		{
			pattern: "ec2:*",
			prefix:  "ec2:",
			mustHave: []string{
				"ec2:DescribeInstances",
				"ec2:RunInstances",
				"ec2:AllocateAddress",
			},
		},
		{
			pattern: "s3:*",
			prefix:  "s3:",
			mustHave: []string{
				"s3:GetObject",
				"s3:PutObject",
				"s3:ListBuckets",
			},
		},
		{
			pattern: "rds:*",
			prefix:  "rds:",
			mustHave: []string{
				"rds:DescribeCertificates",
				"rds:DescribeDBClusterSnapshots",
				"rds:DescribeValidDBInstanceModifications",
			},
		},
	}

	for _, tt := range patterns {
		t.Run(tt.pattern, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{tt.pattern}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt)
			require.NoError(t, err, "pattern %q should not error against real AWS data", tt.pattern)

			var expanded []string
			for _, s := range got {
				expanded = append(expanded, s.Operations...)
			}
			sort.Strings(expanded)

			assert.NotEmpty(t, expanded, "pattern %q matched no real AWS permissions", tt.pattern)

			for _, perm := range expanded {
				assert.True(t,
					strings.HasPrefix(perm, tt.prefix),
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

	// Full round-trip: the combined aws_admin allow list must expand
	// without error against real data.
	t.Run("full aws_admin allow list validates without error", func(t *testing.T) {
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

		// Spot-checks across all three services
		checks := []string{
			"ec2:DescribeInstances",
			"ec2:RunInstances",
			"s3:GetObject",
			"s3:PutObject",
			"s3:ListBuckets",
			"rds:DescribeCertificates",
		}
		for _, want := range checks {
			assert.Contains(t, allOps, want)
		}
	})
}
