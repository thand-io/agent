package models

import internal "github.com/thand-io/agent/internal/models"

// Provider represents a cloud or service provider configuration (e.g., AWS, GCP, Azure).
// It includes the provider name, description, type, provider-specific configuration,
// an optional base role, and whether the provider is enabled.
type Provider = internal.Provider

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
