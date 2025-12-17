package okta

import "github.com/thand-io/agent/internal/models"

var OktaCapabilities = []models.ProviderCapability{
	models.ProviderCapabilityUsers,         // users.go
	models.ProviderCapabilityGroups,        // groups.go
	models.ProviderCapabilityRoles,         // roles.go
	models.ProviderCapabilityPermissions,   // permissions.go
	models.ProviderCapabilityAuthorizeRole, // rbac.go
	models.ProviderCapabilityRevokeRole,    // rbac.go
}
