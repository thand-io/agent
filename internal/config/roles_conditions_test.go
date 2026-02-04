package config

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestConditionsPreservationThroughInheritance verifies that conditions survive role inheritance.
func TestConditionsPreservationThroughInheritance(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"base-role": {
					Name:        "base-role",
					Description: "Base role with simple permissions",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
							},
						},
					},
				},
				"extended-role": {
					Name:        "extended-role",
					Description: "Extended role with conditioned permissions",
					Inherits:    []string{"base-role"},
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:PutObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"StringEquals": map[string]any{
										"s3:x-amz-server-side-encryption": "AES256",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "extended-role")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Verify we have both permissions
	assert.Len(t, composite.Permissions.Allow, 2, "Should have 2 allow statements")

	// Find the conditioned statement
	var foundConditioned bool
	for _, stmt := range composite.Permissions.Allow {
		if len(stmt.Conditions) > 0 {
			foundConditioned = true
			assert.Contains(t, stmt.Operations, "s3:PutObject")
			assert.NotEmpty(t, stmt.Conditions, "Conditions should be preserved")

			// Verify condition structure
			stringEquals, ok := stmt.Conditions["StringEquals"].(map[string]any)
			require.True(t, ok, "StringEquals should be a map")
			assert.Equal(t, "AES256", stringEquals["s3:x-amz-server-side-encryption"])
		}
	}
	assert.True(t, foundConditioned, "Should have found conditioned statement")
}

// TestConditionsInConflictResolution verifies that same operation with and without
// conditions are treated as different statements.
func TestConditionsInConflictResolution(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"mixed-conditions": {
					Name:        "mixed-conditions",
					Description: "Role with same operation with and without conditions",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							// Unconditional access
							{
								Operations: []string{"s3:PutObject"},
								Targets:    []string{"arn:aws:s3:::public-bucket/*"},
							},
							// Conditional access (different target)
							{
								Operations: []string{"s3:PutObject"},
								Targets:    []string{"arn:aws:s3:::secure-bucket/*"},
								Conditions: map[string]any{
									"IpAddress": map[string]any{
										"aws:SourceIp": "10.0.0.0/8",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "mixed-conditions")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Both statements should survive - they're different due to conditions
	assert.Len(t, composite.Permissions.Allow, 2, "Both statements should be preserved")

	// Verify we have both conditioned and unconditioned
	var hasConditioned, hasUnconditioned bool
	for _, stmt := range composite.Permissions.Allow {
		if len(stmt.Conditions) > 0 {
			hasConditioned = true
		} else {
			hasUnconditioned = true
		}
	}
	assert.True(t, hasConditioned, "Should have conditioned statement")
	assert.True(t, hasUnconditioned, "Should have unconditioned statement")
}

// TestMultipleConditionsOnSameOperation verifies that multiple conditions
// on the same operation are preserved independently.
func TestMultipleConditionsOnSameOperation(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"multi-conditions": {
					Name:        "multi-conditions",
					Description: "Multiple conditions on same operation",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"IpAddress": map[string]any{
										"aws:SourceIp": "10.0.0.0/8",
									},
								},
							},
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"StringEquals": map[string]any{
										"s3:x-amz-server-side-encryption": "AES256",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "multi-conditions")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Both conditioned statements should be preserved
	assert.Len(t, composite.Permissions.Allow, 2, "Both conditioned statements should be preserved")

	// Verify we have both types of conditions
	var hasIpCondition, hasEncryptionCondition bool
	for _, stmt := range composite.Permissions.Allow {
		if _, ok := stmt.Conditions["IpAddress"]; ok {
			hasIpCondition = true
		}
		if _, ok := stmt.Conditions["StringEquals"]; ok {
			hasEncryptionCondition = true
		}
	}
	assert.True(t, hasIpCondition, "Should have IP condition")
	assert.True(t, hasEncryptionCondition, "Should have encryption condition")
}

// TestConditionsPreservationWithDeny verifies conditions work with deny statements.
func TestConditionsPreservationWithDeny(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"deny-with-conditions": {
					Name:        "deny-with-conditions",
					Description: "Role with conditional deny",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:*"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
							},
						},
						Deny: models.RoleStatements{
							{
								Operations: []string{"s3:DeleteObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"StringNotEquals": map[string]any{
										"aws:username": "admin",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "deny-with-conditions")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Verify deny with condition is preserved
	require.Len(t, composite.Permissions.Deny, 1, "Should have 1 deny statement")
	assert.NotEmpty(t, composite.Permissions.Deny[0].Conditions, "Deny conditions should be preserved")
}

// TestConditionsInConflictingStatements verifies that identical statements
// with conditions are deduplicated properly.
func TestConditionsInConflictingStatements(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"parent": {
					Name: "parent",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"IpAddress": map[string]any{
										"aws:SourceIp": "10.0.0.0/8",
									},
								},
							},
						},
					},
				},
				"child": {
					Name:     "child",
					Inherits: []string{"parent"},
					Permissions: models.RolePermissions{
						// Same statement in both allow and deny
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"IpAddress": map[string]any{
										"aws:SourceIp": "10.0.0.0/8",
									},
								},
							},
						},
						Deny: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"IpAddress": map[string]any{
										"aws:SourceIp": "10.0.0.0/8",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "child")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// When same statement is in both allow and deny, both should be removed
	// due to deduplication (deny wins = remove both)
	assert.Empty(t, composite.Permissions.Allow, "Conflicting statements should be removed from allow")
	assert.Len(t, composite.Permissions.Deny, 1, "Deny statement should be preserved")
}

// TestEmptyConditionsNotPreserved verifies that empty/nil conditions
// don't cause statements to be treated as conditioned.
func TestEmptyConditionsNotPreserved(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"empty-conditions": {
					Name: "empty-conditions",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{}, // Empty conditions
							},
							{
								Operations: []string{"s3:PutObject"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								// Nil conditions (default)
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "empty-conditions")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Empty conditions should be treated as no conditions
	// Should be merged into a single statement with both operations
	assert.Len(t, composite.Permissions.Allow, 1, "Empty conditions should allow merging")

	// Check that the statement has both operations (may be condensed as "s3:GetObject,PutObject")
	ops := composite.Permissions.Allow[0].Operations
	// Operations might be condensed into comma-separated string
	opsStr := strings.Join(ops, " ")
	assert.True(t, strings.Contains(opsStr, "GetObject"), "Should contain GetObject")
	assert.True(t, strings.Contains(opsStr, "PutObject"), "Should contain PutObject")
}

// TestWildcardOperationsWithConditions verifies wildcard operations
// with conditions are preserved.
func TestWildcardOperationsWithConditions(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"wildcard-conditions": {
					Name: "wildcard-conditions",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:*"},
								Targets:    []string{"arn:aws:s3:::bucket/*"},
								Conditions: map[string]any{
									"StringEquals": map[string]any{
										"s3:x-amz-acl": "private",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "wildcard-conditions")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Wildcard with condition should be preserved as-is
	require.Len(t, composite.Permissions.Allow, 1)
	stmt := composite.Permissions.Allow[0]
	assert.Contains(t, stmt.Operations, "s3:*")
	assert.NotEmpty(t, stmt.Conditions)
}

// TestConditionsPreservationWithDifferentTargets ensures conditions are preserved
// when statements have different targets.
func TestConditionsPreservationWithDifferentTargets(t *testing.T) {
	cfg := &Config{
		Roles: RoleConfig{
			Definitions: map[string]models.Role{
				"multi-target": {
					Name: "multi-target",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::public-bucket/*"},
							},
							{
								Operations: []string{"s3:GetObject"},
								Targets:    []string{"arn:aws:s3:::secure-bucket/*"},
								Conditions: map[string]any{
									"IpAddress": map[string]any{
										"aws:SourceIp": "10.0.0.0/8"},
								},
							},
						},
					},
				},
			},
		},
	}

	composite, err := cfg.GetCompositeRoleByName(nil, "multi-target")
	require.NoError(t, err)
	require.NotNil(t, composite)

	// Both statements should be preserved (different targets + one has conditions)
	require.Len(t, composite.Permissions.Allow, 2)

	// Find the conditioned statement
	var foundConditioned bool
	for _, stmt := range composite.Permissions.Allow {
		if len(stmt.Conditions) > 0 {
			foundConditioned = true
			assert.Contains(t, stmt.Targets, "arn:aws:s3:::secure-bucket/*")
		}
	}
	assert.True(t, foundConditioned, "Should have found conditioned statement")
}
