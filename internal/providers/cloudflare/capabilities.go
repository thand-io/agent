package cloudflare

import "github.com/thand-io/agent/internal/models"

var CloudflareCapabilities = []models.ProviderCapability{
	models.ProviderCapabilityUsers,
	models.ProviderCapabilityRoles,
	models.ProviderCapabilityResources,
	models.ProviderCapabilityPermissions,

	models.ProviderCapabilityAuthorizeRole,
	models.ProviderCapabilityRevokeRole,
}
