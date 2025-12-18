package azure

import "github.com/thand-io/agent/internal/models"

var AzureCapabilities = models.NewProviderCapabilities().
	WithRolesConfiguration(models.RolesConfiguration{
		Enabled:        true,
		Synchronizable: false, // roles are statically defined by Azure
	}).
	WithPermissionsConfiguration(models.PermissionsConfiguration{
		Enabled:        true,
		Synchronizable: false, // permissions are derived from Azure RBAC roles
	}).
	WithDefaultProvisioningConfiguration()
