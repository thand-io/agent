package models_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// TestYAML_BackwardsCompatibility_GroupsAndResourcesToPermissions tests that the old format
// with `groups` and `resources` fields at the role level are properly migrated
// to `permissions` statements with targets.
func TestYAML_BackwardsCompatibility_GroupsAndResourcesToPermissions(t *testing.T) {
	tests := []struct {
		name                   string
		yaml                   string
		expectedAllowOps       []string
		expectedAllowTargets   []string
		expectedDenyOps        []string
		expectedDenyTargets    []string
		expectedStatementCount int
	}{
		{
			name: "old format - groups and resources migrate to permissions targets",
			yaml: `
version: "1.0"
roles:
  test_role:
    name: Test Role
    description: A test role with old format groups and resources
    permissions:
      allow:
        - ec2:*
        - s3:GetObject
    groups:
      allow:
        - oidc:admin
        - oidc:developer
      deny:
        - oidc:guest
    resources:
      allow:
        - "aws:*"
        - "arn:aws:s3:::my-bucket/*"
      deny:
        - "arn:aws:s3:::restricted-bucket/*"
    enabled: true
`,
			expectedAllowOps:     []string{"ec2:*", "s3:GetObject"},
			expectedAllowTargets: []string{"oidc:admin", "oidc:developer", "aws:*", "arn:aws:s3:::my-bucket/*"},
			expectedDenyOps:      []string{},
			expectedDenyTargets:  []string{"oidc:guest", "arn:aws:s3:::restricted-bucket/*"},
		},
		{
			name: "old format - only resources, no groups",
			yaml: `
version: "1.0"
roles:
  gcp_role:
    name: GCP Role
    description: A role with only resources
    permissions:
      allow:
        - compute.instances.*
        - storage.buckets.*
    resources:
      allow:
        - "gcp:*"
    enabled: true
`,
			expectedAllowOps:     []string{"compute.instances.*", "storage.buckets.*"},
			expectedAllowTargets: []string{"gcp:*"},
			expectedDenyOps:      nil,
			expectedDenyTargets:  nil,
		},
		{
			name: "old format - only groups, no resources",
			yaml: `
version: "1.0"
roles:
  group_only_role:
    name: Group Only Role
    description: A role with only groups
    permissions:
      allow:
        - k8s:pods:get,list,watch
    groups:
      allow:
        - oidc:eng
        - oidc:platform
    enabled: true
`,
			expectedAllowOps:     []string{"k8s:pods:get,list,watch"},
			expectedAllowTargets: []string{"oidc:eng", "oidc:platform"},
			expectedDenyOps:      nil,
			expectedDenyTargets:  nil,
		},
		{
			name: "old format - groups and resources with no permissions",
			yaml: `
version: "1.0"
roles:
  no_perms_role:
    name: No Perms Role
    description: A role with groups and resources but no permissions defined
    groups:
      allow:
        - oidc:admin
    resources:
      allow:
        - "namespace:production"
    enabled: true
`,
			expectedAllowOps:     []string{},
			expectedAllowTargets: []string{"oidc:admin", "namespace:production"},
			expectedDenyOps:      nil,
			expectedDenyTargets:  nil,
		},
		{
			name: "new format - permissions with statement objects",
			yaml: `
version: "1.0"
roles:
  new_format_role:
    name: New Format Role
    description: A role using the new statement format
    permissions:
      allow:
        - operations:
            - ec2:*
            - s3:*
          targets:
            - "arn:aws:*"
      deny:
        - operations:
            - s3:DeleteObject
          targets:
            - "arn:aws:s3:::critical-bucket/*"
    enabled: true
`,
			expectedAllowOps:     []string{"ec2:*", "s3:*"},
			expectedAllowTargets: []string{"arn:aws:*"},
			expectedDenyOps:      []string{"s3:DeleteObject"},
			expectedDenyTargets:  []string{"arn:aws:s3:::critical-bucket/*"},
		},
		{
			name: "mixed format - old permissions strings with new groups/resources",
			yaml: `
version: "1.0"
roles:
  mixed_role:
    name: Mixed Role
    description: Uses string permissions but has groups/resources
    permissions:
      allow:
        - ec2:DescribeInstances
        - s3:ListBuckets
      deny:
        - ec2:TerminateInstances
    groups:
      allow:
        - oidc:readonly
    resources:
      allow:
        - "aws:us-east-1:*"
    enabled: true
`,
			expectedAllowOps:     []string{"ec2:DescribeInstances", "s3:ListBuckets"},
			expectedAllowTargets: []string{"oidc:readonly", "aws:us-east-1:*"},
			expectedDenyOps:      []string{"ec2:TerminateInstances"},
			expectedDenyTargets:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse YAML using the common parser (converts YAML to JSON and uses UnmarshalJSON)
			result, err := common.ReadDataToInterface([]byte(tt.yaml), models.RoleDefinitions{})
			require.NoError(t, err, "failed to parse YAML")
			require.NotNil(t, result, "result should not be nil")
			require.NotEmpty(t, result.Roles, "should have at least one role")

			// Get the first role
			var role models.Role
			for _, r := range result.Roles {
				role = r
				break
			}

			// Collect all operations and targets from allow statements
			var actualAllowOps, actualAllowTargets []string
			for _, stmt := range role.Permissions.Allow {
				actualAllowOps = append(actualAllowOps, stmt.Operations...)
				actualAllowTargets = append(actualAllowTargets, stmt.Targets...)
			}

			// Collect all operations and targets from deny statements
			var actualDenyOps, actualDenyTargets []string
			for _, stmt := range role.Permissions.Deny {
				actualDenyOps = append(actualDenyOps, stmt.Operations...)
				actualDenyTargets = append(actualDenyTargets, stmt.Targets...)
			}

			// Validate allow operations
			assert.ElementsMatch(t, tt.expectedAllowOps, actualAllowOps,
				"allow operations mismatch")

			// Validate allow targets (groups and resources should be migrated here)
			assert.ElementsMatch(t, tt.expectedAllowTargets, actualAllowTargets,
				"allow targets mismatch - groups and resources should be migrated to permissions targets")

			// Validate deny operations
			if tt.expectedDenyOps != nil {
				assert.ElementsMatch(t, tt.expectedDenyOps, actualDenyOps,
					"deny operations mismatch")
			}

			// Validate deny targets (groups.deny and resources.deny should be migrated here)
			if tt.expectedDenyTargets != nil {
				assert.ElementsMatch(t, tt.expectedDenyTargets, actualDenyTargets,
					"deny targets mismatch - groups.deny and resources.deny should be migrated to permissions targets")
			}
		})
	}
}

// TestYAML_BackwardsCompatibility_ScopesToAllowDeny tests that the old scopes format
// (users/groups/domains at root) are properly migrated to the new allow/deny structure.
func TestYAML_BackwardsCompatibility_ScopesToAllowDeny(t *testing.T) {
	tests := []struct {
		name                 string
		yaml                 string
		expectedAllowUsers   []string
		expectedAllowGroups  []string
		expectedAllowDomains []string
		expectedDenyUsers    []string
		expectedDenyGroups   []string
		expectedDenyDomains  []string
	}{
		{
			name: "old scopes format - users/groups/domains at root level",
			yaml: `
version: "1.0"
roles:
  old_scopes_role:
    name: Old Scopes Role
    description: Uses old scopes format with fields at root level
    permissions:
      allow:
        - ec2:*
    scopes:
      users:
        - alice@example.com
        - bob@example.com
      groups:
        - oidc:admin
        - oidc:developer
      domains:
        - example.com
        - company.org
    enabled: true
`,
			expectedAllowUsers:   []string{"alice@example.com", "bob@example.com"},
			expectedAllowGroups:  []string{"oidc:admin", "oidc:developer"},
			expectedAllowDomains: []string{"example.com", "company.org"},
			expectedDenyUsers:    nil,
			expectedDenyGroups:   nil,
			expectedDenyDomains:  nil,
		},
		{
			name: "old scopes format - only groups at root",
			yaml: `
version: "1.0"
roles:
  groups_only_scopes:
    name: Groups Only Scopes
    description: Uses old scopes format with only groups
    permissions:
      allow:
        - s3:*
    scopes:
      groups:
        - oidc:user
        - oidc:eng
    enabled: true
`,
			expectedAllowUsers:   nil,
			expectedAllowGroups:  []string{"oidc:user", "oidc:eng"},
			expectedAllowDomains: nil,
			expectedDenyUsers:    nil,
			expectedDenyGroups:   nil,
			expectedDenyDomains:  nil,
		},
		{
			name: "new scopes format - allow/deny structure",
			yaml: `
version: "1.0"
roles:
  new_scopes_role:
    name: New Scopes Role
    description: Uses new scopes format with allow/deny
    permissions:
      allow:
        - ec2:*
    scopes:
      allow:
        users:
          - alice@example.com
        groups:
          - oidc:admin
        domains:
          - example.com
      deny:
        users:
          - mallory@example.com
        groups:
          - oidc:guest
        domains:
          - untrusted.com
    enabled: true
`,
			expectedAllowUsers:   []string{"alice@example.com"},
			expectedAllowGroups:  []string{"oidc:admin"},
			expectedAllowDomains: []string{"example.com"},
			expectedDenyUsers:    []string{"mallory@example.com"},
			expectedDenyGroups:   []string{"oidc:guest"},
			expectedDenyDomains:  []string{"untrusted.com"},
		},
		{
			name: "new scopes format - only allow block",
			yaml: `
version: "1.0"
roles:
  allow_only_scopes:
    name: Allow Only Scopes
    description: Uses new scopes format with only allow block
    permissions:
      allow:
        - compute.instances.*
    scopes:
      allow:
        users:
          - dev@company.com
        groups:
          - oidc:developers
    enabled: true
`,
			expectedAllowUsers:   []string{"dev@company.com"},
			expectedAllowGroups:  []string{"oidc:developers"},
			expectedAllowDomains: nil,
			expectedDenyUsers:    nil,
			expectedDenyGroups:   nil,
			expectedDenyDomains:  nil,
		},
		{
			name: "empty scopes",
			yaml: `
version: "1.0"
roles:
  no_scopes_role:
    name: No Scopes Role
    description: No scopes defined
    permissions:
      allow:
        - ec2:*
    enabled: true
`,
			expectedAllowUsers:   nil,
			expectedAllowGroups:  nil,
			expectedAllowDomains: nil,
			expectedDenyUsers:    nil,
			expectedDenyGroups:   nil,
			expectedDenyDomains:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Parse YAML using the common parser
			result, err := common.ReadDataToInterface([]byte(tt.yaml), models.RoleDefinitions{})
			require.NoError(t, err, "failed to parse YAML")
			require.NotNil(t, result, "result should not be nil")
			require.NotEmpty(t, result.Roles, "should have at least one role")

			// Get the first role
			var role models.Role
			for _, r := range result.Roles {
				role = r
				break
			}

			// Validate scopes allow
			if tt.expectedAllowUsers != nil {
				assert.ElementsMatch(t, tt.expectedAllowUsers, role.Scopes.Allow.Users,
					"allow users mismatch")
			} else {
				assert.Empty(t, role.Scopes.Allow.Users, "expected no allow users")
			}

			if tt.expectedAllowGroups != nil {
				assert.ElementsMatch(t, tt.expectedAllowGroups, role.Scopes.Allow.Groups,
					"allow groups mismatch - old format should migrate to allow.groups")
			} else {
				assert.Empty(t, role.Scopes.Allow.Groups, "expected no allow groups")
			}

			if tt.expectedAllowDomains != nil {
				assert.ElementsMatch(t, tt.expectedAllowDomains, role.Scopes.Allow.Domains,
					"allow domains mismatch")
			} else {
				assert.Empty(t, role.Scopes.Allow.Domains, "expected no allow domains")
			}

			// Validate scopes deny
			if tt.expectedDenyUsers != nil {
				assert.ElementsMatch(t, tt.expectedDenyUsers, role.Scopes.Deny.Users,
					"deny users mismatch")
			} else {
				assert.Empty(t, role.Scopes.Deny.Users, "expected no deny users")
			}

			if tt.expectedDenyGroups != nil {
				assert.ElementsMatch(t, tt.expectedDenyGroups, role.Scopes.Deny.Groups,
					"deny groups mismatch")
			} else {
				assert.Empty(t, role.Scopes.Deny.Groups, "expected no deny groups")
			}

			if tt.expectedDenyDomains != nil {
				assert.ElementsMatch(t, tt.expectedDenyDomains, role.Scopes.Deny.Domains,
					"deny domains mismatch")
			} else {
				assert.Empty(t, role.Scopes.Deny.Domains, "expected no deny domains")
			}
		})
	}
}

// TestYAML_BackwardsCompatibility_RealWorldExamples tests parsing of YAML configurations
// similar to those found in config/roles/*.yaml
func TestYAML_BackwardsCompatibility_RealWorldExamples(t *testing.T) {
	t.Run("AWS admin role with old format", func(t *testing.T) {
		yaml := `
version: "1.0"
roles:
  aws_admin:
    name: Admin
    description: Full access to all resources and capabilities.
    authenticators:
      - google_oauth2
      - thand_oauth2
    workflows: 
      - slack_approval
    inherits:
      - aws_user
      - arn:aws:iam::aws:policy/AdministratorAccess
    permissions:
      allow:
        - ec2:*
        - s3:*
        - rds:*
    resources:
      allow:
        - "aws:*"
    scopes:
      groups:
        - oidc:user
        - oidc:eng
      users:
        - hugh@thand.io
    providers:
      - aws-prod
      - aws-dev
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)
		require.NotNil(t, result)

		role := result.Roles["aws_admin"]
		assert.Equal(t, "Admin", role.Name)
		assert.Equal(t, "Full access to all resources and capabilities.", role.Description)

		// Check permissions - old resources should be migrated to targets
		require.NotEmpty(t, role.Permissions.Allow)
		var allOps, allTargets []string
		for _, stmt := range role.Permissions.Allow {
			allOps = append(allOps, stmt.Operations...)
			allTargets = append(allTargets, stmt.Targets...)
		}
		assert.ElementsMatch(t, []string{"ec2:*", "s3:*", "rds:*"}, allOps)
		assert.Contains(t, allTargets, "aws:*")

		// Check scopes - old format should be migrated to allow
		assert.ElementsMatch(t, []string{"oidc:user", "oidc:eng"}, role.Scopes.Allow.Groups)
		assert.Contains(t, role.Scopes.Allow.Users, "hugh@thand.io")

		// Check other fields preserved
		assert.ElementsMatch(t, []string{"google_oauth2", "thand_oauth2"}, role.Authenticators)
		assert.Contains(t, role.Workflows, "slack_approval")
		assert.Contains(t, role.Inherits, "aws_user")
		assert.ElementsMatch(t, []string{"aws-prod", "aws-dev"}, role.Providers)
		assert.True(t, role.Enabled)
	})

	t.Run("GCP role with resources", func(t *testing.T) {
		yaml := `
version: "1.0"
roles:
  gcp_admin:
    name: Gcp Admin
    description: Full access to all resources and capabilities.
    workflows: 
      - slack_approval
    inherits:
      - gcp_user
      - roles/owner
    permissions:
      allow:
        - compute.instances.*
        - storage.buckets.*
        - iam.serviceAccounts.*
    resources:
      allow:
        - "gcp:*"
    providers:
      - gcp-prod
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)

		role := result.Roles["gcp_admin"]
		assert.Equal(t, "Gcp Admin", role.Name)

		// Resources should be migrated to permission targets
		var allTargets []string
		for _, stmt := range role.Permissions.Allow {
			allTargets = append(allTargets, stmt.Targets...)
		}
		assert.Contains(t, allTargets, "gcp:*")
	})

	t.Run("Kubernetes role with namespace resources", func(t *testing.T) {
		yaml := `
roles:
  dev-pod-reader:
    name: "Dev Pod Reader"
    description: Read pods in development namespace
    authenticators:
      - google_oauth2
      - thand_oauth2
    workflows: 
      - slack_approval
    providers: 
      - kubernetes-dev
      - kubernetes-prod
    resources:
      allow:
        - "namespace:development"
    permissions:
      allow:
        - "k8s:pods:get,list,watch"
        - "k8s:services:get,list"
        - "k8s:events:get,list"
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)

		role := result.Roles["dev-pod-reader"]
		assert.Equal(t, "Dev Pod Reader", role.Name)

		// Resources should be migrated to permission targets
		var allTargets []string
		for _, stmt := range role.Permissions.Allow {
			allTargets = append(allTargets, stmt.Targets...)
		}
		assert.Contains(t, allTargets, "namespace:development")
	})

	t.Run("Multiple roles in single file", func(t *testing.T) {
		yaml := `
version: "1.0"
roles:
  role_a:
    name: Role A
    description: First role
    permissions:
      allow:
        - action:read
    resources:
      allow:
        - "resource:a"
    scopes:
      groups:
        - group:a
    enabled: true
  role_b:
    name: Role B
    description: Second role
    permissions:
      allow:
        - action:write
    resources:
      allow:
        - "resource:b"
    scopes:
      users:
        - user@b.com
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)
		require.Len(t, result.Roles, 2)

		roleA := result.Roles["role_a"]
		roleB := result.Roles["role_b"]

		// Check role_a
		var targetsA []string
		for _, stmt := range roleA.Permissions.Allow {
			targetsA = append(targetsA, stmt.Targets...)
		}
		assert.Contains(t, targetsA, "resource:a")
		assert.Contains(t, roleA.Scopes.Allow.Groups, "group:a")

		// Check role_b
		var targetsB []string
		for _, stmt := range roleB.Permissions.Allow {
			targetsB = append(targetsB, stmt.Targets...)
		}
		assert.Contains(t, targetsB, "resource:b")
		assert.Contains(t, roleB.Scopes.Allow.Users, "user@b.com")
	})
}

// TestYAML_BackwardsCompatibility_PermissionStatementFormats tests different formats
// of permission statements in YAML
func TestYAML_BackwardsCompatibility_PermissionStatementFormats(t *testing.T) {
	t.Run("string format permissions", func(t *testing.T) {
		yaml := `
version: "1.0"
roles:
  string_perms:
    name: String Perms
    description: Permissions as strings
    permissions:
      allow:
        - ec2:DescribeInstances
        - s3:ListBuckets
        - rds:DescribeDBInstances
      deny:
        - ec2:TerminateInstances
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)

		role := result.Roles["string_perms"]

		// Each string should become a statement with single operation
		var allowOps []string
		for _, stmt := range role.Permissions.Allow {
			allowOps = append(allowOps, stmt.Operations...)
		}
		assert.ElementsMatch(t, []string{"ec2:DescribeInstances", "s3:ListBuckets", "rds:DescribeDBInstances"}, allowOps)

		var denyOps []string
		for _, stmt := range role.Permissions.Deny {
			denyOps = append(denyOps, stmt.Operations...)
		}
		assert.ElementsMatch(t, []string{"ec2:TerminateInstances"}, denyOps)
	})

	t.Run("statement object format permissions", func(t *testing.T) {
		yaml := `
version: "1.0"
roles:
  object_perms:
    name: Object Perms
    description: Permissions as statement objects
    permissions:
      allow:
        - operations:
            - ec2:*
          targets:
            - "arn:aws:ec2:*:*:instance/*"
        - operations:
            - s3:GetObject
            - s3:PutObject
          targets:
            - "arn:aws:s3:::my-bucket/*"
          conditions:
            IpAddress:
              "aws:SourceIp": "10.0.0.0/8"
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)

		role := result.Roles["object_perms"]
		require.Len(t, role.Permissions.Allow, 2)

		// First statement
		assert.ElementsMatch(t, []string{"ec2:*"}, role.Permissions.Allow[0].Operations)
		assert.ElementsMatch(t, []string{"arn:aws:ec2:*:*:instance/*"}, role.Permissions.Allow[0].Targets)

		// Second statement with conditions
		assert.ElementsMatch(t, []string{"s3:GetObject", "s3:PutObject"}, role.Permissions.Allow[1].Operations)
		assert.ElementsMatch(t, []string{"arn:aws:s3:::my-bucket/*"}, role.Permissions.Allow[1].Targets)
		assert.NotNil(t, role.Permissions.Allow[1].Conditions)
	})

	t.Run("mixed format permissions", func(t *testing.T) {
		yaml := `
version: "1.0"
roles:
  mixed_perms:
    name: Mixed Perms
    description: Mix of string and object permissions
    permissions:
      allow:
        - ec2:DescribeInstances
        - operations:
            - s3:*
          targets:
            - "arn:aws:s3:::*"
        - rds:DescribeDBInstances
    enabled: true
`
		result, err := common.ReadDataToInterface([]byte(yaml), models.RoleDefinitions{})
		require.NoError(t, err)

		role := result.Roles["mixed_perms"]
		require.Len(t, role.Permissions.Allow, 3)

		var allOps []string
		for _, stmt := range role.Permissions.Allow {
			allOps = append(allOps, stmt.Operations...)
		}
		assert.Contains(t, allOps, "ec2:DescribeInstances")
		assert.Contains(t, allOps, "s3:*")
		assert.Contains(t, allOps, "rds:DescribeDBInstances")
	})
}

// TestYAML_BackwardsCompatibility_AllJsonFile tests parsing the actual config/roles/all.json file
func TestYAML_BackwardsCompatibility_AllJsonFile(t *testing.T) {
	// Find the project root by looking for go.mod
	cwd, err := os.Getwd()
	require.NoError(t, err)

	// Walk up to find the project root
	projectRoot := cwd
	for {
		if _, err := os.Stat(filepath.Join(projectRoot, "go.mod")); err == nil {
			break
		}
		parent := filepath.Dir(projectRoot)
		if parent == projectRoot {
			t.Skip("Could not find project root with go.mod")
		}
		projectRoot = parent
	}

	jsonPath := filepath.Join(projectRoot, "config", "roles", "all.json")
	data, err := os.ReadFile(jsonPath)
	if os.IsNotExist(err) {
		t.Skipf("config/roles/all.json not found at %s", jsonPath)
	}
	require.NoError(t, err, "failed to read all.json")

	result, err := common.ReadDataToInterface(data, models.RoleDefinitions{})
	require.NoError(t, err, "failed to parse all.json")
	require.NotNil(t, result, "result should not be nil")
	require.NotEmpty(t, result.Roles, "should have roles defined")

	t.Logf("Parsed %d roles from all.json", len(result.Roles))

	// Validate each role
	for key, role := range result.Roles {
		t.Run(key, func(t *testing.T) {
			// Role should have a name
			assert.NotEmpty(t, role.Name, "role %s should have a name", key)

			// Check that old format fields are properly migrated
			// Resources and Groups should end up in Permissions.Allow[].Targets
			// (we can't directly check the old fields since they're not in the struct anymore)

			// Validate permissions structure
			for i, stmt := range role.Permissions.Allow {
				t.Logf("  Allow[%d]: ops=%v, targets=%v", i, stmt.Operations, stmt.Targets)
			}
			for i, stmt := range role.Permissions.Deny {
				t.Logf("  Deny[%d]: ops=%v, targets=%v", i, stmt.Operations, stmt.Targets)
			}

			// Validate scopes - old format should be in Allow
			if !role.Scopes.IsEmpty() {
				t.Logf("  Scopes.Allow: users=%v, groups=%v, domains=%v",
					role.Scopes.Allow.Users, role.Scopes.Allow.Groups, role.Scopes.Allow.Domains)
				t.Logf("  Scopes.Deny: users=%v, groups=%v, domains=%v",
					role.Scopes.Deny.Users, role.Scopes.Deny.Groups, role.Scopes.Deny.Domains)
			}
		})
	}
}
