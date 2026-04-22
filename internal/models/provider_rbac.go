package models

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blevesearch/bleve/v2"
	"github.com/sirupsen/logrus"
)

type AuthorizeRoleRequest struct {
	Tenant   *ProviderTenant `json:"tenant,omitempty"`   // Optional tenant ID for multi-account providers
	Identity *Identity       `json:"identity,omitempty"` // User or group identifier
	Role     *CompositeRole  `json:"role,omitempty"`
	Duration *time.Duration  `json:"duration,omitempty"` // Optional duration for temporary access
	Metadata map[string]any  `json:"metadata,omitempty"` // Provider-specific workflow metadata
}

func (r *AuthorizeRoleRequest) IsValid() bool {
	return r.Identity != nil && r.Identity.User != nil && r.Role != nil
}

func (r *AuthorizeRoleRequest) GetUser() *User {
	if r.Identity == nil {
		return nil
	}
	return r.Identity.User
}

func (r *AuthorizeRoleRequest) GetRole() *CompositeRole {
	return r.Role
}

func (r *AuthorizeRoleRequest) GetTenant() *ProviderTenant {
	return r.Tenant
}

func (r *AuthorizeRoleRequest) GetDuration() *time.Duration {
	return r.Duration
}

func (r *AuthorizeRoleRequest) HasTenant() bool {
	return r.Tenant != nil && len(r.Tenant.ID) > 0
}

func CloneAuthorizeRoleRequest(req *AuthorizeRoleRequest) *AuthorizeRoleRequest {
	if req == nil {
		return nil
	}

	clone := *req
	if req.Tenant != nil {
		tenant := *req.Tenant
		clone.Tenant = &tenant
	}
	if req.Identity != nil {
		identity := *req.Identity
		clone.Identity = &identity
	}
	if req.Role != nil {
		role := *req.Role
		clone.Role = &role
	}
	if req.Duration != nil {
		duration := *req.Duration
		clone.Duration = &duration
	}
	if req.Metadata != nil {
		clone.Metadata = maps.Clone(req.Metadata)
	}

	return &clone
}

func CloneRevokeRoleRequest(req *RevokeRoleRequest) *RevokeRoleRequest {
	if req == nil {
		return nil
	}

	clone := &RevokeRoleRequest{
		AuthorizeRoleRequest:  CloneAuthorizeRoleRequest(req.AuthorizeRoleRequest),
		AuthorizeRoleResponse: req.AuthorizeRoleResponse,
	}
	if req.AuthorizeRoleResponse != nil {
		response := *req.AuthorizeRoleResponse
		if req.AuthorizeRoleResponse.Roles != nil {
			response.Roles = append([]string(nil), req.AuthorizeRoleResponse.Roles...)
		}
		if req.AuthorizeRoleResponse.Permissions != nil {
			response.Permissions = append([]string(nil), req.AuthorizeRoleResponse.Permissions...)
		}
		if req.AuthorizeRoleResponse.Groups != nil {
			response.Groups = append([]string(nil), req.AuthorizeRoleResponse.Groups...)
		}
		if req.AuthorizeRoleResponse.Resources != nil {
			response.Resources = append([]string(nil), req.AuthorizeRoleResponse.Resources...)
		}
		if req.AuthorizeRoleResponse.Metadata != nil {
			response.Metadata = maps.Clone(req.AuthorizeRoleResponse.Metadata)
		}
		clone.AuthorizeRoleResponse = &response
	}

	return clone
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
	*AuthorizeRoleRequest
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
	GetPagination() *PaginationOptions
	SetPagination(p *PaginationOptions)
}

type SynchronizeResponseImpl interface {
	GetPagination() *PaginationOptions
	// AddToProvider routes the response data to the correct Add* method on the
	// provider. This is the single place that maps response type → provider
	// store update, shared by both the Temporal and pure-Go sync paths.
	AddToProvider(provider Provider)
	// ResultCount returns the number of items in this page of results.
	ResultCount() int
}

func (r *SynchronizeRolesRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizeRolesRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeRolesResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizeRolesResponse) AddToProvider(p Provider)           { p.AddRoles(r.Roles...) }
func (r SynchronizeRolesResponse) ResultCount() int                   { return len(r.Roles) }

func (r *SynchronizePermissionsRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizePermissionsRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizePermissionsResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizePermissionsResponse) AddToProvider(p Provider)           { p.AddPermissions(r.Permissions...) }
func (r SynchronizePermissionsResponse) ResultCount() int                   { return len(r.Permissions) }

func (r *SynchronizeUsersRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizeUsersRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeUsersResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizeUsersResponse) AddToProvider(p Provider)           { p.AddIdentities(r.Identities...) }
func (r SynchronizeUsersResponse) ResultCount() int                   { return len(r.Identities) }

func (r *SynchronizeGroupsRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizeGroupsRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeGroupsResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizeGroupsResponse) AddToProvider(p Provider)           { p.AddIdentities(r.Identities...) }
func (r SynchronizeGroupsResponse) ResultCount() int                   { return len(r.Identities) }

func (r *SynchronizeResourcesRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizeResourcesRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeResourcesResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizeResourcesResponse) AddToProvider(p Provider)           { p.AddResources(r.Resources...) }
func (r SynchronizeResourcesResponse) ResultCount() int                   { return len(r.Resources) }

func (r *SynchronizeIdentitiesRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizeIdentitiesRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeIdentitiesResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizeIdentitiesResponse) AddToProvider(p Provider)           { p.AddIdentities(r.Identities...) }
func (r SynchronizeIdentitiesResponse) ResultCount() int                   { return len(r.Identities) }

func (r *SynchronizeTenantsRequest) GetPagination() *PaginationOptions  { return r.Pagination }
func (r *SynchronizeTenantsRequest) SetPagination(p *PaginationOptions) { r.Pagination = p }
func (r SynchronizeTenantsResponse) GetPagination() *PaginationOptions  { return r.Pagination }
func (r SynchronizeTenantsResponse) AddToProvider(p Provider)           { p.AddTenants(r.Tenants...) }
func (r SynchronizeTenantsResponse) ResultCount() int                   { return len(r.Tenants) }

// ProviderContext is the execution context passed to provider RBAC operations.
// It is an alias for WorkflowTaskSupport so providers receive both the plain
// Go context and, when inside a Temporal workflow coroutine, the workflow.Context
// needed to schedule sub-activities with proper retries.
type ProviderContext interface {
	Deadline() (deadline time.Time, ok bool)
}

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
		ctx ProviderContext,
		req *AuthorizeRoleRequest,
	) (
		*AuthorizeRoleResponse, // Return any custom metadata the provider wants to store
		error,
	)

	// Revoke a role from a user
	RevokeRole(
		ctx ProviderContext,
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
	ctx ProviderContext,
	req *AuthorizeRoleRequest,
) (*AuthorizeRoleResponse, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement AuthorizeRole", p.GetProvider())
}

func (p *BaseProvider) RevokeRole(
	ctx ProviderContext,
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
	identity := elevateRequest.User.AsIdentity()

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

// ValidateRolePermissions expands wildcard permissions in a role's Allow and Deny lists
// against the given provider's known permission set, then conditionally re-condenses them.
//
// This is the final step of the expand→resolve→condense pipeline: after allow/deny
// conflict resolution, wildcards like "bigquery.datasets.*" are expanded into their
// concrete individual permissions. For providers where SupportsWildcards is true
// (e.g. AWS, Azure) the original wildcard patterns are then restored. For providers
// where SupportsWildcards is false (e.g. GCP, Okta) the permissions remain expanded
// so the cloud API receives fully-qualified permission names.
func ValidateRolePermissions(provider Provider, role *Role) error {
	return validateRolePermissions(provider, role)
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

	// Determine whether this provider's API accepts wildcard patterns.
	supportsWildcards := false
	if caps := provider.GetCapabilities(); caps != nil && caps.Permissions != nil {
		supportsWildcards = caps.Permissions.SupportsWildcards
	}

	return validateRolePermissionLists(role, providerPermissions, supportsWildcards)
}

// validateRolePermissionLists validates both allow and deny permission lists.
// When supportsWildcards is true, validated wildcards are condensed back to
// their original patterns; otherwise they remain fully expanded.
func validateRolePermissionLists(role *Role, providerPermissions []SearchResult[ProviderPermission], supportsWildcards bool) error {
	if role == nil {
		return fmt.Errorf("role is nil")
	}

	var err error

	role.Permissions.Allow, err = validatePermissions(providerPermissions, role.Permissions.Allow, supportsWildcards)
	if err != nil {
		return err
	}

	role.Permissions.Deny, err = validatePermissions(providerPermissions, role.Permissions.Deny, supportsWildcards)
	if err != nil {
		return err
	}

	return nil
}

func validatePermissions(providerPermissions []SearchResult[ProviderPermission], statements RoleStatements, supportsWildcards bool) (RoleStatements, error) {

	validatedStatements := RoleStatements{}

	// Build a set of all known provider permissions once for O(1) lookups.
	providerPermSet := make(map[string]bool, len(providerPermissions))
	for _, p := range providerPermissions {
		providerPermSet[p.Result.Name] = true
	}

	// Validate each statement
	for _, stmt := range statements {

		validatedOperations := []string{}

		// Track original wildcard patterns so we can condense back after
		// validation.  Without this, "ec2:*" would remain expanded into
		// ~500 individual permissions in the validated output.
		originalWildcards := []string{}

		for _, perm := range stmt.Operations {

			if strings.Contains(perm, "*") {
				originalWildcards = append(originalWildcards, perm)
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

			} else if strings.Contains(perm, ":") {

				// If the permission contains a colon, it may use a condensed
				// format like k8s:pods:get,list,watch — expand those into
				// individual permissions.  Each expanded permission is validated
				// against the provider permission set.
				expanded := ExpandCondensedActions(perm)
				for _, ep := range expanded {
					if !providerPermSet[ep] {
						return nil, fmt.Errorf("the requested permission: %s was not found", ep)
					}
				}
				validatedOperations = append(validatedOperations, expanded...)

			} else if !providerPermSet[perm] {
				return nil, fmt.Errorf("the requested permission: %s was not found", perm)
			} else {
				validatedOperations = append(validatedOperations, perm)
			}
		}

		// Condense validated operations back to their original wildcard
		// patterns when the provider's API supports them.  For providers
		// that do not (e.g. GCP, Okta), leave the expanded list so the
		// individual permissions are sent to the cloud API.
		if supportsWildcards {
			validatedOperations = condenseToOriginalWildcards(validatedOperations, originalWildcards)
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

	// Special case: "*/suffix" (e.g. "*/read", "*/write") should match any permission
	// ending with /suffix at any path depth. Go's path.Match treats * as a single-segment
	// wildcard (never crosses /), but Azure RBAC uses "*/read" to mean all read actions
	// across all providers regardless of namespace depth.
	if suffix, ok := plainSuffixWildcard(permission); ok {
		for _, providerPerm := range providerPermissions {
			if strings.HasSuffix(providerPerm.Result.Name, suffix) {
				expandedPermissions = append(expandedPermissions, providerPerm.Result.Name)
			}
		}
		return expandedPermissions, nil
	}

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
// supportsWildcards controls whether validated wildcards are condensed back to
// their original patterns (true for AWS/Azure) or left expanded (false for GCP/Okta).
func ValidatePermissionsPublic(providerPermissions []SearchResult[ProviderPermission], statements RoleStatements, supportsWildcards bool) (RoleStatements, error) {
	return validatePermissions(providerPermissions, statements, supportsWildcards)
}

// ExpandRolePermissionsForProvider expands wildcard permissions in a role's Allow and Deny
// lists against the given provider's known permission set, respecting the provider's
// SupportsWildcards capability flag.
//
// For providers where SupportsWildcards is false (e.g. GCP, Okta), wildcards like
// "bigquery.datasets.*" are expanded into their individual concrete permissions before
// the role is sent to the cloud API. For providers where SupportsWildcards is true
// (e.g. AWS, Azure), the original wildcard patterns are preserved.
//
// This is the exported counterpart of validateRolePermissions, intended for use
// during role assembly (e.g. in GetCompositeRoleForWorkflow) so that the correct
// permission set reaches the provider at authorization time.
func ExpandRolePermissionsForProvider(provider Provider, role *Role) error {
	return validateRolePermissions(provider, role)
}

// ExpandWildcardPermissionsForProvider expands wildcard operations (those containing "*")
// in a role's Allow and Deny lists against the given provider's known permission set.
// Unlike ValidateRolePermissions, non-wildcard operations are never checked against the
// provider dataset — they pass through exactly as-is. This makes it safe to call on
// composite roles that contain generic or cross-provider permissions alongside the
// provider-specific wildcards that need expanding.
//
// A wildcard that matches no permissions in the provider dataset is logged and silently
// dropped from the list.
func ExpandWildcardPermissionsForProvider(provider Provider, role *Role) {
	if provider == nil || role == nil {
		return
	}

	providerPermissions, err := provider.ListPermissions(context.TODO(), &SearchRequest{})
	if err != nil || len(providerPermissions) == 0 {
		return
	}

	expand := func(stmts RoleStatements) RoleStatements {
		result := make(RoleStatements, 0, len(stmts))
		for _, stmt := range stmts {
			var ops []string
			for _, op := range stmt.Operations {
				if !strings.Contains(op, "*") {
					ops = append(ops, op)
					continue
				}
				expanded, expErr := expandPermissionsWildcard(providerPermissions, op)
				if expErr != nil || len(expanded) == 0 {
					logrus.WithField("permission", op).
						WithField("provider", provider.GetIdentifier()).
						Warn("Wildcard permission matched no provider permissions, dropping")
					continue
				}
				ops = append(ops, expanded...)
			}
			if len(ops) > 0 {
				result = append(result, Statement{
					Operations: ops,
					Targets:    stmt.Targets,
					Conditions: stmt.Conditions,
				})
			}
		}
		return result
	}

	role.Permissions.Allow = expand(role.Permissions.Allow)
	role.Permissions.Deny = expand(role.Permissions.Deny)
}

// plainSuffixWildcard reports whether pattern is a "*/suffix" wildcard with a
// plain suffix (no additional glob metacharacters). If so, it returns the suffix
// including the leading "/" (e.g. "/read") and true; otherwise returns "", false.
func plainSuffixWildcard(pattern string) (string, bool) {
	if strings.HasPrefix(pattern, "*/") && !strings.ContainsAny(pattern[2:], `*?[]\`) {
		return pattern[1:], true
	}
	return "", false
}

// condenseToOriginalWildcards replaces individually-expanded permissions with
// their original wildcard patterns. Any permission in operations that matches
// an original wildcard is removed and the wildcard is added back in its place —
// even if some expansions were removed during validation. Matching uses
// path.Match for most patterns and strings.HasSuffix for "*/suffix" wildcards
// (see plainSuffixWildcard). It only restores wildcards that appeared in the
// original input; it never discovers new wildcard groupings.
func condenseToOriginalWildcards(operations []string, originalWildcards []string) []string {
	if len(originalWildcards) == 0 {
		return operations
	}

	// Build a set of current operations for efficient lookup.
	opSet := make(map[string]bool, len(operations))
	for _, op := range operations {
		opSet[op] = true
	}

	for _, wildcard := range originalWildcards {
		// Collect every individual permission that this wildcard covers.
		var covered []string

		if suffix, ok := plainSuffixWildcard(wildcard); ok {
			// "*/suffix" patterns use suffix matching to mirror expandPermissionsWildcard.
			for op := range opSet {
				if strings.HasSuffix(op, suffix) {
					covered = append(covered, op)
				}
			}
		} else {
			for op := range opSet {
				matched, err := path.Match(wildcard, op)
				if err != nil {
					continue
				}
				if matched {
					covered = append(covered, op)
				}
			}
		}

		if len(covered) > 0 {
			// Remove the individual permissions and add back the wildcard.
			for _, op := range covered {
				delete(opSet, op)
			}
			opSet[wildcard] = true
		}
	}

	// Convert back to a sorted slice.
	result := make([]string, 0, len(opSet))
	for op := range opSet {
		result = append(result, op)
	}
	sort.Strings(result)
	return result
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
