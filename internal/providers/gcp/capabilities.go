package gcp

import "github.com/thand-io/agent/internal/models"

var GcpCapabilities = models.NewProviderCapabilities().
	WithDefaultIdentitiesConfiguration().
	WithRolesConfiguration(models.RolesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	}).
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		Enabled:        true,
		Synchronizable: false,
	}).
	WithDefaultProvisioningConfiguration().
	WithDefaultTenantsConfiguration()
