package models

import internal "github.com/thand-io/agent/internal/models"

// Provider represents a cloud or service provider configuration (e.g., AWS, GCP, Azure).
// It includes the provider name, description, type, provider-specific configuration,
// an optional base role, and whether the provider is enabled.
type Provider = internal.Provider

// ProviderConfig defines the configuration for a specific provider,
// including connection details and authentication settings.
type ProviderConfig = internal.ProviderConfig

// ProviderCapability represents a capability that a provider can support,
// such as RBAC, authorization, notifications, or identity management.
type ProviderCapability = internal.ProviderCapability

// ProviderNotifier defines the interface for providers that can send notifications.
type ProviderNotifier = internal.ProviderNotifier

// ProviderAuthorizor defines the interface for providers that can authorize users,
// including session creation, validation, and renewal.
type ProviderAuthorizor = internal.ProviderAuthorizor

// ProviderRoleBasedAccessControl defines the interface for providers that support
// role-based access control, including role/permission management and authorization.
type ProviderRoleBasedAccessControl = internal.ProviderRoleBasedAccessControl

// ProviderIdentities defines the interface for providers that can manage identities,
// including retrieving, listing, and refreshing identity information.
type ProviderIdentities = internal.ProviderIdentities

// ProviderPatchRequest represents a request to patch provider data.
type ProviderPatchRequest = internal.ProviderPatchRequest

// ProviderDefinitions defines a collection of provider configurations,
// mapping provider names to their respective configurations.
type ProviderDefinitions = internal.ProviderDefinitions

// ProvidersResponse represents a response containing multiple providers.
type ProvidersResponse = internal.ProvidersResponse

// ProviderResponse represents a response containing a single provider.
type ProviderResponse = internal.ProviderResponse

// ProviderCapabilities defines the capabilities supported by a provider.
type ProviderCapabilities = internal.ProviderCapabilities

// ProviderWebhook defines the interface for providers that can handle webhook events.
type ProviderWebhook = internal.ProviderWebhook

// ProviderTenants defines the interface for providers that manage multi-tenancy.
type ProviderTenants = internal.ProviderTenants

// ProviderTenant represents a tenant within a provider (e.g. an AWS account or GCP project).
type ProviderTenant = internal.ProviderTenant

// WebhookRequest is the request type passed to ProviderWebhook.HandleWebhook.
type WebhookRequest = internal.WebhookRequest

// AuthorizeUser is the request passed to ProviderAuthorizor.AuthorizeSession and CreateSession.
type AuthorizeUser = internal.AuthorizeUser

// AuthorizeSessionResponse contains the redirect URL returned by ProviderAuthorizor.AuthorizeSession.
type AuthorizeSessionResponse = internal.AuthorizeSessionResponse

// AuthorizeRoleRequest is the request passed to ProviderRoleBasedAccessControl.AuthorizeRole.
type AuthorizeRoleRequest = internal.AuthorizeRoleRequest

// AuthorizeRoleResponse is returned by ProviderRoleBasedAccessControl.AuthorizeRole.
type AuthorizeRoleResponse = internal.AuthorizeRoleResponse

// NotificationRequest is the payload passed to ProviderNotifier.SendNotification.
type NotificationRequest = internal.NotificationRequest

// SynchronizeRequest is passed to Provider.Synchronize.
type SynchronizeRequest = internal.SynchronizeRequest

// SynchronizeRolesRequest / Response are used by ProviderRoleBasedAccessControl.SynchronizeRoles.
type SynchronizeRolesRequest = internal.SynchronizeRolesRequest
type SynchronizeRolesResponse = internal.SynchronizeRolesResponse

// SynchronizePermissionsRequest / Response are used by ProviderRoleBasedAccessControl.SynchronizePermissions.
type SynchronizePermissionsRequest = internal.SynchronizePermissionsRequest
type SynchronizePermissionsResponse = internal.SynchronizePermissionsResponse

// SynchronizeResourcesRequest / Response are used by ProviderRoleBasedAccessControl.SynchronizeResources.
type SynchronizeResourcesRequest = internal.SynchronizeResourcesRequest
type SynchronizeResourcesResponse = internal.SynchronizeResourcesResponse

// SynchronizeIdentitiesRequest / Response are used by ProviderIdentities.SynchronizeIdentities.
type SynchronizeIdentitiesRequest = internal.SynchronizeIdentitiesRequest
type SynchronizeIdentitiesResponse = internal.SynchronizeIdentitiesResponse

// SynchronizeUsersRequest / Response are used by ProviderIdentities.SynchronizeUsers.
type SynchronizeUsersRequest = internal.SynchronizeUsersRequest
type SynchronizeUsersResponse = internal.SynchronizeUsersResponse

// SynchronizeGroupsRequest / Response are used by ProviderIdentities.SynchronizeGroups.
type SynchronizeGroupsRequest = internal.SynchronizeGroupsRequest
type SynchronizeGroupsResponse = internal.SynchronizeGroupsResponse

// SynchronizeTenantsRequest / Response are used by ProviderTenants.SynchronizeTenants.
type SynchronizeTenantsRequest = internal.SynchronizeTenantsRequest
type SynchronizeTenantsResponse = internal.SynchronizeTenantsResponse

// RevokeRoleRequest is the request passed to ProviderRoleBasedAccessControl.RevokeRole.
type RevokeRoleRequest = internal.RevokeRoleRequest

// RevokeRoleResponse is returned by ProviderRoleBasedAccessControl.RevokeRole.
type RevokeRoleResponse = internal.RevokeRoleResponse

// ProviderContext is the execution context passed to provider RBAC operations.
type ProviderContext = internal.ProviderContext
