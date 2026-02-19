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
	p.rbac.mu.RLock()
	defer p.rbac.mu.RUnlock()

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
		p.rbac.mu.RLock()
		permissions := p.rbac.permissions
		p.rbac.mu.RUnlock()
		return ReturnSearchResults(permissions), nil
	}

	// Check if search index is ready
	p.rbac.mu.RLock()
	permissionsIndex := p.rbac.permissionsIndex
	permissions := p.rbac.permissions
	p.rbac.mu.RUnlock()

	if permissionsIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, permissionsIndex, func(a *search.DocumentMatch, b ProviderPermission) bool {
			return strings.EqualFold(a.ID, b.Name)
		}, permissions, searchReq)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []ProviderPermission
	filterText := strings.ToLower(strings.Join(searchReq.Terms, " "))
	limit := searchReq.GetLimit()

	for _, perm := range permissions {
		// Check if any filter matches the permission name or description
		if strings.Contains(strings.ToLower(perm.Name), filterText) ||
			strings.Contains(strings.ToLower(perm.Description), filterText) {
			filtered = append(filtered, perm)
			if limit > 0 && len(filtered) >= limit {
				break
			}
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

	p.rbac.mu.RLock()
	existing := p.rbac.permissions

	if existing == nil {
		existing = make([]ProviderPermission, 0)
	}

	// Make a copy to avoid data races when appending
	existingCopy := make([]ProviderPermission, len(existing))
	copy(existingCopy, existing)

	filtered := FilterDuplicates(
		permissions,
		p.rbac.permissionsMap,
		func(p ProviderPermission) []string {
			return []string{p.Name}
		},
	)
	p.rbac.mu.RUnlock()

	combined := append(existingCopy, filtered...)
	p.SetPermissions(combined)
}
