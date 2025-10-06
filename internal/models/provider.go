package models

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/sirupsen/logrus"
)

var ErrNotImplemented = errors.New("not implemented")

/*
name: aws-prod
description: Production AWS environment
provider: aws
config:

	region: us-east-1
	account_id: "123456789012"

enabled: true
*/
type Provider struct {
	Name        string       `json:"name"`
	Description string       `json:"description"`
	Provider    string       `json:"provider"`         // e.g. aws, gcp, azure
	Config      *BasicConfig `json:"config,omitempty"` // Provider-specific configuration
	Role        *Role        `json:"role,omitempty"`   // The base role for this provider
	Enabled     bool         `json:"enabled"`          // Whether this provider is enabled

	client ProviderImpl `json:"-" yaml:"-"`
}

func (p *Provider) GetClient() ProviderImpl {
	return p.client
}

func (p *Provider) HasPermission(user *User) bool {

	// If no user and no role then allow access
	// This is to allow access to public providers
	// e.g. for authentication
	// If a role is defined then we need a user to check against the role
	if user == nil && p.Role == nil {
		logrus.Debugf("Provider %s has no role defined and no user, allowing access", p.Name)
		return true
	} else if user == nil && p.Role != nil {
		// If we have a role defined but no user then deny access
		logrus.Debugf("Provider %s has a role defined but no user, denying access", p.Name)
		return false
	} else if user != nil && p.Role == nil {
		// If we have a user but no role then allow access
		logrus.Debugf("Provider %s has no role defined but has a user, allowing access", p.Name)
		return true
	}

	// Otherwise, if we have a role defined then check the user has that role
	return p.Role.HasPermission(user)
}

func (p *Provider) SetClient(client ProviderImpl) {
	p.client = client
}

func (p *Provider) GetConfig() *BasicConfig {
	return p.Config
}

// ProvidersResponse represents the response for a providers query
type ProvidersResponse struct {
	Version   string                      `json:"version"`
	Providers map[string]ProviderResponse `json:"providers"`
}

type ProviderResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Provider    string `json:"provider"` // e.g. aws, gcp, azure
	Enabled     bool   `json:"enabled"`
}

type ProviderCapability string

const (
	ProviderCapabilityRBAC       ProviderCapability = "rbac"
	ProviderCapabilityAuthorizor ProviderCapability = "authorizor"
	ProviderCapabilityNotifier   ProviderCapability = "notifier"
)

func GetCapabilityFromString(cap string) (ProviderCapability, error) {
	switch strings.ToLower(cap) {
	case string(ProviderCapabilityRBAC):
		return ProviderCapabilityRBAC, nil
	case string(ProviderCapabilityAuthorizor):
		return ProviderCapabilityAuthorizor, nil
	case string(ProviderCapabilityNotifier):
		return ProviderCapabilityNotifier, nil
	default:
		return "", fmt.Errorf("unknown capability: %s", cap)
	}
}

/*
A user is assigned a role (e.g., "Manager").
This role has associated permissions (e.g., "approve reports," "view employee data").
These permissions, along with access to specific resources (e.g., "company financial reports"), constitute the user's entitlements.
*/

// Interface for provider implementations
type ProviderImpl interface {
	Initialize(provider Provider) error

	// Form base provider
	GetConfig() *BasicConfig
	GetName() string
	GetDescription() string
	GetProvider() string

	GetCapabilities() []ProviderCapability
	HasCapability(capability ProviderCapability) bool
	HasAnyCapability(capabilities ...ProviderCapability) bool

	ProviderNotifier
	ProviderAuthorizor
	ProviderRoleBasedAccessControl
}

type NotificationRequest map[string]any

type ProviderNotifier interface {

	// Allow this provider to send notifications
	SendNotification(ctx context.Context, notification NotificationRequest) error
}

type AuthorizeSessionResponse struct {
	Url string `json:"url"`
}

type ProviderAuthorizor interface {

	// Allow this provider to authorize a user
	AuthorizeSession(ctx context.Context, auth *AuthorizeUser) (*AuthorizeSessionResponse, error)
	CreateSession(ctx context.Context, auth *AuthorizeUser) (*Session, error)
	ValidateSession(ctx context.Context, session *Session) error
	RenewSession(ctx context.Context, session *Session) (*Session, error)
}

type ProviderRoleBasedAccessControl interface {

	// Role
	GetRole(ctx context.Context, role string) (*ProviderRole, error)
	ListRoles(ctx context.Context, filters ...string) ([]ProviderRole, error)

	// Permissions are individual accesses. Used as part of a role
	GetPermission(ctx context.Context, permission string) (*ProviderPermission, error)
	ListPermissions(ctx context.Context, filters ...string) ([]ProviderPermission, error)

	// Resources are things that permissions can be applied to
	GetResource(ctx context.Context, resource string) (*ProviderResource, error)
	ListResources(ctx context.Context, filters ...string) ([]ProviderResource, error)

	// Bind a user to a role
	ValidateRole(ctx context.Context, user *User, role *Role) (map[string]any, error)
	AuthorizeRole(ctx context.Context, user *User, role *Role) (map[string]any, error)
	RevokeRole(ctx context.Context, user *User, role *Role) (map[string]any, error)
}

type BaseProvider struct {
	provider     Provider
	capabilities []ProviderCapability
}

func NewBaseProvider(provider Provider, capabilities ...ProviderCapability) *BaseProvider {
	return &BaseProvider{
		provider:     provider,
		capabilities: capabilities,
	}
}

func (p *BaseProvider) GetConfig() *BasicConfig {
	return p.provider.Config
}

func (p *BaseProvider) SetConfig(config *BasicConfig) {
	p.provider.Config = config
}

func (p *BaseProvider) GetName() string {
	return p.provider.Name
}

func (p *BaseProvider) GetDescription() string {
	return p.provider.Description
}

func (p *BaseProvider) GetProvider() string {
	return p.provider.Provider
}

func (p *BaseProvider) GetCapabilities() []ProviderCapability {
	return p.capabilities
}

func (p *BaseProvider) HasCapability(capability ProviderCapability) bool {
	return slices.Contains(p.capabilities, capability)
}

func (p *BaseProvider) HasAnyCapability(capabilities ...ProviderCapability) bool {
	return slices.ContainsFunc(capabilities, p.HasCapability)
}

type ProviderPermissionsResponse struct {
	Version     string               `json:"version"`
	Provider    string               `json:"provider"`
	Permissions []ProviderPermission `json:"permissions"`
}

type ProviderRolesResponse struct {
	Version  string         `json:"version"`
	Provider string         `json:"provider"`
	Roles    []ProviderRole `json:"roles"`
}

type ProviderRole struct {
	Id          string `json:"id,omitempty"`
	Name        string `json:"name"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

func (p *BaseProvider) Initialize(provider Provider) error {
	// Initialize the provider
	return nil
}

/* Default implementations for notifiers */

func (p *BaseProvider) SendNotification(ctx context.Context, notification NotificationRequest) error {
	// Default implementation does nothing
	return fmt.Errorf("the provider '%s' does not implement SendNotification", p.GetProvider())
}

/* Default implementations for authorizers */

func (p *BaseProvider) AuthorizeSession(ctx context.Context, auth *AuthorizeUser) (*AuthorizeSessionResponse, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement AuthorizeSession", p.GetProvider())
}

func (p *BaseProvider) CreateSession(ctx context.Context, auth *AuthorizeUser) (*Session, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement CreateSession", p.GetProvider())
}

func (p *BaseProvider) ValidateSession(ctx context.Context, session *Session) error {
	// Default implementation does nothing
	return fmt.Errorf("the provider '%s' does not implement ValidateSession", p.GetProvider())
}

func (p *BaseProvider) RenewSession(ctx context.Context, session *Session) (*Session, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement RenewSession", p.GetProvider())
}

/* Default implementations for role-based access control */

func (p *BaseProvider) GetRole(ctx context.Context, role string) (*ProviderRole, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement GetRole", p.GetProvider())
}

func (p *BaseProvider) ListRoles(ctx context.Context, filters ...string) ([]ProviderRole, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement ListRoles", p.GetProvider())
}

func (p *BaseProvider) GetPermission(ctx context.Context, permission string) (*ProviderPermission, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement GetPermission", p.GetProvider())
}

func (p *BaseProvider) ListPermissions(ctx context.Context, filters ...string) ([]ProviderPermission, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement ListPermissions", p.GetProvider())
}

func (p *BaseProvider) GetResource(ctx context.Context, resource string) (*ProviderResource, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement GetResource", p.GetProvider())
}

func (p *BaseProvider) ListResources(ctx context.Context, filters ...string) ([]ProviderResource, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement ListResources", p.GetProvider())
}

func (p *BaseProvider) AuthorizeRole(ctx context.Context, user *User, role *Role) (map[string]any, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement AuthorizeRole", p.GetProvider())
}

func (p *BaseProvider) RevokeRole(ctx context.Context, user *User, role *Role) (map[string]any, error) {
	// Default implementation does nothing
	return nil, fmt.Errorf("the provider '%s' does not implement RevokeRole", p.GetProvider())
}

func (p *BaseProvider) ValidateRole(ctx context.Context, user *User, role *Role) (map[string]any, error) {
	// TODO this won't work as its the base provider. needs to call the actual provider
	// to validate the role
	return nil, ErrNotImplemented
}

// executeUserValidation triggers user approval workflow
func ValidateRole(
	providerCall ProviderImpl,
	elevateRequest ElevateRequestInternal,
) (map[string]any, error) {
	// Check the user has access to the required scopes etc

	res, err := providerCall.ValidateRole(
		context.Background(),
		elevateRequest.User,
		elevateRequest.Role,
	)

	if err != nil {

		if !errors.Is(err, ErrNotImplemented) {
			return nil, fmt.Errorf("failed to validate role: %w", err)
		}

		logrus.Warn("Provider does not implement role validation, using default")
		err = validateRole(providerCall, elevateRequest.User, elevateRequest.Role)

		if err != nil {
			return nil, err
		}
	}

	return res, nil
}

func validateRole(provider ProviderImpl, user *User, role *Role) error {
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
		return fmt.Errorf("role %s has no permissions or inherits from no roles", role.Name)
	}
	return nil
}

// validateRoleInheritance validates that all inherited roles exist in the provider
func validateRoleInheritance(provider ProviderImpl, role *Role) error {
	if len(role.Inherits) == 0 {
		return nil
	}

	providerRoles, err := provider.ListRoles(context.TODO())
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
func validateInheritedRolesExist(provider ProviderImpl, role *Role, providerRoles []ProviderRole) error {
	for _, inherit := range role.Inherits {
		if !strings.HasPrefix(inherit, fmt.Sprintf("%s:", provider.GetName())) {
			// This is a local role, skip validation
			continue
		}

		roleExists := slices.ContainsFunc(providerRoles, func(r ProviderRole) bool {
			return strings.Compare(r.Name, inherit) == 0
		})

		if !roleExists {
			return fmt.Errorf("role %s inherits from non-existent role %s", role.Name, inherit)
		}
	}
	return nil
}

// validateRolePermissions validates that role permissions exist in the provider
func validateRolePermissions(provider ProviderImpl, role *Role) error {
	if len(role.Permissions.Allow) == 0 && len(role.Permissions.Deny) == 0 {
		return nil
	}

	providerPermissions, err := provider.ListPermissions(context.TODO())
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
func validateRolePermissionLists(role *Role, providerPermissions []ProviderPermission) error {
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

func validatePermissions(providerPermissions []ProviderPermission, permissions []string) ([]string, error) {

	validatedPermissions := []string{}

	// Now lets check are remove permissions that don't exist
	for _, perm := range permissions {

		if strings.HasSuffix(perm, ":*") || strings.HasSuffix(perm, ".*") {
			// Permission ends with a wildcard. Lets expand this
			// out to include all permissions. As some IAMs do not
			// support wildcarding.
			validatedPermissions = append(validatedPermissions,
				expandPermissionsWildcard(providerPermissions, perm)...)
		} else if !slices.ContainsFunc(providerPermissions, func(p ProviderPermission) bool {
			found := strings.Compare(p.Name, perm) == 0
			if found {
				validatedPermissions = append(validatedPermissions, p.Name)
			}
			return found
		}) {
			return nil, fmt.Errorf("the requested permission: %s was not found", perm)
		}
	}

	return validatedPermissions, nil
}

func expandPermissionsWildcard(providerPermissions []ProviderPermission, permission string) []string {

	if strings.HasSuffix(permission, ":*") {
		permission = strings.TrimSuffix(permission, ":*")
	} else if strings.HasSuffix(permission, ".*") {
		permission = strings.TrimSuffix(permission, ".*")
	}

	expandedPermissions := []string{}

	for _, providerPerm := range providerPermissions {
		if strings.HasPrefix(providerPerm.Name, permission) {
			expandedPermissions = append(expandedPermissions, providerPerm.Name)
		}
	}

	return expandedPermissions
}
