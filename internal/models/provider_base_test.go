package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseProvider_Permissions(t *testing.T) {
	p := &BaseProvider{
		rbac: &RBACSupport{
			permissions:    make([]ProviderPermission, 0),
			permissionsMap: make(map[string]*ProviderPermission),
		},
	}

	perm1 := ProviderPermission{Name: "perm1"}
	perm2 := ProviderPermission{Name: "perm2"}

	// Test SetPermissions
	p.SetPermissions([]ProviderPermission{perm1})
	assert.Len(t, p.rbac.permissions, 1)
	assert.Equal(t, "perm1", p.rbac.permissions[0].Name)
	assert.Contains(t, p.rbac.permissionsMap, "perm1")

	// Test AddPermissions
	p.AddPermissions(perm2)
	assert.Len(t, p.rbac.permissions, 2)
	assert.Equal(t, "perm2", p.rbac.permissions[1].Name)
	assert.Contains(t, p.rbac.permissionsMap, "perm2")

	// Test AddPermissions duplicate
	p.AddPermissions(perm1)
	// This is expected to fail if duplicates are not filtered
	assert.Len(t, p.rbac.permissions, 2, "Should not add duplicate permission")
}

func TestBaseProvider_Roles(t *testing.T) {
	p := &BaseProvider{
		rbac: &RBACSupport{
			roles:    make([]ProviderRole, 0),
			rolesMap: make(map[string]*ProviderRole),
		},
	}

	role1 := ProviderRole{Name: "role1"}
	role2 := ProviderRole{Name: "role2"}

	// Test SetRoles
	p.SetRoles([]ProviderRole{role1})
	assert.Len(t, p.rbac.roles, 1)
	assert.Equal(t, "role1", p.rbac.roles[0].Name)
	assert.Contains(t, p.rbac.rolesMap, "role1")

	// Test AddRoles
	p.AddRoles(role2)
	assert.Len(t, p.rbac.roles, 2)
	assert.Equal(t, "role2", p.rbac.roles[1].Name)
	assert.Contains(t, p.rbac.rolesMap, "role2")

	// Test AddRoles duplicate
	p.AddRoles(role1)
	assert.Len(t, p.rbac.roles, 2, "Should not add duplicate role")
}

func TestBaseProvider_Resources(t *testing.T) {
	p := &BaseProvider{
		rbac: &RBACSupport{
			resources:    make([]ProviderResource, 0),
			resourcesMap: make(map[string]*ProviderResource),
		},
	}

	res1 := ProviderResource{ID: "res1", Name: "res1"}
	res2 := ProviderResource{ID: "res2", Name: "res2"}

	// Test SetResources
	p.SetResources([]ProviderResource{res1})
	assert.Len(t, p.rbac.resources, 1)
	assert.Equal(t, "res1", p.rbac.resources[0].ID)
	assert.Contains(t, p.rbac.resourcesMap, "res1")

	// Test AddResources
	p.AddResources(res2)
	assert.Len(t, p.rbac.resources, 2)
	assert.Equal(t, "res2", p.rbac.resources[1].ID)
	assert.Contains(t, p.rbac.resourcesMap, "res2")

	// Test AddResources duplicate
	p.AddResources(res1)
	assert.Len(t, p.rbac.resources, 2, "Should not add duplicate resource")
}

func TestBaseProvider_Identities(t *testing.T) {
	p := &BaseProvider{
		identity: &IdentitySupport{
			identities:    make([]Identity, 0),
			identitiesMap: make(map[string]*Identity),
		},
	}

	id1 := Identity{ID: "id1", Label: "id1"}
	id2 := Identity{ID: "id2", Label: "id2"}

	// Test SetIdentities
	p.SetIdentities([]Identity{id1})
	assert.Len(t, p.identity.identities, 1)
	assert.Equal(t, "id1", p.identity.identities[0].ID)
	assert.Contains(t, p.identity.identitiesMap, "id1")

	// Test AddIdentities
	p.AddIdentities(id2)
	assert.Len(t, p.identity.identities, 2)
	assert.Equal(t, "id2", p.identity.identities[1].ID)
	assert.Contains(t, p.identity.identitiesMap, "id2")

	// Test AddIdentities duplicate
	p.AddIdentities(id1)
	assert.Len(t, p.identity.identities, 2, "Should not add duplicate identity")
}
