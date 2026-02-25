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
			name:       "azure: Microsoft.Compute/*/read condensed back to wildcard",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Compute/*/read"),
			wantOps:    []string{"Microsoft.Compute/*/read"},
		},
		{
			name:       "azure: availabilitySets/* condensed back to wildcard",
			perms:      azurePerms,
			statements: stmtOps("Microsoft.Compute/availabilitySets/*"),
			wantOps:    []string{"Microsoft.Compute/availabilitySets/*"},
		},
		{
			name:  "azure: multiple wildcard patterns condensed back",
			perms: azurePerms,
			statements: stmtOps(
				"Microsoft.Compute/*/read",
				"Microsoft.Compute/availabilitySets/*",
			),
			wantOps: []string{
				"Microsoft.Compute/*/read",
				"Microsoft.Compute/availabilitySets/*",
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
			name:       "aws: ec2:* condensed back to wildcard (round-trip)",
			perms:      awsPerms,
			statements: stmtOps("ec2:*"),
			wantOps:    []string{"ec2:*"},
		},
		{
			name:       "aws: s3:* condensed back to wildcard (round-trip)",
			perms:      awsPerms,
			statements: stmtOps("s3:*"),
			wantOps:    []string{"s3:*"},
		},
		{
			name:       "aws: exact permission passes through",
			perms:      awsPerms,
			statements: stmtOps("ec2:DescribeInstances"),
			wantOps:    []string{"ec2:DescribeInstances"},
		},
		{
			name:  "aws: unknown colon-style permission returns error",
			perms: awsPerms,
			// Previously colon-style perms bypassed the existence check.
			// Now each expanded permission is validated against the provider set.
			statements: stmtOps("lambda:InvokeFunction"),
			wantErrMsg: "the requested permission: lambda:InvokeFunction was not found",
		},
		// ── GCP ──────────────────────────────────────────────────────────────
		{
			name:       "gcp: compute.instances.* condensed back to wildcard",
			perms:      gcpPerms,
			statements: stmtOps("compute.instances.*"),
			wantOps:    []string{"compute.instances.*"},
		},
		{
			name:       "gcp: iam.* condensed back to wildcard",
			perms:      gcpPerms,
			statements: stmtOps("iam.*"),
			wantOps:    []string{"iam.*"},
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
			name:       "k8s: wildcard k8s:*:* condensed back to wildcard",
			perms:      k8sPerms,
			statements: stmtOps("k8s:*:*"),
			wantOps:    []string{"k8s:*:*"},
		},
		// ── Colon-style permission validation ────────────────────────────────
		{
			name:       "aws: typo in single colon-style perm returns error",
			perms:      awsPerms,
			statements: stmtOps("ec2:TypoAction"),
			wantErrMsg: "the requested permission: ec2:TypoAction was not found",
		},
		{
			name:       "k8s: condensed with one invalid verb returns error",
			perms:      k8sPerms,
			statements: stmtOps("k8s:pods:get,list,delete"),
			wantErrMsg: "the requested permission: k8s:pods:delete was not found",
		},
		{
			name:       "aws: nonexistent service prefix returns error",
			perms:      awsPerms,
			statements: stmtOps("fake:DoSomething"),
			wantErrMsg: "the requested permission: fake:DoSomething was not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatePermissions(tt.perms, tt.statements, true)
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

	got, err := validatePermissions(perms, allow, true)
	require.NoError(t, err, "azure_admin allow list should validate without error")

	allOps := ops(got)
	// After condensation, original wildcard patterns should be restored.
	assert.Contains(t, allOps, "Microsoft.Compute/*/read")
	assert.Contains(t, allOps, "Microsoft.Compute/availabilitySets/*")
	assert.Contains(t, allOps, "Microsoft.Compute/proximityPlacementGroups/*")
	assert.Contains(t, allOps, "Microsoft.Compute/virtualMachines/*")
	assert.Contains(t, allOps, "Microsoft.Compute/disks/*")
	// virtualMachines/start/action must NOT be included by virtualMachines/* because
	// path.Match(*) does not cross the / separator
	assert.NotContains(t, allOps, "Microsoft.Compute/virtualMachines/start/action")
}

// ---------------------------------------------------------------------------
// condenseToOriginalWildcards (unit)
// ---------------------------------------------------------------------------

func TestCondenseToOriginalWildcards(t *testing.T) {
	tests := []struct {
		name      string
		ops       []string
		wildcards []string
		want      []string
	}{
		{
			name:      "no wildcards — passthrough",
			ops:       []string{"ec2:DescribeInstances", "ec2:StartInstances"},
			wildcards: nil,
			want:      []string{"ec2:DescribeInstances", "ec2:StartInstances"},
		},
		{
			name:      "single wildcard restores all covered perms",
			ops:       []string{"ec2:DescribeInstances", "ec2:StartInstances", "ec2:StopInstances"},
			wildcards: []string{"ec2:*"},
			want:      []string{"ec2:*"},
		},
		{
			name:      "wildcard and non-matching perm coexist",
			ops:       []string{"ec2:DescribeInstances", "s3:GetObject"},
			wildcards: []string{"ec2:*"},
			want:      []string{"ec2:*", "s3:GetObject"},
		},
		{
			name: "overlapping wildcards processed in order",
			ops: []string{
				"Microsoft.Compute/availabilitySets/read",
				"Microsoft.Compute/availabilitySets/write",
				"Microsoft.Compute/disks/read",
			},
			wildcards: []string{
				"Microsoft.Compute/*/read",
				"Microsoft.Compute/availabilitySets/*",
			},
			want: []string{
				"Microsoft.Compute/*/read",
				"Microsoft.Compute/availabilitySets/*",
			},
		},
		{
			name:      "empty operations",
			ops:       []string{},
			wildcards: []string{"ec2:*"},
			want:      []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := condenseToOriginalWildcards(tt.ops, tt.wildcards)
			sort.Strings(tt.want)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Round-trip: wildcard + exact mixed
// ---------------------------------------------------------------------------

func TestValidatePermissionsRoundTrip(t *testing.T) {
	allPerms := makePerms(
		"ec2:DescribeInstances",
		"ec2:StartInstances",
		"ec2:StopInstances",
		"s3:GetObject",
		"s3:PutObject",
		"s3:ListBuckets",
		"rds:DescribeDBInstances",
	)

	tests := []struct {
		name    string
		input   []string
		wantOps []string
	}{
		{
			name:    "wildcard + exact: ec2:* and s3:GetObject",
			input:   []string{"ec2:*", "s3:GetObject"},
			wantOps: []string{"ec2:*", "s3:GetObject"},
		},
		{
			name:    "multiple wildcards: ec2:* and s3:*",
			input:   []string{"ec2:*", "s3:*"},
			wantOps: []string{"ec2:*", "s3:*"},
		},
		{
			name:    "single wildcard with suffix: ec2:Describe*",
			input:   []string{"ec2:Describe*"},
			wantOps: []string{"ec2:Describe*"},
		},
		{
			name:    "all perms via top-level wildcard pattern",
			input:   []string{"*"},
			wantOps: []string{"*"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatePermissions(allPerms, stmtOps(tt.input...), true)
			require.NoError(t, err)
			gotOps := ops(got)
			sort.Strings(tt.wantOps)
			assert.Equal(t, tt.wantOps, gotOps)
		})
	}
}

// ---------------------------------------------------------------------------
// supportsWildcards=false: wildcards stay expanded (GCP/Okta behavior)
// ---------------------------------------------------------------------------

func TestValidatePermissionsNoWildcardSupport(t *testing.T) {
	gcpPerms := makePerms(
		"compute.instances.get",
		"compute.instances.list",
		"compute.instances.start",
		"compute.instances.stop",
		"storage.buckets.list",
		"storage.buckets.create",
		"iam.serviceAccounts.get",
		"iam.serviceAccounts.actAs",
	)

	awsPerms := makePerms(
		"ec2:DescribeInstances",
		"ec2:StartInstances",
		"ec2:StopInstances",
		"s3:GetObject",
		"s3:PutObject",
	)

	tests := []struct {
		name    string
		perms   []SearchResult[ProviderPermission]
		input   []string
		wantOps []string
	}{
		{
			name:  "gcp: compute.instances.* stays expanded",
			perms: gcpPerms,
			input: []string{"compute.instances.*"},
			wantOps: []string{
				"compute.instances.get",
				"compute.instances.list",
				"compute.instances.start",
				"compute.instances.stop",
			},
		},
		{
			name:  "gcp: multiple wildcards stay expanded",
			perms: gcpPerms,
			input: []string{"compute.instances.*", "storage.buckets.*"},
			wantOps: []string{
				"compute.instances.get",
				"compute.instances.list",
				"compute.instances.start",
				"compute.instances.stop",
				"storage.buckets.create",
				"storage.buckets.list",
			},
		},
		{
			name:  "aws: ec2:* stays expanded when provider does not support wildcards",
			perms: awsPerms,
			input: []string{"ec2:*"},
			wantOps: []string{
				"ec2:DescribeInstances",
				"ec2:StartInstances",
				"ec2:StopInstances",
			},
		},
		{
			name:    "exact permission unaffected by supportsWildcards flag",
			perms:   gcpPerms,
			input:   []string{"compute.instances.get"},
			wantOps: []string{"compute.instances.get"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := validatePermissions(tt.perms, stmtOps(tt.input...), false)
			require.NoError(t, err)
			gotOps := ops(got)
			sort.Strings(tt.wantOps)
			assert.Equal(t, tt.wantOps, gotOps)
		})
	}
}
