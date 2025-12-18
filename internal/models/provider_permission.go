package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
)

type ProviderPermissionsResponse struct {
	Version     string                             `json:"version"`
	Provider    string                             `json:"provider"`
	Permissions []SearchResult[ProviderPermission] `json:"permissions"`
}

type ProviderPermission struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description"`

	// Store the underlying provider-specific permission object if needed
	Permission any `json:"-"`
}

func (p *BaseProvider) GetPermission(ctx context.Context, permission string) (*ProviderPermission, error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityPermissions,
	) {
		logrus.Warningln("provider has no permissions")
		return nil, fmt.Errorf("provider has no permissions")
	}

	permission = strings.ToLower(permission)
	// Fast map lookup
	if perm, exists := p.rbac.permissionsMap[permission]; exists {
		return perm, nil
	}
	return nil, fmt.Errorf("permission not found")
}

func (p *BaseProvider) ListPermissions(ctx context.Context, searchReq *SearchRequest) ([]SearchResult[ProviderPermission], error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityPermissions,
	) {
		logrus.Warningln("provider has no permissions")
		return nil, fmt.Errorf("provider has no permissions")
	}

	// If no filters, return all permissions
	if searchReq == nil || searchReq.IsEmpty() {
		return ReturnSearchResults(p.rbac.permissions), nil
	}

	// Check if search index is ready
	p.rbac.mu.RLock()
	permissionsIndex := p.rbac.permissionsIndex
	p.rbac.mu.RUnlock()

	if permissionsIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, permissionsIndex, func(a *search.DocumentMatch, b ProviderPermission) bool {
			return strings.EqualFold(a.ID, b.Name)
		}, p.rbac.permissions, searchReq)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []ProviderPermission
	filterText := strings.ToLower(strings.Join(searchReq.Terms, " "))

	for _, perm := range p.rbac.permissions {
		// Check if any filter matches the permission name or description
		if strings.Contains(strings.ToLower(perm.Name), filterText) ||
			strings.Contains(strings.ToLower(perm.Description), filterText) {
			filtered = append(filtered, perm)
		}
	}

	return ReturnSearchResults(filtered), nil
}

func (p *BaseProvider) SynchronizePermissions(
	ctx context.Context,
	req *SynchronizePermissionsRequest,
) (*SynchronizePermissionsResponse, error) {
	return nil, ErrNotImplemented
}
