package gcp

import "github.com/thand-io/agent/internal/models"

var GcpCapabilities = []models.ProviderCapability{
	models.ProviderCapabilityIdentities,
	models.ProviderCapabilityRoles,
	models.ProviderCapabilityPermissions,

	models.ProviderCapabilityAuthorizeRole,
	models.ProviderCapabilityRevokeRole,
}
