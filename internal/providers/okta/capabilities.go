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
		SynchronizableConfiguration: models.SynchronizableConfiguration{
			Enabled:        true,
			Synchronizable: false,
		},
		// SupportsWildcards defaults to false — Okta requires exact permission names
	}).
	WithDefaultProvisioningConfiguration()
