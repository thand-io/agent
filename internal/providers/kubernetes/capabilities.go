package kubernetes

import "github.com/thand-io/agent/internal/models"

var KubernetesCapabilities = []models.ProviderCapability{
	models.ProviderCapabilityPermissions,   // permissions.go
	models.ProviderCapabilityAuthorizeRole, // rbac.go
	models.ProviderCapabilityRevokeRole,    // rbac.go
}
