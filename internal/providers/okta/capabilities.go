package okta

import "github.com/thand-io/agent/internal/models"

var OktaCapabilities = models.NewProviderCapabilities().
	WithDefaultUsersConfiguration().
	WithDefaultGroupsConfiguration().
	WithRolesConfiguration(models.RolesConfiguration{
		Enabled:        true,
		Synchronizable: false,
	}).
	WithDefaultResourcesConfiguration().
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		Enabled:        true,
		Synchronizable: false,
	}).
	WithDefaultProvisioningConfiguration()
