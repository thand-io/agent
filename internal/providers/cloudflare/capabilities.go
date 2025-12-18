package cloudflare

import "github.com/thand-io/agent/internal/models"

var CloudflareCapabilities = models.NewProviderCapabilities().
	WithDefaultUsersConfiguration().
	WithDefaultRolesConfiguration().
	WithDefaultResourcesConfiguration().
	WithDefaultPermissionsConfiguration().
	WithDefaultProvisioningConfiguration()
