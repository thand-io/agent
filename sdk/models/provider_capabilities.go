package models

import (
	internal "github.com/thand-io/agent/internal/models"
)

// Re-export provider capability functions from internal package
// Provider capability types are already defined in provider.go
// For provider-specific capabilities and validation, use sdk/providers package

var GetCapabilityFromString = internal.GetCapabilityFromString
var NewProviderCapabilities = internal.NewProviderCapabilities

// Capability constants
const (
	ProviderCapabilityIdentities   = internal.ProviderCapabilityIdentities
	ProviderCapabilityUsers        = internal.ProviderCapabilityUsers
	ProviderCapabilityGroups       = internal.ProviderCapabilityGroups
	ProviderCapabilityRoles        = internal.ProviderCapabilityRoles
	ProviderCapabilityPermissions  = internal.ProviderCapabilityPermissions
	ProviderCapabilityResources    = internal.ProviderCapabilityResources
	ProviderCapabilityProvisioning = internal.ProviderCapabilityProvisioning
	ProviderCapabilityAuthorizer   = internal.ProviderCapabilityAuthorizer
	ProviderCapabilityNotifier     = internal.ProviderCapabilityNotifier
	ProviderCapabilityWebhook      = internal.ProviderCapabilityWebhook
	ProviderCapabilityTenants      = internal.ProviderCapabilityTenants
)
