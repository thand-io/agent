package models_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestBaseProvider_Permissions(t *testing.T) {
	p := models.NewBaseProvider(
		"test-provider",
		models.ProviderConfig{Name: "Test Provider"},
		models.NewProviderCapabilities().
			WithDefaultPermissionsConfiguration(),
	)

	perm1 := models.ProviderPermission{Name: "perm1"}
	perm2 := models.ProviderPermission{Name: "perm2"}

	// Test SetPermissions
	p.SetPermissions([]models.ProviderPermission{perm1})
	results, err := p.ListPermissions(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "perm1", results[0].Result.Name)

	// Verify permission is accessible
	perm, err := p.GetPermission(context.Background(), "perm1")
	assert.NoError(t, err)
	assert.Equal(t, "perm1", perm.Name)

	// Test AddPermissions
	p.AddPermissions(perm2)
	results, err = p.ListPermissions(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify both permissions are accessible
	perm2Ret, err := p.GetPermission(context.Background(), "perm2")
	assert.NoError(t, err)
	assert.Equal(t, "perm2", perm2Ret.Name)

	// Test AddPermissions duplicate
	p.AddPermissions(perm1)
	results, err = p.ListPermissions(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2, "Should not add duplicate permission")
}

func TestBaseProvider_Roles(t *testing.T) {
	p := models.NewBaseProvider(
		"test-provider",
		models.ProviderConfig{Name: "Test Provider"},
		models.NewProviderCapabilities().
			WithDefaultRolesConfiguration(),
	)

	role1 := models.ProviderRole{Name: "role1"}
	role2 := models.ProviderRole{Name: "role2"}

	// Test SetRoles
	p.SetRoles([]models.ProviderRole{role1})
	results, err := p.ListRoles(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "role1", results[0].Result.Name)

	// Verify role is accessible
	role, err := p.GetRole(context.Background(), "role1")
	assert.NoError(t, err)
	assert.Equal(t, "role1", role.Name)

	// Test AddRoles
	p.AddRoles(role2)
	results, err = p.ListRoles(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify both roles are accessible
	role2Ret, err := p.GetRole(context.Background(), "role2")
	assert.NoError(t, err)
	assert.Equal(t, "role2", role2Ret.Name)

	// Test AddRoles duplicate
	p.AddRoles(role1)
	results, err = p.ListRoles(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2, "Should not add duplicate role")
}

func TestBaseProvider_Resources(t *testing.T) {
	p := models.NewBaseProvider(
		"test-provider",
		models.ProviderConfig{Name: "Test Provider"},
		models.NewProviderCapabilities().
			WithDefaultResourcesConfiguration(),
	)

	res1 := models.ProviderResource{ID: "res1", Name: "res1"}
	res2 := models.ProviderResource{ID: "res2", Name: "res2"}

	// Test SetResources
	p.SetResources([]models.ProviderResource{res1})
	results, err := p.ListResources(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "res1", results[0].Result.ID)

	// Verify resource is accessible
	res, err := p.GetResource(context.Background(), "res1")
	assert.NoError(t, err)
	assert.Equal(t, "res1", res.ID)

	// Test AddResources
	p.AddResources(res2)
	results, err = p.ListResources(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify both resources are accessible
	res2Ret, err := p.GetResource(context.Background(), "res2")
	assert.NoError(t, err)
	assert.Equal(t, "res2", res2Ret.ID)

	// Test AddResources duplicate
	p.AddResources(res1)
	results, err = p.ListResources(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2, "Should not add duplicate resource")
}

func TestBaseProvider_Identities(t *testing.T) {
	p := models.NewBaseProvider(
		"test-provider",
		models.ProviderConfig{Name: "Test Provider"},
		models.NewProviderCapabilities().
			WithDefaultIdentitiesConfiguration(),
	)

	id1 := models.Identity{ID: "id1", Label: "id1"}
	id2 := models.Identity{ID: "id2", Label: "id2"}

	// Test SetIdentities
	p.SetIdentities([]models.Identity{id1})
	results, err := p.ListIdentities(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "id1", results[0].Result.ID)

	// Verify identity is accessible
	identity, err := p.GetIdentity(context.Background(), "id1")
	assert.NoError(t, err)
	assert.Equal(t, "id1", identity.ID)

	// Test AddIdentities
	p.AddIdentities(id2)
	results, err = p.ListIdentities(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2)

	// Verify both identities are accessible
	id2Ret, err := p.GetIdentity(context.Background(), "id2")
	assert.NoError(t, err)
	assert.Equal(t, "id2", id2Ret.ID)

	// Test AddIdentities duplicate
	p.AddIdentities(id1)
	results, err = p.ListIdentities(context.Background(), nil)
	assert.NoError(t, err)
	assert.Len(t, results, 2, "Should not add duplicate identity")
}
