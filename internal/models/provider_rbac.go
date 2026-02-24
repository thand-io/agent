package models

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/sirupsen/logrus"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

type AuthorizeRoleRequest struct {
	*RoleRequest
}

type AuthorizeRoleResponse struct {
	UserId      string         `json:"user_id,omitempty"`     // The ID of the user the role was authorized for
	Roles       []string       `json:"roles,omitempty"`       // The roles that were authorized
	Permissions []string       `json:"permissions,omitempty"` // The permissions that were authorized
	Groups      []string       `json:"groups,omitempty"`      // The groups that were authorized
	Resources   []string       `json:"resources,omitempty"`   // The resources that were authorized
	Metadata    map[string]any `json:"metadata,omitempty"`    // Any metadata returned from the provider
}

type RevokeRoleRequest struct {
	*RoleRequest
	AuthorizeRoleResponse *AuthorizeRoleResponse `json:"response,omitempty"`
}

type RevokeRoleResponse struct {
}

type SynchronizeTenantsRequest struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizeTenantsResponse struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
	Tenants    []ProviderTenant   `json:"tenants,omitempty"`
}

type SynchronizeRolesRequest struct {
	Tenant     string             `json:"tenant,omitempty"`
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizeRolesResponse struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
	Roles      []ProviderRole     `json:"roles,omitempty"`
}

type SynchronizePermissionsRequest struct {
	Tenant     string             `json:"tenant,omitempty"`
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizePermissionsResponse struct {
	Pagination  *PaginationOptions   `json:"pagination,omitempty"`
	Permissions []ProviderPermission `json:"permissions,omitempty"`
}

type SynchronizeUsersRequest struct {
	Tenant     string             `json:"tenant,omitempty"`
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizeUsersResponse struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
	Identities []Identity         `json:"identities,omitempty"`
}

type SynchronizeGroupsRequest struct {
	Tenant     string             `json:"tenant,omitempty"`
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizeGroupsResponse struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
	Identities []Identity         `json:"identities,omitempty"`
}

type SynchronizeResourcesRequest struct {
	Tenant     string             `json:"tenant,omitempty"`
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizeResourcesResponse struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
	Resources  []ProviderResource `json:"resources,omitempty"`
}

type SynchronizeIdentitiesRequest struct {
	Tenant     string             `json:"tenant,omitempty"`
	Pagination *PaginationOptions `json:"pagination,omitempty"`
}

type SynchronizeIdentitiesResponse struct {
	Pagination *PaginationOptions `json:"pagination,omitempty"`
	Identities []Identity         `json:"identities,omitempty"`
}

type PaginationOptions struct {
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"size,omitempty"`
	Token    string `json:"token,omitempty"`
}

type SynchronizeRequestImpl interface {
	SetPagination(p *PaginationOptions)
}

type SynchronizeResponseImpl interface {
	GetPagination() *PaginationOptions
}

func (r *SynchronizeRolesRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeRolesResponse) GetPagination() *PaginationOptions  { return r.Pagination }

func (r *SynchronizePermissionsRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizePermissionsResponse) GetPagination() *PaginationOptions  { return r.Pagination }

func (r *SynchronizeUsersRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeUsersResponse) GetPagination() *PaginationOptions  { return r.Pagination }

func (r *SynchronizeGroupsRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeGroupsResponse) GetPagination() *PaginationOptions  { return r.Pagination }

func (r *SynchronizeResourcesRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeResourcesResponse) GetPagination() *PaginationOptions  { return r.Pagination }

func (r *SynchronizeIdentitiesRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeIdentitiesResponse) GetPagination() *PaginationOptions  { return r.Pagination }

func (r *SynchronizeTenantsRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeTenantsResponse) GetPagination() *PaginationOptions  { return r.Pagination }

// ProviderContext is the execution context passed to provider RBAC operations.
// It is an alias for WorkflowTaskSupport so providers receive both the plain
// Go context and, when inside a Temporal workflow coroutine, the workflow.Context
// needed to schedule sub-activities with proper retries.
type ProviderContext = sdkWorkflowsModel.WorkflowTaskSupport

// ProviderRoleBasedAccessControl defines the interface for providers that support RBAC
type ProviderRoleBasedAccessControl interface {

	// Sync or Async load the roles, permissions, resources and identities
	SynchronizeRoles(ctx context.Context, req *SynchronizeRolesRequest) (*SynchronizeRolesResponse, error)
	SynchronizePermissions(ctx context.Context, req *SynchronizePermissionsRequest) (*SynchronizePermissionsResponse, error)
	SynchronizeResources(ctx context.Context, req *SynchronizeResourcesRequest) (*SynchronizeResourcesResponse, error)

	// Overrides all existing roles, with the provided list
	SetRoles(roles []ProviderRole)
	// Appends new roles to the existing list
	AddRoles(role ...ProviderRole)

	// Overrides all existing permissions with the provided list
	SetPermissions(permissions []ProviderPermission)
	// Appends new permissions to the existing list
	AddPermissions(permission ...ProviderPermission)

	// Overrides all existing resources with the provided list
	SetResources(resources []ProviderResource)
	// Appends new resources to the existing list
	AddResources(resource ...ProviderResource)

	// Permissions are individual accesses. Used as part of a role
	GetPermission(ctx context.Context, permission string) (*ProviderPermission, error)
	ListPermissions(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[ProviderPermission], error)

	// Resources are things that permissions can be applied to
	GetResource(ctx context.Context, resource string) (*ProviderResource, error)
	ListResources(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[ProviderResource], error)

	// Role is a collection of permissions or whatever the provider defines as a role
	GetRole(ctx context.Context, role string) (*ProviderRole, error)
	ListRoles(ctx context.Context, searchRequest *SearchRequest) ([]SearchResult[ProviderRole], error)

	// Validate a role for a user
	ValidateRole(ctx context.Context, identity *Identity, role *Role) (map[string]any, error)

	// Authorize a role for a user (Bind a user to a role)
	AuthorizeRole(
		taskSupport sdkWorkflowsModel.WorkflowTaskSupport,
		req *AuthorizeRoleRequest,
	) (
		*AuthorizeRoleResponse, // Return any custom metadata the provider wants to store
		error,
	)

	// Revoke a role from a user
	RevokeRole(
		taskSupport sdkWorkflowsModel.WorkflowTaskSupport,
		req *RevokeRoleRequest, // Any metadata returned from AuthorizeRole
	) (*RevokeRoleResponse, error)

	// When applicable, get the URL to redirect the user to after post-authorize
	GetAuthorizedAccessUrl(
		ctx context.Context,
		req *AuthorizeRoleRequest,
		resp *AuthorizeRoleResponse,
	) string
}

func (p *BaseProvider) AuthorizeRole(
	taskSupport sdkWorkflowsModel.WorkflowTaskSupport,
	req *AuthorizeRoleRequest,
) (*AuthorizeRoleResponse, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement AuthorizeRole", p.GetProvider())
}

func (p *BaseProvider) RevokeRole(
	taskSupport sdkWorkflowsModel.WorkflowTaskSupport,
	req *RevokeRoleRequest,
) (*RevokeRoleResponse, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement RevokeRole", p.GetProvider())
}

func (p *BaseProvider) GetAuthorizedAccessUrl(
	ctx context.Context,
	req *AuthorizeRoleRequest,
	resp *AuthorizeRoleResponse,
) string {
	// Default implementation does nothing
	return ""
}

/* Default implementation for ValidateRole */
func (p *BaseProvider) ValidateRole(ctx context.Context, user *Identity, role *Role) (map[string]any, error) {
	// TODO this won't work as its the base provider. needs to call the actual provider
	// to validate the role
	return nil, ErrNotImplemented
}

// executeUserValidation triggers user approval workflow
func ValidateRole(
	provider Provider,
	elevateRequest ElevateRequestInternal,
) (map[string]any, error) {

	if elevateRequest.User == nil {
		return nil, fmt.Errorf("user information is required for role validation")
	}

	// Check the user has access to the required scopes etc
	identity := Identity{
		ID:    elevateRequest.User.GetIdentity(),
		Label: elevateRequest.User.GetName(),
		User:  elevateRequest.User,
	}

	res, err := provider.ValidateRole(
		context.Background(),
		&identity,
		elevateRequest.Role,
	)

	if err != nil {

		if !errors.Is(err, ErrNotImplemented) {
			return nil, fmt.Errorf("failed to validate role: %w", err)
		}

		logrus.Warn("Provider does not implement role validation, using default")
		err = validateRole(provider, &identity, elevateRequest.Role)

		if err != nil {

			logrus.WithError(err).Warn("Role validation failed")
			return nil, err

		}
	}

	return res, nil
}

func validateRole(provider Provider, _ *Identity, role *Role) error {

	if provider == nil {
		return fmt.Errorf("provider implementation is nil. Ensure the provider is initialized")
	}

	if err := validateRoleNotEmpty(role); err != nil {
		return err
	}

	if err := validateRoleInheritance(provider, role); err != nil {
		return err
	}

	return validateRolePermissions(provider, role)
}

// validateRoleNotEmpty checks if the role has any permissions or inherits from other roles
func validateRoleNotEmpty(role *Role) error {
	if len(role.Permissions.Allow) == 0 &&
		len(role.Permissions.Deny) == 0 &&
		len(role.Inherits) == 0 {
		return fmt.Errorf("role %s has no permissions or inherits", role.Name)
	}
	return nil
}

// validateRoleInheritance validates that all inherited roles exist in the provider
func validateRoleInheritance(provider Provider, role *Role) error {

	if provider == nil {
		return fmt.Errorf("provider implementation is nil. Ensure the provider is initialized")
	}

	if len(role.Inherits) == 0 {
		return nil
	}

	providerRoles, err := provider.ListRoles(context.TODO(), &SearchRequest{})
	if err != nil {
		return err
	}

	if len(providerRoles) == 0 {
		logrus.Warning("No roles found in provider")
		return nil
	}

	return validateInheritedRolesExist(provider, role, providerRoles)
}

// validateInheritedRolesExist checks that all inherited roles exist in the provider
func validateInheritedRolesExist(provider Provider, role *Role, providerRoles []SearchResult[ProviderRole]) error {
	if provider == nil {
		return fmt.Errorf("provider implementation is nil. Ensure the provider is initialized")
	}

	for _, inherit := range role.Inherits {
		if !strings.HasPrefix(inherit, fmt.Sprintf("%s:", provider.GetName())) {
			// This is a local role, skip validation
			continue
		}

		roleExists := slices.ContainsFunc(providerRoles, func(r SearchResult[ProviderRole]) bool {
			return strings.Compare(r.Result.Name, inherit) == 0
		})

		if !roleExists {
			return fmt.Errorf("role %s inherits from non-existent role %s", role.Name, inherit)
		}
	}
	return nil
}

// validateRolePermissions validates that role permissions exist in the provider
func validateRolePermissions(provider Provider, role *Role) error {

	if provider == nil {
		return fmt.Errorf("provider implementation is nil. Ensure the provider is initialized")
	}

	if len(role.Permissions.Allow) == 0 && len(role.Permissions.Deny) == 0 {
		return nil
	}

	providerPermissions, err := provider.ListPermissions(context.TODO(), &SearchRequest{})
	if err != nil {
		return err
	}

	if len(providerPermissions) == 0 {
		logrus.Warning("No permissions found in provider")
		return nil
	}

	return validateRolePermissionLists(role, providerPermissions)
}

// validateRolePermissionLists validates both allow and deny permission lists
func validateRolePermissionLists(role *Role, providerPermissions []SearchResult[ProviderPermission]) error {
	if role == nil {
		return fmt.Errorf("role is nil")
	}

	var err error

	role.Permissions.Allow, err = validatePermissions(providerPermissions, role.Permissions.Allow)
	if err != nil {
		return err
	}

	role.Permissions.Deny, err = validatePermissions(providerPermissions, role.Permissions.Deny)
	if err != nil {
		return err
	}

	return nil
}

func validatePermissions(providerPermissions []SearchResult[ProviderPermission], statements RoleStatements) (RoleStatements, error) {

	validatedStatements := RoleStatements{}

	// Validate each statement
	for _, stmt := range statements {

		validatedOperations := []string{}

		for _, perm := range stmt.Operations {

			if strings.Contains(perm, "*") {
				// Permission contains a wildcard. Expand it against the provider
				// permission list using glob matching so that all delimiter styles
				// are handled: Azure (/), GCP (.), AWS/k8s (:), and mid-path
				// wildcards such as Microsoft.Compute/*/read.
				expanded, err := expandPermissionsWildcard(providerPermissions, perm)
				if err != nil {
					return nil, err
				}
				if len(expanded) == 0 {
					return nil, fmt.Errorf("the wildcard permission: %s matched no permissions", perm)
				}
				validatedOperations = append(validatedOperations, expanded...)

			} else if permission := getCondensedActions(perm); permission != nil {

				// If the last part is delimited by comma, e.g., k8s:pods:get,list,watch
				// lets use a more complex parsing with regex and then expand those
				// into individual permissions
				validatedOperations = append(validatedOperations, permission...)
				// We have a match, now expand it

			} else if !slices.ContainsFunc(providerPermissions, func(p SearchResult[ProviderPermission]) bool {
				found := strings.Compare(p.Result.Name, perm) == 0
				if found {
					validatedOperations = append(validatedOperations, p.Result.Name)
				}
				return found
			}) {
				return nil, fmt.Errorf("the requested permission: %s was not found", perm)
			}
		}

		// Create validated statement with expanded operations
		validatedStatements = append(validatedStatements, Statement{
			Operations: validatedOperations,
			Targets:    stmt.Targets,
			Conditions: stmt.Conditions,
		})
	}

	return validatedStatements, nil
}

func expandPermissionsWildcard(providerPermissions []SearchResult[ProviderPermission], permission string) ([]string, error) {

	expandedPermissions := []string{}

	for _, providerPerm := range providerPermissions {
		matched, err := path.Match(permission, providerPerm.Result.Name)
		if err != nil {
			return nil, fmt.Errorf("invalid wildcard pattern %q: %w", permission, err)
		}
		if matched {
			expandedPermissions = append(expandedPermissions, providerPerm.Result.Name)
		}
	}

	return expandedPermissions, nil
}

// ValidatePermissionsPublic is the exported counterpart of validatePermissions,
// intended for use in external test packages (e.g. integration tests that need
// to import both models and a provider package without creating a cycle).
func ValidatePermissionsPublic(providerPermissions []SearchResult[ProviderPermission], statements RoleStatements) (RoleStatements, error) {
	return validatePermissions(providerPermissions, statements)
}

/*
k8s:pods:get,list,watch
*/
func getCondensedActions(permission string) []string {

	// split on the last colon
	idx := strings.LastIndex(permission, ":")

	if idx == -1 {
		return nil
	}

	resource := permission[:idx]
	actions := permission[idx+1:]

	// Check if the second part contains a comma
	actionParts := strings.Split(actions, ",")

	if len(actionParts) == 0 {
		return nil
	}

	permissions := []string{}

	for _, action := range actionParts {
		permissions = append(permissions, fmt.Sprintf("%s:%s", resource, action))
	}

	return permissions
}

func (p *BaseProvider) buildPermissionIndices() error {
	// Placeholder for building indices
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Built RBAC search indices in %s", elapsed)
	}()

	// Create in-memory Bleve indices
	permissionsMapping := bleve.NewIndexMapping()
	permissionsIndex, err := bleve.NewMemOnly(permissionsMapping)
	if err != nil {
		return fmt.Errorf("failed to create permissions search index: %v", err)
	}

	// Index permissions
	p.rbac.mu.RLock()
	permissions := p.rbac.permissions
	roles := p.rbac.roles
	p.rbac.mu.RUnlock()

	for _, perm := range permissions {
		if err := permissionsIndex.Index(perm.Name, perm); err != nil {
			return fmt.Errorf("failed to index permission %s: %v", perm.Name, err)
		}
	}

	logrus.WithFields(logrus.Fields{
		"permissions": len(permissions),
		"roles":       len(roles),
	}).Debug("RBAC search indices ready")

	p.rbac.mu.Lock()
	p.rbac.permissionsIndex = permissionsIndex
	p.rbac.mu.Unlock()

	return nil
}

func (p *BaseProvider) buildRoleIndices() error {
	// Placeholder for building indices
	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Built role search indices in %s", elapsed)
	}()

	rolesMapping := bleve.NewIndexMapping()

	// Create a document mapping for the Role
	roleDocMapping := bleve.NewDocumentMapping()

	// Field: Name (Exact match or case-insensitive keyword is usually best for Role Names)
	nameFieldMapping := bleve.NewTextFieldMapping()
	nameFieldMapping.Analyzer = "keyword"
	roleDocMapping.AddFieldMappingsAt("Name", nameFieldMapping)

	// Field: Description (Full text search is usually best here)
	descFieldMapping := bleve.NewTextFieldMapping()
	descFieldMapping.Analyzer = "en"
	roleDocMapping.AddFieldMappingsAt("Description", descFieldMapping)

	rolesMapping.DefaultMapping = roleDocMapping

	rolesIndex, err := bleve.NewMemOnly(rolesMapping)
	if err != nil {
		return fmt.Errorf("failed to create roles search index: %v", err)
	}

	// Index roles
	p.rbac.mu.RLock()
	roles := p.rbac.roles
	permissions := p.rbac.permissions
	p.rbac.mu.RUnlock()

	for _, role := range roles {
		if err := rolesIndex.Index(role.Name, role); err != nil {
			return fmt.Errorf("failed to index role %s: %v", role.Name, err)
		}
	}

	p.rbac.mu.Lock()
	p.rbac.rolesIndex = rolesIndex
	p.rbac.mu.Unlock()

	logrus.WithFields(logrus.Fields{
		"permissions": len(permissions),
		"roles":       len(roles),
	}).Debug("RBAC search indices ready")

	return nil
}
