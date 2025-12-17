package github

import "github.com/thand-io/agent/internal/models"

var GithubCapabilities = []models.ProviderCapability{
	models.ProviderCapabilityAuthorizer,

	models.ProviderCapabilityUsers,
	models.ProviderCapabilityGroups,
	models.ProviderCapabilityRoles,

	models.ProviderCapabilityAuthorizeRole,
	models.ProviderCapabilityRevokeRole,
}
