package models

import (
	"strings"
	"sync"

	"github.com/blevesearch/bleve/v2"
)

type BaseProvider struct {
	identifier   string
	name         string
	description  string
	provider     string
	config       *BasicConfig
	role         *Role
	capabilities *ProviderCapabilities

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
