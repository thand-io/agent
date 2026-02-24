package models

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makePerms builds a []SearchResult[ProviderPermission] from a list of permission names.
func makePerms(names ...string) []SearchResult[ProviderPermission] {
	results := make([]SearchResult[ProviderPermission], 0, len(names))
	for _, n := range names {
		results = append(results, SearchResult[ProviderPermission]{
			Result: ProviderPermission{Name: n},
		})
	}
	return results
}

// stmtOps wraps operations into a single-statement RoleStatements.
func stmtOps(ops ...string) RoleStatements {
	return RoleStatements{{Operations: ops}}
}

// ops collects all operations from validated statements into a flat sorted slice.
func ops(stmts RoleStatements) []string {
	var out []string
	for _, s := range stmts {
		out = append(out, s.Operations...)
	}
	sort.Strings(out)
	return out
}

// ---------------------------------------------------------------------------
// expandPermissionsWildcard
// ---------------------------------------------------------------------------

func TestExpandPermissionsWildcard(t *testing.T) {
	azureComputePerms := makePerms(
		"Microsoft.Compute/availabilitySets/read",
		"Microsoft.Compute/availabilitySets/write",
		"Microsoft.Compute/availabilitySets/delete",
		"Microsoft.Compute/availabilitySets/vmSizes/read",
		"Microsoft.Compute/virtualMachines/read",
		"Microsoft.Compute/virtualMachines/write",
		"Microsoft.Compute/virtualMachines/delete",
		"Microsoft.Compute/virtualMachines/start/action",
		"Microsoft.Compute/disks/read",
		"Microsoft.Compute/disks/write",
	)

	awsPerms := makePerms(
		"ec2:DescribeInstances",
		"ec2:StartInstances",
		"ec2:StopInstances",
		"s3:GetObject",
		"s3:PutObject",
		"rds:DescribeDBInstances",
	)

	gcpPerms := makePerms(
		"compute.instances.get",
		"compute.instances.list",
		"compute.instances.start",
		"compute.instances.stop",
		"compute.disks.create",
		"iam.serviceAccounts.get",
		"iam.serviceAccounts.actAs",
		"iam.roles.create",
		"storage.buckets.list",
		"storage.buckets.create",
		"storage.objects.get",
	)

	tests := []struct {
		name        string
		perms       []SearchResult[ProviderPermission]
		pattern     string
		wantMatches []string
		wantErr     bool
	}{
		// ── Azure mid-path wildcard ──────────────────────────────────────────
		{
			name:    "azure: mid-path wildcard */read",
			perms:   azureComputePerms,
			pattern: "Microsoft.Compute/*/read",
			wantMatches: []string{
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/disks/read",
				"Microsoft.Compute/virtualMachines/read",
			},
		},
		{
			name:    "azure: mid-path wildcard does NOT match deeper paths",
			perms:   azureComputePerms,
			pattern: "Microsoft.Compute/*/read",
			// availabilitySets/vmSizes/read has two sub-segments so * cannot match it
			wantMatches: []string{
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/disks/read",
				"Microsoft.Compute/virtualMachines/read",
			},
		},
		{
			name:    "azure: trailing /* on resource type",
			perms:   azureComputePerms,
			pattern: "Microsoft.Compute/availabilitySets/*",
			wantMatches: []string{
				"Microsoft.Compute/availabilitySets/delete",
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/availabilitySets/write",
			},
		},
		{
			name:    "azure: trailing /* matches only direct actions, not deeper paths",
			perms:   azureComputePerms,
			pattern: "Microsoft.Compute/availabilitySets/*",
			// availabilitySets/vmSizes/read is two levels deeper, should NOT match
			wantMatches: []string{
				"Microsoft.Compute/availabilitySets/delete",
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/availabilitySets/write",
			},
		},
		{
			name:    "azure: virtualMachines trailing wildcard does not cross / separator",
			perms:   azureComputePerms,
			pattern: "Microsoft.Compute/virtualMachines/*",
			// path.Match: * matches non-/ sequences only, so virtualMachines/start/action
			// is NOT matched (it has an extra path segment).
			wantMatches: []string{
				"Microsoft.Compute/virtualMachines/delete",
				"Microsoft.Compute/virtualMachines/read",
				"Microsoft.Compute/virtualMachines/write",
			},
		},
		{
			name:        "azure: no match returns empty (not error)",
			perms:       azureComputePerms,
			pattern:     "Microsoft.Network/*/read",
			wantMatches: []string{}, // no Network perms in the set
		},
		// ── AWS colon-suffix wildcard ────────────────────────────────────────
		{
			name:    "aws: ec2:* expands all ec2 permissions",
			perms:   awsPerms,
			pattern: "ec2:*",
			wantMatches: []string{
				"ec2:DescribeInstances",
				"ec2:StartInstances",
				"ec2:StopInstances",
			},
		},
		{
			name:    "aws: s3:* expands all s3 permissions",
			perms:   awsPerms,
			pattern: "s3:*",
			wantMatches: []string{
				"s3:GetObject",
				"s3:PutObject",
			},
		},
		{
			name:    "aws: * does not cross service boundaries",
			perms:   awsPerms,
			pattern: "ec2:*",
			// Should not include s3 or rds
			wantMatches: []string{
				"ec2:DescribeInstances",
				"ec2:StartInstances",
				"ec2:StopInstances",
			},
		},
		// ── GCP dot-suffix wildcard ──────────────────────────────────────────
		{
			name:    "gcp: compute.instances.*",
			perms:   gcpPerms,
			pattern: "compute.instances.*",
			wantMatches: []string{
				"compute.instances.get",
				"compute.instances.list",
				"compute.instances.start",
				"compute.instances.stop",
			},
		},
		{
			name:    "gcp: storage.buckets.*",
			perms:   gcpPerms,
			pattern: "storage.buckets.*",
			wantMatches: []string{
				"storage.buckets.create",
				"storage.buckets.list",
			},
		},
		{
			name:    "gcp: iam.* matches multi-part names (no / separator)",
			perms:   gcpPerms,
			pattern: "iam.*",
			// * in path.Match matches any non-/ sequence, including dots
			wantMatches: []string{
				"iam.roles.create",
				"iam.serviceAccounts.actAs",
				"iam.serviceAccounts.get",
			},
		},
		// ── Error case ───────────────────────────────────────────────────────
		{
			name:    "invalid pattern returns error",
			perms:   azureComputePerms,
			pattern: "Microsoft.Compute/[invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandPermissionsWildcard(tt.perms, tt.pattern)
			if tt.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			sort.Strings(got)
			sort.Strings(tt.wantMatches)
			assert.Equal(t, tt.wantMatches, got)
		})
	}
}

// ---------------------------------------------------------------------------
// validatePermissions
// ---------------------------------------------------------------------------

func TestValidatePermissions(t *testing.T) {
	azurePerms := makePerms(
		"Microsoft.Compute/availabilitySets/read",
		"Microsoft.Compute/availabilitySets/write",
		"Microsoft.Compute/availabilitySets/delete",
		"Microsoft.Compute/virtualMachines/read",
		"Microsoft.Compute/virtualMachines/write",
		"Microsoft.Compute/virtualMachines/delete",
		"Microsoft.Compute/virtualMachines/start/action",
		"Microsoft.Compute/disks/read",
		"Microsoft.Compute/disks/write",
		"Microsoft.Compute/proximityPlacementGroups/read",
		"Microsoft.Compute/proximityPlacementGroups/write",
		"Microsoft.Authorization/roleAssignments/delete",
		"Microsoft.Authorization/roleAssignments/write",
		"Microsoft.Authorization/elevateAccess/Action",
	)

	awsPerms := makePerms(
		"ec2:DescribeInstances",
		"ec2:StartInstances",
		"ec2:StopInstances",
		"s3:GetObject",
		"s3:PutObject",
		"s3:ListBuckets",
		"rds:DescribeDBInstances",
	)

	gcpPerms := makePerms(
		"compute.instances.get",
		"compute.instances.list",
		"storage.buckets.list",
		"storage.buckets.create",
		"iam.serviceAccounts.actAs",
		"iam.serviceAccounts.get",
		"iam.roles.create",
	)

	k8sPerms := makePerms(
		"k8s:pods:get",
		"k8s:pods:list",
		"k8s:pods:watch",
		"k8s:services:get",
		"k8s:services:list",
	)

	tests := []struct {
		name       string
		perms      []SearchResult[ProviderPermission]
		statements RoleStatements
		wantOps    []string
		wantErrMsg string
	}{
		// ── Azure: mid-path wildcard ─────────────────────────────────────────
		{
			name:       "azure: Microsoft.Compute/*/read expands correctly",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Compute/*/read"),
			wantOps: []string{
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/disks/read",
				"Microsoft.Compute/proximityPlacementGroups/read",
				"Microsoft.Compute/virtualMachines/read",
			},
		},
		{
			name:       "azure: availabilitySets/* expands correctly",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Compute/availabilitySets/*"),
			wantOps: []string{
				"Microsoft.Compute/availabilitySets/delete",
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/availabilitySets/write",
			},
		},
		{
			name:  "azure: multiple wildcard patterns expand and merge",
			perms: azurePerms,
			statements: stmtOps(
				"Microsoft.Compute/*/read",
				"Microsoft.Compute/availabilitySets/*",
			),
			wantOps: []string{
				// */read
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/disks/read",
				"Microsoft.Compute/proximityPlacementGroups/read",
				"Microsoft.Compute/virtualMachines/read",
				// availabilitySets/*
				"Microsoft.Compute/availabilitySets/delete",
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/availabilitySets/write",
			},
		},
		{
			name:       "azure: exact permission passes through unchanged",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Compute/virtualMachines/read"),
			wantOps:    []string{"Microsoft.Compute/virtualMachines/read"},
		},
		{
			name:       "azure: wildcard matching nothing returns error",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Network/*/read"),
			wantErrMsg: "the wildcard permission: Microsoft.Network/*/read matched no permissions",
		},
		{
			name:       "azure: exact permission not found returns error",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Fake/fake/read"),
			wantErrMsg: "the requested permission: Microsoft.Fake/fake/read was not found",
		},
		// ── AWS ──────────────────────────────────────────────────────────────
		{
			name:       "aws: ec2:* expands all ec2 (regression guard)",
			perms:      awsPerms,
			statements: stmtOps("ec2:*"),
			wantOps: []string{
				"ec2:DescribeInstances",
				"ec2:StartInstances",
				"ec2:StopInstances",
			},
		},
		{
			name:       "aws: s3:* expands all s3 (regression guard)",
			perms:      awsPerms,
			statements: stmtOps("s3:*"),
			wantOps: []string{
				"s3:GetObject",
				"s3:ListBuckets",
				"s3:PutObject",
			},
		},
		{
			name:       "aws: exact permission passes through",
			perms:      awsPerms,
			statements: stmtOps("ec2:DescribeInstances"),
			wantOps:    []string{"ec2:DescribeInstances"},
		},
		{
			name:  "aws: unknown exact permission passes through via getCondensedActions (pre-existing behaviour)",
			perms: awsPerms,
			// getCondensedActions splits on the last colon and returns the permission
			// verbatim when there is no comma – so colon-style perms bypass the
			// exact-match check and are never rejected.  This is pre-existing behaviour
			// that is out of scope for the wildcard fix.
			statements: stmtOps("lambda:InvokeFunction"),
			wantOps:    []string{"lambda:InvokeFunction"},
		},
		// ── GCP ──────────────────────────────────────────────────────────────
		{
			name:       "gcp: compute.instances.* expands (regression guard)",
			perms:      gcpPerms,
			statements: stmtOps("compute.instances.*"),
			wantOps: []string{
				"compute.instances.get",
				"compute.instances.list",
			},
		},
		{
			name:       "gcp: iam.* expands multi-part names",
			perms:      gcpPerms,
			statements: stmtOps("iam.*"),
			wantOps: []string{
				"iam.roles.create",
				"iam.serviceAccounts.actAs",
				"iam.serviceAccounts.get",
			},
		},
		// ── k8s condensed actions ────────────────────────────────────────────
		{
			name:       "k8s: condensed actions expand unchanged",
			perms:      k8sPerms,
			statements: stmtOps("k8s:pods:get,list,watch"),
			wantOps: []string{
				"k8s:pods:get",
				"k8s:pods:list",
				"k8s:pods:watch",
			},
		},
		{
			name:       "k8s: wildcard k8s:*:* expands all k8s permissions",
			perms:      k8sPerms,
			statements: stmtOps("k8s:*:*"),
			wantOps: []string{
				"k8s:pods:get",
				"k8s:pods:list",
				"k8s:pods:watch",
				"k8s:services:get",
				"k8s:services:list",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatePermissions(tt.perms, tt.statements)
			if tt.wantErrMsg != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			require.NoError(t, err)
			gotOps := ops(got)
			wantSorted := make([]string, len(tt.wantOps))
			copy(wantSorted, tt.wantOps)
			sort.Strings(wantSorted)
			assert.Equal(t, wantSorted, gotOps)
		})
	}
}

// TestValidatePermissionsAzureYamlRoleIntegration mirrors the exact azure_admin
// allow list from config/roles/azure.yaml and confirms every wildcard expands.
func TestValidatePermissionsAzureYamlRoleIntegration(t *testing.T) {
	// Simulate a representative subset of the Azure IAM dataset for these resources.
	perms := makePerms(
		// availabilitySets
		"Microsoft.Compute/availabilitySets/read",
		"Microsoft.Compute/availabilitySets/write",
		"Microsoft.Compute/availabilitySets/delete",
		// proximityPlacementGroups
		"Microsoft.Compute/proximityPlacementGroups/read",
		"Microsoft.Compute/proximityPlacementGroups/write",
		// virtualMachines
		"Microsoft.Compute/virtualMachines/read",
		"Microsoft.Compute/virtualMachines/write",
		"Microsoft.Compute/virtualMachines/delete",
		"Microsoft.Compute/virtualMachines/start/action",
		"Microsoft.Compute/virtualMachines/restart/action",
		// disks
		"Microsoft.Compute/disks/read",
		"Microsoft.Compute/disks/write",
		"Microsoft.Compute/disks/delete",
	)

	allow := stmtOps(
		"Microsoft.Compute/*/read",
		"Microsoft.Compute/availabilitySets/*",
		"Microsoft.Compute/proximityPlacementGroups/*",
		"Microsoft.Compute/virtualMachines/*",
		"Microsoft.Compute/disks/*",
	)

	got, err := validatePermissions(perms, allow)
	require.NoError(t, err, "azure_admin allow list should validate without error")

	allOps := ops(got)
	// Every */read pattern must have hit at least the four resource types present
	assert.Contains(t, allOps, "Microsoft.Compute/availabilitySets/read")
	assert.Contains(t, allOps, "Microsoft.Compute/proximityPlacementGroups/read")
	assert.Contains(t, allOps, "Microsoft.Compute/virtualMachines/read")
	assert.Contains(t, allOps, "Microsoft.Compute/disks/read")
	// virtualMachines/* must not leave out write/delete
	assert.Contains(t, allOps, "Microsoft.Compute/virtualMachines/write")
	assert.Contains(t, allOps, "Microsoft.Compute/virtualMachines/delete")
	// virtualMachines/start/action must NOT be included by virtualMachines/* because
	// path.Match(*) does not cross the / separator
	assert.NotContains(t, allOps, "Microsoft.Compute/virtualMachines/start/action")
}
