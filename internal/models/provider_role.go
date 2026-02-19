package models

import (
	"context"
	"fmt"
	"strings"

	"github.com/blevesearch/bleve/v2/search"
	"github.com/sirupsen/logrus"
)

type ProviderIdentitiesResponse struct {
	Version    string                   `json:"version"`
	Provider   string                   `json:"provider"`
	Identities []SearchResult[Identity] `json:"identities"`
}

type ProviderRolesResponse struct {
	Version  string                       `json:"version"`
	Provider string                       `json:"provider"`
	Roles    []SearchResult[ProviderRole] `json:"roles"`
}

type ProviderRole struct {
	ID          string `json:"id,omitempty"`
	Tenant      string `json:"tenant,omitempty"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`

	// Store the underlying provider-specific role object if needed
	Role any `json:"-"`
}

func (p *BaseProvider) SynchronizeRoles(
	ctx context.Context,
	req *SynchronizeRolesRequest,
) (*SynchronizeRolesResponse, error) {
	return nil, ErrNotImplemented
}

func (p *BaseProvider) GetRole(ctx context.Context, role string) (*ProviderRole, error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityRoles,
	) {
		logrus.Warningln("provider has no roles")
		return nil, fmt.Errorf("provider has no roles")
	}

	// If the role is a policy arn: arn:aws:iam::aws:policy/AdministratorAccess
	// Then parse the role and extract the policy name and convert it to a role
	role = strings.TrimPrefix(role, "arn:aws:iam::aws:policy/")
	role = strings.ToLower(role)

	// Fast map lookup
	p.rbac.mu.RLock()
	defer p.rbac.mu.RUnlock()

	if r, exists := p.rbac.rolesMap[role]; exists {
		return r, nil
	}

	return nil, fmt.Errorf("role not found")
}

func (p *BaseProvider) ListRoles(
	ctx context.Context,
	searchRequest *SearchRequest,
) ([]SearchResult[ProviderRole], error) {

	if p.rbac == nil || !p.HasCapability(
		ProviderCapabilityRoles,
	) {
		logrus.Warningln("provider has no roles")
		return nil, fmt.Errorf("provider has no roles")
	}

	// If no filters, return all roles
	if searchRequest == nil || searchRequest.IsEmpty() {
		p.rbac.mu.RLock()
		roles := p.rbac.roles
		p.rbac.mu.RUnlock()
		return ReturnSearchResults(roles), nil
	}

	// Check if search index is ready
	p.rbac.mu.RLock()
	rolesIndex := p.rbac.rolesIndex
	roles := p.rbac.roles
	p.rbac.mu.RUnlock()

	if rolesIndex != nil {
		// Use Bleve search for better search capabilities
		return BleveListSearch(ctx, rolesIndex, func(a *search.DocumentMatch, b ProviderRole) bool {
			return strings.Compare(a.ID, b.Name) == 0
		}, roles, searchRequest)
	}

	// Fallback to simple substring filtering while index is being built
	var filtered []ProviderRole
	filterText := strings.ToLower(strings.Join(searchRequest.Terms, " "))
	limit := searchRequest.GetLimit()

	for _, role := range roles {
		// Check if any filter matches the role name
		if strings.Contains(strings.ToLower(role.Name), filterText) {
			filtered = append(filtered, role)
			if limit > 0 && len(filtered) >= limit {
				break
			}
		}
	}

	return ReturnSearchResults(filtered), nil
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

	p.rbac.mu.RLock()
	existing := p.rbac.roles

	if existing == nil {
		existing = make([]ProviderRole, 0)
	}

	// Make a copy to avoid data races when appending
	existingCopy := make([]ProviderRole, len(existing))
	copy(existingCopy, existing)

	filtered := FilterDuplicates(
		roles,
		p.rbac.rolesMap,
		CreateKeysFromRoles,
	)
	p.rbac.mu.RUnlock()

	combined := append(existingCopy, filtered...)
	p.SetRoles(combined)
}
