package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
)

type ProviderTenantsResponse struct {
	Version   string                         `json:"version"`
	Provider  string                         `json:"provider"`
	Tenants   []SearchResult[ProviderTenant] `json:"tenants"`
	HasMore   bool                           `json:"has_more"`
	NextToken string                         `json:"next_token,omitempty"`
	Total     int                            `json:"total,omitempty"`
}

type ProviderTenant struct {
	ID     string `json:"id"`
	Parent string `json:"parent,omitempty"`
	Type   string `json:"type,omitempty"` // account, folder, organization, etc.
	Name   string `json:"name"`
	Tenant any    `json:"-"` // Underlying provider-specific tenant object
}

func (p ProviderTenant) String() string {
	return fmt.Sprintf("%s (%s)", p.Name, p.ID)
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

	if p.tenants == nil || !p.HasCapability(
		ProviderCapabilityTenants,
	) {
		logrus.Warningln("provider has no tenants support")
		return nil, fmt.Errorf("provider has no tenants support")
	}

	tenant = strings.ToLower(tenant)
	// Fast map lookup
	p.tenants.mu.RLock()
	defer p.tenants.mu.RUnlock()

	if tnt, exists := p.tenants.tenantsMap[tenant]; exists {
		return tnt, nil
	}
	return nil, fmt.Errorf("tenant not found")
}

func (p *BaseProvider) ListTenants(ctx context.Context, searchReq *SearchRequest) ([]SearchResult[ProviderTenant], error) {

	if p.tenants == nil || !p.HasCapability(
		ProviderCapabilityTenants,
	) {
		logrus.Warningln("provider has no tenants support")
		return nil, fmt.Errorf("provider has no tenants support")
	}

	p.tenants.mu.RLock()
	tenants := p.tenants.tenants
	tenantsIndex := p.tenants.tenantsIndex
	p.tenants.mu.RUnlock()

	var results []ProviderTenant

	// If no filters, use all tenants
	if searchReq == nil || searchReq.IsEmpty() {
		results = tenants
	} else if tenantsIndex != nil {
		// Use Bleve search for better search capabilities
		searchResults, err := BleveListSearch(ctx, tenantsIndex, func(a *search.DocumentMatch, b ProviderTenant) bool {
			return strings.EqualFold(a.ID, b.Name)
		}, tenants, searchReq)
		if err != nil {
			return nil, err
		}
		// Extract the actual results from SearchResult
		for _, sr := range searchResults {
			results = append(results, sr.Result)
		}
	} else {
		// Fallback to simple substring filtering while index is being built
		filterText := strings.ToLower(strings.Join(searchReq.Terms, " "))
		for _, tnt := range tenants {
			if strings.Contains(strings.ToLower(tnt.Name), filterText) {
				results = append(results, tnt)
				if len(results) >= searchReq.GetLimit() {
					break
				}
			}
		}
	}

	return ReturnSearchResults(results), nil
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

	p.tenants.mu.RLock()
	existing := p.tenants.tenants

	if existing == nil {
		existing = make([]ProviderTenant, 0)
	}

	// Make a copy to avoid data races when appending
	existingCopy := make([]ProviderTenant, len(existing))
	copy(existingCopy, existing)

	filtered := FilterDuplicates(
		tenants,
		p.tenants.tenantsMap,
		CreateKeysFromTenants,
	)
	p.tenants.mu.RUnlock()

	combined := append(existingCopy, filtered...)
	p.SetTenants(combined)
}
