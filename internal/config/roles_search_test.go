package config

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/aws"
)

func TestRoleSearch(t *testing.T) {
	// Create a config with test roles
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"Admin": {
					Name:        "Admin",
					Description: "Full access to all resources and capabilities with AdministratorAccess",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"ec2:*"}},
							{Operations: []string{"s3:*"}},
							{Operations: []string{"rds:*"}},
							{Operations: []string{}, Targets: []string{"aws:*"}},
						},
					},
					Providers: []string{"aws-prod", "aws-dev"},
					Enabled:   true,
				},
				"User": {
					Name:        "User",
					Description: "Basic access with AmazonEC2ReadOnlyAccess and AmazonS3ReadOnlyAccess policies.",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"ec2:describeInstances"}},
							{Operations: []string{"s3:listBuckets"}},
						},
					},
					Providers: []string{"aws-thand-dev", "aws"},
					Enabled:   true,
				},
				"Developer": {
					Name:        "Developer",
					Description: "Developer role with lambda and dynamodb access",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"lambda:*"}},
							{Operations: []string{"dynamodb:GetItem", "dynamodb:PutItem"}},
						},
					},
					Providers: []string{"aws-dev"},
					Enabled:   true,
				},
				"GCP Admin": {
					Name:        "GCP Admin",
					Description: "Full access to GCP resources",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"compute.instances.list"}},
							{Operations: []string{"storage.buckets.get"}},
						},
					},
					Providers: []string{"gcp-prod"},
					Enabled:   true,
				},
			},
		},
	}

	// Create the search index
	err := cfg.ReloadRoleIndexes()
	require.NoError(t, err, "Failed to create role index")

	tests := []struct {
		name          string
		query         string
		expectedRoles []string // Role names that should be found
		minResults    int      // Minimum number of results expected
	}{
		{
			name:          "search for ec2 permission",
			query:         "ec2",
			expectedRoles: []string{"Admin", "User"},
			minResults:    2,
		},
		{
			name:          "search for specific ec2 operation",
			query:         "ec2:describeInstances",
			expectedRoles: []string{"User"},
			minResults:    1,
		},
		{
			name:          "search for IAM policy name in description",
			query:         "AmazonEC2ReadOnlyAccess",
			expectedRoles: []string{"User"},
			minResults:    1,
		},
		{
			name:          "search for another IAM policy name",
			query:         "AmazonS3ReadOnlyAccess",
			expectedRoles: []string{"User"},
			minResults:    1,
		},
		{
			name:          "search for AdministratorAccess",
			query:         "AdministratorAccess",
			expectedRoles: []string{"Admin"},
			minResults:    1,
		},
		{
			name:          "search for s3 permission",
			query:         "s3",
			expectedRoles: []string{"Admin", "User"},
			minResults:    2,
		},
		{
			name:          "search for lambda",
			query:         "lambda",
			expectedRoles: []string{"Developer"},
			minResults:    1,
		},
		{
			name:          "search for dynamodb",
			query:         "dynamodb",
			expectedRoles: []string{"Developer"},
			minResults:    1,
		},
		{
			name:          "search in description",
			query:         "developer",
			expectedRoles: []string{"Developer"},
			minResults:    1,
		},
		{
			name:          "search for GCP permissions",
			query:         "compute",
			expectedRoles: []string{"GCP Admin"},
			minResults:    1,
		},
		{
			name:          "search for role name",
			query:         "Admin",
			expectedRoles: []string{"Admin", "GCP Admin"},
			minResults:    2,
		},
		{
			name:       "empty query returns all",
			query:      "",
			minResults: 4, // All 4 roles
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			var searchRequest *models.SearchRequest

			if tt.query != "" {
				query := tt.query
				// For simple text queries without special characters, add wildcard for prefix matching
				// For queries with colons or wildcards, pass through as-is (handled by smart query logic)
				hasSpecialChars := strings.ContainsAny(query, ":/*")
				if !hasSpecialChars && !strings.HasSuffix(query, "*") {
					query = query + "*"
				}

				searchRequest = &models.SearchRequest{
					Query: query,
					Limit: 100,
				}
			}

			results, err := cfg.Roles.ListRoles(ctx, searchRequest)
			require.NoError(t, err, "Search should not error")

			assert.GreaterOrEqual(t, len(results), tt.minResults,
				"Should have at least %d results, got %d", tt.minResults, len(results))

			if len(tt.expectedRoles) > 0 {
				foundRoles := make(map[string]bool)
				for _, result := range results {
					foundRoles[result.Result.Name] = true
				}

				for _, expectedRole := range tt.expectedRoles {
					assert.True(t, foundRoles[expectedRole],
						"Expected to find role '%s' in search results for query '%s'", expectedRole, tt.query)
				}
			}

			// Log results for debugging
			t.Logf("Query '%s' returned %d results:", tt.query, len(results))
			for _, result := range results {
				t.Logf("  - %s (score: %.2f)", result.Result.Name, result.Score)
			}
		})
	}
}

func TestRoleSearchWithInheritance(t *testing.T) {
	// Test that inherited permissions are searchable
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"base": {
					Name: "base",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"s3:GetObject"}},
						},
					},
					Enabled: true,
				},
				"extended": {
					Name:     "extended",
					Inherits: []string{"base"},
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"ec2:DescribeInstances"}},
						},
					},
					Enabled: true,
				},
			},
		},
	}

	err := cfg.ReloadRoleIndexes()
	require.NoError(t, err, "Failed to create role index")

	ctx := context.Background()

	// Search for inherited permission
	searchRequest := &models.SearchRequest{
		Query: "s3:GetObject",
		Terms: []string{"s3:GetObject"},
		Limit: 100,
	}

	results, err := cfg.Roles.ListRoles(ctx, searchRequest)
	require.NoError(t, err, "Search should not error")

	// Both base and extended should be found because extended inherits from base
	foundRoles := make(map[string]bool)
	for _, result := range results {
		foundRoles[result.Result.Name] = true
	}

	assert.True(t, foundRoles["base"], "Should find base role with s3:GetObject")
	assert.True(t, foundRoles["extended"], "Should find extended role (inherited s3:GetObject)")

	t.Logf("Found %d roles for 's3:GetObject':", len(results))
	for _, result := range results {
		t.Logf("  - %s", result.Result.Name)
	}
}

func TestRoleSearchCaseSensitivity(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"TestRole": {
					Name:        "TestRole",
					Description: "Test role with EC2 permissions",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"ec2:DescribeInstances"}},
						},
					},
					Enabled: true,
				},
			},
		},
	}

	err := cfg.ReloadRoleIndexes()
	require.NoError(t, err, "Failed to create role index")

	ctx := context.Background()

	tests := []struct {
		name  string
		query string
	}{
		{"lowercase", "ec2"},
		{"uppercase", "EC2"},
		{"mixed case", "Ec2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			searchRequest := &models.SearchRequest{
				Query: tt.query,
				Terms: []string{tt.query},
				Limit: 100,
			}

			results, err := cfg.Roles.ListRoles(ctx, searchRequest)
			require.NoError(t, err, "Search should not error")

			assert.Greater(t, len(results), 0, "Should find results for query '%s'", tt.query)
		})
	}
}

func TestRoleSearchWithProviderFilter(t *testing.T) {

	// Create minimal config for initialization
	testConfig := models.ProviderConfig{
		Name:        "test-aws",
		Description: "Test AWS provider",
		Provider:    "aws",
		Config: &models.BasicConfig{
			"region":            "us-east-1",
			"account_id":        "000000000000",
			"access_key_id":     "test",
			"secret_access_key": "test",
		},
		Enabled: true,
	}

	// Initialize the provider
	mockProvider := aws.NewMockAwsProvider()
	err := mockProvider.Initialize("aws", testConfig)

	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"aws_admin": {
					Name:        "AWS Admin",
					Description: "AWS admin role",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"ec2:*"}},
						},
					},
					Providers: []string{"aws-prod"},
					Enabled:   true,
				},
				"gcp_admin": {
					Name:        "GCP Admin",
					Description: "GCP admin role",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"compute.instances.list"}},
						},
					},
					Providers: []string{"gcp-prod"},
					Enabled:   true,
				},
			},
		},
	}
	cfg.AddProvider("mock", mockProvider)
	err = cfg.ReloadRoleIndexes()
	require.NoError(t, err, "Failed to create role index")

	ctx := context.Background()

	searchRequest := &models.SearchRequest{
		Terms: []string{"admin"},
		Limit: 100,
	}

	results, err := cfg.Roles.ListRoles(ctx, searchRequest)
	require.NoError(t, err, "Search should not error")

	// Both roles should be returned by search
	assert.GreaterOrEqual(t, len(results), 2, "Should find both admin roles, got %d", len(results))

	// Verify provider filtering would work in daemon layer
	for _, result := range results {
		assert.NotEmpty(t, result.Result.Providers, "Each role should have providers defined")
	}
}

func TestRoleSearchLimit(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{},
		},
	}

	// Create 20 test roles
	for i := 0; i < 20; i++ {
		roleName := fmt.Sprintf("role-%d", i)
		cfg.Roles.Definitions[roleName] = models.Role{
			Name:        roleName,
			Description: "Test role with ec2 permissions",
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{Operations: []string{"ec2:DescribeInstances"}}},
			},
			Enabled: true,
		}
	}

	err := cfg.ReloadRoleIndexes()
	require.NoError(t, err, "Failed to create role index")

	ctx := context.Background()
	searchRequest := &models.SearchRequest{
		Query: "ec2",
		Terms: []string{"ec2"},
		Limit: 5, // Limit to 5 results
	}

	results, err := cfg.Roles.ListRoles(ctx, searchRequest)
	require.NoError(t, err, "Search should not error")

	assert.LessOrEqual(t, len(results), 5, "Should respect limit of 5 results")
	t.Logf("Limited search returned %d results (limit was 5)", len(results))
}

func TestRoleSearchNoIndex(t *testing.T) {
	// Test fallback behavior when index is not ready
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"test-role": {
					Name:        "Test Role",
					Description: "Role with ec2 permissions",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{{Operations: []string{"ec2:*"}}},
					},
					Enabled: true,
				},
			},
		},
	}

	// Don't create index - test fallback behavior
	ctx := context.Background()
	searchRequest := &models.SearchRequest{
		Query: "ec2",
		Terms: []string{"ec2"},
		Limit: 100,
	}

	results, err := cfg.Roles.ListRoles(ctx, searchRequest)
	require.NoError(t, err, "Search should not error even without index")

	// Fallback should still find results by name/description matching
	assert.Greater(t, len(results), 0, "Should find results using fallback search")
}

func TestRoleSearchWithInheritsARN(t *testing.T) {
	// Test searching for IAM policies referenced in inherits
	// This tests that we can search for ARN strings in the inherits field
	roles := map[string]models.Role{
		"aws_admin": {
			Name:        "Admin",
			Description: "Full access to all resources and capabilities.",
			Authenticators: []string{
				"google_oauth2",
				"thand_oauth2",
			},
			Workflows: []string{
				"slack_approval",
			},
			Inherits: []string{
				"aws_user",
				"arn:aws:iam::aws:policy/AdministratorAccess",
			},
			Providers: []string{"aws-prod"},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"ec2:*"}},
				},
			},
			Enabled: true,
		},
		"aws_user": {
			Name:        "User",
			Description: "Basic user access",
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"ec2:DescribeInstances"}},
				},
			},
			Enabled: true,
		},
		"developer": {
			Name:        "Developer",
			Description: "Development role without admin access",
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"lambda:*"}},
				},
			},
			Enabled: true,
		},
	}

	// Initialize AWS mock provider to load AWS managed policies
	testConfig := models.ProviderConfig{
		Name:        "aws-prod",
		Description: "AWS Production",
		Provider:    "aws",
		Config: &models.BasicConfig{
			"region":            "us-east-1",
			"account_id":        "000000000000",
			"access_key_id":     "test",
			"secret_access_key": "test",
		},
		Enabled: true,
	}

	mockProvider := aws.NewMockAwsProvider()
	err := mockProvider.Initialize("aws-prod", testConfig)
	require.NoError(t, err, "Failed to initialize AWS mock provider")

	cfg := &Config{
		mode: "server",
		Roles: RoleConfig{
			Definitions: roles,
		},
	}
	cfg.providerInstances = make(map[string]models.Provider)
	cfg.providerInstances["aws-prod"] = mockProvider

	// Use ReloadRoleIndexes to properly index all roles
	err = cfg.ReloadRoleIndexes()
	require.NoError(t, err, "Failed to reload role indexes")

	// Debug: Check what got indexed
	adminRole, err := cfg.GetRoleByName("aws_admin")
	require.NoError(t, err)
	t.Logf("Original aws_admin inherits: %v", adminRole.Inherits)

	compositeAdmin, err := cfg.GetCompositeRoleByName(nil, "aws_admin")
	require.NoError(t, err)
	t.Logf("Composite Admin inherits: %v", compositeAdmin.Inherits)

	ctx := context.Background()

	// Search for AdministratorAccess - should find the aws_admin role
	searchRequest := &models.SearchRequest{
		Query: "AdministratorAccess",
		Terms: []string{"AdministratorAccess"},
		Limit: 100,
	}

	results, err := cfg.Roles.ListRoles(ctx, searchRequest)
	require.NoError(t, err, "Search should not error")

	// Should find the aws_admin role which has AdministratorAccess in inherits
	foundRoles := make(map[string]bool)
	for _, result := range results {
		foundRoles[result.Result.Name] = true
		t.Logf("  - %s (score: %.2f, inherits: %v)", result.Result.Name, result.Score, result.Result.Inherits)
	}

	// We should find the Admin role because it has the ARN in its inherits field
	assert.True(t, foundRoles["Admin"], "Should find Admin role with AdministratorAccess in inherits")
	assert.False(t, foundRoles["Developer"], "Should not find Developer role without AdministratorAccess")

	t.Logf("Query 'AdministratorAccess' returned %d results:", len(results))
}
