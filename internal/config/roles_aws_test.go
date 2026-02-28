package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// stmts converts a []string to models.RoleStatements for test convenience
func stmtsAws(ops ...string) models.RoleStatements {
	if len(ops) == 0 {
		return nil
	}
	return models.RoleStatements{{Operations: ops}}
}

// collectOps collects all operations from statements into a single slice
func collectOps(stmts models.RoleStatements) []string {
	var result []string
	for _, stmt := range stmts {
		result = append(result, stmt.Operations...)
	}
	return result
}

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
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{{
					Operations: []string{
						"ec2:*",
						"s3:*",
						"rds:*",
						"*", // Administrative access
					},
					Targets: []string{
						"aws:*",
					},
				}},
			},
			Scopes: models.RoleScopes{
				Allow: models.ScopeIdentities{
					Groups: []string{
						"oidc:user",
						"oidc:eng",
					},
					Users: []string{
						"admin@example.com",
						"devops@example.com",
					},
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
			Permissions: models.RolePermissions{
				Allow: stmts(
					"ec2:describeInstances",
					"s3:listBuckets",
					"ec2:Describe*", // Read-only EC2 access
					"s3:Get*",       // Read-only S3 access
					"s3:List*",      // List S3 access
				),
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

		// Verify permissions - now includes targets
		assert.Len(t, result.Permissions.Allow, 1)
		assert.ElementsMatch(t, []string{"ec2:*", "s3:*", "rds:*", "*"}, result.Permissions.Allow[0].Operations)
		// Targets: aws:* becomes * since aws matches allowed providers
		assert.ElementsMatch(t, []string{"*"}, result.Permissions.Allow[0].Targets)

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
		}, collectOps(result.Permissions.Allow))

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
				Permissions: models.RolePermissions{
					Allow: stmtsAws("custom:action"),
				},
				Enabled: true,
			},
			"arn:aws:iam::aws:policy/AdministratorAccess": {
				Name:        "AdministratorAccess",
				Description: "AWS managed admin policy",
				Permissions: models.RolePermissions{
					Allow: stmtsAws("*"),
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
		assert.ElementsMatch(t, []string{"custom:action", "*"}, collectOps(result.Permissions.Allow))
	})
}

// TestAWSRoleScenarios tests realistic AWS role usage scenarios
func TestAWSRoleScenarios(t *testing.T) {
	t.Run("developer accessing staging environment", func(t *testing.T) {
		roles := map[string]models.Role{
			"developer": {
				Name:        "Developer",
				Description: "Developer access to staging",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"ec2:DescribeInstances",
							"s3:GetObject",
							"s3:PutObject",
							"logs:DescribeLogGroups",
							"logs:DescribeLogStreams",
						},
						Targets: []string{
							"arn:aws:s3:::staging-*",
							"arn:aws:ec2:*:*:instance/i-staging*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
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
		assert.Len(t, result.Permissions.Allow, 1)
		assert.ElementsMatch(t, []string{
			"ec2:DescribeInstances",
			"logs:DescribeLogGroups,DescribeLogStreams",
			"s3:GetObject,PutObject",
		}, result.Permissions.Allow[0].Operations)

		assert.ElementsMatch(t, []string{
			"arn:aws:s3:::staging-*",
			"arn:aws:ec2:*:*:instance/i-staging*",
		}, result.Permissions.Allow[0].Targets)

		assert.ElementsMatch(t, []string{"aws-staging"}, result.Providers)
	})

	t.Run("production admin with multiple inheritance", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_user": {
				Name:        "Base User",
				Description: "Basic user permissions",
				Permissions: models.RolePermissions{
					Allow: stmts(
						"iam:GetUser",
						"iam:ListMFADevices",
					),
				},
				Enabled: true,
			},
			"s3_admin": {
				Name:        "S3 Admin",
				Description: "S3 administrative access",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:*"},
						Targets: []string{
							"arn:aws:s3:::prod-*",
						},
					}},
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
				Permissions: models.RolePermissions{
					Allow: stmts(
						"ec2:*",
						"rds:*",
					),
					Deny: stmts(
						"iam:DeleteRole",
						"iam:DeleteUser",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{
							"admin@example.com",
							"sre@example.com",
						},
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
		assert.ElementsMatch(t, expectedAllowPerms, collectOps(result.Permissions.Allow))

		// Should have deny permissions (condensed format)
		assert.ElementsMatch(t, []string{
			"iam:DeleteRole,DeleteUser",
		}, collectOps(result.Permissions.Deny))

		// The s3:* permission from s3_admin should have targets merged
		// Find the s3 statement and check its targets
		var s3Targets []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if op == "s3:*" {
					s3Targets = stmt.Targets
					break
				}
			}
		}
		assert.ElementsMatch(t, []string{
			"arn:aws:s3:::prod-*",
		}, s3Targets)

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
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"ec2:*",
							"s3:*",
							"rds:*",
						},
						Targets: []string{
							"aws:*",
						},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{
							"oidc:user",
							"oidc:eng",
						},
						Users: []string{
							"admin@example.com",
						},
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
				Permissions: models.RolePermissions{
					Allow: stmts(
						"ec2:describeInstances",
						"s3:listBuckets",
					),
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
		assert.ElementsMatch(t, expectedPermissions, collectOps(result.Permissions.Allow),
			"Permissions should be merged from inherited roles")

		// Verify other properties are preserved
		assert.Equal(t, "Admin", result.Name)
		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev", "aws-thand-dev"}, result.Providers)
		assert.ElementsMatch(t, []string{"slack_approval"}, result.Workflows)
	})
}

// TestAWSComplexInheritance exercises complex, deeply-nested AWS role
// inheritance with real managed-policy ARN resolution (via the embedded IAM
// dataset), scope allow/deny filtering, composite-role marking, wildcard
// subsumption, deny-permission survival, and target preservation across 1-3
// levels of thand-role nesting.
func TestAWSComplexInheritance(t *testing.T) {
	// Shared AWS providers used by most subtests.
	awsProviders := map[string]models.ProviderConfig{
		"aws-prod": {
			Name:        "aws-prod",
			Description: "AWS Production",
			Provider:    "aws",
		},
		"aws-dev": {
			Name:        "aws-dev",
			Description: "AWS Development",
			Provider:    "aws",
		},
	}

	// ---------------------------------------------------------------
	// 1. Provider-only inheritance is NOT composite (depth 1)
	// ---------------------------------------------------------------
	t.Run("provider-only inheritance is not composite", func(t *testing.T) {
		roles := map[string]models.Role{
			"readonly_viewer": {
				Name:        "ReadOnly Viewer",
				Description: "Inherits only AWS managed policies, no thand roles",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("cloudwatch:GetMetricData", "cloudwatch:ListMetrics"),
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, awsProviders)

		identity := &models.Identity{
			ID: "viewer1",
			User: &models.User{
				Username: "viewer",
				Email:    "viewer@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "readonly_viewer")
		require.NoError(t, err)
		require.NotNil(t, result)

		// No thand-role inheritance → Composite must be false
		assert.False(t, result.Composite,
			"Role inheriting only from provider policies should NOT be composite")

		// Provider role ARNs should be resolved to their short names and kept in Inherits
		assert.ElementsMatch(t, []string{"ReadOnlyAccess", "SecurityAudit"}, result.Inherits,
			"Provider ARN inherits should be resolved to short names")

		// Own permissions should be preserved
		assert.ElementsMatch(t,
			[]string{"cloudwatch:GetMetricData,ListMetrics"},
			collectOps(result.Permissions.Allow),
			"Own permissions should be present")

		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 2. Thand role + multiple AWS managed policies (depth 1)
	// ---------------------------------------------------------------
	t.Run("thand role plus multiple AWS managed policies", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_ops": {
				Name:        "Base Ops",
				Description: "Baseline operational permissions",
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"iam:GetUser",
						"iam:ListMFADevices",
						"sts:GetCallerIdentity",
					),
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
			"senior_ops": {
				Name:        "Senior Ops",
				Description: "Senior operator with thand + managed-policy inheritance",
				Inherits: []string{
					"base_ops",                                // thand role → merged
					"arn:aws:iam::aws:policy/ReadOnlyAccess",  // provider → kept
					"arn:aws:iam::aws:policy/PowerUserAccess", // provider → kept
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "rds:*"),
					Deny:  stmtsAws("iam:CreateUser", "iam:DeleteUser"),
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, awsProviders)

		identity := &models.Identity{
			ID: "ops-senior",
			User: &models.User{
				Username: "senior",
				Email:    "senior@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "senior_ops")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Inherited from thand role base_ops → composite
		assert.True(t, result.Composite,
			"Role inheriting from a thand role should be composite")

		// ── Validate ALL provider-role ARNs resolved to short names ──
		// Both ReadOnlyAccess and PowerUserAccess must be present in Inherits;
		// they are the only entries (base_ops was a thand role, so it's merged
		// and removed from Inherits).
		assert.Len(t, result.Inherits, 2,
			"Exactly 2 AWS managed policies should be in Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "ReadOnlyAccess",
			"ReadOnlyAccess must be in the final composite Inherits: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "PowerUserAccess",
			"PowerUserAccess must be in the final composite Inherits: got %v", result.Inherits)

		// ── Validate merged allow permissions exhaustively ──
		allowOps := collectOps(result.Permissions.Allow)

		// senior_ops own permissions must be present
		assert.Contains(t, allowOps, "ec2:*",
			"senior_ops own ec2:* should be present: got %v", allowOps)
		assert.Contains(t, allowOps, "rds:*",
			"senior_ops own rds:* should be present: got %v", allowOps)

		// base_ops permissions must be merged in (condensed format)
		foundIam := false
		foundSts := false
		for _, op := range allowOps {
			if op == "iam:GetUser,ListMFADevices" || op == "iam:ListMFADevices,GetUser" {
				foundIam = true
			}
			if op == "sts:GetCallerIdentity" {
				foundSts = true
			}
		}
		assert.True(t, foundIam,
			"base_ops IAM perms (GetUser, ListMFADevices) should be merged and condensed: got %v", allowOps)
		assert.True(t, foundSts,
			"base_ops STS perm (GetCallerIdentity) should be merged: got %v", allowOps)

		// Verify no unexpected service prefixes leaked in
		for _, op := range allowOps {
			hasKnownPrefix := false
			for _, pfx := range []string{"ec2:", "rds:", "iam:", "sts:"} {
				if len(op) >= len(pfx) && op[:len(pfx)] == pfx {
					hasKnownPrefix = true
					break
				}
			}
			assert.True(t, hasKnownPrefix,
				"Unexpected permission prefix in allow ops: %q (full list: %v)", op, allowOps)
		}

		// ── Validate deny permissions survive the merge ──
		denyOps := collectOps(result.Permissions.Deny)
		assert.Contains(t, denyOps, "iam:CreateUser,DeleteUser",
			"Deny permissions should survive merge: got %v", denyOps)

		// ── Validate providers are preserved ──
		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev"}, result.Providers,
			"Providers should be preserved from senior_ops")

		// ── Validate basic identity/metadata ──
		assert.Equal(t, "Senior Ops", result.Name)
		assert.Equal(t, "Senior operator with thand + managed-policy inheritance", result.Description)
		assert.True(t, result.Enabled)
	})

	// ---------------------------------------------------------------
	// 3. Two-level deep with group and domain scopes (depth 2)
	// ---------------------------------------------------------------
	t.Run("two-level deep with group and domain scopes", func(t *testing.T) {
		roles := map[string]models.Role{
			"level0_reader": {
				Name:        "Level-0 Reader",
				Description: "Base reader scoped to example.com domain",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:ListBuckets", "s3:GetBucketLocation"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
			"level1_developer": {
				Name:        "Level-1 Developer",
				Description: "Developer inheriting reader + IAM read, scoped to devs group",
				Inherits: []string{
					"level0_reader", // thand (depth 2)
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:RunInstances", "s3:PutObject"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"developers"},
					},
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, awsProviders)

		// Identity that satisfies BOTH scopes: domain example.com + group developers
		identity := &models.Identity{
			ID: "dev-user",
			User: &models.User{
				Username: "developer1",
				Email:    "dev@example.com",
				Groups:   []string{"developers", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "level1_developer")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Inherited from thand role level0_reader → composite
		assert.True(t, result.Composite,
			"Two-level thand inheritance should be composite")

		// Provider ARNs from BOTH levels should be in Inherits
		assert.ElementsMatch(t, []string{"ReadOnlyAccess", "IAMReadOnlyAccess"}, result.Inherits,
			"Provider roles from both inheritance levels should accumulate in Inherits")

		// Merged allow permissions from both levels
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "ec2:RunInstances",
			"level1 own perms should be present: got %v", allowOps)

		// level0 s3 perms + level1 s3 perms should condense
		foundS3 := false
		for _, op := range allowOps {
			// All s3 operations should be condensed into one entry
			if len(op) > 3 && op[:3] == "s3:" {
				foundS3 = true
			}
		}
		assert.True(t, foundS3, "S3 permissions from both levels should be present: got %v", allowOps)

		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 4. Two-level scope denial skips middle role (depth 2)
	// ---------------------------------------------------------------
	t.Run("two-level scope denial skips middle role", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_perms": {
				Name:        "Base Perms",
				Description: "Open base permissions",
				Permissions: models.RolePermissions{
					Allow: stmtsAws("logs:DescribeLogGroups", "logs:GetLogEvents"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"restricted_layer": {
				Name:        "Restricted Layer",
				Description: "High-value perms, deny-scoped to outsiders",
				Inherits:    []string{"base_perms"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "s3:*", "rds:*"),
				},
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Users: []string{"outsider@example.com"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"top_role": {
				Name:        "Top Role",
				Description: "Top-level role inheriting the restricted layer",
				Inherits:    []string{"restricted_layer"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("cloudwatch:GetMetricData"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "aws-prod",
				Description: "AWS Production",
				Provider:    "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		// Identity that is DENIED by restricted_layer's scope
		identity := &models.Identity{
			ID: "outsider1",
			User: &models.User{
				Username: "outsider",
				Email:    "outsider@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "top_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		// restricted_layer was skipped (scope deny), so its perms and
		// base_perms perms (transitive) should NOT be present
		allowOps := collectOps(result.Permissions.Allow)
		assert.ElementsMatch(t, []string{"cloudwatch:GetMetricData"}, allowOps,
			"Only top_role's own perms should remain when middle role is scope-denied: got %v", allowOps)

		// No thand role was successfully merged → not composite
		assert.False(t, result.Composite,
			"Role should NOT be composite when inherited role is scope-denied")
	})

	// ---------------------------------------------------------------
	// 5. Three-level deep with mixed provider + thand roles (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level deep mixed provider and thand roles", func(t *testing.T) {
		roles := map[string]models.Role{
			"tier0_baseline": {
				Name:        "Tier-0 Baseline",
				Description: "Foundation role with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("sts:GetCallerIdentity", "sts:GetSessionToken"),
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
			"tier1_team": {
				Name:        "Tier-1 Team",
				Description: "Team role inheriting baseline, scoped to engineering",
				Inherits:    []string{"tier0_baseline"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:GetObject", "s3:PutObject"},
						Targets:    []string{"arn:aws:s3:::team-*"},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
			"tier2_lead": {
				Name:        "Tier-2 Lead",
				Description: "Team lead with IAMFullAccess and broad EC2",
				Inherits: []string{
					"tier1_team", // thand (→ tier0_baseline → depth 3)
					"arn:aws:iam::aws:policy/IAMFullAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"lead@example.com"},
					},
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, awsProviders)

		// Identity passes ALL three scope gates
		identity := &models.Identity{
			ID: "lead1",
			User: &models.User{
				Username: "teamlead",
				Email:    "lead@example.com",
				Groups:   []string{"engineering", "leads"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "tier2_lead")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand inheritance → composite
		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// Provider roles accumulated from tier0 + tier2
		assert.ElementsMatch(t, []string{"ReadOnlyAccess", "IAMFullAccess"}, result.Inherits,
			"Provider roles from tier0 and tier2 should accumulate")

		// Verify merged permissions
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "ec2:*",
			"tier2 ec2:* should be present: got %v", allowOps)

		// STS permissions from tier0 should be merged
		foundSts := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "sts:" {
				foundSts = true
				break
			}
		}
		assert.True(t, foundSts,
			"tier0 STS permissions should be merged through the chain: got %v", allowOps)

		// S3 permissions from tier1 should be present (with targets)
		foundS3 := false
		for _, op := range allowOps {
			if len(op) >= 3 && op[:3] == "s3:" {
				foundS3 = true
				break
			}
		}
		assert.True(t, foundS3,
			"tier1 S3 permissions should be merged: got %v", allowOps)

		// Verify the tier1 S3 targets are preserved through the merge
		var s3Targets []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if len(op) >= 3 && op[:3] == "s3:" {
					s3Targets = stmt.Targets
					break
				}
			}
		}
		assert.ElementsMatch(t, []string{"arn:aws:s3:::team-*"}, s3Targets,
			"S3 targets from tier1 should be preserved")

		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 6. Three-level deep with deny permissions + wildcard subsumption (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level deny permissions and wildcard subsumption", func(t *testing.T) {
		roles := map[string]models.Role{
			"l0_base": {
				Name:        "L0 Base",
				Description: "Base level with specific EC2 and S3 read perms",
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"ec2:DescribeInstances",
						"ec2:DescribeSecurityGroups",
						"s3:GetObject",
					),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"l1_power": {
				Name:        "L1 Power User",
				Description: "Power user with ec2:* and RDS read, deny rds:Delete",
				Inherits:    []string{"l0_base"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "rds:DescribeDBInstances"),
					Deny:  stmtsAws("rds:DeleteDBInstance"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"l2_admin": {
				Name:        "L2 Admin",
				Description: "Admin with rds:* overriding child deny, plus managed policy",
				Inherits: []string{
					"l1_power",
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("rds:*"),
					Deny:  stmtsAws("iam:DeleteRole"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "aws-prod",
				Description: "AWS Production",
				Provider:    "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "sysadmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "l2_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Inherited from thand roles → composite
		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// Provider only
		assert.ElementsMatch(t, []string{"AdministratorAccess"}, result.Inherits,
			"Only the managed policy should remain in Inherits")

		// --- Allow permissions ---
		allowOps := collectOps(result.Permissions.Allow)

		// ec2:* from l1 should subsume l0's ec2:DescribeInstances + ec2:DescribeSecurityGroups
		assert.Contains(t, allowOps, "ec2:*",
			"ec2:* from l1 should be present: got %v", allowOps)
		for _, op := range allowOps {
			assert.NotEqual(t, "ec2:DescribeInstances", op,
				"ec2:DescribeInstances should be subsumed by ec2:*: got %v", allowOps)
			assert.NotEqual(t, "ec2:DescribeSecurityGroups", op,
				"ec2:DescribeSecurityGroups should be subsumed by ec2:*: got %v", allowOps)
		}

		// rds:* from l2 should subsume l1's rds:DescribeDBInstances
		assert.Contains(t, allowOps, "rds:*",
			"rds:* from l2 should be present: got %v", allowOps)
		for _, op := range allowOps {
			assert.NotEqual(t, "rds:DescribeDBInstances", op,
				"rds:DescribeDBInstances should be subsumed by rds:*: got %v", allowOps)
		}

		// s3:GetObject from l0 should still be present (no wildcard to subsume it)
		assert.Contains(t, allowOps, "s3:GetObject",
			"s3:GetObject from l0 should survive: got %v", allowOps)

		// --- Deny permissions ---
		denyOps := collectOps(result.Permissions.Deny)

		// l2's deny iam:DeleteRole should survive
		assert.Contains(t, denyOps, "iam:DeleteRole",
			"iam:DeleteRole deny should survive: got %v", denyOps)

		// l1's deny rds:DeleteDBInstance should be overridden by l2's allow rds:*
		// (parent allow wins over child deny)
		for _, op := range denyOps {
			assert.NotContains(t, op, "rds:DeleteDBInstance",
				"rds:DeleteDBInstance deny should be overridden by parent rds:* allow: got %v", denyOps)
		}
	})

	// ---------------------------------------------------------------
	// 7. Three-level with targets preserved across the chain (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level targets preserved across chain", func(t *testing.T) {
		roles := map[string]models.Role{
			"t0_s3_reader": {
				Name:        "T0 S3 Reader",
				Description: "S3 reader scoped to data-* buckets",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:GetObject", "s3:ListBucket"},
						Targets:    []string{"arn:aws:s3:::data-*"},
					}},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"t1_s3_writer": {
				Name:        "T1 S3 Writer",
				Description: "S3 writer scoped to uploads-* buckets, inherits reader",
				Inherits:    []string{"t0_s3_reader"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:PutObject", "s3:DeleteObject"},
						Targets:    []string{"arn:aws:s3:::uploads-*"},
					}},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"t2_s3_admin": {
				Name:        "T2 S3 Admin",
				Description: "Full S3 admin, subsumes specific s3 ops from lower tiers",
				Inherits:    []string{"t1_s3_writer"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:*"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:        "aws-prod",
				Description: "AWS Production",
				Provider:    "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "s3admin1",
			User: &models.User{
				Username: "s3admin",
				Email:    "s3admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "t2_s3_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand inheritance → composite
		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// ReadOnlyAccess from t0 should propagate through the chain
		assert.ElementsMatch(t, []string{"ReadOnlyAccess"}, result.Inherits,
			"ReadOnlyAccess from t0 should propagate through inheritance chain")

		// s3:* from t2 should subsume the specific s3 operations from t0 and t1
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "s3:*",
			"s3:* from t2 should be present: got %v", allowOps)

		// Specific s3 operations should be subsumed by s3:*
		for _, op := range allowOps {
			assert.NotEqual(t, "s3:GetObject", op,
				"s3:GetObject should be subsumed by s3:*: got %v", allowOps)
			assert.NotEqual(t, "s3:ListBucket", op,
				"s3:ListBucket should be subsumed by s3:*: got %v", allowOps)
			assert.NotEqual(t, "s3:PutObject", op,
				"s3:PutObject should be subsumed by s3:*: got %v", allowOps)
			assert.NotEqual(t, "s3:DeleteObject", op,
				"s3:DeleteObject should be subsumed by s3:*: got %v", allowOps)
		}

		assert.ElementsMatch(t, []string{"aws-prod"}, result.Providers)
	})
}

// TestAWSInheritanceScopeDenial validates that users outside a role's scope do
// NOT receive that role's permissions or provider-role inherits when the scoped
// role appears at various depths in the inheritance chain.
func TestAWSInheritanceScopeDenial(t *testing.T) {
	awsProviders := map[string]models.ProviderConfig{
		"aws-prod": {
			Name:        "aws-prod",
			Description: "AWS Production",
			Provider:    "aws",
		},
		"aws-dev": {
			Name:        "aws-dev",
			Description: "AWS Development",
			Provider:    "aws",
		},
	}

	// ---------------------------------------------------------------
	// 1. User outside allow-scope gets no permissions from scoped role
	// ---------------------------------------------------------------
	t.Run("user outside allow-scope inherits nothing from scoped child", func(t *testing.T) {
		roles := map[string]models.Role{
			"scoped_power": {
				Name:        "Scoped Power",
				Description: "Power perms restricted to SRE group",
				Inherits: []string{
					"arn:aws:iam::aws:policy/PowerUserAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "s3:*", "rds:*"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"sre-team"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"top_role": {
				Name:        "Top Role",
				Description: "Inherits scoped_power, adds cloudwatch",
				Inherits:    []string{"scoped_power"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("cloudwatch:GetMetricData"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		// User NOT in sre-team → scoped_power should be skipped
		identity := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "regulardev",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "top_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Only top_role's own permissions should be present
		allowOps := collectOps(result.Permissions.Allow)
		assert.ElementsMatch(t, []string{"cloudwatch:GetMetricData"}, allowOps,
			"No permissions from scope-denied scoped_power should leak: got %v", allowOps)

		// scoped_power's provider role (PowerUserAccess) must NOT propagate
		assert.Empty(t, result.Inherits,
			"Provider-role inherits from scope-denied role must not appear: got %v", result.Inherits)

		// Not composite since no thand role was successfully merged
		assert.False(t, result.Composite,
			"Should not be composite when the only inherited thand role was scope-denied")
	})

	// ---------------------------------------------------------------
	// 2. User in deny-scope gets nothing from scoped role (depth 2)
	// ---------------------------------------------------------------
	t.Run("user in deny-scope inherits nothing from denied child at depth 2", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_reader": {
				Name:        "Base Reader",
				Description: "Simple read perms, no scope",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject", "s3:ListBuckets"),
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
			"sensitive_ops": {
				Name:        "Sensitive Ops",
				Description: "Sensitive operations, deny-scoped to contractors",
				Inherits:    []string{"base_reader"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "rds:*", "iam:CreateRole"),
				},
				Scopes: models.RoleScopes{
					Deny: models.ScopeIdentities{
						Groups: []string{"contractors"},
					},
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
			"manager_role": {
				Name:        "Manager",
				Description: "Manager inheriting sensitive_ops",
				Inherits: []string{
					"sensitive_ops",
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("cloudwatch:*"),
				},
				Providers: []string{"aws-prod", "aws-dev"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, awsProviders)

		// Contractor identity → denied by sensitive_ops scope
		contractor := &models.Identity{
			ID: "contractor1",
			User: &models.User{
				Username: "contractor",
				Email:    "contractor@vendor.com",
				Groups:   []string{"contractors", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(contractor, "manager_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		allowOps := collectOps(result.Permissions.Allow)

		// Only manager_role's own permissions should be present
		assert.Contains(t, allowOps, "cloudwatch:*",
			"Manager's own cloudwatch:* must be present: got %v", allowOps)

		// sensitive_ops perms must NOT be present
		for _, op := range allowOps {
			assert.NotContains(t, op, "ec2:",
				"ec2 perms from scope-denied sensitive_ops must not appear: got %v", allowOps)
			assert.NotContains(t, op, "rds:",
				"rds perms from scope-denied sensitive_ops must not appear: got %v", allowOps)
			assert.NotEqual(t, "iam:CreateRole", op,
				"iam:CreateRole from scope-denied sensitive_ops must not appear: got %v", allowOps)
		}

		// base_reader perms must NOT be present (transitive through denied role)
		for _, op := range allowOps {
			assert.NotContains(t, op, "s3:",
				"s3 perms from base_reader (via denied sensitive_ops) must not appear: got %v", allowOps)
		}

		// ReadOnlyAccess from base_reader must NOT propagate through denied chain
		for _, inh := range result.Inherits {
			assert.NotEqual(t, "ReadOnlyAccess", inh,
				"ReadOnlyAccess from base_reader should not propagate through scope-denied chain: got %v", result.Inherits)
		}

		// manager_role's own SecurityAudit provider role should still be present
		assert.Contains(t, result.Inherits, "SecurityAudit",
			"Manager's own SecurityAudit provider role should remain: got %v", result.Inherits)

		// Not composite — sensitive_ops was denied, so no thand role was merged
		assert.False(t, result.Composite,
			"Should not be composite when the inherited thand role was scope-denied")

		// ── Now test with an employee who IS allowed ──
		employee := &models.Identity{
			ID: "emp1",
			User: &models.User{
				Username: "employee",
				Email:    "employee@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEmp, err := config.GetCompositeRoleByName(employee, "manager_role")
		require.NoError(t, err)
		require.NotNil(t, resultEmp)

		// Employee should get EVERYTHING merged
		empAllowOps := collectOps(resultEmp.Permissions.Allow)
		assert.Contains(t, empAllowOps, "cloudwatch:*",
			"Employee should get cloudwatch:*: got %v", empAllowOps)
		assert.Contains(t, empAllowOps, "ec2:*",
			"Employee should get ec2:* from sensitive_ops: got %v", empAllowOps)
		assert.Contains(t, empAllowOps, "rds:*",
			"Employee should get rds:* from sensitive_ops: got %v", empAllowOps)

		// ReadOnlyAccess from base_reader should propagate for allowed user
		assert.Contains(t, resultEmp.Inherits, "ReadOnlyAccess",
			"Employee should get ReadOnlyAccess from base_reader: got %v", resultEmp.Inherits)
		assert.Contains(t, resultEmp.Inherits, "SecurityAudit",
			"Employee should get SecurityAudit from manager_role: got %v", resultEmp.Inherits)

		assert.True(t, resultEmp.Composite,
			"Employee result should be composite since thand roles were merged")
	})

	// ---------------------------------------------------------------
	// 3. Three-level: middle role scoped by domain, bottom role scoped
	//    by group — user fails middle scope (depth 3)
	// ---------------------------------------------------------------
	t.Run("three-level: user fails middle domain scope", func(t *testing.T) {
		roles := map[string]models.Role{
			"bottom_base": {
				Name:        "Bottom Base",
				Description: "Foundation with IAM read",
				Inherits: []string{
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("sts:GetCallerIdentity", "iam:GetUser"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering", "ops"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"middle_team": {
				Name:        "Middle Team",
				Description: "Team role scoped to acme.com domain",
				Inherits:    []string{"bottom_base"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject", "s3:PutObject", "ec2:DescribeInstances"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"acme.com"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"top_lead": {
				Name:        "Top Lead",
				Description: "Lead role, no scope restriction",
				Inherits: []string{
					"middle_team",
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		// ── User from external.com (fails middle_team's acme.com domain scope) ──
		externalUser := &models.Identity{
			ID: "ext1",
			User: &models.User{
				Username: "external",
				Email:    "user@external.com",
				Groups:   []string{"engineering"},
			},
		}

		resultExt, err := config.GetCompositeRoleByName(externalUser, "top_lead")
		require.NoError(t, err)
		require.NotNil(t, resultExt)

		extAllowOps := collectOps(resultExt.Permissions.Allow)

		// Only top_lead's own ec2:* should remain
		assert.Contains(t, extAllowOps, "ec2:*",
			"External user should get top_lead's ec2:*: got %v", extAllowOps)

		// middle_team's s3 permissions should NOT be present
		for _, op := range extAllowOps {
			assert.NotContains(t, op, "s3:",
				"s3 perms from scope-denied middle_team must not appear for external user: got %v", extAllowOps)
		}

		// bottom_base's permissions should NOT propagate (transitive through denied middle)
		for _, op := range extAllowOps {
			assert.NotContains(t, op, "sts:",
				"sts perms from bottom_base must not leak through denied middle: got %v", extAllowOps)
			assert.NotEqual(t, "iam:GetUser", op,
				"iam:GetUser from bottom_base must not leak through denied middle: got %v", extAllowOps)
		}

		// IAMReadOnlyAccess from bottom_base must NOT propagate
		for _, inh := range resultExt.Inherits {
			assert.NotEqual(t, "IAMReadOnlyAccess", inh,
				"IAMReadOnlyAccess from bottom_base should not propagate through denied middle: got %v", resultExt.Inherits)
		}

		// top_lead's own ReadOnlyAccess should still be there
		assert.Contains(t, resultExt.Inherits, "ReadOnlyAccess",
			"Top lead's own ReadOnlyAccess should remain: got %v", resultExt.Inherits)

		// Not composite — middle_team was denied, so no thand role merged
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

		resultAcme, err := config.GetCompositeRoleByName(acmeUser, "top_lead")
		require.NoError(t, err)
		require.NotNil(t, resultAcme)

		acmeAllowOps := collectOps(resultAcme.Permissions.Allow)

		// Should get everything merged from all three levels
		assert.Contains(t, acmeAllowOps, "ec2:*",
			"acme user should get ec2:*: got %v", acmeAllowOps)

		// STS from bottom_base should be present
		foundSts := false
		for _, op := range acmeAllowOps {
			if len(op) >= 4 && op[:4] == "sts:" {
				foundSts = true
				break
			}
		}
		assert.True(t, foundSts,
			"acme user should get sts perms from bottom_base: got %v", acmeAllowOps)

		// Provider roles from all levels should propagate
		assert.Contains(t, resultAcme.Inherits, "ReadOnlyAccess",
			"acme user should get ReadOnlyAccess: got %v", resultAcme.Inherits)
		assert.Contains(t, resultAcme.Inherits, "IAMReadOnlyAccess",
			"acme user should get IAMReadOnlyAccess from bottom_base: got %v", resultAcme.Inherits)

		assert.True(t, resultAcme.Composite,
			"acme user should get composite role when all scopes pass")
	})

	// ---------------------------------------------------------------
	// 4. Mixed: one inherited role allowed, another denied (depth 1)
	// ---------------------------------------------------------------
	t.Run("one inherited role allowed and another denied at same level", func(t *testing.T) {
		roles := map[string]models.Role{
			"open_base": {
				Name:        "Open Base",
				Description: "Open to all, no scope",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject", "s3:ListBuckets"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"restricted_admin": {
				Name:        "Restricted Admin",
				Description: "Admin perms, only for admins group",
				Inherits: []string{
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "rds:*", "iam:*"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"admins"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"combined_role": {
				Name:        "Combined",
				Description: "Inherits both open_base and restricted_admin",
				Inherits: []string{
					"open_base",
					"restricted_admin",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("cloudwatch:GetMetricData"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, providers)

		// ── Non-admin user: open_base should merge, restricted_admin should be skipped ──
		nonadmin := &models.Identity{
			ID: "dev1",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		resultDev, err := config.GetCompositeRoleByName(nonadmin, "combined_role")
		require.NoError(t, err)
		require.NotNil(t, resultDev)

		devAllowOps := collectOps(resultDev.Permissions.Allow)

		// open_base's s3 permissions should be merged
		foundS3 := false
		for _, op := range devAllowOps {
			if len(op) >= 3 && op[:3] == "s3:" {
				foundS3 = true
				break
			}
		}
		assert.True(t, foundS3,
			"Non-admin should get s3 perms from open_base: got %v", devAllowOps)

		// cloudwatch from combined_role itself
		assert.Contains(t, devAllowOps, "cloudwatch:GetMetricData",
			"Non-admin should get cloudwatch:GetMetricData: got %v", devAllowOps)

		// restricted_admin's ec2/rds/iam must NOT be present
		for _, op := range devAllowOps {
			assert.NotContains(t, op, "ec2:",
				"ec2 perms from denied restricted_admin must not appear for non-admin: got %v", devAllowOps)
			assert.NotContains(t, op, "rds:",
				"rds perms from denied restricted_admin must not appear for non-admin: got %v", devAllowOps)
			assert.NotContains(t, op, "iam:",
				"iam perms from denied restricted_admin must not appear for non-admin: got %v", devAllowOps)
		}

		// ReadOnlyAccess from open_base should propagate
		assert.Contains(t, resultDev.Inherits, "ReadOnlyAccess",
			"Non-admin should get ReadOnlyAccess from open_base: got %v", resultDev.Inherits)

		// AdministratorAccess from restricted_admin must NOT propagate
		for _, inh := range resultDev.Inherits {
			assert.NotEqual(t, "AdministratorAccess", inh,
				"AdministratorAccess must not propagate from denied restricted_admin: got %v", resultDev.Inherits)
		}

		// Still composite — open_base WAS merged
		assert.True(t, resultDev.Composite,
			"Should be composite since open_base was successfully merged")

		// ── Admin user: BOTH should merge ──
		adminUser := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
				Groups:   []string{"admins", "developers"},
			},
		}

		resultAdmin, err := config.GetCompositeRoleByName(adminUser, "combined_role")
		require.NoError(t, err)
		require.NotNil(t, resultAdmin)

		adminAllowOps := collectOps(resultAdmin.Permissions.Allow)

		// Admin should get everything from both roles
		assert.Contains(t, adminAllowOps, "cloudwatch:GetMetricData",
			"Admin should get cloudwatch: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "ec2:*",
			"Admin should get ec2:* from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "rds:*",
			"Admin should get rds:* from restricted_admin: got %v", adminAllowOps)
		assert.Contains(t, adminAllowOps, "iam:*",
			"Admin should get iam:* from restricted_admin: got %v", adminAllowOps)

		// Both provider roles should be in Inherits
		assert.Contains(t, resultAdmin.Inherits, "ReadOnlyAccess",
			"Admin should get ReadOnlyAccess: got %v", resultAdmin.Inherits)
		assert.Contains(t, resultAdmin.Inherits, "AdministratorAccess",
			"Admin should get AdministratorAccess: got %v", resultAdmin.Inherits)

		assert.True(t, resultAdmin.Composite,
			"Admin result should be composite since thand roles were merged")
	})

	// ---------------------------------------------------------------
	// 5. Deny-scope user + allow-scope user in same group hierarchy
	// ---------------------------------------------------------------
	t.Run("deny scope takes precedence over allow scope for same user", func(t *testing.T) {
		roles := map[string]models.Role{
			"privileged_role": {
				Name:        "Privileged",
				Description: "High-privilege role with both allow and deny scopes",
				Inherits: []string{
					"arn:aws:iam::aws:policy/PowerUserAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "s3:*", "rds:*"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
					Deny: models.ScopeIdentities{
						Users: []string{"intern@example.com"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"wrapper_role": {
				Name:        "Wrapper",
				Description: "Wraps privileged role",
				Inherits:    []string{"privileged_role"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("logs:GetLogEvents"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
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

		resultIntern, err := config.GetCompositeRoleByName(intern, "wrapper_role")
		require.NoError(t, err)
		require.NotNil(t, resultIntern)

		internOps := collectOps(resultIntern.Permissions.Allow)

		// Only wrapper's own perms
		assert.ElementsMatch(t, []string{"logs:GetLogEvents"}, internOps,
			"Intern (deny-scoped) should only get wrapper's own perms: got %v", internOps)

		// PowerUserAccess must NOT propagate
		assert.Empty(t, resultIntern.Inherits,
			"Intern should not get provider roles from denied privileged_role: got %v", resultIntern.Inherits)

		assert.False(t, resultIntern.Composite,
			"Intern should not get composite when privileged_role was denied")

		// Regular engineer: in engineering group, NOT in deny list → gets everything
		engineer := &models.Identity{
			ID: "eng1",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEng, err := config.GetCompositeRoleByName(engineer, "wrapper_role")
		require.NoError(t, err)
		require.NotNil(t, resultEng)

		engOps := collectOps(resultEng.Permissions.Allow)
		assert.Contains(t, engOps, "ec2:*",
			"Engineer should get ec2:* from privileged_role: got %v", engOps)
		assert.Contains(t, engOps, "s3:*",
			"Engineer should get s3:* from privileged_role: got %v", engOps)
		assert.Contains(t, engOps, "rds:*",
			"Engineer should get rds:* from privileged_role: got %v", engOps)
		assert.Contains(t, engOps, "logs:GetLogEvents",
			"Engineer should get logs from wrapper: got %v", engOps)

		assert.Contains(t, resultEng.Inherits, "PowerUserAccess",
			"Engineer should get PowerUserAccess: got %v", resultEng.Inherits)

		assert.True(t, resultEng.Composite,
			"Engineer should get composite role")
	})
}
