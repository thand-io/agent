package azure

import "github.com/thand-io/agent/internal/models"

var AzureCapabilities = models.NewProviderCapabilities().
	WithDefaultUsersConfiguration().
	WithDefaultGroupsConfiguration().
	WithRolesConfiguration(models.RolesConfiguration{
		Enabled:        true,
		Synchronizable: false, // roles are statically defined by Azure
	}).
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		SynchronizableConfiguration: models.SynchronizableConfiguration{
			Enabled:        true,
			Synchronizable: false, // permissions are derived from Azure RBAC roles
		},
		SupportsWildcards: true, // Azure RBAC natively accepts wildcards like Microsoft.Compute/*/read
	}).
	WithDefaultProvisioningConfiguration().
	WithDefaultTenantsConfiguration()
