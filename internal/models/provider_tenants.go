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
