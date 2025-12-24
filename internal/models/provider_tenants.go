package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
)

type ProviderTenantsResponse struct {
	Version  string                         `json:"version"`
	Provider string                         `json:"provider"`
	Tenants  []SearchResult[ProviderTenant] `json:"tenants"`
}

type ProviderTenant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ProviderTenants interface {
	GetTenant(ctx context.Context, tenant string) (*ProviderTenant, error)
	ListTenants(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[ProviderTenant], error)

	// Synchronize tenants from the provider
	SynchronizeTenants(ctx context.Context, req *SynchronizeTenantsRequest) (*SynchronizeTenantsResponse, error)

	// Overrides all existing resources with the provided list
	SetTenants(tenants []ProviderTenant)
	// Appends new resources to the existing list
	AddTenants(tenant ...ProviderTenant)
}

func (p *BaseProvider) GetTenant(ctx context.Context, tenant string) (*ProviderTenant, error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityPermissions,
	) {
		logrus.Warningln("provider has no permissions")
		return nil, fmt.Errorf("provider has no permissions")
	}

	tenant = strings.ToLower(tenant)
	// Fast map lookup
	if tnt, exists := p.tenants.tenantsMap[tenant]; exists {
		return tnt, nil
	}
	return nil, fmt.Errorf("tenant not found")
}

func (p *BaseProvider) ListTenants(ctx context.Context, searchReq *SearchRequest) ([]SearchResult[ProviderTenant], error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityTenants,
	) {
		logrus.Warningln("provider has no permissions")
		return nil, fmt.Errorf("provider has no permissions")
	}

	// If no filters, return all permissions
	if searchReq == nil || searchReq.IsEmpty() {
		return ReturnSearchResults(p.tenants.tenants), nil
	}

	// Check if search index is ready
	p.tenants.mu.RLock()
	tenantsIndex := p.tenants.tenantsIndex
	p.rbac.mu.RUnlock()

	if tenantsIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, tenantsIndex, func(a *search.DocumentMatch, b ProviderTenant) bool {
			return strings.EqualFold(a.ID, b.Name)
		}, p.tenants.tenants, searchReq)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []ProviderTenant
	filterText := strings.ToLower(strings.Join(searchReq.Terms, " "))

	for _, tnt := range p.tenants.tenants {
		// Check if any filter matches the tenant name or description
		if strings.Contains(strings.ToLower(tnt.Name), filterText) {
			filtered = append(filtered, tnt)
		}
	}

	return ReturnSearchResults(filtered), nil
}

func (p *BaseProvider) SynchronizeTenants(
	ctx context.Context,
	req *SynchronizeTenantsRequest,
) (*SynchronizeTenantsResponse, error) {
	return nil, ErrNotImplemented
}

func (p *BaseProvider) SetTenants(tenants []ProviderTenant) {
	p.SetTenantsWithKey(tenants, CreateKeysFromTenants)
}

func CreateKeysFromTenants(t ProviderTenant) []string {
	return []string{t.ID, t.Name}
}

func (p *BaseProvider) SetTenantsWithKey(
	tenants []ProviderTenant,
	keyFunc func(t ProviderTenant) []string) {

	if p.tenants == nil {
		logrus.Warningln("provider has no tenants support")
		return
	}

	p.tenants.mu.Lock()
	defer p.tenants.mu.Unlock()

	if p.tenants.tenants == nil {
		p.tenants.tenants = make([]ProviderTenant, 0)
	}

	p.tenants.tenants = tenants

	// Create the tenants map
	p.tenants.tenantsMap = make(map[string]*ProviderTenant)
	for i := range tenants {
		tenant := tenants[i]
		keyNames := keyFunc(tenant)
		for _, keyName := range keyNames {
			p.tenants.tenantsMap[strings.ToLower(keyName)] = &tenant
		}
	}

	// Trigger reindex
	go func() {
		err := p.buildTenantIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build tenant search indices")
			return
		}
	}()
}

func (p *BaseProvider) AddTenants(tenants ...ProviderTenant) {
	// Take existing tenants and append new ones
	if p.tenants == nil {
		logrus.Warningln("provider has no tenants support")
		return
	}

	existing := p.tenants.tenants

	if existing == nil {
		existing = make([]ProviderTenant, 0)
	}

	combined := append(existing, FilterDuplicates(
		tenants,
		p.tenants.tenantsMap,
		CreateKeysFromTenants,
	)...)
	p.SetTenants(combined)
}
