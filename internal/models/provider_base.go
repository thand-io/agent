package models

import (
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2"
	"github.com/sirupsen/logrus"
)

type BaseProvider struct {
	identifier      string
	name            string
	description     string
	provider        string
	config          *BasicConfig
	role            *Role
	capabilities    *ProviderCapabilities
	providerWrapper *Provider // Reference back to the Provider wrapper

	// Add other common fields if necessary
	identity *IdentitySupport
	rbac     *RBACSupport
	tenants  *TenantsSupport
}

type IdentitySupport struct {
	mu sync.RWMutex

	// Identity management
	identities      []Identity
	identitiesMap   map[string]*Identity
	identitiesIndex bleve.Index
}

type RBACSupport struct {
	mu sync.RWMutex

	// Permission management
	permissions      []ProviderPermission
	permissionsMap   map[string]*ProviderPermission // map is a pointer back to the permissions list
	permissionsIndex bleve.Index

	// Role management
	roles      []ProviderRole
	rolesMap   map[string]*ProviderRole // map is a pointer back to the roles list
	rolesIndex bleve.Index

	// Resource management
	resources      []ProviderResource
	resourcesMap   map[string]*ProviderResource // map is a pointer back to the resources list
	resourcesIndex bleve.Index
}

type TenantsSupport struct {
	mu sync.RWMutex

	// Tenant management
	tenants      []ProviderTenant
	tenantsMap   map[string]*ProviderTenant
	tenantsIndex bleve.Index
}

func NewBaseProvider(identifier string, provider Provider, capabilities *ProviderCapabilities) *BaseProvider {

	// Lets setup the capabilities first
	if capabilities == nil {
		capabilities = NewProviderCapabilities()
	}

	base := BaseProvider{
		identifier:   identifier,
		name:         provider.Name,
		description:  provider.Description,
		provider:     provider.Provider,
		config:       provider.Config,
		role:         provider.Role,
		capabilities: capabilities,
	}

	// Now that our provider has defined capabilities, we need to take
	// into account what the user has decided to enable/disable
	if provider.Capabilities != nil {
		capabilities.Update(provider.Capabilities)
	}

	if base.HasAnyCapability(
		ProviderCapabilityIdentities,
		ProviderCapabilityUsers,
		ProviderCapabilityGroups,
	) {
		// Initialize identities map or other structures if needed
		base.identity = &IdentitySupport{
			identities:    make([]Identity, 0),
			identitiesMap: make(map[string]*Identity),
		}
	}

	if base.HasAnyCapability(
		ProviderCapabilityPermissions,
		ProviderCapabilityRoles,
		ProviderCapabilityResources,
	) {
		// Initialize RBAC structures if needed
		base.rbac = &RBACSupport{
			permissions:    make([]ProviderPermission, 0),
			permissionsMap: make(map[string]*ProviderPermission),

			roles:    make([]ProviderRole, 0),
			rolesMap: make(map[string]*ProviderRole),

			resources:    make([]ProviderResource, 0),
			resourcesMap: make(map[string]*ProviderResource),
		}
	}

	if base.HasAnyCapability(
		ProviderCapabilityTenants,
	) {

		base.tenants = &TenantsSupport{
			tenants:    make([]ProviderTenant, 0),
			tenantsMap: make(map[string]*ProviderTenant),
		}
	}

	return &base
}

func (p *BaseProvider) GetConfig() *BasicConfig {
	return p.config
}

func (p *BaseProvider) SetConfig(config *BasicConfig) {
	p.config = config
}

func (p *BaseProvider) SetPermissions(permissions []ProviderPermission) {
	p.SetPermissionsWithKey(permissions, func(p *ProviderPermission) string {
		return p.Name
	})
}

// Create the permissions map
func (p *BaseProvider) SetPermissionsWithKey(
	permissions []ProviderPermission,
	keyFunc func(p *ProviderPermission) string,
) {
	if p.rbac == nil {
		logrus.Warningln("provider has no permissions support")
		return
	}

	p.rbac.mu.Lock()
	defer p.rbac.mu.Unlock()

	if p.rbac.permissions == nil {
		p.rbac.permissions = make([]ProviderPermission, 0)
	}

	p.rbac.permissions = permissions

	// Create the permissions map
	p.rbac.permissionsMap = make(map[string]*ProviderPermission)
	for i := range permissions {
		perm := &permissions[i]
		keyName := keyFunc(perm)
		p.rbac.permissionsMap[strings.ToLower(keyName)] = perm
	}

	// Trigger reindex
	go func() {
		err := p.buildPermissionIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build rbac search indices")
			return
		}
	}()
}

func (p *BaseProvider) AddPermissions(permissions ...ProviderPermission) {
	// Take existing permissions and append new ones

	if p.rbac == nil {
		logrus.Warningln("provider has no permissions support")
		return
	}

	existing := p.rbac.permissions

	if existing == nil {
		existing = make([]ProviderPermission, 0)
	}

	combined := append(existing, FilterDuplicates(
		permissions,
		p.rbac.permissionsMap,
		func(p ProviderPermission) []string {
			return []string{p.Name}
		},
	)...)
	p.SetPermissions(combined)
}

func (p *BaseProvider) SetRoles(roles []ProviderRole) {
	p.SetRolesWithKey(roles, CreateKeysFromRoles)
}

func CreateKeysFromRoles(r ProviderRole) []string {
	return []string{r.ID, r.Name}
}

func (p *BaseProvider) SetRolesWithKey(
	roles []ProviderRole,
	keyFunc func(r ProviderRole) []string) {

	if p.rbac == nil {
		logrus.Warningln("provider has no roles support")
		return
	}

	p.rbac.mu.Lock()
	defer p.rbac.mu.Unlock()

	if p.rbac.roles == nil {
		p.rbac.roles = make([]ProviderRole, 0)
	}

	p.rbac.roles = roles

	// Create the roles map
	p.rbac.rolesMap = make(map[string]*ProviderRole)
	for i := range roles {
		role := roles[i]
		keyNames := keyFunc(role)
		for _, keyName := range keyNames {
			p.rbac.rolesMap[strings.ToLower(keyName)] = &role
		}
	}

	// Trigger reindex
	go func() {
		err := p.buildRoleIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build role search indices")
			return
		}
	}()
}

func (p *BaseProvider) AddRoles(roles ...ProviderRole) {
	// Take existing roles and append new ones
	if p.rbac == nil {
		logrus.Warningln("provider has no roles support")
		return
	}

	existing := p.rbac.roles

	if existing == nil {
		existing = make([]ProviderRole, 0)
	}

	combined := append(existing, FilterDuplicates(
		roles,
		p.rbac.rolesMap,
		CreateKeysFromRoles,
	)...)
	p.SetRoles(combined)
}

func (p *BaseProvider) SetResources(resources []ProviderResource) {
	p.SetResourcesWithKey(resources, CreateKeysFromResources)
}

func CreateKeysFromResources(r ProviderResource) []string {
	return []string{r.ID, r.Name}
}

func (p *BaseProvider) SetResourcesWithKey(
	resources []ProviderResource,
	keyFunc func(r ProviderResource) []string,
) {

	if p.rbac == nil {
		logrus.Warningln("provider has no resources support")
		return
	}

	p.rbac.mu.Lock()
	defer p.rbac.mu.Unlock()

	if p.rbac.resources == nil {
		p.rbac.resources = make([]ProviderResource, 0)
	}

	p.rbac.resources = resources

	// Create the resources map
	p.rbac.resourcesMap = make(map[string]*ProviderResource)
	for i := range resources {
		resource := resources[i]
		keyNames := keyFunc(resource)
		for _, keyName := range keyNames {
			p.rbac.resourcesMap[strings.ToLower(keyName)] = &resource
		}
	}

	// Trigger reindex
	go func() {
		err := p.buildResourceIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build resources search indices")
			return
		}
	}()
}

func (p *BaseProvider) AddResources(resources ...ProviderResource) {
	// Take existing resources and append new ones
	if p.rbac == nil {
		logrus.Warningln("provider has no resources support")
		return
	}
	existing := p.rbac.resources

	if existing == nil {
		existing = make([]ProviderResource, 0)
	}

	combined := append(existing, FilterDuplicates(
		resources,
		p.rbac.resourcesMap,
		CreateKeysFromResources,
	)...)
	p.SetResources(combined)
}

func FilterDuplicates[T any](items []T, existing map[string]*T, keyFunc func(i T) []string) []T {

	var result []T

	for _, item := range items {

		keys := keyFunc(item)
		found := false

		for _, key := range keys {
			lowerKey := strings.ToLower(key)
			if len(lowerKey) == 0 {
				continue
			}
			if _, exists := existing[lowerKey]; exists {
				found = true
				break
			}
		}

		if !found {
			result = append(result, item)
		}
	}

	return result
}

func CreateKeysFromIdentity(i Identity) []string {
	var keys []string
	keys = append(keys, i.ID)
	keys = append(keys, i.Label)
	if i.User != nil && len(i.User.Email) != 0 {
		keys = append(keys, i.User.Email)
	}
	if i.Group != nil {
		if len(i.Group.Name) != 0 {
			keys = append(keys, i.Group.Name)
		}
		if len(i.Group.Email) != 0 {
			keys = append(keys, i.Group.Email)
		}
	}
	return keys
}

func (p *BaseProvider) SetIdentities(identities []Identity) {
	p.SetIdentitiesWithKey(identities, CreateKeysFromIdentity)
}

func (p *BaseProvider) SetIdentitiesWithKey(
	identities []Identity,
	keyFunc func(i Identity) []string,
) {

	if p.identity == nil {
		logrus.Warningln("provider has no identity support")
		return
	}

	p.identity.mu.Lock()
	defer p.identity.mu.Unlock()

	if p.identity.identities == nil {
		p.identity.identities = make([]Identity, 0)
	}

	p.identity.identities = identities

	// Build the identities map
	for i := range identities {

		identity := identities[i]

		keys := keyFunc(identity)

		for _, key := range keys {
			p.identity.identitiesMap[strings.ToLower(key)] = &identity
		}
	}

	// Trigger reindex
	go func() {
		err := p.buildIdentitiyIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build identity search indices")
			return
		}
	}()
}

func (p *BaseProvider) AddIdentities(identities ...Identity) {
	// Take existing identities and append new ones
	if p.identity == nil {
		logrus.Warningln("provider has no identity support")
		return
	}

	existing := p.identity.identities
	if existing == nil {
		existing = make([]Identity, 0)
	}

	combined := append(existing, FilterDuplicates(
		identities,
		p.identity.identitiesMap,
		CreateKeysFromIdentity,
	)...)
	p.SetIdentities(combined)
}

func (p *BaseProvider) GetIdentifier() string {
	return p.identifier
}

func (p *BaseProvider) GetName() string {
	return p.name
}

func (p *BaseProvider) GetDescription() string {
	return p.description
}

func (p *BaseProvider) GetProvider() string {
	return p.provider
}

func (p *BaseProvider) Initialize(identifier string, provider Provider) error {
	// Initialize the provider
	return nil
}
