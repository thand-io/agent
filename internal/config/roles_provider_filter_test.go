package config

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestProviderIdentifiers verifies the helper that extracts identifiers/engine
// types from models.Provider objects.
func TestProviderIdentifiers(t *testing.T) {
	t.Run("nil providers returns nil", func(t *testing.T) {
		result := providerIdentifiers()
		assert.Nil(t, result)
	})

	t.Run("single provider returns identifier and engine type", func(t *testing.T) {
		providers := map[string]models.ProviderConfig{
			"aws-prod": {
				Name:     "AWS Production",
				Provider: "aws",
				Enabled:  true,
			},
		}
		config := newTestConfig(t, nil, providers)

		var p models.Provider
		for _, inst := range config.providerInstances {
			p = inst
			break
		}

		result := providerIdentifiers(p)
		require.NotNil(t, result)
		sort.Strings(result)
		assert.Contains(t, result, "aws-prod")
		assert.Contains(t, result, "aws")
	})

	t.Run("deduplicates when identifier equals engine type", func(t *testing.T) {
		// When the provider key IS the engine type, we should get one entry
		providers := map[string]models.ProviderConfig{
			"aws": {
				Name:     "AWS",
				Provider: "aws",
				Enabled:  true,
			},
		}
		config := newTestConfig(t, nil, providers)

		var p models.Provider
		for _, inst := range config.providerInstances {
			p = inst
			break
		}

		result := providerIdentifiers(p)
		require.NotNil(t, result)
		// Should only have one entry since identifier == engine type
		assert.Len(t, result, 1)
		assert.Equal(t, "aws", result[0])
	})

	t.Run("nil provider pointer is skipped", func(t *testing.T) {
		var nilProvider models.Provider
		result := providerIdentifiers(nilProvider)
		assert.Nil(t, result)
	})
}

// TestResolveCompositeRoleWithProviderFilter verifies that passing explicit
// providers to GetCompositeRole filters permissions and inheritance to only
// those matching the supplied providers.
func TestResolveCompositeRoleWithProviderFilter(t *testing.T) {

	t.Run("no providers arg evaluates everything using role providers", func(t *testing.T) {
		// Role has provider-prefixed operations from both aws-prod and gcp-prod.
		// When no explicit providers are passed, filtering falls back to
		// the role's own Providers list (which includes both).
		roles := map[string]models.Role{
			"multi-cloud": {
				Name:      "multi-cloud",
				Providers: []string{"aws-prod", "gcp-prod"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{Operations: []string{"aws-prod:s3:GetObject"}},
						{Operations: []string{"gcp-prod:compute.instances.get"}},
						{Operations: []string{"read"}}, // no provider prefix
					},
				},
				Enabled: true,
			},
		}

		providerDefs := map[string]models.ProviderConfig{
			"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
		}

		config := newTestConfig(t, roles, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		// Call WITHOUT explicit providers
		result, err := config.GetCompositeRoleByName(identity, "multi-cloud")
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)
		// Both provider-prefixed operations should survive (stripped of prefix)
		assert.Contains(t, ops, "read")
		// Provider-prefixed ops are stripped and may be condensed
		foundS3 := false
		foundCompute := false
		for _, op := range ops {
			if op == "s3:GetObject" {
				foundS3 = true
			}
			if op == "compute.instances.get" {
				foundCompute = true
			}
		}
		assert.True(t, foundS3, "expected s3:GetObject from aws-prod prefix")
		assert.True(t, foundCompute, "expected compute.instances.get from gcp-prod prefix")
	})

	t.Run("explicit provider filters permissions to matching provider only", func(t *testing.T) {
		roles := map[string]models.Role{
			"multi-cloud": {
				Name:      "multi-cloud",
				Providers: []string{"aws-prod", "gcp-prod"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{Operations: []string{"aws-prod:s3:GetObject"}},
						{Operations: []string{"gcp-prod:compute.instances.get"}},
						{Operations: []string{"read"}}, // no provider prefix
					},
				},
				Enabled: true,
			},
		}

		providerDefs := map[string]models.ProviderConfig{
			"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
		}

		config := newTestConfig(t, roles, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		// Get the AWS provider instance
		awsProvider := config.providerInstances["aws-prod"]
		require.NotNil(t, awsProvider)

		// Call WITH explicit AWS provider only
		result, err := config.GetCompositeRole(identity, &models.Role{
			Name:      "multi-cloud",
			Providers: []string{"aws-prod", "gcp-prod"},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"aws-prod:s3:GetObject"}},
					{Operations: []string{"gcp-prod:compute.instances.get"}},
					{Operations: []string{"read"}},
				},
			},
			Enabled: true,
		}, awsProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)
		// "read" (no prefix) should always be included
		assert.Contains(t, ops, "read")

		// AWS-prefixed ops should survive (stripped of prefix)
		foundS3 := false
		for _, op := range ops {
			if op == "s3:GetObject" {
				foundS3 = true
			}
		}
		assert.True(t, foundS3, "expected s3:GetObject from aws-prod prefix")

		// GCP-prefixed ops should be excluded
		for _, op := range ops {
			assert.NotContains(t, op, "compute.instances", "gcp-prod ops should be filtered out")
		}
	})

	t.Run("explicit provider filters deny permissions too", func(t *testing.T) {
		roles := map[string]models.Role{
			"restricted": {
				Name:      "restricted",
				Providers: []string{"aws-prod", "gcp-prod"},
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{Operations: []string{"read"}},
					},
					Deny: models.RoleStatements{
						{Operations: []string{"aws-prod:s3:DeleteObject"}},
						{Operations: []string{"gcp-prod:compute.instances.delete"}},
					},
				},
				Enabled: true,
			},
		}

		providerDefs := map[string]models.ProviderConfig{
			"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
		}

		config := newTestConfig(t, roles, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		awsProvider := config.providerInstances["aws-prod"]
		require.NotNil(t, awsProvider)

		result, err := config.GetCompositeRole(identity, &models.Role{
			Name:      "restricted",
			Providers: []string{"aws-prod", "gcp-prod"},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"read"}},
				},
				Deny: models.RoleStatements{
					{Operations: []string{"aws-prod:s3:DeleteObject"}},
					{Operations: []string{"gcp-prod:compute.instances.delete"}},
				},
			},
			Enabled: true,
		}, awsProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		denyOps := collectAllOps(result.Permissions.Deny)
		// AWS deny should survive
		foundS3Delete := false
		for _, op := range denyOps {
			if op == "s3:DeleteObject" {
				foundS3Delete = true
			}
		}
		assert.True(t, foundS3Delete, "expected s3:DeleteObject in deny")

		// GCP deny should be filtered out
		for _, op := range denyOps {
			assert.NotContains(t, op, "compute.instances", "gcp-prod deny ops should be filtered out")
		}
	})

	t.Run("multiple providers filter correctly", func(t *testing.T) {
		providerDefs := map[string]models.ProviderConfig{
			"aws-prod":   {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod":   {Name: "GCP Prod", Provider: "gcp", Enabled: true},
			"azure-prod": {Name: "Azure Prod", Provider: "azure", Enabled: true},
		}

		config := newTestConfig(t, nil, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		awsProvider := config.providerInstances["aws-prod"]
		gcpProvider := config.providerInstances["gcp-prod"]
		require.NotNil(t, awsProvider)
		require.NotNil(t, gcpProvider)

		role := &models.Role{
			Name:      "all-cloud",
			Providers: []string{"aws-prod", "gcp-prod", "azure-prod"},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"aws-prod:s3:GetObject"}},
					{Operations: []string{"gcp-prod:compute.instances.get"}},
					{Operations: []string{"azure-prod:Microsoft.Compute/virtualMachines/read"}},
					{Operations: []string{"read"}}, // no prefix
				},
			},
			Enabled: true,
		}

		// Pass AWS + GCP providers, Azure should be filtered out
		result, err := config.GetCompositeRole(identity, role, awsProvider, gcpProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, ops, "read")

		foundS3 := false
		foundCompute := false
		for _, op := range ops {
			if op == "s3:GetObject" {
				foundS3 = true
			}
			if op == "compute.instances.get" {
				foundCompute = true
			}
		}
		assert.True(t, foundS3, "expected s3:GetObject from aws-prod")
		assert.True(t, foundCompute, "expected compute.instances.get from gcp-prod")

		// Azure ops should be excluded
		for _, op := range ops {
			assert.NotContains(t, op, "Microsoft.Compute", "azure-prod ops should be filtered out")
		}
	})

	t.Run("provider engine type matching works", func(t *testing.T) {
		// Permission prefixed with engine type "aws" (not identifier "aws-prod")
		// should still match when a provider with GetProvider()=="aws" is passed
		providerDefs := map[string]models.ProviderConfig{
			"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
		}

		config := newTestConfig(t, nil, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		awsProvider := config.providerInstances["aws-prod"]
		require.NotNil(t, awsProvider)

		role := &models.Role{
			Name:      "engine-type-role",
			Providers: []string{"aws-prod", "gcp-prod"},
			Permissions: models.RolePermissions{
				Allow: models.RoleStatements{
					// Using engine type "aws" as prefix (parseProviderPrefix resolves
					// this to the provider key "aws-prod")
					{Operations: []string{"aws:s3:GetObject"}},
					{Operations: []string{"gcp:compute.instances.get"}},
				},
			},
			Enabled: true,
		}

		// providerIdentifiers will produce both "aws-prod" and "aws"
		// parseProviderPrefix("aws:s3:GetObject") returns ("aws-prod", "s3:GetObject", true)
		// so "aws-prod" must be in the allowed set — and it is via providerIdentifiers
		result, err := config.GetCompositeRole(identity, role, awsProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)

		foundS3 := false
		for _, op := range ops {
			if op == "s3:GetObject" {
				foundS3 = true
			}
		}
		assert.True(t, foundS3, "expected aws-prefixed s3:GetObject to pass filter via engine type match")

		// GCP operations should be filtered out
		for _, op := range ops {
			assert.NotContains(t, op, "compute.instances", "gcp ops should be filtered out")
		}
	})
}

// TestResolveCompositeRoleWithProviderFilter_Inheritance verifies that the
// providers argument filters inherited roles appropriately.
func TestResolveCompositeRoleWithProviderFilter_Inheritance(t *testing.T) {

	t.Run("provider arg filters provider-prefixed inherits", func(t *testing.T) {
		roles := map[string]models.Role{
			"base-aws": {
				Name: "base-aws",
				Permissions: models.RolePermissions{
					Allow: stmts("s3:GetObject"),
				},
				Enabled: true,
			},
			"base-gcp": {
				Name: "base-gcp",
				Permissions: models.RolePermissions{
					Allow: stmts("compute.instances.get"),
				},
				Enabled: true,
			},
			"combined": {
				Name:      "combined",
				Providers: []string{"aws-prod", "gcp-prod"},
				// Provider-prefixed inherits
				Inherits: []string{"aws-prod:base-aws", "gcp-prod:base-gcp"},
				Permissions: models.RolePermissions{
					Allow: stmts("read"),
				},
				Enabled: true,
			},
		}

		providerDefs := map[string]models.ProviderConfig{
			"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
		}

		config := newTestConfig(t, roles, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		awsProvider := config.providerInstances["aws-prod"]
		require.NotNil(t, awsProvider)

		result, err := config.GetCompositeRole(identity, &models.Role{
			Name:      "combined",
			Providers: []string{"aws-prod", "gcp-prod"},
			Inherits:  []string{"aws-prod:base-aws", "gcp-prod:base-gcp"},
			Permissions: models.RolePermissions{
				Allow: stmts("read"),
			},
			Enabled: true,
		}, awsProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)
		assert.Contains(t, ops, "read")

		// The "gcp-prod:base-gcp" inherit should be skipped because we only
		// passed the AWS provider. The "aws-prod:base-aws" inherit is a
		// provider role lookup — if the provider doesn't have a role named
		// "base-aws" it won't resolve via provider role, but the filtering
		// should still remove the GCP inherit.
		for _, op := range ops {
			assert.NotEqual(t, "compute.instances.get", op,
				"GCP inherited permissions should be filtered out")
		}
	})

	t.Run("inherited role permissions filtered by provider arg", func(t *testing.T) {
		roles := map[string]models.Role{
			"child-role": {
				Name: "child-role",
				Permissions: models.RolePermissions{
					Allow: models.RoleStatements{
						{Operations: []string{"aws-prod:s3:GetObject"}},
						{Operations: []string{"gcp-prod:compute.instances.get"}},
						{Operations: []string{"common:read"}},
					},
				},
				Enabled: true,
			},
			"parent-role": {
				Name:      "parent-role",
				Providers: []string{"aws-prod", "gcp-prod"},
				Inherits:  []string{"child-role"},
				Permissions: models.RolePermissions{
					Allow: stmts("parent:action"),
				},
				Enabled: true,
			},
		}

		providerDefs := map[string]models.ProviderConfig{
			"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
			"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
		}

		config := newTestConfig(t, roles, providerDefs)
		identity := &models.Identity{
			ID:   "user1",
			User: &models.User{Username: "testuser"},
		}

		awsProvider := config.providerInstances["aws-prod"]
		require.NotNil(t, awsProvider)

		result, err := config.GetCompositeRole(identity, &models.Role{
			Name:      "parent-role",
			Providers: []string{"aws-prod", "gcp-prod"},
			Inherits:  []string{"child-role"},
			Permissions: models.RolePermissions{
				Allow: stmts("parent:action"),
			},
			Enabled: true,
		}, awsProvider)
		require.NoError(t, err)
		require.NotNil(t, result)

		ops := collectAllOps(result.Permissions.Allow)

		// Parent's own permission (no provider prefix) should be present
		assert.Contains(t, ops, "parent:action")

		// The child's provider-prefixed ops that don't match AWS should be
		// filtered out. The child role is resolved recursively so the
		// providers arg propagates.
		for _, op := range ops {
			assert.NotContains(t, op, "compute.instances",
				"gcp-prod ops from inherited role should be filtered out")
		}
	})
}

// TestGetCompositeRoleForIdentityPassesProviders verifies that
// GetCompositeRoleForIdentity properly forwards providers through to
// the underlying GetCompositeRole call.
func TestGetCompositeRoleForIdentityPassesProviders(t *testing.T) {
	providerDefs := map[string]models.ProviderConfig{
		"aws-prod": {Name: "AWS Prod", Provider: "aws", Enabled: true},
		"gcp-prod": {Name: "GCP Prod", Provider: "gcp", Enabled: true},
	}

	config := newTestConfig(t, nil, providerDefs)
	identity := &models.Identity{
		ID:   "user1",
		User: &models.User{Username: "testuser", Email: "testuser@example.com"},
	}

	awsProvider := config.providerInstances["aws-prod"]
	require.NotNil(t, awsProvider)

	role := &models.Role{
		Name:       "passthrough-role",
		Identifier: "passthrough_role",
		Providers:  []string{"aws-prod", "gcp-prod"},
		Permissions: models.RolePermissions{
			Allow: models.RoleStatements{
				{Operations: []string{"aws-prod:s3:GetObject"}},
				{Operations: []string{"gcp-prod:compute.instances.get"}},
				{Operations: []string{"read"}},
			},
		},
		Enabled: true,
	}

	// Call GetCompositeRoleForIdentity with the AWS provider
	result, err := config.GetCompositeRoleForIdentity(identity, role, awsProvider)
	require.NoError(t, err)
	require.NotNil(t, result)

	ops := collectAllOps(result.Permissions.Allow)
	assert.Contains(t, ops, "read")

	foundS3 := false
	for _, op := range ops {
		if op == "s3:GetObject" {
			foundS3 = true
		}
	}
	assert.True(t, foundS3, "expected s3:GetObject from aws-prod prefix after passthrough")

	// GCP should be filtered
	for _, op := range ops {
		assert.NotContains(t, op, "compute.instances",
			"gcp-prod ops should be filtered when only aws provider passed through GetCompositeRoleForIdentity")
	}

	// Verify the identifier was updated (GetCompositeRoleForIdentity generates unique IDs)
	assert.NotEqual(t, "passthrough_role", result.GetUniqueName(),
		"GetUniqueName should return a hash-suffixed identifier")
}
