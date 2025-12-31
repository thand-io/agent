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

	for _, role := range roles {
		// Check if any filter matches the role name
		if strings.Contains(strings.ToLower(role.Name), filterText) {
			filtered = append(filtered, role)
		}
	}

	return ReturnSearchResults(filtered), nil
}
