package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/aws"
)

// awsProviderPerms loads the AWS permission list via the provider's
// shared data singleton and wraps it into the SearchResult slice
// expected by ValidatePermissionsPublic.
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

// TestAWSWildcardRoundTrip verifies that broad wildcards like "ec2:*"
// expand against the real IAM dataset and then condense back to the
// original wildcard pattern — never leaving hundreds of individual
// permissions behind.
func TestAWSWildcardRoundTrip(t *testing.T) {
	perms := awsProviderPerms(t)

	wildcards := []string{
		"ec2:*",
		"s3:*",
		"rds:*",
		"iam:*",
	}

	for _, wc := range wildcards {
		t.Run(wc, func(t *testing.T) {
			stmt := models.RoleStatements{{Operations: []string{wc}}}
			got, err := models.ValidatePermissionsPublic(perms, stmt)
			require.NoError(t, err, "%q should not error against real AWS data", wc)

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

	// Mixed wildcards + exact
	t.Run("mixed ec2:* with s3:GetObject", func(t *testing.T) {
		stmt := models.RoleStatements{{Operations: []string{"ec2:*", "s3:GetObject"}}}
		got, err := models.ValidatePermissionsPublic(perms, stmt)
		require.NoError(t, err)

		var resultOps []string
		for _, s := range got {
			resultOps = append(resultOps, s.Operations...)
		}

		assert.Contains(t, resultOps, "ec2:*")
		assert.Contains(t, resultOps, "s3:GetObject")
		assert.LessOrEqual(t, len(resultOps), 3,
			"expected at most 3 operations (ec2:* + s3:GetObject + maybe another), got %d", len(resultOps))
	})
}
