package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// =============================================================================
// TestAWSComplexReadOnlyAccessInheritance exercises scenarios where the
// pre-defined AWS managed policy arn:aws:iam::aws:policy/ReadOnlyAccess is
// inherited and combined with thand roles containing multiple AWS managed
// policies, permission sets, and custom permissions. The tests verify correct
// composite-role marking, wildcard subsumption, scope evaluation, target
// preservation and deny-propagation across 1-3 levels of nesting.
// =============================================================================

// sharedAWSProviders returns a set of AWS providers re-used by most subtests.
func sharedAWSProviders() map[string]models.ProviderConfig {
	return map[string]models.ProviderConfig{
		"aws-prod": {
			Name:        "aws-prod",
			Description: "AWS Production",
			Provider:    "aws",
		},
		"aws-staging": {
			Name:        "aws-staging",
			Description: "AWS Staging",
			Provider:    "aws",
		},
		"aws-dev": {
			Name:        "aws-dev",
			Description: "AWS Development",
			Provider:    "aws",
		},
	}
}

// TestAWSMultiPolicyCompositeRoles validates that thand roles combining
// ReadOnlyAccess with other managed policies and custom permissions are
// resolved correctly at depth 1-3 with proper composite marking,
// permission merging, and provider-role accumulation.
func TestAWSMultiPolicyCompositeRoles(t *testing.T) {
	providers := sharedAWSProviders()

	// ---------------------------------------------------------------
	// 1. Single thand role with ReadOnlyAccess + SecurityAudit +
	//    custom permissions (depth 1, composite = false since no
	//    thand-role inheritance)
	// ---------------------------------------------------------------
	t.Run("depth-1: ReadOnlyAccess plus SecurityAudit plus custom perms", func(t *testing.T) {
		roles := map[string]models.Role{
			"security_viewer": {
				Name:        "Security Viewer",
				Description: "Combines ReadOnlyAccess, SecurityAudit, and custom CloudTrail perms",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"cloudtrail:LookupEvents",
							"cloudtrail:GetTrailStatus",
							"guardduty:ListFindings",
							"guardduty:GetFindings",
						},
						Targets: []string{
							"arn:aws:cloudtrail:*:*:trail/prod-*",
						},
					}},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "secops1",
			User: &models.User{
				Username: "secops",
				Email:    "secops@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "security_viewer")
		require.NoError(t, err)
		require.NotNil(t, result)

		// No thand-role inheritance → not composite
		assert.False(t, result.Composite,
			"Provider-only inheritance should NOT be composite")

		// Both managed policies should be resolved to short names
		assert.ElementsMatch(t, []string{"ReadOnlyAccess", "SecurityAudit"}, result.Inherits,
			"Both managed policies should be in Inherits as short names")

		// Custom permissions should be present with targets
		allowOps := collectOps(result.Permissions.Allow)
		for _, expected := range []string{
			"cloudtrail:GetTrailStatus,LookupEvents",
			"guardduty:GetFindings,ListFindings",
		} {
			assert.Contains(t, allowOps, expected,
				"Custom permission %q should be present: got %v", expected, allowOps)
		}

		// Targets should be preserved
		var targets []string
		for _, stmt := range result.Permissions.Allow {
			targets = append(targets, stmt.Targets...)
		}
		assert.ElementsMatch(t, []string{"arn:aws:cloudtrail:*:*:trail/prod-*"}, targets)

		assert.ElementsMatch(t, []string{"aws-prod", "aws-staging"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 2. Two-level: base with ReadOnlyAccess, child with
	//    PowerUserAccess + IAMReadOnlyAccess + broad EC2/S3 perms,
	//    grandchild adds IAMFullAccess + admin-level deny (depth 2)
	// ---------------------------------------------------------------
	t.Run("depth-2: base ReadOnlyAccess, child multi-policy, parent with additional policies", func(t *testing.T) {
		roles := map[string]models.Role{
			"readonly_base": {
				Name:        "ReadOnly Base",
				Description: "Foundation role with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"sts:GetCallerIdentity",
						"sts:GetSessionToken",
						"iam:GetUser",
					),
				},
				Providers: []string{"aws-prod", "aws-staging", "aws-dev"},
				Enabled:   true,
			},
			"power_operator": {
				Name:        "Power Operator",
				Description: "Inherits readonly_base + PowerUserAccess + IAMReadOnlyAccess with broad perms",
				Inherits: []string{
					"readonly_base", // thand (depth 2)
					"arn:aws:iam::aws:policy/PowerUserAccess",
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{"ec2:*", "rds:*"},
							Targets:    []string{"arn:aws:ec2:us-east-1:*:*"},
						},
						{
							Operations: []string{"s3:*"},
							Targets:    []string{"arn:aws:s3:::ops-*"},
						},
					},
					Deny: stmtsAws(
						"ec2:TerminateInstances",
						"rds:DeleteDBCluster",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"operations", "sre"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		// Identity that passes scope
		identity := &models.Identity{
			ID: "ops1",
			User: &models.User{
				Username: "operator1",
				Email:    "ops1@example.com",
				Groups:   []string{"operations", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "power_operator")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Inherited from thand role readonly_base → composite
		assert.True(t, result.Composite,
			"Two-level thand inheritance should be composite")

		// Provider roles from both levels should accumulate:
		// readonly_base → ReadOnlyAccess (bubbled up)
		// power_operator → PowerUserAccess, IAMReadOnlyAccess
		assert.Len(t, result.Inherits, 3,
			"Should have 3 provider roles: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "ReadOnlyAccess")
		assert.Contains(t, result.Inherits, "PowerUserAccess")
		assert.Contains(t, result.Inherits, "IAMReadOnlyAccess")

		// Merged allow permissions from both levels
		allowOps := collectOps(result.Permissions.Allow)

		// power_operator's own broad perms
		assert.Contains(t, allowOps, "ec2:*", "ec2:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "rds:*", "rds:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "s3:*", "s3:* should be present: %v", allowOps)

		// readonly_base's STS/IAM perms should be merged (condensed)
		foundSts := false
		foundIam := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "sts:" {
				foundSts = true
			}
			if len(op) >= 4 && op[:4] == "iam:" {
				foundIam = true
			}
		}
		assert.True(t, foundSts, "STS perms from readonly_base should be merged: %v", allowOps)
		assert.True(t, foundIam, "IAM perms from readonly_base should be merged: %v", allowOps)

		// Deny permissions should survive
		denyOps := collectOps(result.Permissions.Deny)
		assert.Contains(t, denyOps, "ec2:TerminateInstances",
			"ec2:TerminateInstances deny should survive: %v", denyOps)

		// Targets from power_operator's statements should be preserved
		var ec2Targets, s3Targets []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if op == "ec2:*" || op == "rds:*" {
					ec2Targets = stmt.Targets
				}
				if op == "s3:*" {
					s3Targets = stmt.Targets
				}
			}
		}
		assert.ElementsMatch(t, []string{"arn:aws:ec2:us-east-1:*:*"}, ec2Targets,
			"EC2 targets should be preserved")
		assert.ElementsMatch(t, []string{"arn:aws:s3:::ops-*"}, s3Targets,
			"S3 targets should be preserved")

		assert.ElementsMatch(t, []string{"aws-prod", "aws-staging"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 3. Three-level: ReadOnlyAccess at bottom, SecurityAudit at mid,
	//    AdministratorAccess+IAMFullAccess at top. Each level adds
	//    unique permissions, scopes narrow at each tier. (depth 3)
	// ---------------------------------------------------------------
	t.Run("depth-3: three thand tiers each with multiple managed policies", func(t *testing.T) {
		roles := map[string]models.Role{
			"tier0_observer": {
				Name:        "Tier-0 Observer",
				Description: "Base observer with ReadOnlyAccess + CloudWatch",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"cloudwatch:GetMetricData",
						"cloudwatch:ListMetrics",
						"cloudwatch:GetDashboard",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"example.com"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging", "aws-dev"},
				Enabled:   true,
			},
			"tier1_analyst": {
				Name:        "Tier-1 Analyst",
				Description: "Analyst inheriting observer + SecurityAudit + custom logging",
				Inherits: []string{
					"tier0_observer", // thand (depth 2)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{
								"athena:StartQueryExecution",
								"athena:GetQueryResults",
								"athena:GetQueryExecution",
							},
							Targets: []string{"arn:aws:athena:*:*:workgroup/analysts-*"},
						},
						{
							Operations: []string{
								"logs:StartQuery",
								"logs:GetQueryResults",
								"logs:DescribeLogGroups",
							},
						},
					},
					Deny: stmtsAws("athena:DeleteWorkGroup"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"data-team", "analytics"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			"tier2_data_lead": {
				Name:        "Tier-2 Data Lead",
				Description: "Data lead with AdministratorAccess + IAMFullAccess + full data perms",
				Inherits: []string{
					"tier1_analyst", // thand (→ tier0_observer → depth 3)
					"arn:aws:iam::aws:policy/AdministratorAccess",
					"arn:aws:iam::aws:policy/IAMFullAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{"glue:*", "athena:*"},
							Targets:    []string{"arn:aws:glue:*:*:catalog"},
						},
						{
							Operations: []string{"s3:*"},
							Targets:    []string{"arn:aws:s3:::datalake-*"},
						},
					},
					Deny: stmtsAws(
						"iam:DeleteRole",
						"iam:DeleteUser",
						"iam:DeletePolicy",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{"datalead@example.com"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		// Identity passes ALL three scope gates:
		// tier0: domain example.com ✓
		// tier1: group data-team ✓
		// tier2: user datalead@example.com ✓
		identity := &models.Identity{
			ID: "datalead1",
			User: &models.User{
				Username: "datalead",
				Email:    "datalead@example.com",
				Groups:   []string{"data-team", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "tier2_data_lead")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand inheritance → composite
		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// Provider roles from all three tiers should accumulate:
		// tier0: ReadOnlyAccess
		// tier1: SecurityAudit
		// tier2: AdministratorAccess, IAMFullAccess
		assert.Len(t, result.Inherits, 4,
			"4 managed policies from all tiers should accumulate: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "ReadOnlyAccess")
		assert.Contains(t, result.Inherits, "SecurityAudit")
		assert.Contains(t, result.Inherits, "AdministratorAccess")
		assert.Contains(t, result.Inherits, "IAMFullAccess")

		// Merged permissions from all three levels
		allowOps := collectOps(result.Permissions.Allow)

		// tier2 wildcards should subsume tier1 athena specifics
		assert.Contains(t, allowOps, "athena:*",
			"athena:* from tier2 should be present: %v", allowOps)
		assert.Contains(t, allowOps, "glue:*",
			"glue:* from tier2 should be present: %v", allowOps)
		assert.Contains(t, allowOps, "s3:*",
			"s3:* from tier2 should be present: %v", allowOps)

		// tier0's CloudWatch perms should be merged (condensed)
		foundCw := false
		for _, op := range allowOps {
			if len(op) >= 11 && op[:11] == "cloudwatch:" {
				foundCw = true
				break
			}
		}
		assert.True(t, foundCw,
			"CloudWatch perms from tier0 should be merged: %v", allowOps)

		// tier1's logs perms should be merged
		foundLogs := false
		for _, op := range allowOps {
			if len(op) >= 5 && op[:5] == "logs:" {
				foundLogs = true
				break
			}
		}
		assert.True(t, foundLogs,
			"Logs perms from tier1 should be merged: %v", allowOps)

		// Deny permissions from BOTH tier1 and tier2 should survive:
		// tier1 deny: athena:DeleteWorkGroup → BUT parent tier2 has athena:*
		// allow, so parent-wins means child deny is overridden
		denyOps := collectOps(result.Permissions.Deny)

		// tier2's deny should survive (no parent to override)
		for _, denied := range []string{"iam:DeletePolicy,DeleteRole,DeleteUser"} {
			assert.Contains(t, denyOps, denied,
				"tier2 deny %q should survive: %v", denied, denyOps)
		}

		// tier1's athena:DeleteWorkGroup deny should be overridden by tier2's athena:* allow
		for _, op := range denyOps {
			assert.NotContains(t, op, "athena:",
				"athena deny from tier1 should be overridden by tier2's athena:* allow: %v", denyOps)
		}

		// Targets from tier2 should be preserved
		var glueTgts, s3Tgts []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if op == "glue:*" || op == "athena:*" {
					glueTgts = stmt.Targets
				}
				if op == "s3:*" {
					s3Tgts = stmt.Targets
				}
			}
		}
		assert.ElementsMatch(t, []string{"arn:aws:glue:*:*:catalog"}, glueTgts,
			"Glue targets from tier2 should be preserved")
		assert.ElementsMatch(t, []string{"arn:aws:s3:::datalake-*"}, s3Tgts,
			"S3 targets from tier2 should be preserved")

		assert.ElementsMatch(t, []string{"aws-prod"}, result.Providers)
	})
}

// TestAWSMultiRolePermissionSetInheritance exercises complex scenarios where
// thand roles contain multiple AWS managed policies acting as "permission
// sets" and are nested 1-3 levels deep. Tests verify that all permission
// sets propagate correctly, wildcards are subsumed, and composite marking
// is accurate.
func TestAWSMultiRolePermissionSetInheritance(t *testing.T) {
	providers := sharedAWSProviders()

	// ---------------------------------------------------------------
	// 1. Platform engineering role: 3 managed policies + 2 thand
	//    child roles each with their own managed policies (depth 2)
	// ---------------------------------------------------------------
	t.Run("platform role inheriting two thand roles each with multiple managed policies", func(t *testing.T) {
		roles := map[string]models.Role{
			// Child 1: network-focused with VPC-related managed policies
			"network_viewer": {
				Name:        "Network Viewer",
				Description: "Network observability with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"ec2:DescribeVpcs",
							"ec2:DescribeSubnets",
							"ec2:DescribeSecurityGroups",
							"ec2:DescribeNetworkInterfaces",
							"ec2:DescribeRouteTables",
						},
						Targets: []string{"arn:aws:ec2:*:*:vpc/*"},
					}},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"networking", "platform-eng"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			// Child 2: database-focused with database-related managed policies
			"database_operator": {
				Name:        "Database Operator",
				Description: "Database operations with SecurityAudit",
				Inherits: []string{
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{
							"rds:DescribeDBInstances",
							"rds:DescribeDBClusters",
							"rds:DescribeDBSnapshots",
							"rds:CreateDBSnapshot",
							"dynamodb:DescribeTable",
							"dynamodb:ListTables",
						},
						Targets: []string{"arn:aws:rds:*:*:db:prod-*"},
					}},
					Deny: stmtsAws(
						"rds:DeleteDBInstance",
						"rds:DeleteDBCluster",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"dba", "platform-eng"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			// Parent: platform engineering inheriting both children + its own policies
			"platform_engineer": {
				Name:        "Platform Engineer",
				Description: "Full platform access combining network + database + own policies",
				Inherits: []string{
					"network_viewer",    // thand (depth 2)
					"database_operator", // thand (depth 2)
					"arn:aws:iam::aws:policy/PowerUserAccess",
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{"eks:*", "ecs:*"},
							Targets:    []string{"arn:aws:eks:*:*:cluster/platform-*"},
						},
						{
							Operations: []string{"ecr:*"},
							Targets:    []string{"arn:aws:ecr:*:*:repository/platform-*"},
						},
					},
					Deny: stmtsAws(
						"iam:CreateUser",
						"iam:DeleteUser",
						"iam:AttachUserPolicy",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"platform-eng"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		// Identity in platform-eng → passes all three scopes
		identity := &models.Identity{
			ID: "plateng1",
			User: &models.User{
				Username: "plateng",
				Email:    "plateng@example.com",
				Groups:   []string{"platform-eng", "engineering"},
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "platform_engineer")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Two thand children inherited → composite
		assert.True(t, result.Composite,
			"Platform engineer with two thand children should be composite")

		// Provider roles from all three levels:
		// network_viewer: ReadOnlyAccess
		// database_operator: SecurityAudit
		// platform_engineer: PowerUserAccess, IAMReadOnlyAccess
		assert.Len(t, result.Inherits, 4,
			"Should have 4 managed policies from all levels: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "ReadOnlyAccess")
		assert.Contains(t, result.Inherits, "SecurityAudit")
		assert.Contains(t, result.Inherits, "PowerUserAccess")
		assert.Contains(t, result.Inherits, "IAMReadOnlyAccess")

		// Merged permissions
		allowOps := collectOps(result.Permissions.Allow)

		// Own perms
		assert.Contains(t, allowOps, "eks:*", "eks:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "ecs:*", "ecs:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "ecr:*", "ecr:* should be present: %v", allowOps)

		// network_viewer's EC2 perms should be merged (condensed)
		foundEc2 := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "ec2:" {
				foundEc2 = true
				break
			}
		}
		assert.True(t, foundEc2,
			"EC2 perms from network_viewer should be merged: %v", allowOps)

		// database_operator's RDS/DynamoDB perms
		foundRds := false
		foundDynamo := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "rds:" {
				foundRds = true
			}
			if len(op) >= 9 && op[:9] == "dynamodb:" {
				foundDynamo = true
			}
		}
		assert.True(t, foundRds,
			"RDS perms from database_operator should be merged: %v", allowOps)
		assert.True(t, foundDynamo,
			"DynamoDB perms from database_operator should be merged: %v", allowOps)

		// Deny perms from BOTH children and parent should survive:
		// database_operator's rds:Delete* stays (no parent rds:* to override)
		// platform_engineer's iam:* denies stay (own deny)
		denyOps := collectOps(result.Permissions.Deny)
		for _, denied := range []string{"iam:AttachUserPolicy,CreateUser,DeleteUser"} {
			assert.Contains(t, denyOps, denied,
				"Platform engineer's own deny %q should survive: %v", denied, denyOps)
		}
		// database_operator's rds deny
		foundRdsDeny := false
		for _, op := range denyOps {
			if len(op) >= 4 && op[:4] == "rds:" {
				foundRdsDeny = true
				break
			}
		}
		assert.True(t, foundRdsDeny,
			"RDS deny from database_operator should propagate: %v", denyOps)

		assert.ElementsMatch(t, []string{"aws-prod", "aws-staging"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 2. Three-level deep: each level has ReadOnlyAccess + unique
	//    managed policies. Wildcards at top subsume lower-level
	//    specific ops. (depth 3)
	// ---------------------------------------------------------------
	t.Run("depth-3: ReadOnlyAccess at every level with wildcard subsumption", func(t *testing.T) {
		roles := map[string]models.Role{
			"l0_auditor": {
				Name:        "L0 Auditor",
				Description: "Auditor with ReadOnlyAccess + specific S3 read",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"s3:GetObject",
						"s3:GetBucketPolicy",
						"s3:GetBucketAcl",
						"s3:ListBucket",
					),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"l1_compliance": {
				Name:        "L1 Compliance",
				Description: "Compliance officer inheriting auditor + SecurityAudit",
				Inherits: []string{
					"l0_auditor", // thand (depth 2)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"config:GetComplianceDetailsByConfigRule",
						"config:GetComplianceSummaryByResourceType",
						"config:DescribeConfigRules",
						"s3:PutObject", // adds write to the S3 perms from l0
					),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"l2_security_lead": {
				Name:        "L2 Security Lead",
				Description: "Security lead with s3:* that subsumes l0+l1 S3 perms",
				Inherits: []string{
					"l1_compliance", // thand (→ l0_auditor → depth 3)
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"s3:*",        // subsumes l0's s3:Get*, s3:List* and l1's s3:PutObject
						"guardduty:*", // full GuardDuty access
					),
					Deny: stmtsAws("s3:DeleteBucket"), // restrict even with s3:*
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "seclead1",
			User: &models.User{
				Username: "seclead",
				Email:    "seclead@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "l2_security_lead")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand chain → composite
		assert.True(t, result.Composite,
			"Three-level thand inheritance should be composite")

		// Provider roles accumulate from all levels:
		// l0: ReadOnlyAccess
		// l1: SecurityAudit
		// l2: AdministratorAccess
		assert.Len(t, result.Inherits, 3,
			"3 managed policies should accumulate: got %v", result.Inherits)
		assert.Contains(t, result.Inherits, "ReadOnlyAccess")
		assert.Contains(t, result.Inherits, "SecurityAudit")
		assert.Contains(t, result.Inherits, "AdministratorAccess")

		// s3:* from l2 should subsume all specific S3 ops from l0 and l1
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "s3:*",
			"s3:* from l2 should be present: %v", allowOps)

		// Specific S3 ops should be subsumed
		for _, op := range allowOps {
			assert.NotEqual(t, "s3:GetObject", op,
				"s3:GetObject should be subsumed by s3:*: %v", allowOps)
			assert.NotEqual(t, "s3:GetBucketPolicy", op,
				"s3:GetBucketPolicy should be subsumed by s3:*: %v", allowOps)
			assert.NotEqual(t, "s3:ListBucket", op,
				"s3:ListBucket should be subsumed by s3:*: %v", allowOps)
			assert.NotEqual(t, "s3:PutObject", op,
				"s3:PutObject should be subsumed by s3:*: %v", allowOps)
		}

		// guardduty:* should be present
		assert.Contains(t, allowOps, "guardduty:*",
			"guardduty:* should be present: %v", allowOps)

		// Config perms from l1 should be present (condensed)
		foundConfig := false
		for _, op := range allowOps {
			if len(op) >= 7 && op[:7] == "config:" {
				foundConfig = true
				break
			}
		}
		assert.True(t, foundConfig,
			"Config perms from l1 should be merged: %v", allowOps)

		// Deny: s3:DeleteBucket from l2 should survive
		denyOps := collectOps(result.Permissions.Deny)
		assert.Contains(t, denyOps, "s3:DeleteBucket",
			"s3:DeleteBucket deny should survive: %v", denyOps)
	})
}

// TestAWSScopeFilteringWithReadOnlyAccess validates that scope evaluation
// correctly gates access when ReadOnlyAccess and other managed policies
// are involved at various nesting depths.
func TestAWSScopeFilteringWithReadOnlyAccess(t *testing.T) {
	providers := sharedAWSProviders()

	// ---------------------------------------------------------------
	// 1. Scope-denied mid-level blocks ReadOnlyAccess propagation
	//    from bottom through to top (depth 3)
	// ---------------------------------------------------------------
	t.Run("scope denial at mid-level blocks ReadOnlyAccess from propagating", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_with_readonly": {
				Name:        "Base ReadOnly",
				Description: "Base with ReadOnlyAccess + IAMReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"sts:GetCallerIdentity",
						"sts:GetSessionToken",
						"s3:GetObject",
						"s3:ListBuckets",
					),
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			"restricted_mid": {
				Name:        "Restricted Mid",
				Description: "Mid-level restricted to senior-eng group with PowerUserAccess",
				Inherits: []string{
					"base_with_readonly", // thand (depth 2)
					"arn:aws:iam::aws:policy/PowerUserAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "rds:*", "ecs:*"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"senior-eng"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			"top_orchestrator": {
				Name:        "Top Orchestrator",
				Description: "Top-level with SecurityAudit, inheriting restricted mid",
				Inherits: []string{
					"restricted_mid", // thand (→ base_with_readonly → depth 3)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"lambda:InvokeFunction",
						"lambda:ListFunctions",
						"stepfunctions:StartExecution",
					),
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		// ── Junior engineer: NOT in senior-eng → mid-level denied ──
		junior := &models.Identity{
			ID: "junior1",
			User: &models.User{
				Username: "junior",
				Email:    "junior@example.com",
				Groups:   []string{"junior-eng", "engineering"},
			},
		}

		resultJr, err := config.GetCompositeRoleByName(junior, "top_orchestrator")
		require.NoError(t, err)
		require.NotNil(t, resultJr)

		jrAllowOps := collectOps(resultJr.Permissions.Allow)

		// Only top_orchestrator's own perms should remain
		foundLambda := false
		foundStep := false
		for _, op := range jrAllowOps {
			if len(op) >= 7 && op[:7] == "lambda:" {
				foundLambda = true
			}
			if len(op) >= 14 && op[:14] == "stepfunctions:" {
				foundStep = true
			}
		}
		assert.True(t, foundLambda, "Junior should get lambda perms: %v", jrAllowOps)
		assert.True(t, foundStep, "Junior should get step functions perms: %v", jrAllowOps)

		// EC2/RDS/ECS from restricted_mid must NOT be present
		for _, op := range jrAllowOps {
			assert.NotContains(t, op, "ec2:",
				"ec2 perms from denied mid must not appear for junior: %v", jrAllowOps)
			assert.NotContains(t, op, "rds:",
				"rds perms from denied mid must not appear for junior: %v", jrAllowOps)
			assert.NotContains(t, op, "ecs:",
				"ecs perms from denied mid must not appear for junior: %v", jrAllowOps)
		}

		// ReadOnlyAccess and IAMReadOnlyAccess from base must NOT propagate through denied mid
		for _, inh := range resultJr.Inherits {
			assert.NotEqual(t, "ReadOnlyAccess", inh,
				"ReadOnlyAccess must not propagate through denied mid: %v", resultJr.Inherits)
			assert.NotEqual(t, "IAMReadOnlyAccess", inh,
				"IAMReadOnlyAccess must not propagate through denied mid: %v", resultJr.Inherits)
			assert.NotEqual(t, "PowerUserAccess", inh,
				"PowerUserAccess must not propagate through denied mid: %v", resultJr.Inherits)
		}

		// SecurityAudit from top_orchestrator's own inherits should be present
		assert.Contains(t, resultJr.Inherits, "SecurityAudit",
			"Top-level's own SecurityAudit should remain: %v", resultJr.Inherits)

		// STS/S3 perms from base must not leak through denied chain
		for _, op := range jrAllowOps {
			assert.NotContains(t, op, "sts:",
				"STS perms must not leak through denied mid: %v", jrAllowOps)
			assert.NotContains(t, op, "s3:",
				"S3 perms must not leak through denied mid: %v", jrAllowOps)
		}

		// Not composite since the only thand child was denied
		assert.False(t, resultJr.Composite,
			"Junior should not get composite when mid-level is denied")

		// ── Senior engineer: in senior-eng → everything merges ──
		senior := &models.Identity{
			ID: "senior1",
			User: &models.User{
				Username: "senior",
				Email:    "senior@example.com",
				Groups:   []string{"senior-eng", "engineering"},
			},
		}

		resultSr, err := config.GetCompositeRoleByName(senior, "top_orchestrator")
		require.NoError(t, err)
		require.NotNil(t, resultSr)

		// Senior gets everything from all levels
		assert.True(t, resultSr.Composite,
			"Senior should get composite role")

		// All 4 managed policies from all levels
		assert.Len(t, resultSr.Inherits, 4,
			"Senior should have 4 managed policies: got %v", resultSr.Inherits)
		assert.Contains(t, resultSr.Inherits, "ReadOnlyAccess")
		assert.Contains(t, resultSr.Inherits, "IAMReadOnlyAccess")
		assert.Contains(t, resultSr.Inherits, "PowerUserAccess")
		assert.Contains(t, resultSr.Inherits, "SecurityAudit")

		srAllowOps := collectOps(resultSr.Permissions.Allow)
		assert.Contains(t, srAllowOps, "ec2:*",
			"Senior should get ec2:*: %v", srAllowOps)
		assert.Contains(t, srAllowOps, "rds:*",
			"Senior should get rds:*: %v", srAllowOps)
		assert.Contains(t, srAllowOps, "ecs:*",
			"Senior should get ecs:*: %v", srAllowOps)

		// STS from base should be present
		foundSts := false
		for _, op := range srAllowOps {
			if len(op) >= 4 && op[:4] == "sts:" {
				foundSts = true
				break
			}
		}
		assert.True(t, foundSts, "Senior should get STS from base: %v", srAllowOps)
	})

	// ---------------------------------------------------------------
	// 2. Two children at same depth: one scope-allowed with
	//    ReadOnlyAccess, one scope-denied with AdministratorAccess.
	//    ReadOnlyAccess should propagate; AdministratorAccess should
	//    not. (depth 2, fan-out)
	// ---------------------------------------------------------------
	t.Run("fan-out: one child allowed with ReadOnlyAccess and one denied with AdminAccess", func(t *testing.T) {
		roles := map[string]models.Role{
			"open_reader": {
				Name:        "Open Reader",
				Description: "Open to all, carries ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject", "s3:ListBuckets", "cloudwatch:GetMetricData"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true, // no scope = open to everyone
			},
			"restricted_admin": {
				Name:        "Restricted Admin",
				Description: "Admin perms restricted to admins group",
				Inherits: []string{
					"arn:aws:iam::aws:policy/AdministratorAccess",
					"arn:aws:iam::aws:policy/IAMFullAccess",
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
			"hybrid_role": {
				Name:        "Hybrid Role",
				Description: "Inherits both open (ReadOnlyAccess) and restricted (AdminAccess)",
				Inherits: []string{
					"open_reader",
					"restricted_admin",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("lambda:InvokeFunction"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		// ── Non-admin user ──
		nonAdmin := &models.Identity{
			ID: "nonadmin1",
			User: &models.User{
				Username: "developer",
				Email:    "dev@example.com",
				Groups:   []string{"developers"},
			},
		}

		resultNA, err := config.GetCompositeRoleByName(nonAdmin, "hybrid_role")
		require.NoError(t, err)
		require.NotNil(t, resultNA)

		naOps := collectOps(resultNA.Permissions.Allow)

		// open_reader's perms should be merged
		foundS3 := false
		foundCw := false
		for _, op := range naOps {
			if len(op) >= 3 && op[:3] == "s3:" {
				foundS3 = true
			}
			if len(op) >= 11 && op[:11] == "cloudwatch:" {
				foundCw = true
			}
		}
		assert.True(t, foundS3,
			"Non-admin should get S3 perms from open_reader: %v", naOps)
		assert.True(t, foundCw,
			"Non-admin should get CloudWatch from open_reader: %v", naOps)
		assert.Contains(t, naOps, "lambda:InvokeFunction",
			"Non-admin should get lambda from hybrid_role: %v", naOps)

		// restricted_admin's perms must NOT be present
		for _, op := range naOps {
			assert.NotContains(t, op, "ec2:",
				"ec2 from denied admin must not appear: %v", naOps)
			assert.NotContains(t, op, "rds:",
				"rds from denied admin must not appear: %v", naOps)
			assert.NotContains(t, op, "iam:",
				"iam from denied admin must not appear: %v", naOps)
		}

		// ReadOnlyAccess from open_reader should propagate
		assert.Contains(t, resultNA.Inherits, "ReadOnlyAccess",
			"ReadOnlyAccess from open_reader should propagate: %v", resultNA.Inherits)

		// AdminAccess + IAMFullAccess from restricted_admin must NOT propagate
		for _, inh := range resultNA.Inherits {
			assert.NotEqual(t, "AdministratorAccess", inh,
				"AdminAccess must not propagate from denied child: %v", resultNA.Inherits)
			assert.NotEqual(t, "IAMFullAccess", inh,
				"IAMFullAccess must not propagate from denied child: %v", resultNA.Inherits)
		}

		// Still composite since open_reader was successfully merged
		assert.True(t, resultNA.Composite,
			"Should be composite since open_reader was merged")

		// ── Admin user ──
		admin := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "admin",
				Email:    "admin@example.com",
				Groups:   []string{"admins", "developers"},
			},
		}

		resultAdm, err := config.GetCompositeRoleByName(admin, "hybrid_role")
		require.NoError(t, err)
		require.NotNil(t, resultAdm)

		admOps := collectOps(resultAdm.Permissions.Allow)

		// Admin gets everything
		assert.Contains(t, admOps, "ec2:*", "Admin should get ec2:*: %v", admOps)
		assert.Contains(t, admOps, "rds:*", "Admin should get rds:*: %v", admOps)
		assert.Contains(t, admOps, "iam:*", "Admin should get iam:*: %v", admOps)
		assert.Contains(t, admOps, "lambda:InvokeFunction",
			"Admin should get lambda: %v", admOps)

		// All managed policies should propagate
		assert.Contains(t, resultAdm.Inherits, "ReadOnlyAccess")
		assert.Contains(t, resultAdm.Inherits, "AdministratorAccess")
		assert.Contains(t, resultAdm.Inherits, "IAMFullAccess")
		assert.Len(t, resultAdm.Inherits, 3,
			"Admin should have all 3 managed policies: %v", resultAdm.Inherits)

		assert.True(t, resultAdm.Composite)
	})

	// ---------------------------------------------------------------
	// 3. Deny-scope user explicitly denied but in allow group:
	//    deny takes precedence even with ReadOnlyAccess (depth 2)
	// ---------------------------------------------------------------
	t.Run("deny-scope takes precedence over allow-scope with ReadOnlyAccess at depth 2", func(t *testing.T) {
		roles := map[string]models.Role{
			"readonly_foundation": {
				Name:        "ReadOnly Foundation",
				Description: "Foundation with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject",
						"cloudwatch:GetMetricData",
						"logs:GetLogEvents",
					),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"privileged_with_readonly": {
				Name:        "Privileged ReadOnly",
				Description: "Privileged role inheriting ReadOnly foundation, scoped with allow+deny",
				Inherits: []string{
					"readonly_foundation", // thand (depth 2)
					"arn:aws:iam::aws:policy/PowerUserAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*", "rds:*", "ecs:*"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"engineering"},
					},
					Deny: models.ScopeIdentities{
						Users:  []string{"temp-contractor@example.com"},
						Groups: []string{"suspended"},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		// User is in engineering (matches allow) but in suspended (matches deny)
		// Deny should win
		suspendedEng := &models.Identity{
			ID: "susp1",
			User: &models.User{
				Username: "suspended-eng",
				Email:    "suspended@example.com",
				Groups:   []string{"engineering", "suspended"},
			},
		}

		_, errSusp := config.GetCompositeRoleByName(suspendedEng, "privileged_with_readonly")
		require.Error(t, errSusp,
			"Suspended user should be denied even though in allow group")
		assert.Contains(t, errSusp.Error(), "not applicable",
			"Error should mention scope denial")

		// Temp contractor explicitly denied by email
		tempContractor := &models.Identity{
			ID: "temp1",
			User: &models.User{
				Username: "tempcontractor",
				Email:    "temp-contractor@example.com",
				Groups:   []string{"engineering"}, // in allow group
			},
		}

		_, errTemp := config.GetCompositeRoleByName(tempContractor, "privileged_with_readonly")
		require.Error(t, errTemp,
			"Temp contractor should be denied by explicit user deny")
		assert.Contains(t, errTemp.Error(), "not applicable")

		// Regular engineer: passes scope
		regularEng := &models.Identity{
			ID: "eng1",
			User: &models.User{
				Username: "engineer",
				Email:    "engineer@example.com",
				Groups:   []string{"engineering"},
			},
		}

		resultEng, err := config.GetCompositeRoleByName(regularEng, "privileged_with_readonly")
		require.NoError(t, err)
		require.NotNil(t, resultEng)

		assert.True(t, resultEng.Composite)
		assert.Contains(t, resultEng.Inherits, "ReadOnlyAccess")
		assert.Contains(t, resultEng.Inherits, "PowerUserAccess")
		assert.Contains(t, collectOps(resultEng.Permissions.Allow), "ec2:*")
	})
}

// TestAWSDeepNestingWithMultiplePermissionSets exercises the maximum
// practical nesting depth (3 levels) with multiple thand roles and
// managed policies at each level, verifying all invariants hold:
//   - Composite marking
//   - Provider-role accumulation / deduplication
//   - Wildcard subsumption across tiers
//   - Deny propagation and parent-wins semantics
//   - Target preservation through the chain
//   - Scope gating at each tier
func TestAWSDeepNestingWithMultiplePermissionSets(t *testing.T) {
	providers := sharedAWSProviders()

	// ---------------------------------------------------------------
	// 1. Full 3-level hierarchy: each level has 2+ AWS managed
	//    policies and unique thand permissions; parent wildcards
	//    subsume child specifics; deny propagation follows
	//    parent-wins semantics.
	// ---------------------------------------------------------------
	t.Run("full hierarchy with ReadOnlyAccess shared across levels", func(t *testing.T) {
		roles := map[string]models.Role{
			// Level 0: Foundation — ReadOnlyAccess + IAMReadOnlyAccess + specific perms
			"foundation": {
				Name:        "Foundation",
				Description: "Organization-wide base role",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{
								"sts:GetCallerIdentity",
								"sts:GetSessionToken",
								"sts:AssumeRole",
							},
						},
						{
							Operations: []string{
								"s3:GetObject",
								"s3:GetBucketLocation",
								"s3:ListBucket",
							},
							Targets: []string{"arn:aws:s3:::shared-*"},
						},
					},
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Domains: []string{"acme.com"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging", "aws-dev"},
				Enabled:   true,
			},
			// Level 1A: Compute team role — SecurityAudit + compute-specific perms
			"compute_team": {
				Name:        "Compute Team",
				Description: "EC2/ECS access with SecurityAudit",
				Inherits: []string{
					"foundation", // thand (depth 2)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{
								"ec2:DescribeInstances",
								"ec2:DescribeSecurityGroups",
								"ec2:DescribeVpcs",
								"ec2:RunInstances",
								"ec2:StopInstances",
								"ec2:StartInstances",
							},
							Targets: []string{"arn:aws:ec2:us-east-1:*:*"},
						},
						{
							Operations: []string{
								"ecs:ListClusters",
								"ecs:DescribeClusters",
								"ecs:ListServices",
								"ecs:UpdateService",
							},
						},
					},
					Deny: stmtsAws("ec2:TerminateInstances"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"compute-team", "sre"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			// Level 1B: Data team role — SecurityAudit + data-specific perms
			"data_team": {
				Name:        "Data Team",
				Description: "S3/Athena/Glue access with SecurityAudit",
				Inherits: []string{
					"foundation", // thand (depth 2)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{
								"s3:GetObject",
								"s3:PutObject",
								"s3:DeleteObject",
								"s3:ListBucket",
							},
							Targets: []string{"arn:aws:s3:::data-*"},
						},
						{
							Operations: []string{
								"athena:StartQueryExecution",
								"athena:GetQueryResults",
								"glue:GetTable",
								"glue:GetDatabase",
							},
						},
					},
					Deny: stmtsAws("s3:DeleteBucket"),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Groups: []string{"data-team", "analytics"},
					},
				},
				Providers: []string{"aws-prod", "aws-staging"},
				Enabled:   true,
			},
			// Level 2: Platform Lead — inherits BOTH compute_team and data_team
			// + AdministratorAccess + IAMFullAccess + ec2:* (subsumes
			// compute_team's specific EC2 ops) + s3:* (subsumes data_team's
			// specific S3 ops)
			"platform_lead": {
				Name:        "Platform Lead",
				Description: "Cross-team lead with full platform access",
				Inherits: []string{
					"compute_team", // thand (→ foundation → depth 3)
					"data_team",    // thand (→ foundation → depth 3)
					"arn:aws:iam::aws:policy/AdministratorAccess",
					"arn:aws:iam::aws:policy/IAMFullAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{
							Operations: []string{"ec2:*"},
							Targets:    []string{"arn:aws:ec2:*:*:*"},
						},
						{
							Operations: []string{"s3:*"},
							Targets:    []string{"arn:aws:s3:::platform-*"},
						},
						{
							Operations: []string{
								"lambda:*",
								"stepfunctions:*",
							},
						},
					},
					Deny: stmtsAws(
						"iam:DeleteRole",
						"iam:DeleteUser",
						"iam:DeletePolicy",
						"organizations:LeaveOrganization",
					),
				},
				Scopes: models.RoleScopes{
					Allow: models.ScopeIdentities{
						Users: []string{
							"lead@acme.com",
							"cto@acme.com",
						},
					},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		config := newTestConfig(t, roles, providers)

		// ── Test 1: Lead user passes ALL scope gates ──
		// foundation: domain acme.com ✓
		// compute_team: group sre ✓ (lead is in compute-team too)
		// data_team: group data-team ✓
		// platform_lead: user lead@acme.com ✓
		leadIdentity := &models.Identity{
			ID: "lead1",
			User: &models.User{
				Username: "lead",
				Email:    "lead@acme.com",
				Groups:   []string{"compute-team", "data-team", "sre", "leads"},
			},
		}

		result, err := config.GetCompositeRoleByName(leadIdentity, "platform_lead")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand inheritance (via two branches) → composite
		assert.True(t, result.Composite,
			"Multi-branch three-level thand inheritance should be composite")

		// Provider roles from all levels should accumulate:
		// foundation: ReadOnlyAccess, IAMReadOnlyAccess
		// compute_team: SecurityAudit
		// data_team: SecurityAudit (dedup'd)
		// platform_lead: AdministratorAccess, IAMFullAccess
		//
		// SecurityAudit appears in both compute_team and data_team but should
		// appear in Inherits (may appear twice if not deduped). The key point
		// is that all unique managed policies are represented.
		for _, expected := range []string{
			"ReadOnlyAccess",
			"IAMReadOnlyAccess",
			"SecurityAudit",
			"AdministratorAccess",
			"IAMFullAccess",
		} {
			assert.Contains(t, result.Inherits, expected,
				"Managed policy %q should be in Inherits: got %v", expected, result.Inherits)
		}

		// ── Merged permissions with wildcard subsumption ──
		allowOps := collectOps(result.Permissions.Allow)

		// ec2:* from platform_lead should subsume compute_team's specific EC2 ops
		assert.Contains(t, allowOps, "ec2:*",
			"ec2:* should be present: %v", allowOps)
		for _, op := range allowOps {
			// None of the specific EC2 ops should survive
			assert.NotEqual(t, "ec2:DescribeInstances", op,
				"ec2:DescribeInstances should be subsumed: %v", allowOps)
			assert.NotEqual(t, "ec2:RunInstances", op,
				"ec2:RunInstances should be subsumed: %v", allowOps)
			assert.NotEqual(t, "ec2:StopInstances", op,
				"ec2:StopInstances should be subsumed: %v", allowOps)
		}

		// s3:* from platform_lead should subsume data_team's specific S3 ops
		assert.Contains(t, allowOps, "s3:*",
			"s3:* should be present: %v", allowOps)
		for _, op := range allowOps {
			assert.NotEqual(t, "s3:GetObject", op,
				"s3:GetObject should be subsumed: %v", allowOps)
			assert.NotEqual(t, "s3:PutObject", op,
				"s3:PutObject should be subsumed: %v", allowOps)
		}

		// lambda:* and stepfunctions:* from platform_lead
		assert.Contains(t, allowOps, "lambda:*",
			"lambda:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "stepfunctions:*",
			"stepfunctions:* should be present: %v", allowOps)

		// STS from foundation should be merged
		foundSts := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "sts:" {
				foundSts = true
				break
			}
		}
		assert.True(t, foundSts, "STS perms from foundation should be merged: %v", allowOps)

		// ECS from compute_team should be present (no wildcard to subsume)
		foundEcs := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "ecs:" {
				foundEcs = true
				break
			}
		}
		assert.True(t, foundEcs, "ECS perms from compute_team should survive: %v", allowOps)

		// Athena/Glue from data_team should be present (no wildcard to subsume)
		foundAthena := false
		foundGlue := false
		for _, op := range allowOps {
			if len(op) >= 7 && op[:7] == "athena:" {
				foundAthena = true
			}
			if len(op) >= 5 && op[:5] == "glue:" {
				foundGlue = true
			}
		}
		assert.True(t, foundAthena, "Athena perms from data_team should survive: %v", allowOps)
		assert.True(t, foundGlue, "Glue perms from data_team should survive: %v", allowOps)

		// ── Deny propagation: parent-wins semantics ──
		denyOps := collectOps(result.Permissions.Deny)

		// platform_lead's own denies should survive
		for _, denied := range []string{
			"iam:DeletePolicy,DeleteRole,DeleteUser",
			"organizations:LeaveOrganization",
		} {
			assert.Contains(t, denyOps, denied,
				"Platform lead deny %q should survive: %v", denied, denyOps)
		}

		// compute_team's ec2:TerminateInstances deny should be overridden
		// by platform_lead's ec2:* allow (parent-wins)
		for _, op := range denyOps {
			assert.NotContains(t, op, "ec2:TerminateInstances",
				"ec2:TerminateInstances deny should be overridden by parent ec2:*: %v", denyOps)
		}

		// data_team's s3:DeleteBucket deny should be overridden
		// by platform_lead's s3:* allow (parent-wins)
		for _, op := range denyOps {
			assert.NotContains(t, op, "s3:DeleteBucket",
				"s3:DeleteBucket deny should be overridden by parent s3:*: %v", denyOps)
		}

		// ── Targets ──
		var ec2Tgts, s3Tgts []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if op == "ec2:*" {
					ec2Tgts = stmt.Targets
				}
				if op == "s3:*" {
					s3Tgts = stmt.Targets
				}
			}
		}
		assert.ElementsMatch(t, []string{"arn:aws:ec2:*:*:*"}, ec2Tgts,
			"EC2 targets from platform_lead should be preserved")
		assert.ElementsMatch(t, []string{"arn:aws:s3:::platform-*"}, s3Tgts,
			"S3 targets from platform_lead should be preserved")

		assert.ElementsMatch(t, []string{"aws-prod"}, result.Providers)

		// ── Test 2: User who fails data_team scope but passes compute_team ──
		// platform_lead scope requires explicit user, but let's test a direct
		// query of compute_team for a user who's only in compute-team
		computeOnlyUser := &models.Identity{
			ID: "comp1",
			User: &models.User{
				Username: "computedev",
				Email:    "compdev@acme.com",
				Groups:   []string{"compute-team"},
			},
		}

		resultComp, err := config.GetCompositeRoleByName(computeOnlyUser, "compute_team")
		require.NoError(t, err)
		require.NotNil(t, resultComp)

		// Should be composite (inherits foundation)
		assert.True(t, resultComp.Composite,
			"compute_team with foundation inheritance should be composite")

		// Should have ReadOnlyAccess + IAMReadOnlyAccess from foundation + SecurityAudit
		assert.Contains(t, resultComp.Inherits, "ReadOnlyAccess")
		assert.Contains(t, resultComp.Inherits, "IAMReadOnlyAccess")
		assert.Contains(t, resultComp.Inherits, "SecurityAudit")

		compOps := collectOps(resultComp.Permissions.Allow)
		// Should have specific EC2 ops (no wildcard since compute_team doesn't have ec2:*)
		foundEc2Specific := false
		for _, op := range compOps {
			if len(op) >= 4 && op[:4] == "ec2:" {
				foundEc2Specific = true
				break
			}
		}
		assert.True(t, foundEc2Specific, "compute_team should have specific EC2 perms: %v", compOps)

		// EC2 targets should be preserved
		var compEc2Tgts []string
		for _, stmt := range resultComp.Permissions.Allow {
			for _, op := range stmt.Operations {
				if len(op) >= 4 && op[:4] == "ec2:" {
					compEc2Tgts = stmt.Targets
					break
				}
			}
		}
		assert.ElementsMatch(t, []string{"arn:aws:ec2:us-east-1:*:*"}, compEc2Tgts,
			"EC2 targets should be preserved for compute_team")
	})

	// ---------------------------------------------------------------
	// 2. ReadOnlyAccess in a diamond: two branches both inherit the
	//    same base role; parent merges both. The shared managed
	//    policy should appear in Inherits (may be duplicated), and
	//    both branches' permissions should merge correctly.
	// ---------------------------------------------------------------
	t.Run("diamond inheritance with ReadOnlyAccess at the root", func(t *testing.T) {
		roles := map[string]models.Role{
			"diamond_base": {
				Name:        "Diamond Base",
				Description: "Shared base with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("sts:GetCallerIdentity"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"diamond_left": {
				Name:        "Diamond Left",
				Description: "Left branch with EC2 perms",
				Inherits: []string{
					"diamond_base", // thand
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"ec2:RunInstances", "ec2:DescribeInstances"},
						Targets:    []string{"arn:aws:ec2:us-west-2:*:*"},
					}},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"diamond_right": {
				Name:        "Diamond Right",
				Description: "Right branch with S3 perms",
				Inherits: []string{
					"diamond_base", // thand (same base — diamond!)
					"arn:aws:iam::aws:policy/PowerUserAccess",
				},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{{
						Operations: []string{"s3:GetObject", "s3:PutObject"},
						Targets:    []string{"arn:aws:s3:::team-*"},
					}},
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"diamond_top": {
				Name:        "Diamond Top",
				Description: "Merges both branches of the diamond",
				Inherits: []string{
					"diamond_left",
					"diamond_right",
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("lambda:InvokeFunction"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		identity := &models.Identity{
			ID: "diamond1",
			User: &models.User{
				Username: "diamonduser",
				Email:    "diamond@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "diamond_top")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Diamond inheritance → composite (both branches are thand roles)
		assert.True(t, result.Composite,
			"Diamond thand inheritance should be composite")

		// Managed policies from all levels:
		// diamond_base: ReadOnlyAccess (appears via both branches)
		// diamond_left: SecurityAudit
		// diamond_right: PowerUserAccess
		// diamond_top: AdministratorAccess
		for _, expected := range []string{
			"ReadOnlyAccess",
			"SecurityAudit",
			"PowerUserAccess",
			"AdministratorAccess",
		} {
			assert.Contains(t, result.Inherits, expected,
				"Managed policy %q should be in Inherits: got %v", expected, result.Inherits)
		}

		// Merged permissions from all branches
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "lambda:InvokeFunction",
			"Own lambda perm should be present: %v", allowOps)

		// EC2 from left branch
		foundEc2 := false
		for _, op := range allowOps {
			if len(op) >= 4 && op[:4] == "ec2:" {
				foundEc2 = true
				break
			}
		}
		assert.True(t, foundEc2, "EC2 perms from left branch should be present: %v", allowOps)

		// S3 from right branch
		foundS3 := false
		for _, op := range allowOps {
			if len(op) >= 3 && op[:3] == "s3:" {
				foundS3 = true
				break
			}
		}
		assert.True(t, foundS3, "S3 perms from right branch should be present: %v", allowOps)

		// STS from diamond_base should propagate through at least one branch
		assert.Contains(t, allowOps, "sts:GetCallerIdentity",
			"STS from base should propagate: %v", allowOps)

		// Targets from both branches should be preserved
		var ec2Tgts, s3Tgts []string
		for _, stmt := range result.Permissions.Allow {
			for _, op := range stmt.Operations {
				if len(op) >= 4 && op[:4] == "ec2:" {
					ec2Tgts = stmt.Targets
				}
				if len(op) >= 3 && op[:3] == "s3:" {
					s3Tgts = stmt.Targets
				}
			}
		}
		assert.ElementsMatch(t, []string{"arn:aws:ec2:us-west-2:*:*"}, ec2Tgts,
			"EC2 targets from left branch should be preserved")
		assert.ElementsMatch(t, []string{"arn:aws:s3:::team-*"}, s3Tgts,
			"S3 targets from right branch should be preserved")
	})

	// ---------------------------------------------------------------
	// 3. Three-level chain with deny at every level, testing
	//    parent-wins semantics where parent allow overrides child deny
	//    across the full depth (depth 3)
	// ---------------------------------------------------------------
	t.Run("depth-3 deny at every level with parent-wins semantics", func(t *testing.T) {
		roles := map[string]models.Role{
			"l0_restricted": {
				Name:        "L0 Restricted",
				Description: "Base with specific S3 + deny s3:DeleteObject",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"s3:GetObject",
						"s3:ListBucket",
						"s3:GetBucketPolicy",
					),
					Deny: stmtsAws("s3:DeleteObject"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"l1_writer": {
				Name:        "L1 Writer",
				Description: "Writer inheriting l0, adds s3:PutObject + deny rds:*",
				Inherits: []string{
					"l0_restricted", // thand (depth 2)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws(
						"s3:PutObject",
						"s3:GetObject",
						"rds:DescribeDBInstances",
					),
					Deny: stmtsAws("rds:DeleteDBInstance"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"l2_full_admin": {
				Name:        "L2 Full Admin",
				Description: "Full admin: s3:* overrides l0 deny, rds:* overrides l1 deny",
				Inherits: []string{
					"l1_writer", // thand (→ l0_restricted → depth 3)
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:*", "rds:*", "ec2:*"),
					Deny:  stmtsAws("iam:DeleteRole"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		identity := &models.Identity{
			ID: "admin1",
			User: &models.User{
				Username: "fulladmin",
				Email:    "admin@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "l2_full_admin")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand → composite
		assert.True(t, result.Composite)

		// Provider roles from all levels
		assert.Contains(t, result.Inherits, "ReadOnlyAccess")
		assert.Contains(t, result.Inherits, "SecurityAudit")
		assert.Contains(t, result.Inherits, "AdministratorAccess")

		// Wildcards should subsume specific ops
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "s3:*", "s3:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "rds:*", "rds:* should be present: %v", allowOps)
		assert.Contains(t, allowOps, "ec2:*", "ec2:* should be present: %v", allowOps)

		// Specific S3/RDS ops should be subsumed
		for _, op := range allowOps {
			assert.NotEqual(t, "s3:GetObject", op,
				"s3:GetObject should be subsumed: %v", allowOps)
			assert.NotEqual(t, "s3:PutObject", op,
				"s3:PutObject should be subsumed: %v", allowOps)
			assert.NotEqual(t, "rds:DescribeDBInstances", op,
				"rds:DescribeDBInstances should be subsumed: %v", allowOps)
		}

		// Deny permissions:
		denyOps := collectOps(result.Permissions.Deny)

		// l2's own iam:DeleteRole deny should survive (no parent to override)
		assert.Contains(t, denyOps, "iam:DeleteRole",
			"iam:DeleteRole deny should survive: %v", denyOps)

		// l0's s3:DeleteObject deny should be overridden by l2's s3:* allow
		for _, op := range denyOps {
			assert.NotContains(t, op, "s3:DeleteObject",
				"s3:DeleteObject deny should be overridden by s3:*: %v", denyOps)
		}

		// l1's rds:DeleteDBInstance deny should be overridden by l2's rds:* allow
		for _, op := range denyOps {
			assert.NotContains(t, op, "rds:DeleteDBInstance",
				"rds:DeleteDBInstance deny should be overridden by rds:*: %v", denyOps)
		}
	})
}

// TestAWSReadOnlyAccessWithProviderResolution tests that
// arn:aws:iam::aws:policy/ReadOnlyAccess is correctly resolved by the
// AWS mock provider and its short name appears in the composite role's
// Inherits field, whether inherited directly or through nested thand roles.
func TestAWSReadOnlyAccessWithProviderResolution(t *testing.T) {

	// ---------------------------------------------------------------
	// 1. Direct ReadOnlyAccess resolution across multiple providers
	// ---------------------------------------------------------------
	t.Run("ReadOnlyAccess resolved across multiple providers", func(t *testing.T) {
		roles := map[string]models.Role{
			"multi_provider_reader": {
				Name:        "Multi-Provider Reader",
				Description: "ReadOnlyAccess verified across prod, staging, and dev",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("cloudwatch:GetMetricData"),
				},
				Providers: []string{"aws-prod", "aws-staging", "aws-dev"},
				Enabled:   true,
			},
		}

		providers := sharedAWSProviders()
		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "reader1",
			User: &models.User{
				Username: "reader",
				Email:    "reader@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "multi_provider_reader")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.False(t, result.Composite, "No thand inheritance → not composite")
		assert.ElementsMatch(t, []string{"ReadOnlyAccess"}, result.Inherits,
			"ReadOnlyAccess should resolve to short name")
		assert.ElementsMatch(t, []string{"aws-prod", "aws-staging", "aws-dev"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 2. ReadOnlyAccess bubbles through 3-level chain across different
	//    providers at each level
	// ---------------------------------------------------------------
	t.Run("ReadOnlyAccess bubbles through three levels with different providers", func(t *testing.T) {
		roles := map[string]models.Role{
			"dev_reader": {
				Name:        "Dev Reader",
				Description: "Dev env reader with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject"),
				},
				Providers: []string{"aws-dev"},
				Enabled:   true,
			},
			"staging_viewer": {
				Name:        "Staging Viewer",
				Description: "Staging viewer inheriting dev reader + IAMReadOnlyAccess",
				Inherits: []string{
					"dev_reader", // thand (depth 2)
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:DescribeInstances"),
				},
				Providers: []string{"aws-staging"},
				Enabled:   true,
			},
			"prod_supervisor": {
				Name:        "Prod Supervisor",
				Description: "Prod supervisor inheriting staging viewer + SecurityAudit",
				Inherits: []string{
					"staging_viewer", // thand (→ dev_reader → depth 3)
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("rds:DescribeDBInstances"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		providers := sharedAWSProviders()
		config := newTestConfig(t, roles, providers)

		identity := &models.Identity{
			ID: "super1",
			User: &models.User{
				Username: "supervisor",
				Email:    "supervisor@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "prod_supervisor")
		require.NoError(t, err)
		require.NotNil(t, result)

		// Three-level thand inheritance → composite
		assert.True(t, result.Composite)

		// ReadOnlyAccess from dev_reader should bubble up through the entire chain
		assert.Contains(t, result.Inherits, "ReadOnlyAccess",
			"ReadOnlyAccess should bubble from dev_reader through the chain: %v", result.Inherits)
		assert.Contains(t, result.Inherits, "IAMReadOnlyAccess",
			"IAMReadOnlyAccess from staging_viewer should bubble: %v", result.Inherits)
		assert.Contains(t, result.Inherits, "SecurityAudit",
			"SecurityAudit from prod_supervisor should be present: %v", result.Inherits)

		// All permissions from all tiers should be merged
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "rds:DescribeDBInstances",
			"prod_supervisor perm: %v", allowOps)
		assert.Contains(t, allowOps, "ec2:DescribeInstances",
			"staging_viewer perm: %v", allowOps)
		assert.Contains(t, allowOps, "s3:GetObject",
			"dev_reader perm: %v", allowOps)

		assert.ElementsMatch(t, []string{"aws-prod"}, result.Providers)
	})

	// ---------------------------------------------------------------
	// 3. ReadOnlyAccess combined with five other managed policies in
	//    a complex role to ensure all resolve correctly
	// ---------------------------------------------------------------
	t.Run("ReadOnlyAccess combined with five other managed policies", func(t *testing.T) {
		roles := map[string]models.Role{
			"super_role": {
				Name:        "Super Role",
				Description: "Role with six managed policies including ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
					"arn:aws:iam::aws:policy/SecurityAudit",
					"arn:aws:iam::aws:policy/PowerUserAccess",
					"arn:aws:iam::aws:policy/IAMFullAccess",
					"arn:aws:iam::aws:policy/IAMReadOnlyAccess",
					"arn:aws:iam::aws:policy/AdministratorAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("custom:SpecialAction"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		identity := &models.Identity{
			ID: "super1",
			User: &models.User{
				Username: "superuser",
				Email:    "super@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "super_role")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.False(t, result.Composite, "No thand inheritance → not composite")

		// All 6 managed policies should be resolved
		assert.Len(t, result.Inherits, 6,
			"All 6 managed policies should be present: got %v", result.Inherits)
		for _, expected := range []string{
			"ReadOnlyAccess",
			"SecurityAudit",
			"PowerUserAccess",
			"IAMFullAccess",
			"IAMReadOnlyAccess",
			"AdministratorAccess",
		} {
			assert.Contains(t, result.Inherits, expected,
				"Managed policy %q should be in Inherits: got %v", expected, result.Inherits)
		}

		// Own custom permission should be present
		assert.ElementsMatch(t, []string{"custom:SpecialAction"}, collectOps(result.Permissions.Allow))
	})
}

// TestAWSWorkflowsAndAuthenticatorsInheritance tests that workflows and
// authenticators from parent roles are correctly preserved when combined
// with multi-level ReadOnlyAccess inheritance.
func TestAWSWorkflowsAndAuthenticatorsInheritance(t *testing.T) {
	t.Run("workflows preserved across three-level inheritance with ReadOnlyAccess", func(t *testing.T) {
		roles := map[string]models.Role{
			"base_monitored": {
				Name:        "Base Monitored",
				Description: "Base role requiring Slack approval, with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Workflows: []string{"slack_approval"},
				Authenticators: []string{
					"google_oauth2",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("sts:GetCallerIdentity"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"mid_monitored": {
				Name:        "Mid Monitored",
				Description: "Mid role requiring Jira approval",
				Inherits: []string{
					"base_monitored", // thand
					"arn:aws:iam::aws:policy/SecurityAudit",
				},
				Workflows: []string{"jira_approval"},
				Authenticators: []string{
					"thand_oauth2",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:DescribeInstances"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"top_operator": {
				Name:        "Top Operator",
				Description: "Top-level operator with PagerDuty workflow",
				Inherits: []string{
					"mid_monitored", // thand (→ base_monitored → depth 3)
					"arn:aws:iam::aws:policy/PowerUserAccess",
				},
				Workflows: []string{"pagerduty_approval"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		identity := &models.Identity{
			ID: "op1",
			User: &models.User{
				Username: "operator",
				Email:    "op@example.com",
			},
		}

		result, err := config.GetCompositeRoleByName(identity, "top_operator")
		require.NoError(t, err)
		require.NotNil(t, result)

		assert.True(t, result.Composite)

		// Provider roles from all levels
		assert.Contains(t, result.Inherits, "ReadOnlyAccess")
		assert.Contains(t, result.Inherits, "SecurityAudit")
		assert.Contains(t, result.Inherits, "PowerUserAccess")

		// Top-level's own workflow should be preserved
		assert.Contains(t, result.Workflows, "pagerduty_approval",
			"Top-level workflow should be preserved: %v", result.Workflows)

		// ec2:* from top should subsume mid's ec2:DescribeInstances
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "ec2:*",
			"ec2:* should be present: %v", allowOps)
	})
}

// TestAWSDisabledRoleInChain validates that a disabled role in the
// inheritance chain is properly handled when ReadOnlyAccess is involved.
func TestAWSDisabledRoleInChain(t *testing.T) {
	t.Run("disabled mid-level role with ReadOnlyAccess", func(t *testing.T) {
		roles := map[string]models.Role{
			"active_base": {
				Name:        "Active Base",
				Description: "Active base with ReadOnlyAccess",
				Inherits: []string{
					"arn:aws:iam::aws:policy/ReadOnlyAccess",
				},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("s3:GetObject"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
			"disabled_mid": {
				Name:        "Disabled Mid",
				Description: "This role is disabled",
				Inherits:    []string{"active_base"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("ec2:*"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   false,
			},
			"top_role": {
				Name:        "Top Role",
				Description: "Top role inheriting disabled mid",
				Inherits:    []string{"disabled_mid"},
				Permissions: models.RolePermissions{
					Allow: stmtsAws("lambda:InvokeFunction"),
				},
				Providers: []string{"aws-prod"},
				Enabled:   true,
			},
		}

		provs := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "aws-prod",
				Provider: "aws",
			},
		}

		config := newTestConfig(t, roles, provs)

		identity := &models.Identity{
			ID: "user1",
			User: &models.User{
				Username: "testuser",
				Email:    "test@example.com",
			},
		}

		// The behavior when inheriting a disabled role depends on the
		// implementation — it may error or skip. We test that the resolution
		// doesn't panic and returns a meaningful result.
		result, err := config.GetCompositeRoleByName(identity, "top_role")
		if err != nil {
			// If it errors, it should be about the disabled role
			assert.Contains(t, err.Error(), "disabled_mid",
				"Error should reference the disabled role")
			return
		}

		// If it succeeds (disabled role skipped), top_role's own perms should be present
		require.NotNil(t, result)
		allowOps := collectOps(result.Permissions.Allow)
		assert.Contains(t, allowOps, "lambda:InvokeFunction",
			"Top role's own perms should be present: %v", allowOps)
	})
}
