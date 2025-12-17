package azure

import "github.com/thand-io/agent/internal/models"

var AzureCapabilities = []models.ProviderCapability{
	models.ProviderCapabilityRoles,       // roles.go
	models.ProviderCapabilityPermissions, // permissions.go

	models.ProviderCapabilityAuthorizeRole, // rbac.go
	models.ProviderCapabilityRevokeRole,    // rbac.go
}
