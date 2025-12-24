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

type ProviderResourcesResponse struct {
	Version   string                           `json:"version"`
	Provider  string                           `json:"provider"`
	Resources []SearchResult[ProviderResource] `json:"resources"`
}

func (p *BaseProvider) SynchronizeResources(ctx context.Context, req *SynchronizeResourcesRequest) (*SynchronizeResourcesResponse, error) {
	return nil, ErrNotImplemented
}

func (p *BaseProvider) GetResource(ctx context.Context, resource string) (*ProviderResource, error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityResources,
	) {
		logrus.Warningln("provider has no resources")
		return nil, fmt.Errorf("provider has no resources")
	}

	// If the resource is a policy arn: arn:aws:iam::aws:policy/AdministratorAccess
	// Then parse the resource and extract the policy name and convert it to a resource name
	resource = strings.ToLower(resource)

	// Fast map lookup
	if r, exists := p.rbac.resourcesMap[resource]; exists {
		return r, nil
	}

	return nil, fmt.Errorf("resource not found")
}

func (p *BaseProvider) ListResources(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[ProviderResource], error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityResources,
	) {
		logrus.Warningln("provider has no resources")
		return nil, fmt.Errorf("provider has no resources")
	}

	// If no filters, return all resources
	if searchRequest == nil || searchRequest.IsEmpty() {
		return ReturnSearchResults(p.rbac.resources), nil
	}

	// Check if search index is ready
	p.rbac.mu.RLock()
	resourcesIndex := p.rbac.resourcesIndex
	p.rbac.mu.RUnlock()

	if resourcesIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, resourcesIndex, func(a *search.DocumentMatch, b ProviderResource) bool {
			return strings.Compare(a.ID, b.Name) == 0
		}, p.rbac.resources, searchRequest)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []ProviderResource
	filterText := strings.ToLower(strings.Join(searchRequest.Terms, " "))

	for _, resource := range p.rbac.resources {
		// Check if any filter matches the resource name
		if strings.Contains(strings.ToLower(resource.Name), filterText) {
			filtered = append(filtered, resource)
		}
	}

	return ReturnSearchResults(filtered), nil
}

func (p *BaseProvider) buildResourceIndices() error {
	// Placeholder for building indices
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Built resource search indices in %s", elapsed)
	}()

	resourceMapping := bleve.NewIndexMapping()
	resourceIndex, err := bleve.NewMemOnly(resourceMapping)
	if err != nil {
		return fmt.Errorf("failed to create resource search index: %v", err)
	}

	// Index resources
	for _, resource := range p.rbac.resources {
		if err := resourceIndex.Index(resource.ID, resource); err != nil {
			return fmt.Errorf("failed to index resource %s: %v", resource.ID, err)
		}
	}

	p.rbac.mu.Lock()
	p.rbac.resourcesIndex = resourceIndex
	p.rbac.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"resources": len(p.rbac.resources),
	}).Debug("Resource search indices ready")

	return nil
}
