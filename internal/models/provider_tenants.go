package models

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
)

type ProviderTenantsResponse struct {
	Version  string                         `json:"version"`
	Provider string                         `json:"provider"`
	Tenants  []SearchResult[ProviderTenant] `json:"tenants"`
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
		logrus.Warningln("provider has no tenants support for provider: ", p.GetIdentifier())
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

func (p *BaseProvider) ListTenants(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[ProviderTenant], error) {

	if p.tenants == nil || !p.HasCapability(
		ProviderCapabilityTenants,
	) {
		logrus.Warningln("provider has no tenants support for provider: ", p.GetIdentifier())
		return nil, fmt.Errorf("provider has no tenants support")
	}

	// If no filters, return all tenants
	if searchRequest == nil || searchRequest.IsEmpty() {
		p.tenants.mu.RLock()
		tenants := p.tenants.tenants
		p.tenants.mu.RUnlock()
		return ReturnSearchResults(tenants), nil
	}

	// Check if search index is ready
	p.tenants.mu.RLock()
	tenantsIndex := p.tenants.tenantsIndex
	tenants := p.tenants.tenants
	p.tenants.mu.RUnlock()

	if tenantsIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, tenantsIndex, func(a *search.DocumentMatch, b ProviderTenant) bool {
			return strings.EqualFold(a.ID, b.ID)
		}, tenants, searchRequest)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []ProviderTenant
	filterText := strings.ToLower(strings.Join(searchRequest.Terms, " "))
	limit := searchRequest.GetLimit()

	for _, tenant := range tenants {
		// Check if any filter matches the tenant name or ID
		if strings.Contains(strings.ToLower(tenant.Name), filterText) ||
			strings.Contains(strings.ToLower(tenant.ID), filterText) {
			filtered = append(filtered, tenant)
			if limit > 0 && len(filtered) >= limit {
				break
			}
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
		logrus.Warningln("provider has no tenants support for provider: ", p.GetIdentifier())
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

	logrus.WithFields(logrus.Fields{
		"total_tenants": len(p.tenants.tenants),
	}).Debug("Set provider tenants for provider: ", p.GetIdentifier())

	// Trigger reindex
	go func() {
		err := p.buildTenantIndices()
		if err != nil {
			logrus.WithError(err).Error("Failed to build tenant search indices for provider: ", p.GetIdentifier())
			return
		}
	}()
}

func (p *BaseProvider) AddTenants(tenants ...ProviderTenant) {
	// Take existing tenants and append new ones
	if p.tenants == nil {
		logrus.Warningln("provider has no tenants support for provider: ", p.GetIdentifier())
		return
	}

	// Hold a single write lock for the entire read-modify-write to prevent
	// concurrent Add* calls from overwriting each other's changes (TOCTOU race).
	p.tenants.mu.Lock()

	if p.tenants.tenants == nil {
		p.tenants.tenants = make([]ProviderTenant, 0)
	}
	if p.tenants.tenantsMap == nil {
		p.tenants.tenantsMap = make(map[string]*ProviderTenant)
	}

	filtered := FilterDuplicates(
		tenants,
		p.tenants.tenantsMap,
		CreateKeysFromTenants,
	)

	existingCount := len(p.tenants.tenants)

	if len(filtered) > 0 {
		p.tenants.tenants = append(p.tenants.tenants, filtered...)

		// Update the map in place for the newly added tenants.
		for i := range filtered {
			tenant := &p.tenants.tenants[existingCount+i]
			for _, key := range CreateKeysFromTenants(filtered[i]) {
				p.tenants.tenantsMap[strings.ToLower(key)] = tenant
			}
		}
	}

	totalCount := len(p.tenants.tenants)
	p.tenants.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"existing": existingCount,
		"new":      len(tenants),
		"added":    len(filtered),
		"total":    totalCount,
	}).Debug("Adding tenants to provider: ", p.GetIdentifier())

	if len(filtered) > 0 {
		// Trigger reindex asynchronously (buildTenantIndices acquires its own lock).
		go func() {
			if err := p.buildTenantIndices(); err != nil {
				logrus.WithError(err).Error("Failed to build tenant search indices for provider: ", p.GetIdentifier())
			}
		}()
	}
}

func (p *BaseProvider) buildTenantIndices() error {
	// Placeholder for building tenant indices
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Built tenant search indices in %s", elapsed)
	}()

	tenantsMapping := bleve.NewIndexMapping()

	// Create a document mapping for the Tenant
	tenantDocMapping := bleve.NewDocumentMapping()

	// Field: ID (keyword — preserved as-is for exact/prefix/wildcard matching)
	idFieldMapping := bleve.NewTextFieldMapping()
	idFieldMapping.Analyzer = "keyword"
	tenantDocMapping.AddFieldMappingsAt("ID", idFieldMapping)

	// Field: Name (standard analyzer — tokenises and lowercases so partial
	// word queries like "prod" match "Production Account")
	nameFieldMapping := bleve.NewTextFieldMapping()
	nameFieldMapping.Analyzer = "standard"
	tenantDocMapping.AddFieldMappingsAt("Name", nameFieldMapping)

	tenantsMapping.DefaultMapping = tenantDocMapping

	tenantsIndex, err := bleve.NewMemOnly(tenantsMapping)
	if err != nil {
		logrus.WithError(err).Error("Failed to create tenant search index")
		return fmt.Errorf("failed to create tenants search index: %v", err)
	}

	// Index tenants
	p.tenants.mu.RLock()
	tenants := p.tenants.tenants
	p.tenants.mu.RUnlock()

	for _, tenant := range tenants {
		if err := tenantsIndex.Index(tenant.ID, tenant); err != nil {
			logrus.WithError(err).Errorf("Failed to index tenant %s for provider: %s", tenant.ID, p.GetIdentifier())
			return fmt.Errorf("failed to index tenant %s: %v", tenant.ID, err)
		}
	}

	p.tenants.mu.Lock()
	p.tenants.tenantsIndex = tenantsIndex
	p.tenants.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"tenants": len(tenants),
	}).Debug("Tenant search indices ready for provider: ", p.GetIdentifier())

	return nil
}
