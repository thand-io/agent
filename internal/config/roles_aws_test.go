package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestAWSRoles tests AWS-specific role configurations based on config/roles/aws.yaml
func TestAWSRoles(t *testing.T) {
	// AWS role definitions based on config/roles/aws.yaml
	awsRoles := map[string]models.Role{
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
			// Removed IAM policy inheritance for test simplicity
			Permissions: models.Permissions{
				Allow: []string{
					"ec2:*",
					"s3:*",
					"rds:*",
					"*", // Administrative access
				},
			},
			Resources: models.Resources{
				Allow: []string{
					"aws:*",
				},
			},
			Scopes: &models.RoleScopes{
				Groups: []string{
					"oidc:user",
					"oidc:eng",
				},
				Users: []string{
					"admin@example.com",
					"devops@example.com",
				},
			},
			Providers: []string{
				"aws-prod",
				"aws-dev",
				"aws-thand-dev",
			},
			Enabled: true,
		},
		"aws_user": {
			Name:        "User",
			Description: "Basic access to user resources.",
			Workflows:   []string{"slack_approval"},
			// Removed IAM policy inheritance for test simplicity
			Permissions: models.Permissions{
				Allow: []string{
					"ec2:describeInstances",
					"s3:listBuckets",
					"ec2:Describe*", // Read-only EC2 access
					"s3:Get*",       // Read-only S3 access
					"s3:List*",      // List S3 access
				},
			},
			Providers: []string{
				"aws-thand-dev",
				"aws",
			},
			Enabled: true,
		},
	}

	// AWS providers
	awsProviders := map[string]models.ProviderConfig{
		"aws-prod": {
			Name:        "aws-prod",
			Description: "AWS Production Environment",
			Provider:    "aws",
		},
		"aws-dev": {
			Name:        "aws-dev",
			Description: "AWS Development Environment",
			Provider:    "aws",
		},
		"aws-thand-dev": {
			Name:        "aws-thand-dev",
			Description: "AWS Thand Development Environment",
			Provider:    "aws",
		},
	}

	t.Run("aws_admin role composition", func(t *testing.T) {
		config := newTestConfig(t, awsRoles, awsProviders)

		// Test with a user in the allowed group
		identity := &models.Identity{
			ID: "eng-user",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"oidc:eng", "developers"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "aws_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify basic properties
		assert.Equal(t, "Admin", result.Name)
		assert.Equal(t, "Full access to all resources and capabilities.", result.Description)
		assert.True(t, result.Enabled)

		// Verify permissions
		assert.ElementsMatch(t, []string{"ec2:*", "s3:*", "rds:*", "*"}, result.Permissions.Allow)

		// Verify resources - aws:* becomes * since aws matches allowed providers
		assert.ElementsMatch(t, []string{"*"}, result.Resources.Allow)

		// Verify providers
		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev", "aws-thand-dev"}, result.Providers)

		// Verify workflows
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)
	})

	t.Run("aws_user role composition", func(t *testing.T) {
		config := newTestConfig(t, awsRoles, awsProviders)

		identity := &models.Identity{
			ID: "basic-user",
			User: &models.User{
				Username: "basicuser",
				Email:    "user@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "aws_user")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify basic properties
		assert.Equal(t, "User", result.Name)
		assert.Equal(t, "Basic access to user resources.", result.Description)
		assert.True(t, result.Enabled)

		// Verify permissions (condensed format)
		assert.ElementsMatch(t, []string{
			"ec2:Describe*,describeInstances",
			"s3:Get*,List*,listBuckets",
		}, result.Permissions.Allow)

		// Verify providers
		assert.ElementsMatch(t, []string{"aws-thand-dev", "aws"}, result.Providers)

		// Verify workflows
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)
	})

	t.Run("aws role inheritance with IAM policies", func(t *testing.T) {
		// Test that AWS roles can inherit from IAM policy ARNs
		// This tests the inheritance mechanism for AWS-specific patterns
		roles := map[string]models.Role{
			"base_admin": {
				Name:        "Base Admin",
				Description: "Base admin with IAM policy inheritance",
				Inherits: []string{
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.Permissions{
					Allow: []string{"custom:action"},
				},
				Enabled: true,
			},
			"arn:aws:iam::aws:policy/AdministratorAccess": {
				Name:        "AdministratorAccess",
				Description: "AWS managed admin policy",
				Permissions: models.Permissions{
					Allow: []string{"*"},
				},
				Enabled: true,
			},
		}

		config := newTestConfig(t, roles, nil)

		identity := &models.Identity{
			ID: "admin-user",
			User: &models.User{
				Username: "admin",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "base_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should merge permissions from both roles
		assert.ElementsMatch(t, []string{"custom:action", "*"}, result.Permissions.Allow)
	})
}

// TestAWSRoleScenarios tests realistic AWS role usage scenarios
func TestAWSRoleScenarios(t *testing.T) {
	t.Run("developer accessing staging environment", func(t *testing.T) {
		roles := map[string]models.Role{
			"developer": {
				Name:        "Developer",
				Description: "Developer access to staging",
				Permissions: models.Permissions{
					Allow: []string{
						"ec2:DescribeInstances",
						"s3:GetObject",
						"s3:PutObject",
						"logs:DescribeLogGroups",
						"logs:DescribeLogStreams",
					},
				},
				Resources: models.Resources{
					Allow: []string{
						"arn:aws:s3:::staging-*",
						"arn:aws:ec2:*:*:instance/i-staging*",
					},
				},
				Scopes: &models.RoleScopes{
					Groups: []string{"developers"},
				},
				Providers: []string{"aws-staging"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-staging": {
				Name:        "aws-staging",
				Description: "AWS Staging Environment",
				Provider:    "aws",
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

		result, err := config.GetCompositeRoleByName(identity, "developer")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.Equal(t, "Developer", result.Name)
		assert.ElementsMatch(t, []string{
			"ec2:DescribeInstances",
			"logs:DescribeLogGroups,DescribeLogStreams",
			"s3:GetObject,PutObject",
		}, result.Permissions.Allow)

		assert.ElementsMatch(t, []string{
			"arn:aws:s3:::staging-*",
			"arn:aws:ec2:*:*:instance/i-staging*",
		}, result.Resources.Allow)

		assert.ElementsMatch(t, []string{"aws-staging"}, result.Providers)
	})

	t.Run("production admin with multiple inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_user": {
				Name:        "Base User",
				Description: "Basic user permissions",
				Permissions: models.Permissions{
					Allow: []string{
						"iam:GetUser",
						"iam:ListMFADevices",
					},
				},
				Enabled: true,
			},
			"s3_admin": {
				Name:        "S3 Admin",
				Description: "S3 administrative access",
				Permissions: models.Permissions{
					Allow: []string{
						"s3:*",
					},
				},
				Resources: models.Resources{
					Allow: []string{
						"arn:aws:s3:::prod-*",
					},
				},
				Enabled: true,
			},
			"prod_admin": {
				Name:        "Production Admin",
				Description: "Full production access",
				Inherits: []string{
					"base_user",
					"s3_admin",
				},
				Permissions: models.Permissions{
					Allow: []string{
						"ec2:*",
						"rds:*",
					},
					Deny: []string{
						"iam:DeleteRole",
						"iam:DeleteUser",
					},
				},
				Scopes: &models.RoleScopes{
					Users: []string{
						"admin@example.com",
						"sre@example.com",
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "aws-prod",
				Description: "AWS Production Environment",
				Provider:    "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "prod_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Should have merged permissions from all inherited roles (condensed format)
		expectedAllowPerms := []string{
			"ec2:*", "rds:*", // from prod_admin
			"iam:GetUser,ListMFADevices", // from base_user (condensed)
			"s3:*",                       // from s3_admin
		}
		assert.ElementsMatch(t, expectedAllowPerms, result.Permissions.Allow)

		// Should have deny permissions (condensed format)
		assert.ElementsMatch(t, []string{
			"iam:DeleteRole,DeleteUser",
		}, result.Permissions.Deny)

		// Should have merged resources
		assert.ElementsMatch(t, []string{
			"arn:aws:s3:::prod-*",
		}, result.Resources.Allow)

		assert.ElementsMatch(t, []string{"aws-prod"}, result.Providers)
	})

	t.Run("aws_admin inherits from aws_user", func(t *testing.T) {
		// AWS providers for this test
		testProviders := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "aws-prod",
				Description: "AWS Production Environment",
				Provider:    "aws",
			},
			"aws-dev": {
				Name:        "aws-dev",
				Description: "AWS Development Environment",
				Provider:    "aws",
			},
			"aws-thand-dev": {
				Name:        "aws-thand-dev",
				Description: "AWS Thand Development Environment",
				Provider:    "aws",
			},
		}

		// Create roles that demonstrate inheritance behavior
		awsRolesWithInheritance := map[string]models.Role{
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
					"aws_user", // This should be resolved and removed from final Inherits
					"arn:aws:iam::aws:policy/AdministratorAccess", // Provider role - should remain
				},
				Permissions: models.Permissions{
					Allow: []string{
						"ec2:*",
						"s3:*",
						"rds:*",
					},
				},
				Resources: models.Resources{
					Allow: []string{
						"aws:*",
					},
				},
				Scopes: &models.RoleScopes{
					Groups: []string{
						"oidc:user",
						"oidc:eng",
					},
					Users: []string{
						"admin@example.com",
					},
				},
				Providers: []string{
					"aws-prod",
					"aws-dev",
					"aws-thand-dev",
				},
				Enabled: true,
			},
			"aws_user": {
				Name:        "User",
				Description: "Basic access to user resources.",
				Workflows:   []string{"slack_approval"},
				Permissions: models.Permissions{
					Allow: []string{
						"ec2:describeInstances",
						"s3:listBuckets",
					},
				},
				Providers: []string{
					"aws-thand-dev",
					"aws",
				},
				Enabled: true,
			},
		}

		// Create config with mock providers
		config := newTestConfig(t, awsRolesWithInheritance, testProviders)

		// Test with a user in the allowed group
		identity := &models.Identity{
			ID: "eng-user",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"oidc:eng", "developers"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "aws_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Verify that aws_user inheritance was resolved and removed from Inherits
		// but provider roles (ARN policies) are preserved
		expectedInherits := []string{
			"AdministratorAccess",
		}
		assert.ElementsMatch(t, expectedInherits, result.Inherits,
			"Provider roles should remain in Inherits, but regular inherited roles (aws_user) should be removed")

		// Verify that permissions from aws_user were merged into aws_admin
		expectedPermissions := []string{
			"ec2:*", // from aws_admin (overrides aws_user's ec2:describeInstances)
			"s3:*",  // from aws_admin (overrides aws_user's s3:listBuckets)
			"rds:*", // from aws_admin
		}
		assert.ElementsMatch(t, expectedPermissions, result.Permissions.Allow,
			"Permissions should be merged from inherited roles")

		// Verify other properties are preserved
		assert.Equal(t, "Admin", result.Name)
		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev", "aws-thand-dev"}, result.Providers)
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)
	})
}
